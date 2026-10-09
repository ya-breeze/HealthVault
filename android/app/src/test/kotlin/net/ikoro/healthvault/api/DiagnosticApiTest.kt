package net.ikoro.healthvault.api

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.time.Instant
import java.util.UUID
import net.ikoro.healthvault.diagnostics.DiagnosticEvent
import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.*
import org.junit.Test

class DiagnosticApiTest {
    @Test fun `auth challenge records once and call timeout has safe category`() {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(200).setHeader("Content-Type", "text/html").setBody("<html>challenge</html>"))
            val store = SecureStore(FakeSharedPreferences())
            store.saveSession(server.url("/").toString(), "alice", "password")
            val api = HealthVaultApi(store, SessionCookieJar(store))
            assertTrue(api.login(server.url("/").toString(), "alice", "password") is ApiResult.AccessChallenge)
            assertEquals("access_challenge", store.diagnosticJournal().events.single().category)
            assertEquals("timeout", net.ikoro.healthvault.diagnostics.networkCategory(java.io.InterruptedIOException()))
        }
    }

    @Test fun `invalid summary gets correlated safe category without persisting response content`() {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(200).setHeader("Content-Type", "application/json").setBody("secret-invalid-json"))
            val store = SecureStore(FakeSharedPreferences())
            store.saveSession(server.url("/").toString(), "alice", "password")
            val api = HealthVaultApi(store, SessionCookieJar(store))
            assertTrue(api.summaryToday() is ApiResult.ServerError)
            val request = server.takeRequest()
            val event = store.diagnosticJournal().events.single()
            assertEquals(request.getHeader("X-Request-ID"), event.requestId)
            assertEquals("invalid_response", event.category)
            assertNull(store.diagnosticJournal().lastSummarySuccess)
            assertFalse(event.toString().contains("secret"))
        }
    }

    @Test fun `failed or incorrect receipt keeps reports pending and successful retry acknowledges exact ids`() {
        MockWebServer().use { server ->
            val store = SecureStore(FakeSharedPreferences())
            store.saveSession(server.url("/").toString(), "alice", "password")
            val event = DiagnosticEvent(UUID.randomUUID().toString(), Instant.now().toString(), "summary", "network",
                UUID.randomUUID().toString(), appVersion = "1.0", androidApi = 36)
            store.recordDiagnostic(event, store.currentSessionGeneration)
            val api = HealthVaultApi(store, SessionCookieJar(store))
            server.enqueue(MockResponse().setResponseCode(503))
            assertTrue(api.sendDiagnostics() is ApiResult.ServerError)
            val payload = Json.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
            assertEquals("1", payload.getValue("events").jsonArray.single().jsonObject.getValue("attempt").jsonPrimitive.content)
            server.enqueue(MockResponse().setResponseCode(202).setBody("""{"accepted_ids":["wrong"]}"""))
            assertTrue(api.sendDiagnostics() is ApiResult.ServerError)
            assertEquals(listOf(event.id), store.diagnosticJournal().pendingIds)
            assertEquals(1, store.diagnosticJournal().events.size) // upload failures never recurse
            server.enqueue(MockResponse().setResponseCode(202).setBody("""{"accepted_ids":["${event.id}"]}"""))
            assertTrue(api.sendDiagnostics() is ApiResult.Success)
            assertTrue(store.diagnosticJournal().pendingIds.isEmpty())
            assertEquals(1, store.diagnosticJournal().events.size)
            assertFalse(store.refreshFailed)
        }
    }
}
