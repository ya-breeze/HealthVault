package net.ikoro.healthvault.widget

import android.content.Context
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetManager
import androidx.glance.appwidget.GlanceAppWidgetReceiver
import kotlinx.coroutines.runBlocking
import net.ikoro.healthvault.work.RefreshScheduler

/**
 * Enables (this widget's first placement) and disables (its last removal)
 * the periodic background refresh — RefreshScheduler runs the 30-minute
 * WorkManager job only while at least one widget is placed.
 */
class SummaryWidgetReceiver : GlanceAppWidgetReceiver() {

    override val glanceAppWidget: GlanceAppWidget = SummaryWidget()

    override fun onEnabled(context: Context) {
        super.onEnabled(context)
        RefreshScheduler.ensurePeriodic(context)
        RefreshScheduler.enqueueOneOff(context)
    }

    override fun onDisabled(context: Context) {
        super.onDisabled(context)
        RefreshScheduler.cancelPeriodic(context)
    }
}

/** Forces every placed widget instance to recompute — used after sign-out and by RefreshWorker. */
object WidgetUpdater {
    suspend fun updateAll(context: Context) {
        SummaryWidget().updateAll(context)
    }

    /** Non-suspend convenience for call sites that aren't already in a coroutine (MainActivity's sign-out path uses the suspend version instead). */
    fun updateAllBlocking(context: Context) {
        runBlocking { updateAll(context) }
    }

    suspend fun placedWidgetCount(context: Context): Int =
        GlanceAppWidgetManager(context).getGlanceIds(SummaryWidget::class.java).size
}
