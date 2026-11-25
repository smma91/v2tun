package io.github.smma91.v2tun

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.GlobalHistogramBinarizer
import com.google.zxing.common.HybridBinarizer
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import io.github.smma91.v2tun.mobile.Mobile
import io.github.smma91.v2tun.mobile.Protector
import io.github.smma91.v2tun.ui.AppUi
import kotlin.concurrent.thread

class MainActivity : ComponentActivity() {

    private val vpnPermission = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        if (VpnService.prepare(this) == null) Vpn.start(this)
    }

    /** Called after something was added (the screen reloads its lists). */
    var onAdded: () -> Unit = {}

    private val scanCamera = registerForActivityResult(ScanContract()) { r ->
        r.contents?.let { addText(it) { onAdded() } }
    }

    private val pickImage = registerForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri != null) thread {
            val text = runCatching { decodeQr(uri) }.getOrNull()
            runOnUiThread {
                if (text == null) Toast.makeText(this, tr("No QR code found in this image"), Toast.LENGTH_LONG).show()
                else addText(text) { onAdded() }
            }
        }
    }

    /** Connect, asking Android for the VPN permission the first time. */
    fun connect() {
        val ask = VpnService.prepare(this)
        if (ask != null) vpnPermission.launch(ask) else Vpn.start(this)
    }

    val hasCamera: Boolean get() = packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)

    fun scanWithCamera() {
        scanCamera.launch(
            ScanOptions()
                .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                .setPrompt(tr("Point the camera at the QR code"))
                .setBeepEnabled(false)
                .setOrientationLocked(false)
        )
    }

    fun scanFromImage() = pickImage.launch("image/*")

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
        handleShare(intent)
        setContent { AppUi(this) }
    }

    override fun onStart() {
        super.onStart()
        Vpn.uiVisible = true
    }

    override fun onStop() {
        Vpn.uiVisible = false
        super.onStop()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleShare(intent)
    }

    /** A link, subscription or QR image shared from another app. */
    private fun handleShare(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        intent.getStringExtra(Intent.EXTRA_TEXT)?.let { addText(it) { onAdded() }; return }
        @Suppress("DEPRECATION")
        val uri = intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM) ?: return
        thread {
            val text = runCatching { decodeQr(uri) }.getOrNull()
            runOnUiThread {
                if (text == null) Toast.makeText(this, tr("No QR code found in this image"), Toast.LENGTH_LONG).show()
                else addText(text) { onAdded() }
            }
        }
    }

    fun addText(text: String, done: () -> Unit = {}) {
        thread {
            val msg = try {
                val n = Mobile.addText(text)
                tr("{0} servers added").replace("{0}", n.toString())
            } catch (e: Exception) {
                e.message ?: e.toString()
            }
            runOnUiThread {
                Toast.makeText(this, msg, Toast.LENGTH_LONG).show()
                done()
            }
        }
    }

    /** TCP ping of every server over the real network (also while connected). */
    fun pingServers(done: () -> Unit) {
        val dns = runCatching {
            val cm = getSystemService(ConnectivityManager::class.java)
            cm.allNetworks.filter {
                cm.getNetworkCapabilities(it)?.let { c ->
                    !c.hasTransport(NetworkCapabilities.TRANSPORT_VPN) && c.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                } == true
            }.flatMap { cm.getLinkProperties(it)?.dnsServers.orEmpty() }
                .mapNotNull { it.hostAddress }.joinToString(",")
        }.getOrDefault("")
        thread {
            runCatching {
                Mobile.pingServers(object : Protector {
                    override fun protect(fd: Int): Boolean = V2tunVpnService.instance?.protect(fd) ?: true
                }, dns)
            }
            runOnUiThread(done)
        }
    }

    /** Reads a QR code from an image (a screenshot or a photo). */
    private fun decodeQr(uri: Uri): String? {
        val opts = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, opts) }
        var sample = 1
        while (maxOf(opts.outWidth, opts.outHeight) / sample > 2000) sample *= 2
        val bmp = contentResolver.openInputStream(uri)?.use {
            BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample })
        } ?: return null
        return decodeQr(bmp)
    }

    private fun decodeQr(bmp: Bitmap): String? {
        val px = IntArray(bmp.width * bmp.height)
        bmp.getPixels(px, 0, bmp.width, 0, 0, bmp.width, bmp.height)
        val source = RGBLuminanceSource(bmp.width, bmp.height, px)
        val hints = mapOf(
            DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
            DecodeHintType.TRY_HARDER to true,
        )
        for (b in listOf(HybridBinarizer(source), GlobalHistogramBinarizer(source), HybridBinarizer(source.invert()))) {
            runCatching { return MultiFormatReader().decode(BinaryBitmap(b), hints).text }
        }
        return null
    }
}
