package net.ikoro.healthvault.ui

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.widget.WidgetUpdater
import net.ikoro.healthvault.work.RefreshScheduler
import net.ikoro.healthvault.weather.WeatherScheduler

/** Hosts setup, the daily summary and the separate settings screen. */
class MainActivity : AppCompatActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as HealthVaultApp
        // A background worker can invalidate the session while no activity
        // delegate exists. Reset once AppCompat's delegate is active so its
        // persisted locale cannot leak onto the next setup screen.
        if (!app.secureStore.hasSession()) applyDisplayLanguage("")

        setContent {
            var hasSession by remember { mutableStateOf(app.secureStore.hasSession()) }
            val scope = rememberCoroutineScope()
            var showingSettings by rememberSaveable { mutableStateOf(false) }

            val signOut: () -> Unit = {
                scope.launch {
                    withContext(Dispatchers.IO) {
                        // Clear durable credentials before the cookie jar or UI.
                        app.secureStore.clearSession()
                        WeatherScheduler.reconcile(applicationContext)
                        app.cookieJar.clearInMemory()
                    }
                    applyDisplayLanguage("")
                    WidgetUpdater.updateAll(applicationContext)
                    showingSettings = false
                    hasSession = false
                }
            }

            if (hasSession) {
                if (showingSettings) {
                    SettingsScreen(
                        secureStore = app.secureStore,
                        onBack = { showingSettings = false },
                        onSignedOut = signOut,
                    )
                } else {
                    TodayScreen(
                        api = app.api,
                        secureStore = app.secureStore,
                        onSignedOut = signOut,
                        onOpenSettings = { showingSettings = true },
                    )
                }
            } else {
                SetupScreen(
                    api = app.api,
                    secureStore = app.secureStore,
                    onSignedIn = {
                        showingSettings = false
                        hasSession = true
                        scope.launch(Dispatchers.IO) { WeatherScheduler.reconcile(applicationContext) }
                    },
                )
            }
        }
    }

    override fun onResume() {
        super.onResume()
        lifecycleScope.launch(Dispatchers.IO) { WeatherScheduler.reconcile(applicationContext) }
        RefreshScheduler.enqueueOneOff(applicationContext)
    }
}
