package io.github.smma91.v2tun.ui

import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.smma91.v2tun.MainActivity
import io.github.smma91.v2tun.Status
import io.github.smma91.v2tun.Store
import io.github.smma91.v2tun.Vpn
import io.github.smma91.v2tun.mobile.Mobile
import io.github.smma91.v2tun.strings
import io.github.smma91.v2tun.tr
import kotlinx.coroutines.delay
import org.json.JSONArray
import java.net.Inet4Address
import java.net.NetworkInterface
import kotlin.concurrent.thread

// The desktop app's colours.
private val Bg = Color(0xFF0B0F19)
private val Card = Color(0xFF111827)
private val Line = Color(0xFF1F2937)
private val Muted = Color(0xFF94A3B8)
private val Blue = Color(0xFF3B82F6)
private val Green = Color(0xFF22C55E)
private val Red = Color(0xFFF87171)

private val Theme = darkColorScheme(primary = Blue, background = Bg, surface = Card, onSurface = Color.White)

@Composable
fun AppUi(activity: MainActivity) {
    // Changing the language or the data redraws everything.
    var version by remember { mutableIntStateOf(0) }
    val refresh: () -> Unit = { version++ }
    val lang = remember(version) { Store.settings().optString("lang", "en") }
    val dir = if (lang == "fa" || lang == "ar") LayoutDirection.Rtl else LayoutDirection.Ltr
    var tab by remember { mutableIntStateOf(0) }

    MaterialTheme(colorScheme = Theme) {
        CompositionLocalProvider(LocalLayoutDirection provides dir) {
            Surface(Modifier.fillMaxSize(), color = Bg) {
                Column(Modifier.fillMaxSize()) {
                    Tabs(tab, version) { tab = it }
                    Box(Modifier.weight(1f).fillMaxWidth()) {
                        when (tab) {
                            0 -> HomeScreen(activity, version)
                            1 -> ServersScreen(activity, version, refresh)
                            else -> SettingsScreen(activity, version, refresh)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Tabs(selected: Int, version: Int, onSelect: (Int) -> Unit) {
    val names = remember(version) { listOf(tr("Connect"), tr("Servers"), tr("Settings")) }
    Row(
        Modifier.fillMaxWidth().padding(12.dp).clip(RoundedCornerShape(14.dp)).background(Card).padding(4.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        names.forEachIndexed { i, n ->
            FocusBox(
                Modifier.weight(1f).clip(RoundedCornerShape(10.dp))
                    .background(if (i == selected) Line else Color.Transparent),
                onClick = { onSelect(i) },
            ) {
                Text(n, Modifier.fillMaxWidth().padding(vertical = 10.dp), color = if (i == selected) Color.White else Muted,
                    fontWeight = FontWeight.SemiBold, textAlign = androidx.compose.ui.text.style.TextAlign.Center)
            }
        }
    }
}

/** Clickable and visibly focusable (TV remote). */
@Composable
fun FocusBox(modifier: Modifier = Modifier, onClick: () -> Unit, content: @Composable () -> Unit) {
    val source = remember { MutableInteractionSource() }
    val focused by source.collectIsFocusedAsState()
    Box(
        modifier
            .border(BorderStroke(2.dp, if (focused) Blue else Color.Transparent), RoundedCornerShape(10.dp))
            .clickable(interactionSource = source, indication = null, onClick = onClick)
            .focusable(interactionSource = source),
    ) { content() }
}

@Composable
private fun CardBox(content: @Composable () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp).clip(RoundedCornerShape(16.dp))
            .background(Card).border(1.dp, Line, RoundedCornerShape(16.dp)).padding(14.dp),
    ) { content() }
}

// ---------------- Connect ----------------

@Composable
private fun HomeScreen(activity: MainActivity, version: Int) {
    val status by Vpn.status.collectAsState()
    val error by Vpn.error.collectAsState()
    val list = remember(version, status) { Store.servers() }
    val on = status == Status.STARTED
    val busy = status == Status.STARTING || status == Status.STOPPING
    var now by remember { mutableStateOf(System.currentTimeMillis()) }
    LaunchedEffect(on) { while (on) { now = System.currentTimeMillis(); delay(1000) } }

    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 24.dp)) {
        item {
            Column(Modifier.fillMaxWidth().padding(top = 24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                val ring = when { on -> Green; busy -> Blue; else -> Line }
                FocusBox(Modifier.size(180.dp).clip(CircleShape), onClick = {
                    if (on || busy) Vpn.stop(activity) else activity.connect()
                }) {
                    Box(
                        Modifier.fillMaxSize().clip(CircleShape).background(Card).border(10.dp, ring, CircleShape),
                        contentAlignment = Alignment.Center,
                    ) { Text("⏻", fontSize = 64.sp, color = if (on) Green else Muted) }
                }
                Spacer(Modifier.height(16.dp))
                Text(
                    when (status) {
                        Status.STARTED -> tr("Connected")
                        Status.STARTING -> tr("Connecting…")
                        Status.STOPPING -> tr("Disconnecting…")
                        Status.STOPPED -> tr("Tap to connect")
                    },
                    fontSize = 20.sp, fontWeight = FontWeight.Bold, color = Color.White,
                )
                if (on) {
                    val s = (now - Vpn.since) / 1000
                    Text("%02d:%02d:%02d".format(s / 3600, s / 60 % 60, s % 60), color = Muted)
                }
                error?.let { Text(it, color = Red, modifier = Modifier.padding(16.dp)) }
            }
        }
        item {
            CardBox {
                Text(tr("Server"), color = Muted, fontSize = 13.sp)
                val sel = list.servers.firstOrNull { it.id == list.selected }
                Text(sel?.name ?: tr("Auto (fastest server)"), color = Color.White, fontWeight = FontWeight.SemiBold)
                Text(tr("{0} servers").replace("{0}", list.servers.size.toString()), color = Muted, fontSize = 13.sp)
            }
        }
    }
}

// ---------------- Servers ----------------

@Composable
private fun ServersScreen(activity: MainActivity, version: Int, refresh: () -> Unit) {
    val list = remember(version) { Store.servers() }
    var adding by remember { mutableStateOf(false) }
    var text by remember { mutableStateOf("") }
    var updating by remember { mutableStateOf(false) }

    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 24.dp)) {
        item {
            Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = {
                    val cm = activity.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                    val clip = cm.primaryClip?.getItemAt(0)?.coerceToText(activity)?.toString().orEmpty()
                    if (clip.isBlank()) adding = true else activity.addText(clip) { refresh() }
                }, Modifier.weight(1f)) { Text(tr("Add from clipboard")) }
                OutlinedButton(onClick = {
                    updating = true
                    thread { Mobile.updateSubscriptions(); activity.runOnUiThread { updating = false; refresh() } }
                }, Modifier.weight(1f), enabled = !updating) { Text(if (updating) "…" else tr("Update")) }
            }
            TextButton(onClick = { adding = !adding }, Modifier.padding(horizontal = 8.dp)) {
                Text(tr("Add subscription or links manually"), color = Muted)
            }
            if (adding) {
                Column(Modifier.padding(horizontal = 12.dp)) {
                    OutlinedTextField(text, { text = it }, Modifier.fillMaxWidth().height(120.dp),
                        placeholder = { Text("vless://…  ss://…  https://…", color = Muted) })
                    Button(onClick = {
                        activity.addText(text) { text = ""; adding = false; refresh() }
                    }, Modifier.padding(top = 8.dp)) { Text(tr("Add")) }
                }
            }
        }
        item {
            ServerRow("auto", tr("Auto (fastest server)"), "", 0, list.selected == "auto") {
                Mobile.select("auto"); refresh()
            }
        }
        val groups = list.servers.groupBy { it.sub }
        for ((sub, servers) in groups) {
            item {
                val name = if (sub.isEmpty()) tr("Manual servers") else list.subs.firstOrNull { it.id == sub }?.name ?: sub
                Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(name, Modifier.weight(1f), color = Color.White, fontWeight = FontWeight.Bold)
                    TextButton(onClick = {
                        if (sub.isEmpty()) servers.forEach { Mobile.removeServer(it.id) } else Mobile.removeSubscription(sub)
                        refresh()
                    }) { Text("✕", color = Muted) }
                }
            }
            items(servers, key = { it.id }) { s ->
                ServerRow(s.id, s.name, "${s.type} · ${s.addr}", s.delay, list.selected == s.id, s.error) {
                    if (s.error.isEmpty()) { Mobile.select(s.id); refresh() }
                }
            }
        }
    }
}

