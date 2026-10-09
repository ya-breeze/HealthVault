package net.ikoro.healthvault.api

import java.io.IOException
import java.time.Instant
import java.util.UUID
import java.util.concurrent.TimeUnit
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock
import net.ikoro.healthvault.diagnostics.DiagnosticEvent
import net.ikoro.healthvault.diagnostics.networkCategory
import okhttp3.Interceptor
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.weather.WeatherObservation
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response

private val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()

/**
 * POST /api/auth/refresh carries no body — the refresh token travels as a
 * cookie. Built from the public `toRequestBody` extension rather than
 * `okhttp3.internal.EMPTY_REQUEST`: that symbol lives in OkHttp's `internal`
 * package, is not part of its published API, and stops being reachable from
 * outside the library in OkHttp 5.
 */
private val EMPTY_BODY = ByteArray(0).toRequestBody()

@Serializable
private data class WeatherAccepted(val id: String, val status: String)

@Serializable
private data class DiagnosticBatch(val events: List<DiagnosticEvent>)
@Serializable
private data class DiagnosticReceipt(@kotlinx.serialization.SerialName("accepted_ids") val acceptedIds: List<String>)
@Serializable
private data class FiberSetting(val grams: Int?)

private data class ObservedRequest(val id: String, val operation: String, val generation: Long, val started: Long, var recorded: Boolean = false)

@Serializable
private data class LoginRequest(val username: String, val password: String)

/**
 * The whole HTTP surface this app calls: login, refresh, and the one summary
 * endpoint the widget and the today screen both read. `client` carries
 * [SessionCookieJar] and [RefreshInterceptor]; `plainClient` shares the same
 * cookie jar but skips the interceptor, since login/refresh are themselves
 * the calls RefreshInterceptor would otherwise try to recover with — see
 * isAuthExemptPath, which intercept() also checks as a second guard.
 */
