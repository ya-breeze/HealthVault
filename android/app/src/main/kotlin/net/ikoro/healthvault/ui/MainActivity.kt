package net.ikoro.healthvault.ui

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.appcompat.app.AppCompatActivity
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

/**
 * The app's single screen host: routes to [SetupScreen] when no session
 * exists and to [TodayScreen] when one does. There is no back-stack-worthy
 * navigation beyond that one fork — see the spec's "the app is thin and
 * read-only".
 */
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

            if (hasSession) {
                TodayScreen(
                    api = app.api,
                    secureStore = app.secureStore,
                    onSignedOut = {
                        // Durable session clearing runs on IO; locale reset and
                        // UI routing happen only after it succeeds. The widget
                        // must be redrawn only after the session is actually
                        // gone or it would re-render the signed-in state.
                        //
                        // Periodic refresh is tied to widget placement, not
                        // to the session (RefreshScheduler.ensurePeriodic is
                        // only ever cancelled by the last widget being
                        // removed) — a signed-out widget keeps polling and
                        // keeps rendering the sign-in prompt WidgetState.SignedOut
                        // maps to, so nothing here needs to touch scheduling.
                        scope.launch {
                            withContext(Dispatchers.IO) {
                                // Clear the durable credentials first. If the
                                // process dies before the in-memory jar is
                                // emptied, the next process still starts
                                // signed out instead of re-logging itself in.
                                app.secureStore.clearSession()
                                app.cookieJar.clearInMemory()
                            }
                            applyDisplayLanguage("")
                            WidgetUpdater.updateAll(applicationContext)
                            hasSession = false
                        }
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
