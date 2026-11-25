package io.github.smma91.v2tun.ui

import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.util.LruCache
import androidx.compose.foundation.Image
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.graphics.drawable.toBitmap
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.Status
import io.github.smma91.v2tun.Store
import io.github.smma91.v2tun.Vpn
import io.github.smma91.v2tun.strings
import io.github.smma91.v2tun.tr
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONArray

/**
 * An installed app. [user]: installed by the user. [system]: part of the system,
 * with no launcher icon (hidden unless asked for).
 */
private data class InstalledApp(val pkg: String, val label: String, val user: Boolean, val system: Boolean)

/** Split tunneling by app: which apps skip the tunnel (or are the only ones using it). */
@Composable
fun AppsScreen(onBack: () -> Unit) {
    val ctx = LocalContext.current
    val status by Vpn.status.collectAsState()
    val initial = remember { Store.settings() }
    var mode by remember { mutableStateOf(initial.optString("appMode", "exclude")) }
    // Kept in the saved order; packages that are no longer installed stay in the list.
    val selected = remember { mutableStateListOf<String>().apply { addAll(initial.optJSONArray("apps").strings().distinct()) } }
    var apps by remember { mutableStateOf<List<InstalledApp>?>(null) }
    var query by remember { mutableStateOf("") }
    var showSystem by remember { mutableStateOf(false) }
    val keyboard = LocalSoftwareKeyboardController.current
    val focusManager = LocalFocusManager.current
    val backFocus = remember { FocusRequester() }

    LaunchedEffect(Unit) {
        val first = selected.toSet()
        apps = withContext(Dispatchers.Default) { runCatching { loadApps(ctx) }.getOrDefault(emptyList()) }
            .sortedWith(compareBy({ it.pkg !in first }, { it.system }, { !it.user }, { it.label.lowercase() }))
    }
    LaunchedEffect(Unit) { runCatching { backFocus.requestFocus() } } // TV remote; does nothing on touch screens

    fun saveApps() = Store.saveSettings { it.put("apps", JSONArray(selected.toList())) }
    fun saveMode(m: String) {
        mode = m
        Store.saveSettings { it.put("appMode", m) }
    }

    val q = query.trim().lowercase()
    val shown = apps?.filter { a ->
        if (q.isEmpty()) showSystem || !a.system || a.pkg in selected
        else a.label.lowercase().contains(q) || a.pkg.lowercase().contains(q)
    }

    LazyColumn(
        Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 32.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        item {
            PageHeader(tr("Apps"), onBack, backFocus) {
                if (selected.isNotEmpty()) Pill(num(selected.size), Accent)
            }
        }
        if (status == Status.STARTED) item { Notice(tr("Changes in this section apply on the next connection")) }
        item {
            Group(
                { ModeRow(tr("Everything through the tunnel, except these"), tr("Listed apps go direct"), mode == "exclude") { saveMode("exclude") } },
                { ModeRow(tr("Only these through the tunnel"), tr("Other apps go direct"), mode == "include") { saveMode("include") } },
            )
        }
        if (selected.isEmpty()) {
            item { Text(tr("The list is empty; all apps go through the tunnel"), Modifier.padding(horizontal = 6.dp, vertical = 4.dp), color = Muted, fontSize = 13.sp) }
        }
        item {
            OutlinedTextField(
                query, { query = it },
                Modifier.fillMaxWidth().padding(top = 6.dp).onPreviewKeyEvent { e ->
                    // Up / down leave the field (one line, so they have no other use): the TV remote is never stuck in it.
                    val dir = when (e.key) {
                        Key.DirectionDown -> FocusDirection.Down
                        Key.DirectionUp -> FocusDirection.Up
                        else -> null
                    }
                    if (dir != null && e.type == KeyEventType.KeyDown) {
                        keyboard?.hide()
                        focusManager.moveFocus(dir)
                    }
                    dir != null
                },
                placeholder = { Text(tr("Search apps"), color = Muted) },
                leadingIcon = { Icon(Icons.Filled.Search, null, tint = Muted) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { keyboard?.hide() }),
                shape = RoundedCornerShape(14.dp),
                colors = OutlinedTextFieldDefaults.colors(unfocusedBorderColor = Line, focusedBorderColor = Accent),
            )
        }
        item {
            Group({
                SwitchRow(tr("Show system apps"), null, showSystem) { showSystem = it }
            })
        }
        when {
            shown == null -> item {
                Box(Modifier.fillMaxWidth().padding(vertical = 40.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = Accent)
                }
            }
            shown.isEmpty() -> item {
                Text(
                    tr("No apps found"), Modifier.fillMaxWidth().padding(vertical = 32.dp),
                    color = Muted, fontSize = 14.sp, textAlign = TextAlign.Center,
                )
            }
            else -> items(shown, key = { it.pkg }) { a ->
                AppRow(a, a.pkg in selected) {
                    if (a.pkg in selected) selected.remove(a.pkg) else selected.add(a.pkg)
                    saveApps()
                }
            }
        }
    }
}

