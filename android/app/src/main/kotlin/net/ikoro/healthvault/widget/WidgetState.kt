package net.ikoro.healthvault.widget

import net.ikoro.healthvault.api.TodaySummary

/** 6 hours — a snapshot older than this is marked stale rather than shown as current. */
const val WIDGET_STALE_AFTER_MILLIS = 6L * 60 * 60 * 1000

sealed class WidgetState {
    data class Loaded(val summary: TodaySummary, val fetchedAtMillis: Long) : WidgetState()
    data class Stale(val summary: TodaySummary, val fetchedAtMillis: Long) : WidgetState()
    data object SignedOut : WidgetState()
    data object Error : WidgetState()
}

/**
 * Pure mapping from the persisted snapshot and session presence to what the
 * widget renders. Kept free of Android types (Context, Glance, WorkManager)
 * on purpose, so it's a plain-JVM unit test target (Task 7) — the widget
 * itself (SummaryWidget.kt) is the only caller and does nothing but read
 * SecureStore and hand the result here.
 */
fun widgetState(
    summary: TodaySummary?,
    fetchedAtMillis: Long?,
    nowMillis: Long,
    hasSession: Boolean,
): WidgetState {
    if (!hasSession) return WidgetState.SignedOut
    if (summary == null || fetchedAtMillis == null) return WidgetState.Error
    return if (nowMillis - fetchedAtMillis > WIDGET_STALE_AFTER_MILLIS) {
        WidgetState.Stale(summary, fetchedAtMillis)
    } else {
        WidgetState.Loaded(summary, fetchedAtMillis)
    }
}
