package net.ikoro.healthvault.api

import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.weather.WeatherObservation
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.*
import org.junit.Test

class WeatherApiTest {
    @Test fun `weather retry keeps content and session recovery reuses auth path`() {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setResponseCode(401)) // ingestion
            server.enqueue(MockResponse().setResponseCode(401)) // refresh
            server.enqueue(MockResponse().setResponseCode(200)) // credential login
            server.enqueue(MockResponse().setResponseCode(202).setHeader("Content-Type", "application/json")
                .setBody("""{"id":"a","status":"accepted"}"""))
            val store = SecureStore(FakeSharedPreferences())
            store.saveSession(server.url("/").toString(), "alice", "password")
            val gen = store.currentSessionGeneration
            store.setWeatherEnabled(true, gen)
            val observation = WeatherObservation("a", "2026-10-06T10:00:00Z", 50.1, 14.4, 100.0)
            val result = HealthVaultApi(store, SessionCookieJar(store)).uploadWeather(observation, store.weatherConsent!!, gen) { true }
            assertTrue(result is ApiResult.Success)
            val first = server.takeRequest()
            assertEquals("/api/weather/locations", first.path)
            assertEquals("/api/auth/refresh", server.takeRequest().path)
            assertEquals("/api/auth/login", server.takeRequest().path)
            assertEquals(first.body.readUtf8(), server.takeRequest().body.readUtf8())
        }
    }

    @Test fun `disabled consent sends no HTTP request`() {
        MockWebServer().use { server ->
            val store = SecureStore(FakeSharedPreferences())
            store.saveSession(server.url("/").toString(), "alice", "password")
            val gen = store.currentSessionGeneration
            store.setWeatherEnabled(true, gen)
            val token = store.weatherConsent!!
            store.setWeatherEnabled(false, gen)
            val result = HealthVaultApi(store, SessionCookieJar(store)).uploadWeather(
                WeatherObservation("a", "2026-10-06T10:00:00Z", 50.1, 14.4, 100.0), token, gen) { true }
            assertTrue(result is ApiResult.NetworkFailure)
            assertEquals(0, server.requestCount)
        }
    }
}
