package net.ikoro.healthvault.work

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import java.util.concurrent.TimeUnit
import kotlin.time.Duration
import kotlin.time.Duration.Companion.ZERO
import kotlin.time.toJavaDuration

private const val PERIODIC_WORK_NAME = "healthvault-refresh-periodic"
private const val ONE_OFF_WORK_NAME = "healthvault-refresh-one-off"

/**
 * Owns every trigger for RefreshWorker. The periodic job runs only while at
 * least one widget is placed — [RefreshingSummaryWidgetReceiver] calls [ensurePeriodic]
 * on the first placement (onEnabled) and [cancelPeriodic] on the last removal
 * (onDisabled). [enqueueOneOff] backs every other trigger: widget placement,
 * a manual refresh, the app resuming, and a 429's Retry-After-scheduled
 * catch-up (see RefreshWorker) — all go through the same unique work name, so
 * a later trigger (e.g. the owner opening the app right after a 429) simply
 * replaces an earlier, still-delayed one rather than stacking attempts.
 * Work intentionally has no CONNECTED constraint: even while offline the
 * periodic worker must wake up to redraw a snapshot when it crosses the
 * six-hour stale boundary. RefreshWorker records the network failure and
 * returns Result.retry(), so the actual request still backs off.
 */
object RefreshScheduler {

    fun ensurePeriodic(context: Context) {
        val request = PeriodicWorkRequestBuilder<RefreshWorker>(30, TimeUnit.MINUTES)
            .build()
        WorkManager.getInstance(context)
            .enqueueUniquePeriodicWork(PERIODIC_WORK_NAME, ExistingPeriodicWorkPolicy.KEEP, request)
    }

    fun cancelPeriodic(context: Context) {
        WorkManager.getInstance(context).cancelUniqueWork(PERIODIC_WORK_NAME)
    }

    fun enqueueOneOff(context: Context, initialDelay: Duration = ZERO) {
        val request = OneTimeWorkRequestBuilder<RefreshWorker>()
            .setInitialDelay(initialDelay.toJavaDuration())
            // Backs off retries WorkManager itself schedules on Result.retry()
            // (e.g. a transient network error inside doWork before this
            // request's own attempt even reaches the API). Distinct from the
            // 429 path, which never returns Result.retry() and instead
            // enqueues its own replacement request with the exact
            // Retry-After delay.
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 1, TimeUnit.MINUTES)
            .build()
        WorkManager.getInstance(context)
            .enqueueUniqueWork(ONE_OFF_WORK_NAME, ExistingWorkPolicy.REPLACE, request)
    }
}
