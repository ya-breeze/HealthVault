package net.ikoro.healthvault.api

import java.time.Instant
import java.util.concurrent.TimeUnit
import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.Assert.*
import org.junit.Test

class WeatherHistoryApiTest {
    private val now = Instant.parse("2026-10-06T11:45:00Z")
    private fun response(body: String = HISTORY_JSON) = MockResponse().setHeader("Content-Type", "application/json").setBody(body)
    private fun store(server: MockWebServer) = SecureStore(FakeSharedPreferences()).apply {
        saveSession(server.url("/").toString(), "alice", "password")
    }

    @Test fun `history reads with consent off using complete seven day query`() {
        MockWebServer().use { server ->
            server.enqueue(response())
            val store = store(server)
            assertNull(store.weatherConsent)
            val result = HealthVaultApi(store, SessionCookieJar(store)).weatherHistory(now)
            assertTrue(result is ApiResult.Success)
            val request = server.takeRequest(5, TimeUnit.SECONDS)!!
            assertEquals("GET", request.method)
            assertEquals("/api/weather/history", request.requestUrl!!.encodedPath)
            assertEquals("2026-09-29T11:00:00Z", request.requestUrl!!.queryParameter("from"))
            assertEquals("2026-10-06T11:00:00Z", request.requestUrl!!.queryParameter("to"))
        }
    }

    @Test fun `history rotates expired cookie then falls back to credentials when refresh fails`() {
        for (refreshWorks in listOf(true, false)) MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(401))
            server.enqueue(MockResponse().setResponseCode(if (refreshWorks) 200 else 401))
            if (!refreshWorks) server.enqueue(MockResponse().setResponseCode(200))
            server.enqueue(response())
            val store = store(server)
            assertTrue(HealthVaultApi(store, SessionCookieJar(store)).weatherHistory(now) is ApiResult.Success)
            assertEquals("/api/weather/history", server.takeRequest().requestUrl!!.encodedPath)
            assertEquals("/api/auth/refresh", server.takeRequest().path)
            if (!refreshWorks) assertEquals("/api/auth/login", server.takeRequest().path)
            assertEquals("/api/weather/history", server.takeRequest().requestUrl!!.encodedPath)
        }
    }

    @Test fun `history errors preserve credentials and reject unit mismatch`() {
        MockWebServer().use { server ->
            val store = store(server)
            val api = HealthVaultApi(store, SessionCookieJar(store))
            server.enqueue(MockResponse().setResponseCode(503))
            assertTrue(api.weatherHistory(now) is ApiResult.ServerError)
            server.enqueue(MockResponse().setResponseCode(429).setHeader("Retry-After", "120"))
            assertTrue(api.weatherHistory(now) is ApiResult.RateLimited)
            server.enqueue(response(HISTORY_JSON.replace("\"surface_pressure_hpa\":\"hPa\"", "\"surface_pressure_hpa\":\"Pa\"")))
            assertTrue(api.weatherHistory(now) is ApiResult.ServerError)
            assertTrue(store.hasSession())
        }
    }

    @Test fun `changed session cannot read or relogin with new account credentials`() {
        MockWebServer().use { server ->
            val store = store(server)
            val api = HealthVaultApi(store, SessionCookieJar(store))
            val generation = store.currentSessionGeneration
            server.dispatcher = object : Dispatcher() {
                override fun dispatch(request: RecordedRequest): MockResponse {
                    if (request.requestUrl!!.encodedPath == "/api/weather/history") {
                        store.saveSession(server.url("/").toString(), "bob", "new password")
                    }
                    return MockResponse().setResponseCode(401)
                }
            }
            val result = api.weatherHistory(now, generation)
            assertTrue(result !is ApiResult.Success)
            assertEquals("bob", store.username)
            assertEquals("/api/weather/history", server.takeRequest().requestUrl!!.encodedPath)
            assertEquals("/api/auth/refresh", server.takeRequest().path)
            assertEquals(2, server.requestCount) // No credential login after replacement.
            assertTrue(api.weatherHistory(now, generation) is ApiResult.NetworkFailure)
            assertEquals(2, server.requestCount)
        }
    }
}
