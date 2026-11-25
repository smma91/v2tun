package io.github.smma91.v2tun.ui

import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.smma91.v2tun.MainActivity
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.ServerItem
import io.github.smma91.v2tun.Store
import io.github.smma91.v2tun.mobile.Mobile
import io.github.smma91.v2tun.tr
import java.text.DateFormat
import java.util.Date
import kotlin.concurrent.thread

@Composable
fun ServersScreen(activity: MainActivity, version: Int, refresh: () -> Unit) {
    val list = remember(version) { Store.servers() }
    var adding by remember { mutableStateOf(false) }
    var updating by remember { mutableStateOf(false) }
    var pinging by remember { mutableStateOf(false) }
    var qrMenu by remember { mutableStateOf(false) }
    SideEffect { activity.onAdded = refresh }
    var removing by remember { mutableStateOf<Pair<String, String>?>(null) } // sub id, name

    val paste = {
        val cm = activity.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val clip = cm.primaryClip?.getItemAt(0)?.coerceToText(activity)?.toString().orEmpty()
        if (clip.isBlank()) Toast.makeText(activity, tr("The clipboard is empty"), Toast.LENGTH_SHORT).show()
        else activity.addText(clip) { refresh() }
    }

    LazyColumn(
        Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                ActionButton(painterResource(R.drawable.ic_paste), tr("Add from clipboard"), Modifier.weight(1f), primary = true, onClick = paste)
                Box(Modifier.weight(1f)) {
                    ActionButton(painterResource(R.drawable.ic_qr), tr("Scan QR code"), Modifier.fillMaxWidth()) {
                        if (activity.hasCamera) qrMenu = true else activity.scanFromImage()
                    }
                    DropdownMenu(qrMenu, { qrMenu = false }, containerColor = Card2) {
                        DropdownMenuItem(
                            { Text(tr("Camera")) }, { qrMenu = false; activity.scanWithCamera() },
                            leadingIcon = { Icon(painterResource(R.drawable.ic_camera), null, tint = Accent) },
                        )
                        DropdownMenuItem(
                            { Text(tr("From image")) }, { qrMenu = false; activity.scanFromImage() },
                            leadingIcon = { Icon(painterResource(R.drawable.ic_image), null, tint = Accent) },
                        )
                    }
                }
                ActionButton(rememberVectorPainter(Icons.Filled.Add), tr("Add"), Modifier.weight(1f)) { adding = true }
            }
        }
        if (list.servers.isNotEmpty()) {
            item {
                Row(Modifier.fillMaxWidth().padding(top = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        tr("{0} servers").replace("{0}", num(list.servers.size)), Modifier.weight(1f).padding(start = 6.dp),
                        color = Muted, fontSize = 13.sp,
                    )
                    SmallAction(painterResource(R.drawable.ic_ping), tr("Ping"), pinging) {
                        if (!pinging) { pinging = true; activity.pingServers { pinging = false; refresh() } }
                    }
                    if (list.subs.isNotEmpty()) {
                        SmallAction(rememberVectorPainter(Icons.Filled.Refresh), tr("Update"), updating) {
                            if (!updating) {
                                updating = true
                                thread { Mobile.updateSubscriptions(); activity.runOnUiThread { updating = false; refresh() } }
                            }
                        }
                    }
                }
            }
        }
        if (list.servers.isEmpty()) {
            item {
                Panel {
                    Text(
                        tr("No servers yet. Copy a config link and press \"Add from clipboard\", or enter a subscription URL."),
                        color = Muted, fontSize = 14.sp, lineHeight = 22.sp, textAlign = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth().padding(vertical = 24.dp),
                    )
                }
            }
        } else {
            item {
                ServerRow(
                    tr("Auto (fastest server)"), tr("{0} servers").replace("{0}", num(list.servers.size)),
                    0, list.selected == "auto", alone = true,
                ) { Mobile.select("auto"); refresh() }
            }
        }

        val groups = list.servers.groupBy { it.sub }
        for ((sub, servers) in groups) {
            val info = list.subs.firstOrNull { it.id == sub }
            item(key = "g-$sub") {
                Column(
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).background(Card)
                        .border(1.dp, Line, RoundedCornerShape(20.dp)),
                ) {
                    Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 6.dp, top = 8.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(
                                if (sub.isEmpty()) tr("Manual servers") else info?.name ?: sub,
                                color = TextMain, fontWeight = FontWeight.Bold, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis,
                            )
                            val meta = buildString {
                                append(tr("{0} servers").replace("{0}", num(servers.size)))
                                if (info != null && info.updated > 0) {
                                    append(" · ")
                                    append(DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT, appLocale()).format(Date(info.updated * 1000)))
                                }
                            }
                            Text(meta, color = Muted, fontSize = 12.sp)
                            if (!info?.error.isNullOrEmpty()) Text(info!!.error, color = Bad, fontSize = 12.sp)
                        }
                        Tappable(Modifier.size(44.dp), CircleShape, onClick = {
                            removing = sub to (if (sub.isEmpty()) tr("Manual servers") else info?.name ?: sub)
                        }) {
                            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                Icon(Icons.Filled.Delete, tr("Remove"), Modifier.size(20.dp), tint = Muted)
                            }
                        }
                    }
                    servers.forEachIndexed { i, s ->
                        if (i > 0) HorizontalDivider(Modifier.padding(horizontal = 16.dp), color = Line)
                        ServerRow(s.name, "${s.type} · ${s.addr}", if (s.delay != 0) s.delay else s.ping, list.selected == s.id, s.error) {
                            if (s.error.isEmpty()) { Mobile.select(s.id); refresh() }
                        }
                    }
                }
            }
        }
    }

    if (adding) AddDialog(activity, onDone = { adding = false; refresh() }, onCancel = { adding = false })

    removing?.let { (sub, name) ->
        AlertDialog(
            onDismissRequest = { removing = null },
            containerColor = Card,
            title = { Text(tr("Remove subscription")) },
            text = { Text(tr("Remove subscription \"{0}\" and its servers?").replace("{0}", name), color = Muted) },
            confirmButton = {
                Button(onClick = {
                    if (sub.isEmpty()) list.servers.filter { it.sub.isEmpty() }.forEach { s: ServerItem -> Mobile.removeServer(s.id) }
                    else Mobile.removeSubscription(sub)
                    removing = null
                    refresh()
                }) { Text(tr("Remove")) }
            },
            dismissButton = { TextButton(onClick = { removing = null }) { Text(tr("Cancel")) } },
        )
    }
}

