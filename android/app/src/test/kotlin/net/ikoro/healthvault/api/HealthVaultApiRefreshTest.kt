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
      "eating_occasions_today": 1, "usual_meals_per_day": 3,
      "last_logged_at": null, "display_language": "en",
      "target": {"available": true, "calories": 2000, "protein_grams": 150, "carbs_grams": 200, "fat_grams": 70}
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
        val prefs = FakeSharedPreferences()

        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/auth/refresh" -> MockResponse()
                    .setResponseCode(200)
                    .addHeader("Set-Cookie", "kin_access=rotated-access; Path=/api")
                    .addHeader("Set-Cookie", "kin_refresh=rotated-refresh; Path=/api/auth/refresh")
                "/api/summary/today" -> {
                    summaryCallCount++
                    if (summaryCallCount == 1) {
                        MockResponse().setResponseCode(401)
                    } else {
                        val persisted = secureStore.loadCookies()
                        committedBeforeRetry.set(
                            persisted.any { it.name == "kin_access" && it.value == "rotated-access" } &&
                                persisted.any {
                                    it.name == "kin_refresh" &&
                                        it.value == "rotated-refresh" &&
                                        it.path == "/api/auth/refresh"
                                },
                        )
                        MockResponse().setResponseCode(200).setBody(VALID_SUMMARY_JSON)
                    }
                }
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        secureStore = SecureStore(prefs).apply {
            serverUrl = server.url("/").toString().trimEnd('/')
        }
        prefs.resetWriteCounts()
        val api = HealthVaultApi(secureStore, SessionCookieJar(secureStore))

        val result = api.summaryToday()

        assertTrue(result is ApiResult.Success)
        assertTrue("rotated cookie must be persisted before the retried request is sent", committedBeforeRetry.get())
        assertTrue("the rotated cookie must use a synchronous commit", prefs.commitCount > 0)
        assertEquals("cookie persistence must not use apply", 0, prefs.applyCount)
    }

    @Test
    fun `fallback login cannot install old account cookies after the session changes`() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/summary/today", "/api/auth/refresh" -> MockResponse().setResponseCode(401)
                "/api/auth/login" -> MockResponse()
                    .setResponseCode(200)
                    .addHeader("Set-Cookie", "kin_access=late-alice-token; Path=/api")
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        val prefs = FakeSharedPreferences()
        val secureStore = SecureStore(prefs)
        val jar = SessionCookieJar(secureStore)
        val baseUrl = server.url("/").toString().trimEnd('/')
        secureStore.saveSession(baseUrl, "alice", "old-secret")
        val switched = AtomicBoolean(false)
        prefs.afterGetString = { key ->
            if (key == "password" && switched.compareAndSet(false, true)) {
                secureStore.clearSession()
                jar.clearInMemory()
                secureStore.saveSession(baseUrl, "bob", "new-secret")
                jar.withSessionGeneration(secureStore.currentSessionGeneration) {
                    jar.saveFromResponse(
                        server.url("/api/auth/login"),
                        listOf(
                            okhttp3.Cookie.Builder()
                                .name("kin_access")
                                .value("bob-token")
                                .hostOnlyDomain(server.url("/").host)
                                .path("/api")
                                .expiresAt(Long.MAX_VALUE)
                                .build(),
                        ),
                    )
                }
            }
        }
        val api = HealthVaultApi(secureStore, jar)

        api.summaryToday()

        assertTrue(switched.get())
        assertEquals(
            listOf("bob-token"),
            jar.loadForRequest(server.url("/api/summary/today")).map { it.value },
        )
        assertEquals(listOf("bob-token"), secureStore.loadCookies().map { it.value })
    }

    @Test
    fun `standalone login response is pinned to the session that started it`() {
        val requestArrived = CountDownLatch(1)
        val releaseResponse = CountDownLatch(1)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                requestArrived.countDown()
                assertTrue(releaseResponse.await(5, TimeUnit.SECONDS))
                return MockResponse()
                    .setResponseCode(200)
                    .addHeader("Set-Cookie", "kin_access=late-alice-token; Path=/api")
            }
        }
        server.start()

        val secureStore = SecureStore(FakeSharedPreferences())
        val jar = SessionCookieJar(secureStore)
        val baseUrl = server.url("/").toString().trimEnd('/')
        secureStore.saveSession(baseUrl, "alice", "old-secret")
        val api = HealthVaultApi(secureStore, jar)
        val executor = Executors.newSingleThreadExecutor()
        val login = executor.submit<ApiResult<Unit>> { api.login(baseUrl, "alice", "old-secret") }
        assertTrue(requestArrived.await(5, TimeUnit.SECONDS))

        secureStore.clearSession()
        jar.clearInMemory()
        secureStore.saveSession(baseUrl, "bob", "new-secret")
        jar.withSessionGeneration(secureStore.currentSessionGeneration) {
            jar.saveFromResponse(
                server.url("/api/auth/login"),
                listOf(
                    okhttp3.Cookie.Builder()
                        .name("kin_access")
                        .value("bob-token")
                        .hostOnlyDomain(server.url("/").host)
                        .path("/api")
                        .expiresAt(Long.MAX_VALUE)
                        .build(),
                ),
            )
        }
        releaseResponse.countDown()
        assertTrue(login.get(5, TimeUnit.SECONDS) is ApiResult.Success)
        executor.shutdown()

        assertEquals(
            listOf("bob-token"),
            jar.loadForRequest(server.url("/api/summary/today")).map { it.value },
        )
        assertEquals(listOf("bob-token"), secureStore.loadCookies().map { it.value })
    }

    /**
     * The re-login fallback's own failure must not be laundered into
     * "signed out". Here the session really is dead (refresh 401s, so the
     * summary's 401 stands) but the login the client falls back to is held off
     * by the rate limiter — nothing about that says the credentials are wrong,
     * and reporting it as Unauthenticated would make the today screen announce
     * a sign-out and the widget drop a session that is still fine.
     */
    @Test
    fun `a rate-limited re-login is reported as RateLimited, not as a sign-out`() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/summary/today" -> MockResponse().setResponseCode(401)
                "/api/auth/refresh" -> MockResponse().setResponseCode(401)
                "/api/auth/login" -> MockResponse()
                    .setResponseCode(429)
                    .addHeader("Retry-After", "31")
                    .setBody("""{"error":"too_many_attempts","retry_after_seconds":31}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        val secureStore = SecureStore(FakeSharedPreferences()).apply {
            serverUrl = server.url("/").toString().trimEnd('/')
            username = "alice"
            password = "secret"
        }
        val api = HealthVaultApi(secureStore, SessionCookieJar(secureStore))

        val result = api.summaryToday()

        assertTrue("expected RateLimited, got $result", result is ApiResult.RateLimited)
        assertEquals(31L, (result as ApiResult.RateLimited).retryAfter.inWholeSeconds)
    }

    /** The same path, but with the credentials genuinely rejected: this one *is* a sign-out. */
    @Test
    fun `a re-login rejected with 401 is reported as Unauthenticated`() {
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
                "/api/summary/today", "/api/auth/refresh", "/api/auth/login" ->
                    MockResponse().setResponseCode(401)
                else -> MockResponse().setResponseCode(404)
            }
        }
        server.start()

        val secureStore = SecureStore(FakeSharedPreferences()).apply {
            serverUrl = server.url("/").toString().trimEnd('/')
            username = "alice"
            password = "wrong"
        }
        val api = HealthVaultApi(secureStore, SessionCookieJar(secureStore))

        assertTrue(api.summaryToday() is ApiResult.Unauthenticated)
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
