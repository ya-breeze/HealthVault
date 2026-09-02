package net.ikoro.healthvault.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.widget.WidgetUpdater
import net.ikoro.healthvault.work.RefreshScheduler

/**
 * The app's single screen host: routes to [SetupScreen] when no session
 * exists and to [TodayScreen] when one does. There is no back-stack-worthy
 * navigation beyond that one fork — see the spec's "the app is thin and
 * read-only".
 */
class MainActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = application as HealthVaultApp

        setContent {
            var hasSession by remember { mutableStateOf(app.secureStore.hasSession()) }
            val scope = rememberCoroutineScope()

            if (hasSession) {
                TodayScreen(
                    api = app.api,
                    secureStore = app.secureStore,
                    onSignedOut = {
                        app.cookieJar.clear()
                        app.secureStore.clearSession()
                        scope.launch(Dispatchers.IO) {
                            WidgetUpdater.updateAll(applicationContext)
                        }
                        RefreshScheduler.cancelIfNoWidgets(applicationContext)
                        hasSession = false
                    },
                )
            } else {
                SetupScreen(
                    api = app.api,
                    secureStore = app.secureStore,
                    onSignedIn = { hasSession = true },
                )
            }
        }
    }

    override fun onResume() {
        super.onResume()
        RefreshScheduler.enqueueOneOff(applicationContext)
    }
}
