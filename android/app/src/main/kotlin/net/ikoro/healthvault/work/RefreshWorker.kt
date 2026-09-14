package net.ikoro.healthvault.work

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.store.SummarySnapshot
import net.ikoro.healthvault.ui.applyDisplayLanguage
import net.ikoro.healthvault.widget.WidgetUpdater
import kotlin.time.Duration.Companion.milliseconds

/**
 * Performs exactly one GET /api/summary/today per run, persists the result,
 * and updates every placed widget.
 *
 * A *recoverable* failure — unreachable server, 429, Access challenge, 5xx —
 * is never treated as a sign-out: the cached snapshot is left untouched and
 * the widget keeps rendering it with a staleness marker immediately; age also
 * marks a snapshot stale after 6 hours (widget/WidgetState.kt).
 * [ApiResult.Unauthenticated] is the one outcome
 * that is not recoverable, because HealthVaultApi.summaryToday only returns it
 * after its refresh *and* its re-login from stored credentials were both
 * rejected — see the handler below.
 */
class RefreshWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result {
        val app = applicationContext as HealthVaultApp
        var workResult = Result.success()

        if (app.secureStore.hasSession()) {
            val sessionGeneration = app.secureStore.currentSessionGeneration
            val now = System.currentTimeMillis()
            val nextRefreshAt = app.secureStore.nextRefreshAtMillis
            if (nextRefreshAt > now) {
                // Every trigger, including periodic work and a manual tap,
                // respects the same persisted server holdoff. Re-enqueueing
                // also restores the delayed one-off if another trigger used
                // REPLACE to wake this worker before Retry-After elapsed.
                RefreshScheduler.enqueueOneOff(applicationContext, (nextRefreshAt - now).milliseconds)
                WidgetUpdater.updateAll(applicationContext)
                return Result.success()
            }

            when (val result = app.api.summaryToday()) {
                is ApiResult.Success -> {
                    app.secureStore.saveSnapshot(
                        SummarySnapshot(result.value, System.currentTimeMillis()),
                        sessionGeneration,
                    )
                    app.secureStore.saveRefreshState(
                        failed = false,
                        nextAttemptAtMillis = 0L,
                        expectedSessionGeneration = sessionGeneration,
                    )
                }
                is ApiResult.RateLimited -> {
                    // Not Result.retry(): that would follow this request's
                    // fixed backoff policy, not the server's actual
                    // Retry-After. Schedule the next attempt for exactly
                    // that delay instead, and let this run end normally.
                    val receivedAt = System.currentTimeMillis()
                    val delayMillis = result.retryAfter.inWholeMilliseconds.coerceAtLeast(0L)
                    val nextAttemptAt =
                        if (delayMillis > Long.MAX_VALUE - receivedAt) Long.MAX_VALUE else receivedAt + delayMillis
                    if (app.secureStore.saveRefreshState(
                            failed = true,
                            nextAttemptAtMillis = nextAttemptAt,
                            expectedSessionGeneration = sessionGeneration,
                        )
                    ) {
                        RefreshScheduler.enqueueOneOff(applicationContext, delayMillis.milliseconds)
                    }
                }
                is ApiResult.Unauthenticated -> {
                    // The stored credentials themselves were rejected, so no
                    // unattended retry can recover this session — every later
                    // run would 401 the same way and the widget would sit on a
                    // snapshot that never updates again. Clear the session so
                    // widgetState() reports SignedOut and the widget shows the
                    // sign-in prompt, exactly as MainActivity's sign-out does.
                    if (app.secureStore.clearSession(sessionGeneration)) {
                        app.cookieJar.clearInMemory()
                        withContext(Dispatchers.Main) {
                            applyDisplayLanguage("")
                        }
                    }
                }
                is ApiResult.NetworkFailure -> {
                    // Ask WorkManager to use the exponential backoff configured
                    // by RefreshScheduler instead of silently waiting for the
                    // next 30-minute periodic run.
                    app.secureStore.saveRefreshState(
                        failed = true,
                        nextAttemptAtMillis = 0L,
                        expectedSessionGeneration = sessionGeneration,
                    )
                    workResult = Result.retry()
                }
                is ApiResult.AccessChallenge,
                is ApiResult.ServerError,
                -> {
                    // Recoverable: cached snapshot and session stay as-is.
                    app.secureStore.saveRefreshState(
                        failed = true,
                        nextAttemptAtMillis = 0L,
                        expectedSessionGeneration = sessionGeneration,
                    )
                }
            }
        }

        WidgetUpdater.updateAll(applicationContext)
        return workResult
    }
}
