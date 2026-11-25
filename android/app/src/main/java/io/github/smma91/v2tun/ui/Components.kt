package io.github.smma91.v2tun.ui

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.smma91.v2tun.tr

/**
 * Clickable, with a press effect and a visible focus ring for the TV remote.
 */
@Composable
fun Tappable(
    modifier: Modifier = Modifier,
    shape: Shape = RoundedCornerShape(14.dp),
    enabled: Boolean = true,
    onClick: () -> Unit,
    content: @Composable () -> Unit,
) {
    val source = remember { MutableInteractionSource() }
    val focused by source.collectIsFocusedAsState()
    val pressed by source.collectIsPressedAsState()
    val ring by animateColorAsState(if (focused) Accent else Color.Transparent, label = "ring")
    val scale by animateFloatAsState(if (pressed) 0.97f else 1f, label = "press")
    Box(
        modifier
            .scale(scale)
            .clip(shape)
            .border(2.dp, ring, shape)
            .clickable(interactionSource = source, indication = null, enabled = enabled, onClick = onClick),
    ) { content() }
}

/** A rounded card, as on the desktop. */
@Composable
fun Panel(modifier: Modifier = Modifier, padding: Dp = 16.dp, content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(20.dp))
            .background(Card)
            .border(1.dp, Line, RoundedCornerShape(20.dp))
            .padding(padding),
        content = content,
    )
}

@Composable
fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(
        text, modifier.padding(start = 6.dp, end = 6.dp, top = 18.dp, bottom = 8.dp),
        color = Muted, fontSize = 13.sp, fontWeight = FontWeight.Medium,
    )
}

/** A settings row: icon, title, description, and something at the end. */
@Composable
fun ItemRow(
    title: String,
    desc: String? = null,
    icon: Painter? = null,
    iconTint: Color = Accent,
    onClick: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
    end: (@Composable () -> Unit)? = null,
) {
    val body = @Composable {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (icon != null) {
                Box(
                    Modifier.size(36.dp).clip(RoundedCornerShape(10.dp)).background(iconTint.copy(alpha = 0.14f)),
                    contentAlignment = Alignment.Center,
                ) { Icon(icon, null, Modifier.size(20.dp), tint = iconTint) }
                Spacer(Modifier.width(14.dp))
            }
            Column(Modifier.weight(1f)) {
                Text(title, color = TextMain, fontSize = 15.sp, fontWeight = FontWeight.Medium)
                if (!desc.isNullOrEmpty()) {
                    Spacer(Modifier.height(2.dp))
                    Text(desc, color = Muted, fontSize = 12.5.sp, lineHeight = 18.sp)
                }
            }
            if (end != null) {
                Spacer(Modifier.width(12.dp))
                end()
            }
        }
    }
    if (onClick != null) Tappable(modifier.fillMaxWidth(), RoundedCornerShape(16.dp), onClick = onClick) { body() }
    else body()
}

/** A highlighted note (e.g. "applies on the next connection"). */
@Composable
fun Notice(text: String, modifier: Modifier = Modifier) {
    Text(
        text,
        modifier.fillMaxWidth().clip(RoundedCornerShape(14.dp))
            .background(Warn.copy(alpha = 0.12f)).border(1.dp, Warn.copy(alpha = 0.3f), RoundedCornerShape(14.dp))
            .padding(12.dp),
        color = Color(0xFFFCD34D), fontSize = 13.sp,
    )
}

/** The top of a sub-page: a back button (its own focus stop), the title and actions at the end. */
@Composable
fun PageHeader(
    title: String,
    onBack: () -> Unit,
    backFocus: FocusRequester? = null,
    actions: @Composable RowScope.() -> Unit = {},
) {
    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Tappable(
            Modifier.size(44.dp).then(if (backFocus != null) Modifier.focusRequester(backFocus) else Modifier),
            CircleShape, onClick = onBack,
        ) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, tr("Back"), Modifier.size(22.dp), tint = TextMain)
            }
        }
        Spacer(Modifier.width(8.dp))
        Text(
            title, Modifier.weight(1f), color = TextMain, fontSize = 18.sp, fontWeight = FontWeight.Bold,
            maxLines = 1, overflow = TextOverflow.Ellipsis,
        )
        actions()
    }
}

/** A small text button with an icon (its own focus stop on TV). */
@Composable
fun TextAction(icon: Painter, label: String, onClick: () -> Unit) {
    Tappable(shape = RoundedCornerShape(99.dp), onClick = onClick) {
        Row(Modifier.padding(horizontal = 12.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, null, Modifier.size(16.dp), tint = Accent)
            Spacer(Modifier.width(6.dp))
            Text(label, color = Accent, fontSize = 13.sp, fontWeight = FontWeight.Medium)
        }
    }
}

/** A settings row with a switch; the whole row toggles (one focus stop on TV). */
@Composable
fun SwitchRow(title: String, desc: String?, value: Boolean, icon: Painter? = null, onChange: (Boolean) -> Unit) {
    ItemRow(title, desc, icon, onClick = { onChange(!value) }) {
        Switch(
            checked = value, onCheckedChange = null,
            colors = SwitchDefaults.colors(
                checkedTrackColor = Accent, checkedThumbColor = Color.White,
                uncheckedTrackColor = Line, uncheckedThumbColor = Muted, uncheckedBorderColor = Color.Transparent,
            ),
        )
    }
}

/** A group of rows in one card, separated by thin lines. */
@Composable
fun Group(vararg rows: @Composable () -> Unit) {
    Column(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(20.dp)).background(Card)
            .border(1.dp, Line, RoundedCornerShape(20.dp)).padding(vertical = 4.dp),
    ) {
        rows.forEachIndexed { i, r ->
            if (i > 0) HorizontalDivider(Modifier.padding(horizontal = 14.dp), color = Line)
            r()
        }
    }
}

/** A small rounded label (delay, status). */
@Composable
fun Pill(text: String, color: Color, modifier: Modifier = Modifier, dot: Boolean = false) {
    Row(
        modifier.clip(RoundedCornerShape(99.dp)).background(color.copy(alpha = 0.14f))
            .padding(horizontal = 10.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        if (dot) Box(Modifier.size(7.dp).clip(RoundedCornerShape(99.dp)).background(color))
        Text(text, color = color, fontSize = 12.sp, fontWeight = FontWeight.Medium)
    }
}
