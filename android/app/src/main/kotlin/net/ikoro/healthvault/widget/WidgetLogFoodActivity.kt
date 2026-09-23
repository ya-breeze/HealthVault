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
    private val logFood = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        RefreshScheduler.enqueueOneOff(applicationContext)
        finish()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // ComponentActivity restores the pending Activity Result registration
        // after process recreation. Relaunching here would open a second tab.
        if (savedInstanceState != null) return

        val app = application as HealthVaultApp
        val url = widgetLogFoodUrl(app.secureStore.serverUrl)
        if (url == null) {
            finish()
            return
        }

        val intent = CustomTabsIntent.Builder().build().intent.apply {
            data = Uri.parse(url)
        }
        logFood.launch(intent)
    }
}
