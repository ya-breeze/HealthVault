package net.ikoro.healthvault.widget

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.text.FontWeight
import androidx.glance.unit.ColorProvider
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
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
        assertTrue(source.contains("color: ColorProvider = GlanceTheme.colors.onSurface"))
        assertTrue(source.contains("GlanceTheme.colors.onSurfaceVariant"))
    }

    @Test
    fun directTextGuard_detectsABypassOfWidgetText() {
        val unsafeSource = "fun WidgetText() { Text(\"safe\") }\nfun body() { Text(\"black\") }"

        assertEquals(2, directGlanceTextCallCount(unsafeSource))
    }

    @Test
    fun progressFraction_clampsToTheIndicatorRange() {
        assertEquals(1546f / 2139f, progressFraction(1546, 2139), 0.0001f)
        assertEquals(1f, progressFraction(2400, 2000), 0f)
        assertEquals(0f, progressFraction(-1, 2000), 0f)
        assertEquals(0f, progressFraction(100, 0), 0f)
    }

    @Test
    fun progressPercent_staysTruthfulAboveTargetAndRejectsMissingTargets() {
        assertEquals(72, progressPercent(1546, 2139))
        assertEquals(120, progressPercent(2400, 2000))
        assertNull(progressPercent(100, 0))
    }

    @Test
    fun summaryWidget_keepsResponsiveMaterialStructure() {
        val source = summaryWidgetSource().readText()

        assertTrue(source.contains("private fun MicroSummary"))
        assertTrue(source.contains("private fun ShortSummary"))
        assertTrue(source.contains("private fun WideShortSummary"))
        assertTrue(source.contains("private fun TallSummary"))
        assertTrue(source.contains("private fun CompactSummary"))
        assertTrue(source.contains("private fun WideSummary"))
        assertTrue(source.contains("LinearProgressIndicator("))
        assertTrue(source.contains("SquareIconButton("))
        assertTrue(source.contains("CircleIconButton("))
        assertTrue(source.contains("GlanceTheme.colors.widgetBackground"))
        assertTrue(source.contains("cornerRadius(R.dimen.widget_corner_radius)"))
        assertTrue(source.contains("background(ImageProvider(R.drawable.widget_background))"))
        assertTrue(source.contains(".MacroRows("))
        assertTrue(source.contains("private fun MacroRow"))
        assertTrue(source.contains("GlanceModifier.defaultWeight().height(5.dp)"))
        assertTrue(source.contains("accessibleCardModifier.clickable(actionRunCallback<LogFoodAction>())"))
        assertTrue(source.contains("accessibleCardModifier.clickable(actionStartActivity("))
        assertTrue(source.contains("semantics {"))
        assertTrue(source.contains("private fun AppIdentity"))
        assertTrue(source.contains("useReducedWidgetContent(fontScale, layout)"))
        assertTrue(source.contains("useMinimalMicroContent(fontScale)"))
        assertTrue(source.contains("fontSize = 11.sp"))
        assertTrue(!Regex("""fontSize = (9|10)\.sp""").containsMatchIn(source))
        assertTrue(source.contains("DpSize(48.dp, 48.dp)"))
        assertTrue(source.contains("DpSize(109.dp, 48.dp)"))
        assertTrue(source.contains("DpSize(48.dp, 110.dp)"))
        assertTrue(source.contains("DpSize(230.dp, 110.dp)"))
    }

    @Test
    fun summaryWidget_selectsEveryDeclaredBreakpoint() {
        val source = summaryWidgetSource().readText()

        assertTrue(
            source.contains(
                "setOf(MICRO_SIZE, SHORT_SIZE, WIDE_SHORT_SIZE, TALL_NARROW_SIZE, COMPACT_SIZE, WIDE_SIZE)",
            ),
        )
        assertEquals(SummaryWidgetLayout.MICRO, summaryWidgetLayout(DpSize(48.dp, 48.dp)))
        assertEquals(SummaryWidgetLayout.SHORT, summaryWidgetLayout(DpSize(109.dp, 48.dp)))
        assertEquals(SummaryWidgetLayout.WIDE_SHORT, summaryWidgetLayout(DpSize(230.dp, 48.dp)))
        assertEquals(SummaryWidgetLayout.TALL, summaryWidgetLayout(DpSize(48.dp, 110.dp)))
        assertEquals(SummaryWidgetLayout.COMPACT, summaryWidgetLayout(DpSize(110.dp, 110.dp)))
        assertEquals(SummaryWidgetLayout.WIDE, summaryWidgetLayout(DpSize(230.dp, 110.dp)))
        assertEquals(SummaryWidgetLayout.MICRO, summaryWidgetLayout(DpSize(108.dp, 109.dp)))
        assertEquals(SummaryWidgetLayout.TALL, summaryWidgetLayout(DpSize(109.dp, 110.dp)))
        assertEquals(SummaryWidgetLayout.COMPACT, summaryWidgetLayout(DpSize(229.dp, 110.dp)))
        assertEquals(SummaryWidgetLayout.WIDE_SHORT, summaryWidgetLayout(DpSize(230.dp, 109.dp)))
    }

    @Test
    fun summaryWidget_keepsActionsAndDetailsAtTheirIntendedSizes() {
        val source = summaryWidgetSource().readText()
        val compactFunctions = listOf("MicroSummary", "ShortSummary", "WideShortSummary", "TallSummary", "CompactSummary")
            .map { functionBody(source, it) }
        val compact = functionBody(source, "CompactSummary")
        val wide = functionBody(source, "WideSummary")

        compactFunctions.forEach { body ->
            assertTrue(!body.contains("SquareIconButton("))
            assertTrue(!body.contains("CircleIconButton("))
            assertTrue(!body.contains("MacroRows("))
        }
        assertTrue(compact.indexOf("GlanceModifier.defaultWeight()") < compact.indexOf("calorieHeroText("))
        assertTrue(wide.contains("SquareIconButton("))
        assertTrue(wide.contains("CircleIconButton("))
        assertTrue(wide.contains("MacroRows(resourceContext, summary)"))
        assertTrue(wide.contains("if (useReducedContent)"))
    }

    @Test
    fun summaryWidget_reducesSecondaryContentForLargeText() {
        assertTrue(!useReducedWidgetContent(1.09f, SummaryWidgetLayout.MICRO))
        assertTrue(useReducedWidgetContent(1.1f, SummaryWidgetLayout.MICRO))
        assertTrue(!useReducedWidgetContent(1.29f, SummaryWidgetLayout.SHORT))
        assertTrue(useReducedWidgetContent(1.3f, SummaryWidgetLayout.SHORT))
        assertTrue(useReducedWidgetContent(2f, SummaryWidgetLayout.WIDE))
        assertTrue(!useMinimalMicroContent(1.79f))
        assertTrue(useMinimalMicroContent(1.8f))
        assertEquals("1928", calorieHeroText(1928, isStale = false, useReducedContent = true))
        assertEquals("1928", calorieHeroText(1928, isStale = true, useReducedContent = false))
        assertEquals("1928!", calorieHeroText(1928, isStale = true, useReducedContent = true))
    }

    @Test
    fun widgetPicker_usesARepresentativePreviewAndDescription() {
        val provider = appSource("src/main/res/xml/summary_widget_info.xml").readText()
        val preview = appSource("src/main/res/layout/summary_widget_preview.xml").readText()

        assertTrue(provider.contains("android:description=\"@string/widget_description\""))
        assertTrue(provider.contains("android:previewLayout=\"@layout/summary_widget_preview\""))
        assertTrue(provider.contains("android:minWidth=\"110dp\""))
        assertTrue(provider.contains("android:minHeight=\"48dp\""))
        assertTrue(provider.contains("android:minResizeWidth=\"48dp\""))
        assertTrue(provider.contains("android:minResizeHeight=\"48dp\""))
        assertTrue(provider.contains("android:targetCellWidth=\"2\""))
        assertTrue(provider.contains("android:targetCellHeight=\"1\""))
        assertTrue(provider.contains("android:maxResizeWidth=\"624dp\""))
        assertTrue(provider.contains("android:maxResizeHeight=\"276dp\""))
        assertTrue(provider.contains("android:resizeMode=\"horizontal|vertical\""))
        assertTrue(preview.contains("android:text=\"@string/widget_preview_consumed\""))
        assertTrue(preview.contains("android:text=\"@string/widget_preview_percent\""))
        assertTrue(preview.contains("android:src=\"@mipmap/ic_launcher\""))
        assertTrue(preview.contains("<ProgressBar"))
    }

    @Test
    fun widgetResources_keepLocaleAndThemeVariantsInSync() {
        val englishWidgetStrings = resourceNames(appSource("src/main/res/values/strings.xml"), "string")
            .filterTo(sortedSetOf()) { it.startsWith("widget_") }
        val russianWidgetStrings = resourceNames(appSource("src/main/res/values-ru/strings.xml"), "string")
            .filterTo(sortedSetOf()) { it.startsWith("widget_") }
        assertEquals(englishWidgetStrings, russianWidgetStrings)

        val dayColors = resourceNames(appSource("src/main/res/values/colors.xml"), "color")
        val nightColors = resourceNames(appSource("src/main/res/values-night/colors.xml"), "color")
        val dynamicDayColors = resourceNames(appSource("src/main/res/values-v31/colors.xml"), "color")
        val dynamicNightColors = resourceNames(appSource("src/main/res/values-night-v31/colors.xml"), "color")
        assertEquals(dayColors, nightColors)
        assertEquals(dayColors, dynamicDayColors)
        assertEquals(dayColors, dynamicNightColors)
    }
}

private fun directGlanceTextCallCount(source: String) = Regex("""\bText\(""").findAll(source).count()

private fun summaryWidgetSource(): File {
    return appSource("src/main/kotlin/net/ikoro/healthvault/widget/SummaryWidget.kt")
}

private fun functionBody(source: String, name: String): String =
    source.substringAfter("private fun $name(").substringBefore("\n@Composable\nprivate fun")

private fun appSource(relative: String): File {
    val candidates = listOf(File(relative), File("app/$relative"))
    return candidates.firstOrNull(File::isFile)
        ?: error("$relative not found from ${File(".").absolutePath}")
}

private fun resourceNames(file: File, tag: String): Set<String> =
    Regex("""<$tag name="([^"]+)"""").findAll(file.readText()).map { it.groupValues[1] }.toSet()
