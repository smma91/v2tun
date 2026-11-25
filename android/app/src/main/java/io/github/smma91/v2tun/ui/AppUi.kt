package io.github.smma91.v2tun.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.smma91.v2tun.MainActivity
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.Status
import io.github.smma91.v2tun.Store
import io.github.smma91.v2tun.Vpn
import io.github.smma91.v2tun.tr
import java.util.Locale

private var currentLang = "en"

/** The UI language as a Locale (digits and dates in that language). */
fun appLocale(): Locale = Locale.forLanguageTag(currentLang)

@Composable
fun AppUi(activity: MainActivity) {
    // Changing the language or the data redraws everything.
    var version by remember { mutableIntStateOf(0) }
    val refresh: () -> Unit = { version++ }
    val lang = remember(version) { Store.settings().optString("lang", "en").also { currentLang = it } }
    val dir = if (lang == "fa" || lang == "ar") LayoutDirection.Rtl else LayoutDirection.Ltr
    var tab by rememberSaveable { mutableIntStateOf(0) }
    // TVs and tablets in landscape: navigation on the side.
    val wide = LocalConfiguration.current.screenWidthDp >= 720

    val tabs = listOf(
        Triple(tr("Connect"), painterResource(R.drawable.ic_power), 0),
        Triple(tr("Servers"), painterResource(R.drawable.ic_servers), 1),
        Triple(tr("Settings"), rememberVectorPainter(Icons.Filled.Settings), 2),
    )
    val content = @Composable { m: Modifier ->
        AnimatedContent(tab, m, transitionSpec = { fadeIn(tween(180)) togetherWith fadeOut(tween(120)) }, label = "tab") { t ->
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
                Box(Modifier.widthIn(max = 720.dp).fillMaxSize()) {
                    when (t) {
                        0 -> HomeScreen(activity, version) { tab = 1 }
                        1 -> ServersScreen(activity, version, refresh)
                        else -> SettingsScreen(activity, version, refresh)
                    }
                }
            }
        }
    }

    V2tunTheme {
        CompositionLocalProvider(LocalLayoutDirection provides dir) {
            Surface(Modifier.fillMaxSize(), color = Bg) {
                if (wide) {
                    Row(Modifier.fillMaxSize()) {
                        Column(
                            Modifier.width(200.dp).fillMaxHeight().background(Card).padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp),
                        ) {
                            Header(compact = true)
                            Spacer(Modifier.height(12.dp))
                            for ((label, icon, i) in tabs) SideItem(label, icon, tab == i) { tab = i }
                        }
                        Column(Modifier.weight(1f).fillMaxHeight().padding(top = 16.dp)) { content(Modifier.weight(1f)) }
                    }
                } else {
                    Column(Modifier.fillMaxSize()) {
                        Header(compact = false)
                        content(Modifier.weight(1f))
                        HorizontalDivider(color = Line)
                        Row(Modifier.fillMaxWidth().background(Card).padding(horizontal = 8.dp, vertical = 8.dp)) {
                            for ((label, icon, i) in tabs) BottomItem(label, icon, tab == i, Modifier.weight(1f)) { tab = i }
                        }
                    }
                }
            }
        }
    }
}

/** The logo, the name and the connection state. */
@Composable
private fun Header(compact: Boolean) {
    val status by Vpn.status.collectAsState()
    Row(
        Modifier.fillMaxWidth().padding(horizontal = if (compact) 4.dp else 20.dp, vertical = if (compact) 8.dp else 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(painterResource(R.drawable.ic_tile), null, Modifier.size(26.dp), tint = Accent)
        Spacer(Modifier.width(10.dp))
        Text("v2tun", color = TextMain, fontSize = 20.sp, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
        if (!compact) {
            when (status) {
                Status.STARTED -> Pill(tr("Connected"), Ok, dot = true)
                Status.STARTING, Status.STOPPING -> Pill("…", Accent, dot = true)
                Status.STOPPED -> Pill(tr("Not connected"), Muted, dot = true)
            }
        }
    }
}

@Composable
private fun BottomItem(label: String, icon: Painter, selected: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val bg by animateColorAsState(if (selected) Accent.copy(alpha = 0.18f) else Color.Transparent, label = "bg")
    val fg by animateColorAsState(if (selected) TextMain else Muted, label = "fg")
    Tappable(modifier, RoundedCornerShape(16.dp), onClick = onClick) {
        Column(Modifier.fillMaxWidth().padding(vertical = 4.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Box(
                Modifier.width(60.dp).height(32.dp).clip(RoundedCornerShape(99.dp)).background(bg),
                contentAlignment = Alignment.Center,
            ) { Icon(icon, null, Modifier.size(22.dp), tint = if (selected) Accent else Muted) }
            Spacer(Modifier.height(4.dp))
            Text(label, color = fg, fontSize = 12.sp, fontWeight = if (selected) FontWeight.Bold else FontWeight.Medium)
        }
    }
}

@Composable
private fun SideItem(label: String, icon: Painter, selected: Boolean, onClick: () -> Unit) {
    val bg by animateColorAsState(if (selected) Accent.copy(alpha = 0.18f) else Color.Transparent, label = "bg")
    Tappable(Modifier.fillMaxWidth(), RoundedCornerShape(14.dp), onClick = onClick) {
        Row(
            Modifier.fillMaxWidth().background(bg).padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(icon, null, Modifier.size(22.dp), tint = if (selected) Accent else Muted)
            Spacer(Modifier.width(12.dp))
            Text(label, color = if (selected) TextMain else Muted, fontSize = 15.sp, fontWeight = FontWeight.Medium)
        }
    }
}
