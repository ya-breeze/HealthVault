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
 * and updates every placed widget. Every outcome except
 * [ApiResult.Success] leaves the cached snapshot untouched — a failed
 * refresh is never treated as a sign-out (HealthVaultApi.summaryToday
 * already tried its own re-login fallback before returning
 * [ApiResult.Unauthenticated]), so the widget keeps rendering the last
 * snapshot, with a staleness marker once it's 6+ hours old
 * (widget/WidgetState.kt).
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
                is ApiResult.Unauthenticated,
                is ApiResult.NetworkFailure,
                is ApiResult.AccessChallenge,
                is ApiResult.ServerError,
                -> {
                    // Cached snapshot stays as-is; see the class doc above.
                }
            }
        }

        WidgetUpdater.updateAll(applicationContext)
        return Result.success()
    }
}
