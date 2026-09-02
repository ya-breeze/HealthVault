package net.ikoro.healthvault.api

import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

private val VALID_SUMMARY_JSON = """
    {
      "date": "2026-09-02", "calories_consumed": 500, "protein_grams_consumed": 30,
      "carbs_grams_consumed": 50, "fat_grams_consumed": 20, "meal_count": 1,
      "last_logged_at": null, "display_language": "en",
      "target": {"available": true, "calories": 2000, "protein_grams": 150, "carbs_grams": 200, "fat_grams": 70},
      "recommendation": null
    }
""".trimIndent()

class HealthVaultApiRefreshTest {

    private val server = MockWebServer()

    @After
    fun tearDown() {
        server.shutdown()
    }

    /**
     * Exercises both single-flight refresh rules at once, the same way they
     * actually arise: two requests race a corrupted access token. Proves
     * exactly one /api/auth/refresh call happens (the concurrent-401 rule)
     * and that both original requests still succeed (the
     * dispatched-before-the-completed-refresh rule — the second one arrives
     * after the first has already refreshed, and must retry rather than
     * refresh again).
     */
    @Test
    fun `concurrent 401s produce exactly one refresh call, and both requests still succeed`() {
        val refreshCount = AtomicInteger(0)
        val unlocked = AtomicBoolean(false)
        val bothArrived = CountDownLatch(2)

        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/auth/refresh" -> {
                    refreshCount.incrementAndGet()
                    unlocked.set(true)
                    MockResponse().setResponseCode(200)
                }
                "/api/summary/today" -> {
                    bothArrived.countDown()
                    bothArrived.await(2, TimeUnit.SECONDS)
                    if (unlocked.get()) {
                        MockResponse().setResponseCode(200).setBody(VALID_SUMMARY_JSON)
                    } else {
                        MockResponse().setResponseCode(401)
                    }
                }
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        val secureStore = SecureStore(FakeSharedPreferences()).apply {
            serverUrl = server.url("/").toString().trimEnd('/')
        }
        val api = HealthVaultApi(secureStore, SessionCookieJar(secureStore))

        val executor = Executors.newFixedThreadPool(2)
        val futures = (1..2).map { executor.submit<ApiResult<TodaySummary>> { api.summaryToday() } }
        val results = futures.map { it.get(5, TimeUnit.SECONDS) }
        executor.shutdown()

        assertTrue(results.all { it is ApiResult.Success })
        assertEquals(1, refreshCount.get())
    }

    @Test
    fun `a rotated refresh token is committed to the store before the retried request is issued`() {
        var summaryCallCount = 0
        val committedBeforeRetry = AtomicBoolean(false)
        lateinit var secureStore: SecureStore

        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/auth/refresh" -> MockResponse()
                    .setResponseCode(200)
                    .addHeader("Set-Cookie", "kin_access=rotated-access; Path=/api")
                "/api/summary/today" -> {
                    summaryCallCount++
                    if (summaryCallCount == 1) {
                        MockResponse().setResponseCode(401)
                    } else {
                        committedBeforeRetry.set(secureStore.loadCookies().any { it.value == "rotated-access" })
                        MockResponse().setResponseCode(200).setBody(VALID_SUMMARY_JSON)
                    }
                }
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        secureStore = SecureStore(FakeSharedPreferences()).apply {
            serverUrl = server.url("/").toString().trimEnd('/')
        }
        val api = HealthVaultApi(secureStore, SessionCookieJar(secureStore))

        val result = api.summaryToday()

        assertTrue(result is ApiResult.Success)
        assertTrue("rotated cookie must be persisted before the retried request is sent", committedBeforeRetry.get())
    }

    @Test
    fun `429 is reported as RateLimited, never as a sign-out`() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/summary/today" -> MockResponse()
                    .setResponseCode(429)
                    .addHeader("Retry-After", "17")
                    .setBody("""{"error":"too_many_attempts","retry_after_seconds":17}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        val secureStore = SecureStore(FakeSharedPreferences()).apply {
            serverUrl = server.url("/").toString().trimEnd('/')
        }
        val api = HealthVaultApi(secureStore, SessionCookieJar(secureStore))

        val result = api.summaryToday()

        assertTrue(result is ApiResult.RateLimited)
        assertEquals(17L, (result as ApiResult.RateLimited).retryAfter.inWholeSeconds)
    }
}