class HealthVaultApi(
    private val secureStore: SecureStore,
    private val cookieJar: SessionCookieJar,
    private val json: Json = Json { ignoreUnknownKeys = true },
    private val diagnosticEnvironment: () -> Pair<String, Int> = { "unknown" to 26 },
    private val onSuccessfulSync: (Long) -> Unit = {},
) {
    private val uploadLock = ReentrantLock()
    private val observer = Interceptor { chain ->
        val operation = when (chain.request().url.encodedPath) {
            "/api/summary/today" -> "summary"
            "/api/weather/locations" -> "weather"
            "/api/auth/login" -> "auth_login"
            "/api/auth/refresh" -> "auth_refresh"
            else -> null
        }
        if (operation == null) return@Interceptor chain.proceed(chain.request())
        val observed = ObservedRequest(UUID.randomUUID().toString(), operation,
            cookieJar.pinnedSessionGeneration, System.nanoTime())
        val request = chain.request().newBuilder().header("X-Request-ID", observed.id)
            .tag(ObservedRequest::class.java, observed).build()
        try {
            val response = chain.proceed(request)
            if (!response.isSuccessful) {
                val category = when {
                    response.code == 401 -> "unauthenticated"
                    response.code == 429 -> "rate_limited"
                    response.request.url.host.endsWith("cloudflareaccess.com") ||
                        (response.code < 500 && response.header("Content-Type")?.contains("text/html") == true) -> "access_challenge"
                    response.isSuccessful -> "success"
                    else -> "server"
                }
                record(observed, category, response.code)
            }
            response
        } catch (e: IOException) {
            record(observed, networkCategory(e))
            throw e
        }
    }

    private fun record(observed: ObservedRequest, category: String, code: Int = 0) {
        // Diagnostics must never turn a successful primary request into a failure.
        observed.recorded = true
        runCatching {
            val (version, sdk) = diagnosticEnvironment()
            secureStore.recordDiagnostic(DiagnosticEvent(
                id = UUID.randomUUID().toString(), occurredAt = Instant.now().toString(),
                operation = observed.operation, category = category, requestId = observed.id,
                httpCode = code, durationMillis = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - observed.started),
                appVersion = version.replace(Regex("[^a-zA-Z0-9.+_-]"), "_").take(80).ifEmpty { "unknown" }, androidApi = sdk,
            ), observed.generation)
        }
    }

    private fun buildClient(recoverAuth: Boolean): OkHttpClient {
        val builder = OkHttpClient.Builder().cookieJar(cookieJar)
            .addInterceptor(cookieJar.sessionGenerationInterceptor())
        if (recoverAuth) builder.addInterceptor(RefreshInterceptor { doRefresh() })
        return builder.addInterceptor(observer).build()
    }
    private val plainClient = buildClient(false)
    private val client = buildClient(true)

    fun login(serverUrl: String, username: String, password: String): ApiResult<Unit> {
        val body = json.encodeToString(LoginRequest(username, password)).toRequestBody(JSON_MEDIA_TYPE)
        val request = Request.Builder()
            .url(serverUrl.trimEnd('/') + "/api/auth/login")
            .post(body)
            .build()
        return runCatching { plainClient.newCall(request).execute() }
            .fold(
                onSuccess = { response -> response.use { classify(it) {} } },
                onFailure = { ApiResult.NetworkFailure(it) },
            )
    }

    /** Used by RefreshInterceptor. Success here also rotates the stored refresh cookie, via [SessionCookieJar]. */
    private fun doRefresh(): Boolean {
        val serverUrl = secureStore.serverUrl ?: return false
        val request = Request.Builder()
            .url(serverUrl.trimEnd('/') + "/api/auth/refresh")
            .post(EMPTY_BODY)
            .build()
        return try {
            plainClient.newCall(request).execute().use { classify(it) {} is ApiResult.Success }
        } catch (e: IOException) {
            false
        }
    }

    /**
     * GET /api/summary/today. On a 401 that survives RefreshInterceptor's own
     * refresh-and-retry (the refresh token itself is dead), this re-logs-in
     * once from SecureStore's stored credentials and retries once more before
     * reporting [ApiResult.Unauthenticated] — see the spec's "why the
     * password is stored at all" note: an unattended widget update should
     * recover a dead session rather than go dark until the owner opens the
     * app.
     *
     * The re-login's own outcome is reported as itself. Only a 401 from
     * /api/auth/login says the stored credentials are genuinely rejected;
     * a 429, an unreachable server or a Cloudflare Access challenge say
     * nothing about the session, and reporting them as "signed out" would
     * make a network blip look like an account problem — and, worse, invite
     * the caller to discard a session that is still perfectly valid.
     */
    fun summaryToday(): ApiResult<TodaySummary> = cookieJar.withSessionGeneration(
        secureStore.currentSessionGeneration,
    ) {
        val result = summaryTodayForPinnedSession()
        if (result is ApiResult.Success) runCatching { onSuccessfulSync(cookieJar.pinnedSessionGeneration) }
        result
    }

    private fun summaryTodayForPinnedSession(): ApiResult<TodaySummary> {
        val serverUrl = secureStore.serverUrl ?: return ApiResult.Unauthenticated
        val request = Request.Builder().url(serverUrl.trimEnd('/') + "/api/summary/today").get().build()

        val result = execute(request)
        if (result !is ApiResult.Unauthenticated) return result

        val username = secureStore.username
        val password = secureStore.password
        if (username == null || password == null) return result

        val reLogin = login(serverUrl, username, password)
        reLogin.failureOrNull()?.let { return it }

        return execute(request)
    }

    /** Uses the same cookie rotation and credential recovery as summaryToday. */
    fun uploadWeather(observation: WeatherObservation, consent: String, generation: Long,
                      allowed: () -> Boolean): ApiResult<Unit> = cookieJar.withSessionGeneration(generation) {
        fun active() = secureStore.weatherActive(consent, generation) && allowed()
        if (!active()) return@withSessionGeneration ApiResult.NetworkFailure(IOException("Weather collection stopped"))
        val server = secureStore.serverUrl ?: return@withSessionGeneration ApiResult.Unauthenticated
        val username = secureStore.username
        val password = secureStore.password
        val request = Request.Builder().url(server.trimEnd('/') + "/api/weather/locations")
            .post(json.encodeToString(observation).toRequestBody(JSON_MEDIA_TYPE)).build()
        val weatherClient = client.newBuilder().callTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
            .addInterceptor { chain ->
                if (!active()) throw IOException("Weather collection stopped")
                chain.proceed(chain.request())
            }.build()
        fun send(): ApiResult<Unit> = runCatching { weatherClient.newCall(request).execute() }.fold(
            onSuccess = { response -> response.use {
                classify(it) { body ->
                    check(it.code == 202)
                    val receipt = json.decodeFromString<WeatherAccepted>(body)
                    check(receipt.status == "accepted" && receipt.id == observation.id)
                    Unit
                }
            } }, onFailure = { ApiResult.NetworkFailure(it) },
        )
        val result = send()
        if (result !is ApiResult.Unauthenticated || !active() || username == null || password == null) {
            if (result is ApiResult.Success) runCatching { onSuccessfulSync(cookieJar.pinnedSessionGeneration) }
            return@withSessionGeneration result
        }
        val relogin = login(server, username, password)
        relogin.failureOrNull()?.let { return@withSessionGeneration it }
        val retried = send()
        if (retried is ApiResult.Success) runCatching { onSuccessfulSync(cookieJar.pinnedSessionGeneration) }
        retried
    }

    /** Sends only this session's pending events. Failure leaves every unacknowledged ID intact. */
    fun sendDiagnostics(generation: Long = secureStore.currentSessionGeneration): ApiResult<Int> = uploadLock.withLock {
        cookieJar.withSessionGeneration(generation) {
            val server = secureStore.serverUrl ?: return@withSessionGeneration ApiResult.Unauthenticated
            var count = 0
            // At most four bounded batches; never chase an unbounded concurrently growing queue.
            repeat(4) {
                val journal = secureStore.diagnosticJournal(generation)
                val events = journal.events.filter { it.id in journal.pendingIds }.take(50)
                if (events.isEmpty()) return@withSessionGeneration ApiResult.Success(count)
                if (!secureStore.isCurrentSession(generation)) return@withSessionGeneration ApiResult.Unauthenticated
                val request = Request.Builder().url(server.trimEnd('/') + "/api/diagnostics/events")
                    .post(json.encodeToString(DiagnosticBatch(events)).toRequestBody(JSON_MEDIA_TYPE)).build()
                val result = runCatching { plainClient.newBuilder().callTimeout(30, TimeUnit.SECONDS).build().newCall(request).execute() }.fold(
                    onSuccess = { response -> response.use {
                        classify(it) { body ->
                            check(it.code == 202)
                            val ids = json.decodeFromString<DiagnosticReceipt>(body).acceptedIds
                            check(ids.toSet() == events.map { e -> e.id }.toSet())
                            ids.toSet()
                        }
                    } }, onFailure = { ApiResult.NetworkFailure(it) },
                )
                if (result !is ApiResult.Success) return@withSessionGeneration result.failureOrNull()!!
                if (!secureStore.acknowledgeDiagnostics(result.value, generation)) return@withSessionGeneration ApiResult.Unauthenticated
                count += result.value.size
            }
            ApiResult.Success(count)
        }
    }

    fun setFiberTarget(grams: Int?): ApiResult<Unit> {
        val generation = secureStore.currentSessionGeneration
        return cookieJar.withSessionGeneration(generation) {
            val server = secureStore.serverUrl ?: return@withSessionGeneration ApiResult.Unauthenticated
            val request = Request.Builder().url(server.trimEnd('/') + "/api/users/me/fiber-target")
                .put(json.encodeToString(FiberSetting(grams)).toRequestBody(JSON_MEDIA_TYPE)).build()
            runCatching { client.newCall(request).execute() }.fold(
                onSuccess = { response -> response.use { classify(it) { body -> json.decodeFromString<FiberSetting>(body); Unit } } },
                onFailure = { ApiResult.NetworkFailure(it) },
            )
        }
    }

    private fun execute(request: Request): ApiResult<TodaySummary> =
        runCatching { client.newCall(request).execute() }
            .fold(
                onSuccess = { response -> response.use { classify(it) { body -> json.decodeFromString(body) } } },
                onFailure = { ApiResult.NetworkFailure(it) },
            )

    private fun <T> classify(response: Response, parse: (String) -> T): ApiResult<T> {
        val observed = response.request.tag(ObservedRequest::class.java)
        val body = try { response.body?.string() ?: "" } catch (e: IOException) {
            if (observed != null) record(observed, networkCategory(e), response.code)
            return ApiResult.NetworkFailure(e)
        }
        val outcome = classifyRawResponse(
            code = response.code,
            contentType = response.header("Content-Type"),
            body = body,
            // response.request.url reflects the *final* URL after OkHttp's
            // default redirect-following, so a Cloudflare Access challenge
            // that redirected to its own login host is visible here.
            finalUrlHost = response.request.url.host,
            retryAfterHeader = response.header("Retry-After"),
        )
        if (observed != null && !observed.recorded && outcome !is RawOutcome.Success) {
            val category = when (outcome) {
                is RawOutcome.Unauthenticated -> "unauthenticated"
                is RawOutcome.RateLimited -> "rate_limited"
                is RawOutcome.AccessChallenge -> "access_challenge"
                else -> "server"
            }
            record(observed, category, response.code)
        }
        return when (outcome) {
            is RawOutcome.Unauthenticated -> ApiResult.Unauthenticated
            is RawOutcome.RateLimited -> ApiResult.RateLimited(outcome.retryAfter)
            is RawOutcome.AccessChallenge -> ApiResult.AccessChallenge
            is RawOutcome.ServerError -> ApiResult.ServerError(outcome.code, outcome.body)
            is RawOutcome.Success -> {
                val parsed = runCatching { ApiResult.Success(parse(outcome.body)) }
                    .getOrElse { ApiResult.ServerError(response.code, "unparseable response") }
                if (observed != null && (!observed.recorded || parsed !is ApiResult.Success)) {
                    record(observed, if (parsed is ApiResult.Success) "success" else "invalid_response", response.code)
                }
                parsed
            }
        }
    }
}
