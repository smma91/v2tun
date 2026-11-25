package io.github.smma91.v2tun.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import io.github.smma91.v2tun.R

// The desktop app's colours (frontend/style.css).
val Bg = Color(0xFF0B0F19)
val Card = Color(0xFF121826)
val Card2 = Color(0xFF182033)
val Line = Color(0xFF222B40)
val TextMain = Color(0xFFE7EBF3)
val Muted = Color(0xFF8A94A8)
val Accent = Color(0xFF3B82F6)
val Ok = Color(0xFF22C55E)
val Warn = Color(0xFFF59E0B)
val Bad = Color(0xFFEF4444)

// Vazirmatn, as on the desktop (Latin and Persian / Arabic in one font).
val Vazirmatn = FontFamily(
    Font(R.font.vazirmatn_regular, FontWeight.Normal),
    Font(R.font.vazirmatn_medium, FontWeight.Medium),
    Font(R.font.vazirmatn_bold, FontWeight.Bold),
)

private val Colors = darkColorScheme(
    primary = Accent, onPrimary = Color.White,
    secondary = Accent, onSecondary = Color.White,
    background = Bg, onBackground = TextMain,
    surface = Card, onSurface = TextMain,
    surfaceVariant = Card2, onSurfaceVariant = Muted,
    surfaceContainerHigh = Card, surfaceContainerHighest = Card2,
    outline = Line, outlineVariant = Line,
    error = Bad,
)

private fun Typography.withFont(f: FontFamily): Typography {
    fun TextStyle.f() = copy(fontFamily = f)
    return copy(
        displayLarge = displayLarge.f(), displayMedium = displayMedium.f(), displaySmall = displaySmall.f(),
        headlineLarge = headlineLarge.f(), headlineMedium = headlineMedium.f(), headlineSmall = headlineSmall.f(),
        titleLarge = titleLarge.f(), titleMedium = titleMedium.f(), titleSmall = titleSmall.f(),
        bodyLarge = bodyLarge.f(), bodyMedium = bodyMedium.f(), bodySmall = bodySmall.f(),
        labelLarge = labelLarge.f(), labelMedium = labelMedium.f(), labelSmall = labelSmall.f(),
    )
}

@Composable
fun V2tunTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = Colors, typography = Typography().withFont(Vazirmatn), content = content)
}
