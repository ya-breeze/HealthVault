package net.ikoro.healthvault.widget

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.sp
import androidx.glance.text.FontWeight
import androidx.glance.unit.ColorProvider
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

class WidgetTextTest {
    @Test
    fun widgetTextStyle_usesTheExplicitForeground() {
        val foreground = ColorProvider(Color.White)

        val style = widgetTextStyle(foreground, fontSize = 16.sp, fontWeight = FontWeight.Bold)

        assertSame(foreground, style.color)
        assertEquals(16.sp, style.fontSize)
        assertEquals(FontWeight.Bold, style.fontWeight)
    }

    @Test
    fun summaryWidget_routesEveryTextCallThroughWidgetText() {
        val source = summaryWidgetSource().readText()

        assertEquals(1, directGlanceTextCallCount(source))
        assertTrue(source.contains("color: ColorProvider = GlanceTheme.colors.onBackground"))
        assertTrue(source.contains("color = GlanceTheme.colors.onPrimary"))
    }

    @Test
    fun directTextGuard_detectsABypassOfWidgetText() {
        val unsafeSource = "fun WidgetText() { Text(\"safe\") }\nfun body() { Text(\"black\") }"

        assertEquals(2, directGlanceTextCallCount(unsafeSource))
    }
}

private fun directGlanceTextCallCount(source: String) = Regex("""\bText\(""").findAll(source).count()

private fun summaryWidgetSource(): File {
    val relative = "src/main/kotlin/net/ikoro/healthvault/widget/SummaryWidget.kt"
    val candidates = listOf(File(relative), File("app/$relative"))
    return candidates.firstOrNull(File::isFile)
        ?: error("SummaryWidget.kt not found from ${File(".").absolutePath}")
}
