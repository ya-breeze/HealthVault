package net.ikoro.healthvault.store

import net.ikoro.healthvault.diagnostics.DiagnosticEvent
import org.junit.Assert.*
import org.junit.Test
import java.time.Instant
import java.util.UUID

class DiagnosticStoreTest {
    private fun event(category: String = "timeout", at: String = Instant.now().toString()) = DiagnosticEvent(
        UUID.randomUUID().toString(), at, "summary", category, UUID.randomUUID().toString(),
        appVersion = "1.0", androidApi = 36)

    @Test fun `journal survives recreation acknowledgements preserve concurrent entries and recovery is recorded`() {
        val prefs = FakeSharedPreferences()
        val store = SecureStore(prefs)
        store.saveSession("https://example.com", "alice", "secret")
        val gen = store.currentSessionGeneration
        val failed = event()
        store.recordDiagnostic(failed, gen)
        val other = event("success")
        store.recordDiagnostic(other, gen)
        store.acknowledgeDiagnostics(setOf(failed.id), gen)
        val loaded = SecureStore(prefs).diagnosticJournal()
        assertEquals(listOf(other.id), loaded.pendingIds)
        assertEquals("recovered", loaded.events.last().category)
        assertEquals(2, loaded.events.last().attempt)
        assertEquals(other.occurredAt, loaded.lastSummarySuccess)
        assertFalse(prefs.getString("diagnostics", "")!!.contains("secret"))
    }

    @Test fun `bounds expiry and account changes prevent old diagnostic uploads`() {
        val store = SecureStore(FakeSharedPreferences())
        store.saveSession("https://example.com", "alice", "password")
        val gen = store.currentSessionGeneration
        store.recordDiagnostic(event(at = Instant.now().minusSeconds(15 * 86400L).toString()), gen)
        assertTrue(store.diagnosticJournal().events.isEmpty())
        repeat(205) { store.recordDiagnostic(event(), gen) }
        assertEquals(200, store.diagnosticJournal().events.size)
        assertEquals(200, store.diagnosticJournal().pendingIds.size)
        store.saveSession("https://other.example", "bob", "other")
        assertTrue(store.diagnosticJournal().events.isEmpty())
        assertFalse(store.recordDiagnostic(event(), gen))
        assertFalse(store.acknowledgeDiagnostics(emptySet(), gen))
        store.recordDiagnostic(event(), store.currentSessionGeneration)
        store.clearSession()
        assertTrue(store.diagnosticJournal().events.isEmpty())
    }
}
