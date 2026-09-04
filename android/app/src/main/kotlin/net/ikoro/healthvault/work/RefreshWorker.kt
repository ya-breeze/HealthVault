package net.ikoro.healthvault.work

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.store.SummarySnapshot
import net.ikoro.healthvault.widget.WidgetUpdater

/**
 * Performs exactly one GET /api/summary/today per run, persists the result,
 * and updates every placed widget.
 *
 * A *recoverable* failure — unreachable server, 429, Access challenge, 5xx —
 * is never treated as a sign-out: the cached snapshot is left untouched and
 * the widget keeps rendering it, with a staleness marker once it's 6+ hours
 * old (widget/WidgetState.kt). [ApiResult.Unauthenticated] is the one outcome
 * that is not recoverable, because HealthVaultApi.summaryToday only returns it
 * after its refresh *and* its re-login from stored credentials were both
 * rejected — see the handler below.
 */
class RefreshWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result {
        val app = applicationContext as HealthVaultApp

        if (app.secureStore.hasSession()) {
            when (val result = app.api.summaryToday()) {
                is ApiResult.Success -> {
                    app.secureStore.saveSnapshot(SummarySnapshot(result.value, System.currentTimeMillis()))
                }
                is ApiResult.RateLimited -> {
                    // Not Result.retry(): that would follow this request's
                    // fixed backoff policy, not the server's actual
                    // Retry-After. Schedule the next attempt for exactly
                    // that delay instead, and let this run end normally.
                    RefreshScheduler.enqueueOneOff(applicationContext, result.retryAfter)
                }
                is ApiResult.Unauthenticated -> {
                    // The stored credentials themselves were rejected, so no
                    // unattended retry can recover this session — every later
                    // run would 401 the same way and the widget would sit on a
                    // snapshot that never updates again. Clear the session so
                    // widgetState() reports SignedOut and the widget shows the
                    // sign-in prompt, exactly as MainActivity's sign-out does.
                    app.cookieJar.clear()
                    app.secureStore.clearSession()
                }
                is ApiResult.NetworkFailure,
                is ApiResult.AccessChallenge,
                is ApiResult.ServerError,
                -> {
                    // Recoverable: cached snapshot and session stay as-is.
                }
            }
        }

        WidgetUpdater.updateAll(applicationContext)
        return Result.success()
    }
}
