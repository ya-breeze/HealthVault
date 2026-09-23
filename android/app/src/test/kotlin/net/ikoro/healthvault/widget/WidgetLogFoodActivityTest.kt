package net.ikoro.healthvault.widget

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class WidgetLogFoodActivityTest {
    @Test
    fun `food entry URL comes only from the stored server origin`() {
        assertEquals("https://healthvault.example/food/upload/", widgetLogFoodUrl("https://healthvault.example"))
        assertEquals("http://192.168.1.54:8892/food/upload/", widgetLogFoodUrl(" http://192.168.1.54:8892/// "))
        assertNull(widgetLogFoodUrl(null))
        assertNull(widgetLogFoodUrl("  "))
    }

    @Test
    fun `bridge refreshes and finishes only after food entry returns`() {
        val source = widgetLogFoodActivitySource().readText()
        val resultCallback = source
            .substringAfter("registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {")
            .substringBefore("\n    }")

        assertTrue(resultCallback.contains("RefreshScheduler.enqueueOneOff(applicationContext)"))
        assertTrue(resultCallback.indexOf("RefreshScheduler.enqueueOneOff") < resultCallback.indexOf("finish()"))
        assertTrue(source.contains("if (savedInstanceState != null) return"))
        assertTrue(source.contains("app.secureStore.serverUrl"))
        assertTrue(source.contains("logFood.launch(intent)"))
        assertFalse(source.contains("intent.get"))

        val onCreateBeforeLaunch = source
            .substringAfter("override fun onCreate")
            .substringBefore("logFood.launch(intent)")
        assertTrue(onCreateBeforeLaunch.contains("if (url == null) {"))
        val missingUrlBranch = onCreateBeforeLaunch
            .substringAfter("if (url == null) {")
            .substringBefore("}")
        assertTrue(missingUrlBranch.contains("finish()"))
        assertTrue(missingUrlBranch.contains("return"))
        assertTrue(missingUrlBranch.indexOf("finish()") < missingUrlBranch.indexOf("return"))
        assertFalse(onCreateBeforeLaunch.contains("RefreshScheduler.enqueueOneOff"))
    }

    @Test
    fun `both widget surfaces start the bridge directly`() {
        val summary = appFile("src/main/kotlin/net/ikoro/healthvault/widget/SummaryWidget.kt").readText()
        val flexWindow = appFile("src/main/kotlin/net/ikoro/healthvault/widget/FlexWindowSummaryWidget.kt").readText()

        assertTrue(summary.contains("actionStartActivity(widgetLogFoodIntent(resourceContext))"))
        assertTrue(summary.contains("Intent(context, WidgetLogFoodActivity::class.java)"))
        assertFalse(summary.contains("class LogFoodAction"))
        assertTrue(flexWindow.contains("actionStartActivity("))
        assertTrue(flexWindow.contains("widgetLogFoodIntent(resourceContext)"))
        assertTrue(flexWindow.contains("activityOptions = flexWindowLogFoodActivityOptions()"))
        assertFalse(flexWindow.contains("FlexWindowLogFoodAction"))
    }

    @Test
    fun `manifest keeps bridge private translucent and out of recents`() {
        val manifest = appFile("src/main/AndroidManifest.xml").readText()
        val declaration = manifest
            .substringAfter("android:name=\".widget.WidgetLogFoodActivity\"")
            .substringBefore("/>")

        assertTrue(declaration.contains("android:exported=\"false\""))
        assertTrue(declaration.contains("android:excludeFromRecents=\"true\""))
        assertTrue(declaration.contains("android:taskAffinity=\"\""))
        assertTrue(declaration.contains("@android:style/Theme.Translucent.NoTitleBar"))
    }
}

private fun widgetLogFoodActivitySource(): File =
    appFile("src/main/kotlin/net/ikoro/healthvault/widget/WidgetLogFoodActivity.kt")

private fun appFile(relative: String): File {
    val candidates = listOf(File(relative), File("app/$relative"))
    return candidates.firstOrNull(File::isFile)
        ?: error("$relative not found from ${File(".").absolutePath}")
}
