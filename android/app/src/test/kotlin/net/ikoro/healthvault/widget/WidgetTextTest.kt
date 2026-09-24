package net.ikoro.healthvault.widget

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.text.FontWeight
import androidx.glance.unit.ColorProvider
import java.io.File
import java.security.MessageDigest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.coroutines.test.runTest

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
        assertFalse(source.contains("SquareIconButton("))
        assertTrue(source.contains("CircleIconButton("))
        assertTrue(source.contains("GlanceTheme.colors.widgetBackground"))
        assertTrue(source.contains("cornerRadius(R.dimen.widget_corner_radius)"))
        assertTrue(source.contains("background(ImageProvider(R.drawable.widget_background))"))
        assertTrue(source.contains(".MacroRows("))
        assertTrue(source.contains("private fun MacroRow"))
        assertTrue(source.contains("internal fun PaceProgress"))
        assertTrue(source.contains("MiniMacroRails(resourceContext, summary)"))
        assertTrue(source.contains("DayNightColorProvider("))
        listOf("0xFF2E7D32", "0xFF69D68B", "0xFF8A5A00", "0xFFF3C75D", "0xFFD93025", "0xFFFF453A")
            .forEach { assertTrue("missing theme-aware pace color $it", source.contains(it)) }
        assertTrue(source.contains("accessibleCardModifier.clickable(actionStartActivity(widgetLogFoodIntent(resourceContext)))"))
        assertTrue(source.contains("accessibleCardModifier.clickable(actionStartActivity("))
        assertTrue(source.contains("semantics {"))
        assertTrue(!source.contains("R.mipmap.ic_launcher"))
        assertTrue(source.contains("ImageProvider(R.drawable.healthvault_mark)"))
        assertFalse(source.contains("widget_identity_short"))
        assertTrue(source.contains("useReducedWidgetContent(fontScale, layout)"))
        assertTrue(source.contains("useMinimalMicroContent(fontScale)"))
        assertTrue(source.contains("fontSize = 11.sp"))
        assertTrue(source.contains("valueSize = 9.sp"))
        assertTrue(source.contains("DpSize(48.dp, 48.dp)"))
        assertTrue(source.contains("DpSize(109.dp, 48.dp)"))
        assertTrue(source.contains("DpSize(48.dp, 110.dp)"))
        assertTrue(source.contains("DpSize(230.dp, 110.dp)"))
    }

    @Test
    fun summaryWidget_selectsEveryDeclaredBreakpoint() {
        val source = summaryWidgetSource().readText()

        val declared = source.substringAfter("SizeMode.Responsive(").substringBefore(")\n")
        listOf(
            "MICRO_SIZE",
            "SHORT_SIZE",
            "WIDE_SHORT_SIZE",
            "TALL_NARROW_SIZE",
            "COMPACT_SIZE",
            "WIDE_SIZE",
            "ROOMY_SHORT_SIZE",
            "ROOMY_COMPACT_SIZE",
        ).forEach { name -> assertTrue(name, Regex("""\b$name\b""").containsMatchIn(declared)) }
        assertTrue(source.contains("DpSize(140.dp, 68.dp)"))
        assertTrue(source.contains("DpSize(150.dp, 150.dp)"))
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
    fun summaryWidget_givesRoomyLauncherCellsTheirOwnLayouts() {
        // Samsung One UI: 2x1 is about 156x72dp, 2x2 about 156x167dp.
        assertEquals(SummaryWidgetLayout.ROOMY_SHORT, summaryWidgetLayout(DpSize(140.dp, 68.dp)))
        assertEquals(SummaryWidgetLayout.ROOMY_SHORT, summaryWidgetLayout(DpSize(156.dp, 72.dp)))
        assertEquals(SummaryWidgetLayout.SHORT, summaryWidgetLayout(DpSize(139.dp, 72.dp)))
        assertEquals(SummaryWidgetLayout.SHORT, summaryWidgetLayout(DpSize(156.dp, 67.dp)))
        assertEquals(SummaryWidgetLayout.ROOMY_COMPACT, summaryWidgetLayout(DpSize(150.dp, 150.dp)))
        assertEquals(SummaryWidgetLayout.ROOMY_COMPACT, summaryWidgetLayout(DpSize(156.dp, 167.dp)))
        assertEquals(SummaryWidgetLayout.COMPACT, summaryWidgetLayout(DpSize(149.dp, 167.dp)))
        assertEquals(SummaryWidgetLayout.COMPACT, summaryWidgetLayout(DpSize(156.dp, 149.dp)))
        // Wider cells keep their existing layouts.
        assertEquals(SummaryWidgetLayout.WIDE_SHORT, summaryWidgetLayout(DpSize(230.dp, 72.dp)))
        assertEquals(SummaryWidgetLayout.WIDE, summaryWidgetLayout(DpSize(230.dp, 167.dp)))
    }

    @Test
    fun summaryWidget_roomyLayoutsFallBackToMinimumCompositionsForLargeText() {
        val body = summaryWidgetSource().readText()
            .substringAfter("private fun SummaryBody(").substringBefore("\n@Composable\nprivate fun")
        assertTrue(!useReducedWidgetContent(1.29f, SummaryWidgetLayout.ROOMY_SHORT))
        assertTrue(useReducedWidgetContent(1.3f, SummaryWidgetLayout.ROOMY_SHORT))
        assertTrue(useReducedWidgetContent(1.3f, SummaryWidgetLayout.ROOMY_COMPACT))
        assertTrue(
            Regex("""ROOMY_SHORT -> if \(useReducedContent\) \{\s*ShortSummary\(""").containsMatchIn(body),
        )
        assertTrue(
            Regex("""ROOMY_COMPACT -> if \(useReducedContent\) \{\s*CompactSummary\(""").containsMatchIn(body),
        )
        assertTrue(body.contains("RoomyShortSummary(resourceContext, summary, isStale)"))
        assertTrue(body.contains("RoomyCompactSummary(resourceContext, summary, isStale)"))
    }

    @Test
    fun summaryWidget_keepsActionsAndDetailsAtTheirIntendedSizes() {
        val source = summaryWidgetSource().readText()
        val compactFunctions = listOf("MicroSummary", "ShortSummary", "WideShortSummary", "TallSummary", "CompactSummary")
            .map { functionBody(source, it) }
        val compact = functionBody(source, "CompactSummary")
        val wide = functionBody(source, "WideSummary")
        val micro = functionBody(source, "MicroSummary")
        val short = functionBody(source, "ShortSummary")
        val wideShort = functionBody(source, "WideShortSummary")
        val tall = functionBody(source, "TallSummary")
        val minimalMicro = micro.substringAfter("if (useMinimalContent) {").substringBefore("\n        return")
        val brandMark = source.substringAfter("internal fun WidgetBrandMark(").substringBefore("\n@Composable\ninternal fun WidgetHeader")
        val header = source.substringAfter("internal fun WidgetHeader(").substringBefore("\n@Composable\nprivate fun androidx.glance.layout.ColumnScope.MacroRows")

        compactFunctions.forEach { body ->
            assertTrue(!body.contains("SquareIconButton("))
            assertTrue(!body.contains("CircleIconButton("))
            assertTrue(!body.contains("MacroRows("))
        }
        assertTrue(compact.contains("MiniMacroRails(resourceContext, summary, showValues = true)"))
        assertTrue(compact.contains("contentAlignment = Alignment.CenterStart"))
        listOf(short, wideShort).forEach { body ->
            assertFalse(
                Regex(
                    """if \(target != null\) \{\s*Spacer\(modifier = GlanceModifier\.defaultWeight\(\)\)""",
                ).containsMatchIn(body),
            )
            assertTrue(body.contains("contentAlignment = Alignment.Center"))
        }
        assertTrue(micro.contains("contentAlignment = Alignment.Center"))
        listOf(short, wideShort).forEach { body ->
            assertTrue(body.contains("Spacer(modifier = GlanceModifier.height(3.dp))"))
        }
        listOf(short, tall).forEach { body ->
            assertTrue(body.contains("WidgetBrandMark("))
        }
        assertFalse(micro.contains("WidgetBrandMark("))
        listOf(tall).forEach { body ->
            assertTrue(body.contains("isStale = isStale"))
        }
        assertTrue(micro.contains("if (useMinimalContent)"))
        assertTrue(micro.contains("valueSize = 9.sp"))
        assertTrue(micro.contains("return"))
        assertFalse(minimalMicro.contains("WidgetBrandMark("))
        assertTrue(micro.contains("MiniMacroRails(resourceContext, summary)"))
        assertTrue(brandMark.contains("if (isStale)"))
        assertTrue(brandMark.contains("text = \"!\""))
        assertTrue(header.contains("WidgetBrandMark(markSize, isStale = isStale)"))
        assertFalse(wide.contains("SquareIconButton("))
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
        assertTrue(!useMinimalMicroContent(1.39f))
        assertTrue(useMinimalMicroContent(1.4f))
        assertTrue(useMinimalMicroContent(1.5f))
        assertTrue(useMinimalMicroContent(2f))
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
        assertTrue(preview.contains("android:src=\"@drawable/healthvault_mark\""))
        assertFalse(preview.contains("widget_identity_short"))
        assertTrue(!preview.contains("@mipmap/ic_launcher"))
        assertTrue(preview.contains("<ProgressBar"))
        assertTrue(preview.contains("android:layout_marginTop=\"3dp\""))
    }

    @Test
    fun widgetBrandMark_matchesTheOwnerSelectedWebArtworkSha256() {
        val widgetMark = appSource("src/main/res/drawable-nodpi/healthvault_mark.png")
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(widgetMark.readBytes())
            .joinToString("") { byte -> "%02x".format(byte) }

        assertEquals("9f63311ce0667e7581fc6e05b95ae6ec115afe0485d6e9a535aece57f5e6d661", digest)
    }

    @Test
    fun launcherIcon_usesTheOwnerSelectedMarkInsteadOfTheGenericWhiteCircle() {
        val foreground = appSource("src/main/res/drawable/ic_launcher_foreground.xml").readText()
        val background = appSource("src/main/res/drawable/ic_launcher_background.xml").readText()

        assertTrue(foreground.contains("@drawable/healthvault_mark"))
        assertTrue(foreground.contains("android:width=\"88dp\""))
        assertTrue(foreground.contains("android:height=\"88dp\""))
        assertFalse(foreground.contains("plus-in-circle"))
        assertFalse(foreground.contains("android:pathData"))
        assertTrue(background.contains("android:fillColor=\"#F7F8FA\""))
    }

    @Test
    fun flexWindowWidget_registersASeparateSamsungSurface() {
        val manifest = appSource("src/main/AndroidManifest.xml").readText()
        val provider = appSource("src/main/res/xml/flex_window_widget_info.xml").readText()
        val samsungProvider = appSource("src/main/res/xml/flex_window_samsung_info.xml").readText()
        val source = appSource("src/main/kotlin/net/ikoro/healthvault/widget/FlexWindowSummaryWidget.kt").readText()
        val preview = appSource("src/main/res/layout/flex_window_widget_preview.xml").readText()
        val message = functionBody(source, "FlexWindowMessage")
        val receiverBlock = manifest
            .substringAfter("android:name=\".widget.FlexWindowSummaryWidgetReceiver\"")
            .substringBefore("</receiver>")

        assertTrue(manifest.contains(".widget.FlexWindowSummaryWidgetReceiver"))
        assertTrue(receiverBlock.contains("android:name=\"com.samsung.android.appwidget.provider\""))
        assertTrue(receiverBlock.contains("android:resource=\"@xml/flex_window_samsung_info\""))
        assertTrue(provider.contains("android:minWidth=\"352dp\""))
        assertTrue(provider.contains("android:minHeight=\"339dp\""))
        assertTrue(provider.contains("android:widgetCategory=\"keyguard\""))
        assertTrue(provider.contains("android:previewLayout=\"@layout/flex_window_widget_preview\""))
        assertTrue(samsungProvider.contains("display=\"sub_screen\""))
        assertTrue(source.contains("class FlexWindowSummaryWidget"))
        assertTrue(source.contains("activityOptions = flexWindowLogFoodActivityOptions()"))
        assertTrue(source.contains("private fun FlexWindowMacroRow"))
        assertTrue(source.contains("WidgetHeader(resourceContext, isStale, markSize = 24"))
        assertTrue(message.contains("WidgetBrandMark(size = 28)"))
        assertFalse(source.contains("fontScale"))
        assertTrue(source.contains("modifier = GlanceModifier.defaultWeight()"))
        assertTrue(preview.contains("android:src=\"@drawable/ic_add_24\""))
        assertTrue(preview.contains("android:src=\"@drawable/ic_refresh_24\""))
        assertTrue(preview.contains("android:src=\"@drawable/healthvault_mark\""))
        assertTrue(preview.contains("android:text=\"@string/widget_preview_target\""))
        val homeSource = summaryWidgetSource().readText()
        assertTrue(homeSource.contains("setLaunchDisplayId(MAIN_DISPLAY_ID)"))
    }

    @Test
    fun widgetLifecycle_updatesAndCountsBothProviders() {
        val source = appSource("src/main/kotlin/net/ikoro/healthvault/widget/SummaryWidgetReceiver.kt").readText()

        assertTrue(source.contains("class FlexWindowSummaryWidgetReceiver"))
        assertTrue(source.contains("getGlanceIds(FlexWindowSummaryWidget::class.java)"))
        assertTrue(source.contains("WidgetUpdater.placementCounts(context).shouldCancelPeriodic"))
        assertTrue(source.contains("RefreshScheduler.ensurePeriodic(context)"))
        assertTrue(source.contains("RefreshScheduler.enqueueOneOff(context)"))

        val none = WidgetPlacementCounts(home = 0, flexWindow = 0)
        assertEquals(0, none.total)
        assertTrue(none.shouldCancelPeriodic)
        listOf(
            WidgetPlacementCounts(home = 1, flexWindow = 0),
            WidgetPlacementCounts(home = 0, flexWindow = 1),
            WidgetPlacementCounts(home = 1, flexWindow = 1),
        ).forEach { placement ->
            assertTrue(placement.total > 0)
            assertFalse(placement.shouldCancelPeriodic)
        }
    }

    @Test
    fun refreshWorker_redrawsBeforeReplacingItsOwnOneOffWork() {
        val source = appSource("src/main/kotlin/net/ikoro/healthvault/work/RefreshWorker.kt").readText()
        val persistedHoldoff = source
            .substringAfter("if (nextRefreshAt > now)")
            .substringBefore("when (val result")
        val newRateLimit = source
            .substringAfter("is ApiResult.RateLimited")
            .substringBefore("is ApiResult.Unauthenticated")

        listOf(persistedHoldoff, newRateLimit).forEach { branch ->
            val redraw = branch.indexOf("WidgetUpdater.updateAll(applicationContext)")
            val replace = branch.indexOf("RefreshScheduler.enqueueOneOff(applicationContext")
            assertTrue("expected a widget redraw in the refresh branch", redraw >= 0)
            assertTrue("expected a replacement one-off in the refresh branch", replace >= 0)
            assertTrue("redraw must finish before REPLACE can cancel this worker", redraw < replace)
        }
    }

    @Test
    fun widgetLifecycle_attemptsBothProviderUpdatesAndPreservesFailures() = runTest {
        var homeUpdates = 0
        var flexUpdates = 0
        val homeFailure = IllegalStateException("home")
        var thrown: Throwable? = null

        try {
            updateBothWidgetProviders(
                updateHome = {
                    homeUpdates++
                    throw homeFailure
                },
                updateFlexWindow = { flexUpdates++ },
            )
        } catch (failure: Throwable) {
            thrown = failure
        }

        assertSame(homeFailure, thrown)
        assertEquals(1, homeUpdates)
        assertEquals(1, flexUpdates)

        val flexFailure = IllegalArgumentException("flex")
        thrown = null
        try {
            updateBothWidgetProviders(
                updateHome = { homeUpdates++ },
                updateFlexWindow = {
                    flexUpdates++
                    throw flexFailure
                },
            )
        } catch (failure: Throwable) {
            thrown = failure
        }

        assertSame(flexFailure, thrown)
        assertEquals(2, homeUpdates)
        assertEquals(2, flexUpdates)
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
