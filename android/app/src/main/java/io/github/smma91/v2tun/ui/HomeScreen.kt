package io.github.smma91.v2tun.ui

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.smma91.v2tun.MainActivity
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.Status
import io.github.smma91.v2tun.Store
import io.github.smma91.v2tun.Vpn
import io.github.smma91.v2tun.libbox.Libbox
import io.github.smma91.v2tun.tr
import kotlinx.coroutines.delay

@Composable
fun HomeScreen(
    activity: MainActivity, version: Int,
    powerFocus: FocusRequester = remember { FocusRequester() },
    openServers: () -> Unit,
) {
    val status by Vpn.status.collectAsState()
    val list = remember(version, status) { Store.servers() }
    val onPower = {
        when {
            status == Status.STARTED || status == Status.STARTING || status == Status.STOPPING -> Vpn.stop(activity)
            list.servers.isEmpty() -> openServers()
            else -> activity.connect()
        }
    }

    BoxWithConstraints(Modifier.fillMaxSize()) {
        if (maxWidth >= 680.dp) {
            // TV / landscape tablet: the button on one side, the details on the other.
            Row(Modifier.fillMaxSize().padding(horizontal = 24.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally) {
                    PowerSection(status, tv = true, powerFocus, onPower)
                }
                Spacer(Modifier.width(24.dp))
                Column(
                    Modifier.weight(1f).fillMaxHeight().verticalScroll(rememberScrollState()).padding(vertical = 24.dp),
                    verticalArrangement = Arrangement.Center,
                ) { Details(status, list, openServers) }
            }
        } else {
            Column(
                Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Spacer(Modifier.height(20.dp))
                PowerSection(status, tv = false, powerFocus, onPower)
                Spacer(Modifier.height(24.dp))
                Box(Modifier.widthIn(max = 560.dp)) { Column { Details(status, list, openServers) } }
                Spacer(Modifier.height(24.dp))
            }
        }
    }
}

@Composable
private fun PowerSection(status: Status, tv: Boolean, focus: FocusRequester, onClick: () -> Unit) {
    val error by Vpn.error.collectAsState()
    val on = status == Status.STARTED
    val busy = status == Status.STARTING || status == Status.STOPPING
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(on) { while (on) { now = System.currentTimeMillis(); delay(1000) } }

    PowerButton(status, focusFirst = tv, focus = focus, onClick = onClick)
    Spacer(Modifier.height(8.dp))
    Text(
        when (status) {
            Status.STARTED -> tr("Connected")
            Status.STARTING -> tr("Connecting…")
            Status.STOPPING -> tr("Disconnecting…")
            Status.STOPPED -> tr("Not connected")
        },
        fontSize = 24.sp, fontWeight = FontWeight.Bold, color = TextMain,
    )
    Spacer(Modifier.height(4.dp))
    Text(
        when {
            on -> duration((now - Vpn.since) / 1000)
            busy -> " "
            tv -> tr("Press OK to connect")
            else -> tr("Tap to connect")
        },
        color = Muted, fontSize = 15.sp,
    )
    val online by Vpn.online.collectAsState()
    if (on && !online) {
        Spacer(Modifier.height(12.dp))
        Pill(tr("This device has no internet (Wi-Fi and mobile data are off or out of reach)"), Warn, dot = true)
    }
    error?.let {
        Spacer(Modifier.height(16.dp))
        Text(
            it, Modifier.fillMaxWidth().widthIn(max = 560.dp).clip(RoundedCornerShape(14.dp))
                .background(Bad.copy(alpha = 0.12f)).border(1.dp, Bad.copy(alpha = 0.35f), RoundedCornerShape(14.dp))
                .padding(12.dp),
            color = Color(0xFFFCA5A5), fontSize = 13.sp,
        )
    }
}

