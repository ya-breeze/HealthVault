package net.ikoro.healthvault.diagnostics

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.widget.updateBothWidgetProviders
import org.junit.Assert.*
import org.junit.Test
import java.net.UnknownHostException
import java.util.UUID

class WidgetDiagnosticsTest {
    private val events = mutableListOf<DiagnosticEvent>()
    private val generations = mutableListOf<Long>()
    private val flow = WidgetFlow(generation = 42, trigger = "food_entry")
    private val workId = UUID.randomUUID().toString()
    private fun trace(record: (DiagnosticEvent, Long) -> Unit = { event, gen -> events += event; generations += gen }) =
        WidgetDiagnostics(flow, { "1.0" to 36 }, record, workId, attempt = 3)

    @Test fun `flow metadata serializes and aggregate fetch records only safe outcomes`() {
        val trace = trace()
        trace.event("widget_work", "started")
        trace.fetch(ApiResult.NetworkFailure(UnknownHostException("private host and credentials")), 14)
        trace.event("widget_snapshot", "success", snapshotAgeMillis = 900, snapshotChanged = false)
        assertEquals(listOf("started", "dns", "success"), events.map { it.category })
        assertTrue(events.all { it.requestId == flow.id && it.workId == workId && it.trigger == "food_entry" && it.attempt == 3 })
        assertEquals(listOf(42L, 42L, 42L), generations)
        val encoded = Json.encodeToString(events.last())
        assertTrue(encoded.contains("\"snapshot_changed\":false"))
        assertTrue(encoded.contains("\"snapshot_age_millis\":900"))
        assertFalse(Json.encodeToString(events).contains("private host"))
        assertEquals(events.last(), Json.decodeFromString<DiagnosticEvent>(encoded))
    }

    @Test fun `malformed successful HTTP response is classified without response body`() {
        trace().fetch(ApiResult.ServerError(200, "private response"), 10)
        assertEquals("invalid_response", events.single().category)
        assertEquals(200, events.single().httpCode)
        assertFalse(events.single().toString().contains("private response"))
    }

    @Test fun `telemetry failure is best effort but cancellation propagates`() {
        trace { _, _ -> error("storage unavailable") }.event("widget_work", "started")
        try {
            trace { _, _ -> throw CancellationException("cancel") }.event("widget_work", "started")
            fail("cancellation must propagate")
        } catch (_: CancellationException) { }
    }

    @Test fun `redraw submits both providers after ordinary failure and reports actual outcome`() = runTest {
        val trace = trace()
        var flexSubmitted = false
        val failure = IllegalStateException("Glance failed")
        try {
            updateBothWidgetProviders({ throw failure }, { flexSubmitted = true }, trace)
            fail("redraw must preserve failure")
        } catch (caught: IllegalStateException) { assertSame(failure, caught) }
        assertTrue(flexSubmitted)
        assertEquals(listOf("home:started", "home:failed", "flex_window:started", "flex_window:success"),
            events.map { "${it.provider}:${it.category}" })
    }

    @Test fun `redraw cancellation reports locally and stops further submission`() = runTest {
        var flexSubmitted = false
        try {
            updateBothWidgetProviders({ throw CancellationException("cancel") }, { flexSubmitted = true }, trace())
            fail("redraw must preserve cancellation")
        } catch (_: CancellationException) { }
        assertFalse(flexSubmitted)
        assertEquals(listOf("started", "cancelled"), events.map { it.category })
    }

    @Test fun `widget stages retain categories attempts and optional metadata across storage recreation`() {
        val prefs = FakeSharedPreferences()
        val store = SecureStore(prefs)
        store.saveSession("https://example.com", "alice", "secret")
        val gen = store.currentSessionGeneration
        val diagnostic = WidgetDiagnostics(flow.copy(generation = gen), { "1.0" to 36 },
            { event, generation -> store.recordDiagnostic(event, generation); Unit }, workId, 7)
        diagnostic.event("widget_work", "started")
        diagnostic.event("widget_work", "success")
        diagnostic.event("widget_snapshot", "success", snapshotAgeMillis = 12, snapshotChanged = false)
        val stored = SecureStore(prefs).diagnosticJournal()
        assertEquals(listOf("started", "success", "success"), stored.events.map { it.category })
        assertTrue(stored.events.all { it.attempt == 7 })
        assertEquals(false, stored.events.last().snapshotChanged)
        assertNull(stored.lastSummarySuccess)
        store.saveSession("https://example.com", "bob", "other")
        diagnostic.event("widget_work", "cancelled")
        assertTrue(store.diagnosticJournal().events.isEmpty())
    }

    @Test fun `periodic intervals get new flows while retry retains execution flow`() {
        val first = periodicWidgetFlow(42, workId, false, emptyList())
        val event = DiagnosticEvent(UUID.randomUUID().toString(), java.time.Instant.now().toString(),
            "widget_work", "deferred", first.id, appVersion = "1.0", androidApi = 36, workId = workId)
        assertEquals(first.id, periodicWidgetFlow(42, workId, true, listOf(event)).id)
        assertEquals("retry", periodicWidgetFlow(42, workId, true, listOf(event)).trigger)
        assertNotEquals(first.id, periodicWidgetFlow(42, workId, false, listOf(event)).id)
    }

    @Test fun `snapshot age is nonnegative and bounded without overflow`() {
        assertNull(snapshotAge(12, null))
        assertEquals(0L, snapshotAge(12, 13))
        assertEquals(5L, snapshotAge(12, 7))
        assertEquals(Long.MAX_VALUE, snapshotAge(Long.MAX_VALUE, Long.MIN_VALUE))
    }
    @Test fun `queued replacement records intention only for a pending earlier work item`() {
        trace().event("widget_work", "enqueue_requested")
        assertEquals(events.first(), pendingWidgetReplacement(events, UUID.randomUUID().toString()))
        assertNull(pendingWidgetReplacement(events, workId))
        trace().event("widget_work", "started")
        assertEquals(events.first(), pendingWidgetReplacement(events, UUID.randomUUID().toString()))
        trace().event("widget_work", "success")
        assertNull(pendingWidgetReplacement(events, UUID.randomUUID().toString()))
    }

}
