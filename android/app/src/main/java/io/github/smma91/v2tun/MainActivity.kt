package io.github.smma91.v2tun

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import io.github.smma91.v2tun.mobile.Mobile
import io.github.smma91.v2tun.ui.AppUi
import kotlin.concurrent.thread

class MainActivity : ComponentActivity() {

    private val vpnPermission = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        if (VpnService.prepare(this) == null) Vpn.start(this)
    }

    /** Connect, asking Android for the VPN permission the first time. */
    fun connect() {
        val ask = VpnService.prepare(this)
        if (ask != null) vpnPermission.launch(ask) else Vpn.start(this)
    }

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

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleShare(intent)
    }

    /** A link or subscription shared from another app. */
    private fun handleShare(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        val text = intent.getStringExtra(Intent.EXTRA_TEXT) ?: return
        addText(text)
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
}