@Composable
private fun ServerRow(id: String, name: String, meta: String, delay: Int, selected: Boolean, error: String = "", onClick: () -> Unit) {
    FocusBox(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 3.dp), onClick = onClick) {
        Row(
            Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp))
                .background(if (selected) Color(0xFF172554) else Card).padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(Modifier.size(14.dp).clip(CircleShape).border(2.dp, if (selected) Blue else Muted, CircleShape)
                .background(if (selected) Blue else Color.Transparent))
            Spacer(Modifier.size(12.dp))
            Column(Modifier.weight(1f)) {
                Text(name, color = Color.White, maxLines = 1)
                if (meta.isNotEmpty()) Text(meta, color = Muted, fontSize = 12.sp, maxLines = 1)
                if (error.isNotEmpty()) Text(error, color = Red, fontSize = 12.sp, maxLines = 2)
            }
            if (delay > 0) Text("${delay}ms", color = if (delay < 300) Green else Color(0xFFFBBF24), fontSize = 13.sp)
        }
    }
}

// ---------------- Settings ----------------

@Composable
private fun SettingsScreen(activity: MainActivity, version: Int, refresh: () -> Unit) {
    val s = remember(version) { Store.settings() }
    val status by Vpn.status.collectAsState()

    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 24.dp)) {
        item {
            CardBox {
                Text(tr("Language"), color = Muted, fontSize = 13.sp)
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(top = 6.dp)) {
                    for ((code, label) in listOf("en" to "EN", "fa" to "FA", "ar" to "AR", "zh" to "ZH")) {
                        val on = s.optString("lang") == code
                        OutlinedButton(onClick = { Store.saveSettings { it.put("lang", code) }; refresh() },
                            colors = ButtonDefaults.outlinedButtonColors(containerColor = if (on) Line else Color.Transparent)) {
                            Text(label, color = Color.White)
                        }
                    }
                }
            }
        }
        item {
            CardBox {
                Toggle(tr("Let the server resolve DNS (recommended)"),
                    tr("DNS queries never leave this PC; the server looks names up in the exit country, so DNS leak tests come out clean"),
                    !s.optBoolean("localDns")) { v -> Store.saveSettings { it.put("localDns", !v) }; refresh() }
                Toggle(tr("Allow QUIC / web UDP"),
                    tr("Only if your server supports UDP; otherwise YouTube and Google are slow or do not load"),
                    s.optBoolean("allowQuic")) { v -> Store.saveSettings { it.put("allowQuic", v) }; refresh() }
                Toggle(tr("Auto-connect"), tr("Connect when the device starts"),
                    s.optBoolean("autoConnect")) { v -> Store.saveSettings { it.put("autoConnect", v) }; refresh() }
            }
        }
        item {
            CardBox {
                Text(tr("Local proxy"), color = Color.White, fontWeight = FontWeight.Bold)
                Toggle(tr("Allow from local network"),
                    tr("Devices on the same Wi-Fi or on this phone's hotspot can use this proxy (set one of the addresses below as their proxy); it has no password"),
                    s.optBoolean("proxyLan")) { v -> Store.saveSettings { it.put("proxyLan", v) }; refresh() }
                if (s.optBoolean("proxyLan")) {
                    val port = s.optInt("proxyPort", 2080)
                    for (ip in lanAddresses()) {
                        Text("socks5 / http   $ip:$port", color = Muted, fontSize = 13.sp, modifier = Modifier.padding(top = 4.dp))
                    }
                }
            }
        }
        item {
            CardBox {
                Text(tr("Domains & IPs"), color = Color.White, fontWeight = FontWeight.Bold)
                Text(tr("Everything through the tunnel, except these"), color = Muted, fontSize = 13.sp)
                var domains by remember(version) { mutableStateOf(s.optJSONArray("domains").strings().joinToString("\n")) }
                OutlinedTextField(domains, { domains = it }, Modifier.fillMaxWidth().height(120.dp).padding(top = 6.dp),
                    placeholder = { Text("example.com\n.com\n192.168.0.0/16", color = Muted) })
                Button(onClick = {
                    Store.saveSettings { it.put("domains", JSONArray(domains.lines().map { l -> l.trim() }.filter { l -> l.isNotEmpty() })) }
                    refresh()
                }, Modifier.padding(top = 6.dp)) { Text(tr("Save")) }
            }
        }
        if (status == Status.STARTED) {
            item { Text(tr("Changes in this section apply on the next connection"), color = Color(0xFFFBBF24), modifier = Modifier.padding(16.dp)) }
        }
        item {
            Text("v2tun ${activity.packageManager.getPackageInfo(activity.packageName, 0).versionName}",
                color = Muted, fontSize = 12.sp, modifier = Modifier.padding(16.dp))
        }
    }
}

@Composable
private fun Toggle(title: String, desc: String, value: Boolean, onChange: (Boolean) -> Unit) {
    FocusBox(Modifier.fillMaxWidth().padding(vertical = 6.dp), onClick = { onChange(!value) }) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f).widthIn(max = 600.dp)) {
                Text(title, color = Color.White, fontWeight = FontWeight.SemiBold)
                Text(desc, color = Muted, fontSize = 12.sp)
            }
            Switch(value, onCheckedChange = onChange)
        }
    }
}

/** This device's addresses other devices can reach: Wi-Fi and its own hotspot. */
private fun lanAddresses(): List<String> = runCatching {
    NetworkInterface.getNetworkInterfaces().toList()
        .filter { it.isUp && !it.isLoopback && !it.name.startsWith("tun") && !it.name.startsWith("rmnet") }
        .flatMap { it.inetAddresses.toList() }
        .filterIsInstance<Inet4Address>()
        .filter { it.isSiteLocalAddress }
        .map { it.hostAddress ?: "" }
        .distinct()
}.getOrDefault(emptyList())
