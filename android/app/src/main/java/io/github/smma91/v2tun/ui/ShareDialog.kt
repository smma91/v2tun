package io.github.smma91.v2tun.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.widget.Toast
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.tr

/** A server's link (or a subscription URL) as a QR code, to copy or share. */
@Composable
fun ShareDialog(title: String, link: String, onClose: () -> Unit) {
    val ctx = LocalContext.current
    val qr = remember(link) { qrBitmap(link) }
    val landscape = LocalConfiguration.current.let { it.screenHeightDp < 600 && it.screenWidthDp > it.screenHeightDp }
    AlertDialog(
        onDismissRequest = onClose,
        containerColor = Card,
        // Side by side on a TV needs a wider window than Android's default.
        properties = androidx.compose.ui.window.DialogProperties(usePlatformDefaultWidth = !landscape),
        modifier = if (landscape) Modifier.width(600.dp) else Modifier,
        title = { Text(title, maxLines = 2, overflow = TextOverflow.Ellipsis) },
        text = {
            val conf = LocalConfiguration.current
            val qrView = @Composable { size: androidx.compose.ui.unit.Dp ->
                if (qr != null) {
                    Box(Modifier.clip(RoundedCornerShape(16.dp)).background(Color.White).padding(12.dp)) {
                        Image(qr.asImageBitmap(), null, Modifier.size(size), filterQuality = FilterQuality.None) // sharp modules
                    }
                } else {
                    Text(tr("Too long for a QR code; copy or share the link instead"), color = Warn, fontSize = 13.sp)
                }
            }
            val linkView = @Composable {
                Text(
                    link,
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp)).background(Bg)
                        .border(1.dp, Line, RoundedCornerShape(10.dp)).padding(10.dp),
                    color = Muted, fontFamily = FontFamily.Monospace, fontSize = 11.sp, maxLines = 4,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            val copyButton = @Composable { m: Modifier ->
                OutlinedButton(
                    onClick = { copy(ctx, link) }, m,
                    border = androidx.compose.foundation.BorderStroke(1.dp, Line),
                    colors = ButtonDefaults.outlinedButtonColors(contentColor = TextMain),
                ) {
                    Icon(painterResource(R.drawable.ic_copy), null, Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(tr("Copy"), fontWeight = FontWeight.Medium)
                }
            }
            val shareButton = @Composable { m: Modifier ->
                Button(onClick = { share(ctx, title, link) }, m) {
                    Icon(Icons.Filled.Share, null, Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(tr("Share"), fontWeight = FontWeight.Medium)
                }
            }
            if (landscape) {
                // TV / landscape: side by side, everything visible (and reachable with a remote).
                Row(verticalAlignment = Alignment.CenterVertically) {
                    qrView(minOf(220, conf.screenHeightDp * 2 / 5).dp)
                    Spacer(Modifier.width(16.dp))
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        linkView()
                        copyButton(Modifier.fillMaxWidth())
                        shareButton(Modifier.fillMaxWidth())
                    }
                }
            } else {
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
                ) {
                    qrView(240.dp)
                    Spacer(Modifier.height(14.dp))
                    linkView()
                    Spacer(Modifier.height(14.dp))
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        copyButton(Modifier.weight(1f))
                        shareButton(Modifier.weight(1f))
                    }
                }
            }
        },        confirmButton = { TextButton(onClick = onClose) { Text(tr("Close")) } },
    )
}

private fun copy(ctx: Context, text: String) {
    val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    cm.setPrimaryClip(ClipData.newPlainText("v2tun", text))
    Toast.makeText(ctx, tr("Copied"), Toast.LENGTH_SHORT).show()
}

private fun share(ctx: Context, title: String, text: String) {
    val send = Intent(Intent.ACTION_SEND).setType("text/plain")
        .putExtra(Intent.EXTRA_TEXT, text).putExtra(Intent.EXTRA_SUBJECT, title)
    ctx.startActivity(Intent.createChooser(send, title).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
}

/** Black-on-white QR code, one pixel per module (scaled up sharply when drawn). */
private fun qrBitmap(text: String): Bitmap? {
    if (text.length > 2000) return null
    return runCatching {
        val m = QRCodeWriter().encode(
            text, BarcodeFormat.QR_CODE, 0, 0,
            mapOf(
                EncodeHintType.CHARACTER_SET to "UTF-8",
                EncodeHintType.MARGIN to 0,
                EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.L,
            ),
        )
        val px = IntArray(m.width * m.height) { i ->
            if (m.get(i % m.width, i / m.width)) android.graphics.Color.BLACK else android.graphics.Color.WHITE
        }
        Bitmap.createBitmap(px, m.width, m.height, Bitmap.Config.ARGB_8888)
    }.getOrNull()
}
