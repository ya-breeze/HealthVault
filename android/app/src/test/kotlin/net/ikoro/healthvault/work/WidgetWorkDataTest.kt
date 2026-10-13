package net.ikoro.healthvault.work

import androidx.work.Data
import net.ikoro.healthvault.diagnostics.WidgetFlow
import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import org.junit.Assert.*
import org.junit.Test
import java.util.UUID

class WidgetWorkDataTest {
    @Test fun `browser flow survives real WorkManager Data roundtrip and process restart`() {
        val prefs = FakeSharedPreferences()
        val original = SecureStore(prefs)
        original.saveSession("https://example.com", "alice", "secret")
        val browser = WidgetFlow(generation = original.currentSessionGeneration, trigger = "food_entry")
        val queued = widgetWorkData(browser)
        val restarted = SecureStore(prefs)
        val worker = widgetWorkFlow(queued, emptySet(), restarted.currentSessionGeneration,
            UUID.randomUUID().toString(), false, emptyList())
        assertEquals(browser, worker)
        assertTrue(restarted.isCurrentSession(worker.generation))
        restarted.saveSession("https://example.com", "bob", "other")
        assertFalse(SecureStore(prefs).isCurrentSession(worker.generation))
        val retry = widgetWorkFlow(queued, emptySet(), restarted.currentSessionGeneration,
            UUID.randomUUID().toString(), true, emptyList())
        assertEquals(browser.id, retry.id)
        assertEquals(browser.generation, retry.generation)
        assertEquals("retry", retry.trigger)
    }

    @Test fun `untagged legacy work remains unspecified and tagged periodic work is distinguished`() {
        val work = UUID.randomUUID().toString()
        assertEquals("", widgetWorkFlow(Data.EMPTY, emptySet(), 0, work, false, emptyList()).trigger)
        assertEquals("periodic", widgetWorkFlow(Data.EMPTY, setOf(RefreshScheduler.PERIODIC_TAG), 0, work, false, emptyList()).trigger)
    }
}
