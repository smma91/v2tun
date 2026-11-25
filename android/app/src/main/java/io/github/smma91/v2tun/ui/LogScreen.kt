package io.github.smma91.v2tun.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.runtime.CompositionLocalProvider
import io.github.smma91.v2tun.CoreLog
import io.github.smma91.v2tun.LogLine
import io.github.smma91.v2tun.R
import io.github.smma91.v2tun.Status
import io.github.smma91.v2tun.V2tunVpnService
import io.github.smma91.v2tun.Vpn
import io.github.smma91.v2tun.tr
import kotlinx.coroutines.launch

/** The core's log, live while connected (by default only warnings and errors). */
@Composable
fun LogScreen(onBack: () -> Unit) {
    val ctx = LocalContext.current
    val status by Vpn.status.collectAsState()
    var onlyWarnings by rememberSaveable { mutableStateOf(true) }
    val all by CoreLog.lines.collectAsState()
    val warnings by CoreLog.important.collectAsState()
    val lines = if (onlyWarnings) warnings else all
    val list = rememberLazyListState()
    val scope = rememberCoroutineScope()
    val backFocus = remember { FocusRequester() }
    // Follows new lines while the list is at its end.
    var follow by remember { mutableStateOf(true) }

    DisposableEffect(Unit) {
        CoreLog.visible = true
        onDispose { CoreLog.visible = false }
    }
    LaunchedEffect(Unit) { runCatching { backFocus.requestFocus() } } // TV remote; does nothing on touch screens
    LaunchedEffect(list) {
        snapshotFlow { list.isScrollInProgress }.collect { scrolling -> if (!scrolling) follow = !list.canScrollForward }
    }
    LaunchedEffect(lines.lastOrNull()?.id, onlyWarnings) {
        if (follow && lines.isNotEmpty()) list.scrollToItem(lines.lastIndex)
    }

    Column(Modifier.fillMaxSize().padding(start = 16.dp, end = 16.dp, bottom = 16.dp)) {
        PageHeader(tr("Logs"), onBack, backFocus) {
            TextAction(painterResource(R.drawable.ic_copy), tr("Copy")) {
                val text = lines.joinToString("\n") { it.text }
                val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                cm.setPrimaryClip(ClipData.newPlainText("log", text))
                Toast.makeText(ctx, tr("Copied"), Toast.LENGTH_SHORT).show()
            }
            TextAction(rememberVectorPainter(Icons.Filled.Delete), tr("Clear")) {
                CoreLog.clear()
                V2tunVpnService.instance?.clearLog()
                follow = true
            }
        }
        if (status != Status.STARTED) {
            Notice(tr("Connect to see the log live"), Modifier.padding(top = 4.dp, bottom = 8.dp))
        }
        Group({
            SwitchRow(tr("Only warnings and errors"), null, onlyWarnings) { onlyWarnings = it; follow = true }
        })

        // One focus stop on TV: up / down scroll the lines.
        val source = remember { MutableInteractionSource() }
        val focused by source.collectIsFocusedAsState()
        val shape = RoundedCornerShape(16.dp)
        Box(
            Modifier.weight(1f).fillMaxWidth().padding(top = 4.dp).clip(shape).background(Card)
                .border(if (focused) 2.dp else 1.dp, if (focused) Accent else Line, shape)
                .onKeyEvent { e ->
                    if (e.type != KeyEventType.KeyDown) return@onKeyEvent false
                    val step = list.layoutInfo.viewportSize.height * 0.4f
                    when (e.key) {
                        Key.DirectionDown -> {
                            if (list.canScrollForward) scope.launch { list.scrollBy(step) }
                            true
                        }
                        Key.DirectionUp -> {
                            if (list.canScrollBackward) { scope.launch { list.scrollBy(-step) }; true } else false
                        }
                        else -> false
                    }
                }
                .focusable(interactionSource = source),
        ) {
            if (lines.isEmpty()) {
                Text(
                    if (onlyWarnings) tr("No warnings or errors so far") else tr("Nothing logged yet"),
                    Modifier.fillMaxWidth().align(Alignment.Center).padding(24.dp),
                    color = Muted, fontSize = 14.sp, textAlign = TextAlign.Center,
                )
            }
            // Log lines are always left to right, also in Persian and Arabic.
            CompositionLocalProvider(LocalLayoutDirection provides LayoutDirection.Ltr) {
                LazyColumn(
                    Modifier.fillMaxSize(), list,
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 10.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    items(lines, key = { it.id }) { LogRow(it) }
                }
            }
        }
    }
}

/** A line with a bar in its level's colour (red: error, amber: warning, blue: info, grey: debug). */
@Composable
private fun LogRow(line: LogLine) {
    val (bar, text) = when {
        line.level <= CoreLog.LEVEL_ERROR -> Bad to Color(0xFFFCA5A5)
        line.level == CoreLog.LEVEL_WARN -> Warn to Color(0xFFFCD34D)
        line.level == CoreLog.LEVEL_INFO -> Accent to TextMain
        else -> Muted to Muted
    }
    Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min)) {
        Box(Modifier.fillMaxHeight().width(3.dp).clip(RoundedCornerShape(2.dp)).background(bar))
        Spacer(Modifier.width(8.dp))
        Text(line.text, color = text, fontFamily = FontFamily.Monospace, fontSize = 11.5.sp, lineHeight = 16.sp)
    }
}
