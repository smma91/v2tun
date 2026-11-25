package io.github.smma91.v2tun.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.provider.Settings
import android.widget.Toast
import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.smma91.v2tun.MainActivity
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.Status
import io.github.smma91.v2tun.Store
import io.github.smma91.v2tun.Vpn
import io.github.smma91.v2tun.strings
import io.github.smma91.v2tun.tr
import org.json.JSONArray
import java.net.Inet4Address
import java.net.NetworkInterface

@Composable
fun SettingsScreen(activity: MainActivity, version: Int, refresh: () -> Unit) {
    // A sub-page ("apps" or "log") open over the settings, and the row it was opened from.
    var page by rememberSaveable { mutableStateOf("") }
    var from by rememberSaveable { mutableStateOf("") }
    val listState = rememberLazyListState()
    val appsFocus = remember { FocusRequester() }
    val logFocus = remember { FocusRequester() }
    BackHandler(enabled = page.isNotEmpty()) { page = "" }
    when (page) {
        "apps" -> { AppsScreen { page = ""; refresh() }; return }
        "log" -> { LogScreen { page = "" }; return }
    }
    // Back from a sub-page: the TV remote's focus returns to its row.
    LaunchedEffect(Unit) {
        when (from) {
            "apps" -> runCatching { appsFocus.requestFocus() }
            "log" -> runCatching { logFocus.requestFocus() }
        }
        from = ""
    }

    val s = remember(version) { Store.settings() }
    val status by Vpn.status.collectAsState()
    val save = { change: (org.json.JSONObject) -> Unit -> Store.saveSettings(change); refresh() }

    LazyColumn(Modifier.fillMaxSize(), listState, contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 32.dp)) {
        if (status == Status.STARTED) {
            item { Notice(tr("Changes in this section apply on the next connection"), Modifier.padding(top = 4.dp)) }
        }

        item { SectionTitle(tr("Language")) }
        item {
            Row(
                Modifier.fillMaxWidth().clip(RoundedCornerShape(16.dp)).background(Card)
                    .border(1.dp, Line, RoundedCornerShape(16.dp)).padding(4.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                for ((code, label) in listOf("en" to "EN", "fa" to "FA", "ar" to "AR", "zh" to "ZH")) {
                    val on = s.optString("lang", "en") == code
                    Tappable(Modifier.weight(1f), RoundedCornerShape(12.dp), onClick = { save { it.put("lang", code) } }) {
                        Text(
                            label,
                            Modifier.fillMaxWidth().background(if (on) Accent else Color.Transparent).padding(vertical = 10.dp),
                            color = if (on) Color.White else Muted, fontWeight = FontWeight.Bold,
                            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                        )
                    }
                }
            }
        }

        item { SectionTitle(tr("Connection")) }
        item {
            Group(
                {
                    SwitchRow(
                        tr("Let the server resolve DNS (recommended)"),
                        tr("DNS queries never leave this device; the server looks names up in the exit country, so DNS leak tests come out clean"),
                        !s.optBoolean("localDns"), rememberVectorPainter(Icons.Filled.Search),
                    ) { v -> save { it.put("localDns", !v) } }
                },
                {
                    SwitchRow(
                        tr("Allow QUIC / web UDP"),
                        tr("Only if your server supports UDP; otherwise YouTube and Google are slow or do not load"),
                        s.optBoolean("allowQuic"), rememberVectorPainter(Icons.Filled.PlayArrow),
                    ) { v -> save { it.put("allowQuic", v) } }
                },
                {
                    SwitchRow(
                        tr("Auto-connect"), tr("Connect when the device starts"),
                        s.optBoolean("autoConnect"), painterResource(R.drawable.ic_power),
                    ) { v -> save { it.put("autoConnect", v) } }
                },
                {
                    ItemRow(
                        tr("Kill switch"),
                        tr("In Android's VPN settings, turn on \"Always-on VPN\" and \"Block connections without VPN\" for v2tun; then nothing leaks if the tunnel drops"),
                        rememberVectorPainter(Icons.Filled.Lock), Ok,
                        onClick = { runCatching { activity.startActivity(Intent(Settings.ACTION_VPN_SETTINGS)) } },
                    ) { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = Muted) }
                },
            )
        }

        item { SectionTitle(tr("Local proxy")) }
        item {
            Group(
                {
                    SwitchRow(
                        tr("Allow from local network"),
                        tr("Devices on the same Wi-Fi or on this phone's hotspot can use this proxy (set one of the addresses below as their proxy); it has no password"),
                        s.optBoolean("proxyLan"), rememberVectorPainter(Icons.Filled.Share),
                    ) { v -> save { it.put("proxyLan", v) } }
                },
                {
                    val port = s.optInt("proxyPort", 2080)
                    val addrs = if (s.optBoolean("proxyLan")) lanAddresses() + "127.0.0.1" else listOf("127.0.0.1")
                    Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp)) {
                        Text("SOCKS5 / HTTP", color = Muted, fontSize = 12.sp)
                        Spacer(Modifier.height(8.dp))
                        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                            for (ip in addrs) Address("$ip:$port", activity)
                        }
                    }
                },
            )
        }

        item { SectionTitle(tr("Split tunneling")) }
        item {
            val apps = s.optJSONArray("apps").strings()
            Group({
                ItemRow(
                    tr("Apps"),
                    when {
                        apps.isEmpty() -> tr("The list is empty; all apps go through the tunnel")
                        s.optString("appMode") == "include" -> tr("Other apps go direct")
                        else -> tr("Listed apps go direct")
                    },
                    painterResource(R.drawable.ic_apps),
                    onClick = { from = "apps"; page = "apps" },
                    modifier = Modifier.focusRequester(appsFocus),
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        if (apps.isNotEmpty()) {
                            Pill(num(apps.size), Accent)
                            Spacer(Modifier.width(4.dp))
                        }
                        Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = Muted)
                    }
                }
            })
        }

        item { SectionTitle(tr("Domains & IPs")) }
        item {
            Panel {
                Text(tr("Everything through the tunnel, except these"), color = Muted, fontSize = 13.sp)
                val focusManager = LocalFocusManager.current
                val keyboard = LocalSoftwareKeyboardController.current
                var domains by remember(version) { mutableStateOf(s.optJSONArray("domains").strings().joinToString("\n")) }
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    domains, { domains = it },
                    Modifier.fillMaxWidth().heightIn(min = 120.dp).onPreviewKeyEvent { e ->
                        // A TV remote's up / down leave the box (it would trap the focus otherwise).
                        val dir = when (e.key) {
                            Key.DirectionDown -> FocusDirection.Down
                            Key.DirectionUp -> FocusDirection.Up
                            else -> null
                        }
                        val remote = e.nativeKeyEvent.isFromSource(android.view.InputDevice.SOURCE_DPAD)
                        if (dir != null && remote && e.type == KeyEventType.KeyDown) {
                            keyboard?.hide()
                            focusManager.moveFocus(dir)
                        }
                        dir != null && remote
                    },
                    placeholder = { Text("example.com\n.ir\n192.168.0.0/16", color = Muted) },
                    colors = OutlinedTextFieldDefaults.colors(unfocusedBorderColor = Line, focusedBorderColor = Accent),
                    textStyle = androidx.compose.ui.text.TextStyle(fontFamily = FontFamily.Monospace, fontSize = 14.sp),
                )
                Spacer(Modifier.height(10.dp))
                Button(onClick = {
                    save { it.put("domains", JSONArray(domains.lines().map { l -> l.trim() }.filter { l -> l.isNotEmpty() })) }
                    Toast.makeText(activity, tr("Saved"), Toast.LENGTH_SHORT).show()
                }) { Text(tr("Save")) }
            }
        }

        item { Spacer(Modifier.height(18.dp)) }
        item {
            Group({
                ItemRow(
                    tr("Logs"), null, rememberVectorPainter(Icons.AutoMirrored.Filled.List), Muted,
                    onClick = { from = "log"; page = "log" },
                    modifier = Modifier.focusRequester(logFocus),
                ) { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = Muted) }
            })
        }

        item {
            Row(
                Modifier.fillMaxWidth().padding(top = 24.dp),
                horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Filled.Info, null, Modifier.size(14.dp), tint = Muted)
                Text(
                    "  v2tun " + activity.packageManager.getPackageInfo(activity.packageName, 0).versionName,
                    color = Muted, fontSize = 12.sp,
                )
            }
        }
    }
}

/** An address that copies itself when tapped. */
@Composable
private fun Address(text: String, ctx: Context) {
    Tappable(Modifier.fillMaxWidth(), RoundedCornerShape(10.dp), onClick = {
        val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(ClipData.newPlainText("proxy", text))
        Toast.makeText(ctx, tr("Copied"), Toast.LENGTH_SHORT).show()
    }) {
        Row(
            Modifier.fillMaxWidth().background(Bg).padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(text, Modifier.weight(1f), color = TextMain, fontFamily = FontFamily.Monospace, fontSize = 14.sp)
            Box { Icon(painterResource(R.drawable.ic_copy), null, Modifier.size(16.dp), tint = Muted) }
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
        .mapNotNull { it.hostAddress }
        .distinct()
}.getOrDefault(emptyList())
