package net.ikoro.healthvault

import android.app.Application
import kotlinx.coroutines.launch
import net.ikoro.healthvault.diagnostics.*
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.api.SessionCookieJar
import net.ikoro.healthvault.store.SecureStore

/**
 * Owns the process-wide singletons every entry point needs: the today screen,
 * the setup screen, and RefreshWorker (which runs in its own freshly started
 * process almost every time, so this construction must be cheap and cannot
 * assume anything survived from a previous run). Deliberately no DI
 * framework — three objects with one dependency edge each doesn't need one.
 */
class HealthVaultApp : Application() {

    private val diagnosticScope = kotlinx.coroutines.CoroutineScope(
        kotlinx.coroutines.SupervisorJob() + kotlinx.coroutines.Dispatchers.IO)

    // Serialize widget events and their terminal upload without blocking UI callbacks.
    private val widgetDiagnosticQueue = WidgetDiagnosticQueue(diagnosticScope)
    private val diagnosticEnvironment by lazy {
        val info = packageManager.getPackageInfo(packageName, 0)
        "${info.versionName ?: "unknown"}" to android.os.Build.VERSION.SDK_INT
    }

    fun widgetDiagnostics(flow: WidgetFlow, workId: String = "", attempt: Int = 1) =
        WidgetDiagnostics(flow, environment = { diagnosticEnvironment }, record = { event, generation ->
            widgetDiagnosticQueue.offer { secureStore.recordDiagnostic(event, generation) }
            Unit
        }, workId = workId, attempt = attempt)

    fun recordWidgetReplacement(flow: WidgetFlow, workId: String) {
        widgetDiagnosticQueue.offer {
            val history = secureStore.diagnosticJournal(flow.generation).events
            val previous = pendingWidgetReplacement(history, workId)
            if (previous != null) {
                // Record directly inside the ordered queue before the new enqueue event.
                WidgetDiagnostics(WidgetFlow(previous.requestId, flow.generation, previous.trigger),
                    { diagnosticEnvironment }, { event, generation -> secureStore.recordDiagnostic(event, generation); Unit },
                    previous.workId, previous.attempt).event("widget_work", "replacement_requested")
            }
        }
    }

    fun uploadWidgetDiagnostics(generation: Long) {
        widgetDiagnosticQueue.offer { api.sendDiagnostics(generation) }
    }

    val secureStore: SecureStore by lazy { SecureStore.create(this) }
    val cookieJar: SessionCookieJar by lazy { SessionCookieJar(secureStore) }
    val api: HealthVaultApi by lazy { HealthVaultApi(secureStore, cookieJar, diagnosticEnvironment = {
        val info = packageManager.getPackageInfo(packageName, 0)
        "${info.versionName ?: "unknown"}" to android.os.Build.VERSION.SDK_INT
    }, onSuccessfulSync = { generation ->
        diagnosticScope.launch { runCatching { api.sendDiagnostics(generation) } }
    }) }
}