@Composable
private fun ModeRow(title: String, desc: String, on: Boolean, onClick: () -> Unit) {
    ItemRow(title, desc, onClick = onClick) {
        Box(
            Modifier.size(22.dp).clip(CircleShape).border(2.dp, if (on) Accent else Line, CircleShape),
            contentAlignment = Alignment.Center,
        ) { if (on) Box(Modifier.size(12.dp).clip(CircleShape).background(Accent)) }
    }
}

/** One app; the whole row toggles it (one focus stop on TV). */
@Composable
private fun AppRow(app: InstalledApp, on: Boolean, onClick: () -> Unit) {
    val shape = RoundedCornerShape(14.dp)
    Tappable(Modifier.fillMaxWidth(), shape, onClick = onClick) {
        Row(
            Modifier.fillMaxWidth().background(if (on) Accent.copy(alpha = 0.10f) else Card)
                .border(1.dp, if (on) Accent.copy(alpha = 0.5f) else Line, shape)
                .padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            AppIcon(app.pkg)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(app.label, color = TextMain, fontSize = 15.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(app.pkg, color = Muted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            Spacer(Modifier.width(10.dp))
            Box(
                Modifier.size(22.dp).clip(RoundedCornerShape(6.dp))
                    .background(if (on) Accent else Color.Transparent)
                    .border(2.dp, if (on) Accent else Muted, RoundedCornerShape(6.dp)),
                contentAlignment = Alignment.Center,
            ) { if (on) Icon(Icons.Filled.Check, null, Modifier.size(16.dp), tint = Color.White) }
        }
    }
}

private val icons = LruCache<String, ImageBitmap>(300)

@Composable
private fun AppIcon(pkg: String) {
    val ctx = LocalContext.current
    val icon by produceState(icons.get(pkg), pkg) {
        if (value == null) {
            value = withContext(Dispatchers.IO) {
                runCatching {
                    ctx.packageManager.getApplicationIcon(pkg).toBitmap(96, 96).asImageBitmap().also { icons.put(pkg, it) }
                }.getOrNull()
            }
        }
    }
    val m = Modifier.size(40.dp).clip(RoundedCornerShape(10.dp))
    val bmp = icon
    if (bmp != null) Image(bmp, null, m)
    else Box(m.background(Card2), contentAlignment = Alignment.Center) {
        Icon(painterResource(R.drawable.ic_apps), null, Modifier.size(20.dp), tint = Muted)
    }
}

/** Installed apps (slow: call off the main thread). v2tun itself is left out. */
private fun loadApps(ctx: Context): List<InstalledApp> {
    val pm = ctx.packageManager
    val launchable = HashSet<String>()
    for (category in listOf(Intent.CATEGORY_LAUNCHER, Intent.CATEGORY_LEANBACK_LAUNCHER)) {
        runCatching {
            @Suppress("DEPRECATION")
            pm.queryIntentActivities(Intent(Intent.ACTION_MAIN).addCategory(category), 0)
                .forEach { launchable += it.activityInfo.packageName }
        }
    }
    @Suppress("DEPRECATION")
    val installed = pm.getInstalledApplications(0)
    return installed.filter { it.packageName != ctx.packageName }.map { ai ->
        val preinstalled = ai.flags and ApplicationInfo.FLAG_SYSTEM != 0
        InstalledApp(
            ai.packageName,
            runCatching { ai.loadLabel(pm).toString() }.getOrDefault(ai.packageName).ifBlank { ai.packageName },
            !preinstalled,
            preinstalled && ai.packageName !in launchable,
        )
    }
}
