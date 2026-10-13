package net.ikoro.healthvault.widget

import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.browser.customtabs.CustomTabsIntent
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.work.RefreshScheduler

internal fun widgetLogFoodUrl(serverUrl: String?): String? {
    val base = serverUrl?.trim()?.trimEnd('/')
    if (base.isNullOrEmpty()) return null
    return "$base/food/upload/"
}

/**
 * Invisible bridge between a widget tap and the browser food-entry flow.
 *
 * The widget callback has no lifecycle of its own, so launching the Custom
 * Tab directly gives it no signal when the user returns. This activity owns
 * that result, schedules the existing one-off refresh, then gets out of the
 * task without showing another HealthVault screen.
 */
class WidgetLogFoodActivity : ComponentActivity() {
    private lateinit var flow: net.ikoro.healthvault.diagnostics.WidgetFlow
    private val logFood = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        val app = application as HealthVaultApp
        app.widgetDiagnostics(flow).event("widget_browser", "returned")
        RefreshScheduler.enqueueOneOff(applicationContext, trigger = "food_entry", flow = flow)
        finish()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putString(RefreshScheduler.FLOW_ID, flow.id)
        outState.putLong(RefreshScheduler.FLOW_GENERATION, flow.generation)
        super.onSaveInstanceState(outState)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val app = application as HealthVaultApp
        flow = net.ikoro.healthvault.diagnostics.WidgetFlow(
            id = savedInstanceState?.getString(RefreshScheduler.FLOW_ID) ?: java.util.UUID.randomUUID().toString(),
            generation = savedInstanceState?.getLong(RefreshScheduler.FLOW_GENERATION) ?: app.secureStore.currentSessionGeneration,
            trigger = "food_entry",
        )

        // ComponentActivity restores the pending Activity Result registration
        // after process recreation. Relaunching here would open a second tab.
        if (savedInstanceState != null) return

        val url = widgetLogFoodUrl(app.secureStore.serverUrl)
        if (url == null) {
            app.widgetDiagnostics(flow).event("widget_browser", "failed")
            app.uploadWidgetDiagnostics(flow.generation)
            finish()
            return
        }

        val intent = CustomTabsIntent.Builder().build().intent.apply {
            data = Uri.parse(url)
        }
        try {
            logFood.launch(intent)
            app.widgetDiagnostics(flow).event("widget_browser", "launched")
        } catch (failure: Exception) {
            app.widgetDiagnostics(flow).event("widget_browser",
                if (failure is kotlinx.coroutines.CancellationException) "cancelled" else "failed")
            app.uploadWidgetDiagnostics(flow.generation)
            throw failure
        }
    }
}
