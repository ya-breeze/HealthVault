package net.ikoro.healthvault.widget

import android.appwidget.AppWidgetManager
import android.content.Context
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetManager
import androidx.glance.appwidget.GlanceAppWidgetReceiver
import androidx.glance.appwidget.updateAll
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.runBlocking
import net.ikoro.healthvault.work.RefreshScheduler

/**
 * Enables (this widget's first placement) and disables (its last removal)
 * the periodic background refresh — RefreshScheduler runs the 30-minute
 * WorkManager job only while at least one widget is placed.
 */
abstract class RefreshingSummaryWidgetReceiver : GlanceAppWidgetReceiver() {
    override fun onEnabled(context: Context) {
        super.onEnabled(context)
        RefreshScheduler.ensurePeriodic(context)
    }

    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        super.onUpdate(context, appWidgetManager, appWidgetIds)
        // onEnabled runs only for the first instance. onUpdate also runs when
        // each later instance is placed, so every placement gets fresh data.
        RefreshScheduler.enqueueOneOff(context)
    }

    override fun onDisabled(context: Context) {
        super.onDisabled(context)
        runBlocking {
            if (WidgetUpdater.placementCounts(context).shouldCancelPeriodic) {
                RefreshScheduler.cancelPeriodic(context)
            }
        }
    }
}

class SummaryWidgetReceiver : RefreshingSummaryWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = SummaryWidget()
}

class FlexWindowSummaryWidgetReceiver : RefreshingSummaryWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = FlexWindowSummaryWidget()
}

/** Forces every placed widget instance to recompute — used after sign-out and by RefreshWorker. */
object WidgetUpdater {
    suspend fun updateAll(context: Context) {
        updateBothWidgetProviders(
            updateHome = { SummaryWidget().updateAll(context) },
            updateFlexWindow = { FlexWindowSummaryWidget().updateAll(context) },
        )
    }

    /** Non-suspend convenience for call sites that aren't already in a coroutine (MainActivity's sign-out path uses the suspend version instead). */
    fun updateAllBlocking(context: Context) {
        runBlocking { updateAll(context) }
    }

    internal suspend fun placementCounts(context: Context): WidgetPlacementCounts {
        val manager = GlanceAppWidgetManager(context)
        return WidgetPlacementCounts(
            home = manager.getGlanceIds(SummaryWidget::class.java).size,
            flexWindow = manager.getGlanceIds(FlexWindowSummaryWidget::class.java).size,
        )
    }
}

internal data class WidgetPlacementCounts(val home: Int, val flexWindow: Int) {
    val total: Int = home + flexWindow
    val shouldCancelPeriodic: Boolean = total == 0
}

/** Attempt both providers even if the first update fails, then preserve the failures for callers. */
internal suspend fun updateBothWidgetProviders(
    updateHome: suspend () -> Unit,
    updateFlexWindow: suspend () -> Unit,
) {
    var firstFailure: Throwable? = null

    try {
        updateHome()
    } catch (failure: Throwable) {
        if (failure is CancellationException) throw failure
        firstFailure = failure
    }

    try {
        updateFlexWindow()
    } catch (failure: Throwable) {
        if (failure is CancellationException) throw failure
        if (firstFailure == null) firstFailure = failure else firstFailure.addSuppressed(failure)
    }

    firstFailure?.let { throw it }
}