@Composable
private fun AddDialog(activity: MainActivity, onDone: () -> Unit, onCancel: () -> Unit) {
    var text by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onCancel,
        containerColor = Card,
        title = { Text(tr("Add subscription or links manually")) },
        text = {
            OutlinedTextField(
                text, { text = it }, Modifier.fillMaxWidth().heightIn(min = 140.dp),
                placeholder = { Text("vless://…\nss://…\nhttps://…", color = Muted) },
                colors = OutlinedTextFieldDefaults.colors(unfocusedBorderColor = Line, focusedBorderColor = Accent),
            )
        },
        confirmButton = {
            Button(enabled = text.isNotBlank() && !busy, onClick = {
                busy = true
                activity.addText(text) { onDone() }
            }) { Text(tr("Add")) }
        },
        dismissButton = { TextButton(onClick = onCancel) { Text(tr("Cancel")) } },
    )
}

@Composable
private fun ActionButton(
    icon: Painter, label: String, modifier: Modifier = Modifier, primary: Boolean = false, spinning: Boolean = false,
    onClick: () -> Unit,
) {
    val angle = if (spinning) {
        val t = rememberInfiniteTransition(label = "spin")
        t.animateFloat(0f, 360f, infiniteRepeatable(tween(900, easing = LinearEasing)), label = "a").value
    } else 0f
    val content = @Composable {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Icon(icon, null, Modifier.size(20.dp).rotate(angle))
            Spacer(Modifier.height(6.dp))
            Text(label, fontSize = 12.sp, lineHeight = 15.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.Center)
        }
    }
    val shape = RoundedCornerShape(16.dp)
    val pad = PaddingValues(horizontal = 6.dp, vertical = 6.dp)
    if (primary) Button(onClick, modifier.height(80.dp), shape = shape, contentPadding = pad) { content() }
    else OutlinedButton(
        onClick, modifier.height(80.dp), shape = shape, contentPadding = pad,
        border = androidx.compose.foundation.BorderStroke(1.dp, Line),
        colors = androidx.compose.material3.ButtonDefaults.outlinedButtonColors(containerColor = Card, contentColor = TextMain),
    ) { content() }
}

@Composable
private fun ServerRow(
    name: String, meta: String, delay: Int, selected: Boolean, error: String = "", alone: Boolean = false,
    onClick: () -> Unit,
) {
    val shape = RoundedCornerShape(if (alone) 20.dp else 14.dp)
    Tappable(Modifier.fillMaxWidth(), shape, onClick = onClick) {
        Row(
            Modifier.fillMaxWidth()
                .then(if (alone) Modifier.background(Card).border(1.dp, if (selected) Accent else Line, shape) else Modifier)
                .background(if (selected) Accent.copy(alpha = 0.10f) else Color.Transparent)
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier.size(22.dp).clip(CircleShape)
                    .background(if (selected) Accent else Color.Transparent)
                    .border(2.dp, if (selected) Accent else Line, CircleShape),
                contentAlignment = Alignment.Center,
            ) { if (selected) Icon(Icons.Filled.Check, null, Modifier.size(14.dp), tint = Color.White) }
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(name, color = if (error.isEmpty()) TextMain else Muted, fontSize = 15.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(meta, color = Muted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (error.isNotEmpty()) Text(error, color = Bad, fontSize = 12.sp, maxLines = 2)
            }
            when {
                delay > 0 -> Pill(num(delay) + " ms", if (delay < 300) Ok else if (delay < 800) Warn else Bad)
                delay < 0 -> Pill("✕", Bad)
            }
        }
    }
}

/** A small text button with an icon that spins while working. */
@Composable
private fun SmallAction(icon: Painter, label: String, busy: Boolean, onClick: () -> Unit) {
    val angle = if (busy) {
        val t = rememberInfiniteTransition(label = "spin")
        t.animateFloat(0f, 360f, infiniteRepeatable(tween(900, easing = LinearEasing)), label = "a").value
    } else 0f
    Tappable(shape = RoundedCornerShape(99.dp), onClick = onClick) {
        Row(
            Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(icon, null, Modifier.size(16.dp).rotate(angle), tint = Accent)
            Spacer(Modifier.width(6.dp))
            Text(label, color = Accent, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        }
    }
}