package net.ikoro.healthvault.widget

import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.api.TodaySummaryTarget
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

private val SAMPLE_SUMMARY = TodaySummary(
    date = "2026-09-02",
    caloriesConsumed = 1200.0,
    proteinGramsConsumed = 80.0,
    carbsGramsConsumed = 120.0,
    fatGramsConsumed = 40.0,
    mealCount = 2,
    lastLoggedAt = "2026-09-02T12:00:00Z",
    displayLanguage = "en",
    target = TodaySummaryTarget(available = true, calories = 2000, proteinGrams = 150, carbsGrams = 200, fatGrams = 70),
    recommendation = null,
)

class WidgetStateTest {

    @Test
    fun `no session is SignedOut regardless of a cached snapshot`() {
        val state = widgetState(SAMPLE_SUMMARY, fetchedAtMillis = 0L, nowMillis = 0L, hasSession = false)
        assertEquals(WidgetState.SignedOut, state)
    }

    @Test
    fun `a session with no snapshot yet is Error`() {
        val state = widgetState(summary = null, fetchedAtMillis = null, nowMillis = 0L, hasSession = true)
        assertEquals(WidgetState.Error, state)
    }

    @Test
    fun `a fresh snapshot is Loaded`() {
        val now = 10 * 60 * 60 * 1000L // 10h on the clock
        val fetchedAt = now - 60_000 // fetched 1 minute ago
        val state = widgetState(SAMPLE_SUMMARY, fetchedAt, now, hasSession = true)
        assertTrue(state is WidgetState.Loaded)
        assertEquals(SAMPLE_SUMMARY, (state as WidgetState.Loaded).summary)
    }

    @Test
    fun `a snapshot exactly at the 6-hour boundary is not yet stale`() {
        val fetchedAt = 0L
        val now = WIDGET_STALE_AFTER_MILLIS
        val state = widgetState(SAMPLE_SUMMARY, fetchedAt, now, hasSession = true)
        assertTrue(state is WidgetState.Loaded)
    }

    @Test
    fun `a snapshot older than 6 hours is Stale`() {
        val fetchedAt = 0L
        val now = WIDGET_STALE_AFTER_MILLIS + 1
        val state = widgetState(SAMPLE_SUMMARY, fetchedAt, now, hasSession = true)
        assertTrue(state is WidgetState.Stale)
        assertEquals(SAMPLE_SUMMARY, (state as WidgetState.Stale).summary)
    }
}