/** Speed, exit IP and the selected server. */
@Composable
private fun Details(status: Status, list: io.github.smma91.v2tun.ServerList, openServers: () -> Unit) {
    if (status == Status.STARTED) {
        val traffic by Vpn.traffic.collectAsState()
        val exit by Vpn.exit.collectAsState()
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Stat(R.drawable.ic_down, tr("Download"), traffic?.down, traffic?.downTotal, Ok, Modifier.weight(1f))
            Stat(R.drawable.ic_up, tr("Upload"), traffic?.up, traffic?.upTotal, Accent, Modifier.weight(1f))
        }
        Spacer(Modifier.height(10.dp))
        Panel {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    Modifier.size(44.dp).clip(RoundedCornerShape(12.dp)).background(Card2),
                    contentAlignment = Alignment.Center,
                ) { Text(exit?.country?.let { flag(it) } ?: "🌐", fontSize = 24.sp) }
                Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
                    Text(tr("Exit IP"), color = Muted, fontSize = 12.sp)
                    Text(exit?.ip ?: "…", color = TextMain, fontSize = 16.sp, fontWeight = FontWeight.Medium)
                    val where = exit?.let { e ->
                        listOf(countryName(e.country), e.city, e.org).filter { it.isNotBlank() }.joinToString(" · ")
                    }.orEmpty()
                    if (where.isNotEmpty()) Text(where, color = Muted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
        }
        Spacer(Modifier.height(10.dp))
    }
    if (list.servers.isEmpty()) {
        Panel {
            Text(tr("No servers yet. Add a link or subscription in the \"Servers\" tab."), color = Muted, fontSize = 14.sp)
            Spacer(Modifier.height(12.dp))
            Button(onClick = openServers) { Text(tr("Add servers")) }
        }
    } else {
        val sel = list.servers.firstOrNull { it.id == list.selected }
        Tappable(Modifier.fillMaxWidth(), RoundedCornerShape(20.dp), onClick = openServers) {
            Panel {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        Modifier.size(44.dp).clip(RoundedCornerShape(12.dp)).background(Accent.copy(alpha = 0.14f)),
                        contentAlignment = Alignment.Center,
                    ) { Icon(painterResource(R.drawable.ic_servers), null, Modifier.size(22.dp), tint = Accent) }
                    Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
                        Text(tr("Server"), color = Muted, fontSize = 12.sp)
                        Text(
                            sel?.name ?: tr("Auto (fastest server)"), color = TextMain, fontSize = 16.sp,
                            fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis,
                        )
                        Text(
                            sel?.let { "${it.type} · ${it.addr}" } ?: tr("{0} servers").replace("{0}", num(list.servers.size)),
                            color = Muted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis,
                        )
                    }
                    Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null, tint = Muted)
                }
            }
        }
    }
}
/** The big round button: grey when off, a spinning arc while connecting, green with a slow pulse when on. */
@Composable
private fun PowerButton(status: Status, focusFirst: Boolean, focus: FocusRequester, onClick: () -> Unit) {
    if (focusFirst) LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    val on = status == Status.STARTED
    val busy = status == Status.STARTING || status == Status.STOPPING
    val color by animateColorAsState(
        when { on -> Ok; busy -> Accent; else -> Color(0xFF475569) }, tween(600), label = "color",
    )
    val t = rememberInfiniteTransition(label = "t")
    val pulse by t.animateFloat(0f, 1f, infiniteRepeatable(tween(2200, easing = LinearEasing)), label = "pulse")
    val spin by t.animateFloat(0f, 360f, infiniteRepeatable(tween(1000, easing = LinearEasing)), label = "spin")

    Box(Modifier.size(260.dp), contentAlignment = Alignment.Center) {
        Canvas(Modifier.fillMaxSize()) {
            val r = size.minDimension / 2
            if (on || busy) {
                drawCircle(Brush.radialGradient(listOf(color.copy(alpha = 0.28f), Color.Transparent), center, r), r)
            }
            if (on) {
                for (k in 0..1) {
                    val p = (pulse + k * 0.5f) % 1f
                    drawCircle(color.copy(alpha = 0.4f * (1 - p)), r * (0.62f + 0.36f * p), style = Stroke(1.5.dp.toPx()))
                }
            }
        }
        Tappable(Modifier.size(168.dp).focusRequester(focus), CircleShape, onClick = onClick) {
            Box(
                Modifier.fillMaxSize().clip(CircleShape)
                    .background(Brush.verticalGradient(listOf(Card2, Card)))
                    .border(1.dp, Line, CircleShape),
            ) {
                Canvas(Modifier.fillMaxSize()) {
                    val w = 5.dp.toPx()
                    val inset = 10.dp.toPx()
                    val arcSize = Size(size.width - inset * 2, size.height - inset * 2)
                    val tl = Offset(inset, inset)
                    drawArc(Line, 0f, 360f, false, tl, arcSize, style = Stroke(w))
                    when {
                        busy -> drawArc(color, spin - 90f, 100f, false, tl, arcSize, style = Stroke(w, cap = StrokeCap.Round))
                        on -> drawArc(color, 0f, 360f, false, tl, arcSize, style = Stroke(w))
                    }
                    // the power symbol
                    val ir = size.minDimension * 0.17f
                    val iw = 6.dp.toPx()
                    val icon = if (on) color else if (busy) Accent else Muted
                    drawArc(
                        icon, -60f, 300f, false, Offset(center.x - ir, center.y - ir), Size(ir * 2, ir * 2),
                        style = Stroke(iw, cap = StrokeCap.Round),
                    )
                    drawLine(icon, Offset(center.x, center.y - ir * 1.3f), Offset(center.x, center.y - ir * 0.1f), iw, StrokeCap.Round)
                }
            }
        }
    }
}

fun duration(s: Long): String =
    num2(s / 3600) + ":" + num2(s / 60 % 60) + ":" + num2(s % 60)

private fun num2(n: Long) = String.format(appLocale(), "%02d", n)

fun num(n: Int): String = String.format(appLocale(), "%d", n)

@Composable
private fun Stat(icon: Int, label: String, perSecond: Long?, total: Long?, color: Color, modifier: Modifier) {
    Panel(modifier, padding = 14.dp) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.size(28.dp).clip(CircleShape).background(color.copy(alpha = 0.15f)),
                contentAlignment = Alignment.Center,
            ) { Icon(painterResource(icon), null, Modifier.size(16.dp), tint = color) }
            Spacer(Modifier.size(8.dp))
            Text(label, color = Muted, fontSize = 12.sp)
        }
        Spacer(Modifier.height(8.dp))
        Text(
            if (perSecond == null) "—" else Libbox.formatBytes(perSecond) + "/s",
            color = TextMain, fontSize = 18.sp, fontWeight = FontWeight.Bold, maxLines = 1,
        )
        Text(if (total == null) " " else Libbox.formatBytes(total), color = Muted, fontSize = 12.sp)
    }
}

/** The country's flag emoji from its two-letter code. */
private fun flag(cc: String): String? {
    if (cc.length != 2 || !cc.all { it.isLetter() }) return null
    return cc.uppercase().map { String(Character.toChars(0x1F1E6 + (it - 'A'))) }.joinToString("")
}

private fun countryName(cc: String): String =
    if (cc.length == 2) java.util.Locale("", cc).getDisplayCountry(appLocale()).ifBlank { cc } else cc