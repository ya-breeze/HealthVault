package net.ikoro.healthvault.api

import java.io.IOException
import java.time.Instant
import java.time.temporal.ChronoUnit
import okhttp3.HttpUrl.Companion.toHttpUrl
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
) {
    private val plainClient = OkHttpClient.Builder()
        .addInterceptor(cookieJar.sessionGenerationInterceptor())
        .cookieJar(cookieJar)
        .build()

    private val client = plainClient.newBuilder()
        .addInterceptor(RefreshInterceptor { doRefresh() })
        .build()

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
            plainClient.newCall(request).execute().use { it.isSuccessful }
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
        summaryTodayForPinnedSession()
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
            return@withSessionGeneration result
        }
        val relogin = login(server, username, password)
        relogin.failureOrNull()?.let { return@withSessionGeneration it }
        send()
    }

    /** Read saved history independently of location consent; preserve the initiating account. */
    fun weatherHistory(now: Instant, generation: Long = secureStore.currentSessionGeneration): ApiResult<WeatherHistory> =
        cookieJar.withSessionGeneration(generation) {
            if (!secureStore.isCurrentSession(generation)) {
                return@withSessionGeneration ApiResult.NetworkFailure(IOException("Session changed"))
            }
            val server = secureStore.serverUrl ?: return@withSessionGeneration ApiResult.Unauthenticated
            val username = secureStore.username
            val password = secureStore.password
            val end = now.truncatedTo(ChronoUnit.HOURS)
            val url = (server.trimEnd('/') + "/api/weather/history").toHttpUrl().newBuilder()
                .addQueryParameter("from", end.minus(7, ChronoUnit.DAYS).toString())
                .addQueryParameter("to", end.toString()).build()
            val request = Request.Builder().url(url).get().build()
            val historyClient = client.newBuilder().callTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
                .addInterceptor { chain ->
                    if (!secureStore.isCurrentSession(generation)) throw IOException("Session changed")
                    chain.proceed(chain.request())
                }.build()
            fun send(): ApiResult<WeatherHistory> = runCatching { historyClient.newCall(request).execute() }.fold(
                onSuccess = { response -> response.use { classify(it) { body ->
                    json.decodeFromString<WeatherHistory>(body).requireValid()
                } } }, onFailure = { ApiResult.NetworkFailure(it) },
            )
            val result = send()
            if (result !is ApiResult.Unauthenticated || !secureStore.isCurrentSession(generation) ||
                username == null || password == null) return@withSessionGeneration result
            val relogin = login(server, username, password)
            relogin.failureOrNull()?.let { return@withSessionGeneration it }
            send()
        }

    private fun execute(request: Request): ApiResult<TodaySummary> =
        runCatching { client.newCall(request).execute() }
            .fold(
                onSuccess = { response -> response.use { classify(it) { body -> json.decodeFromString(body) } } },
                onFailure = { ApiResult.NetworkFailure(it) },
            )

    private fun <T> classify(response: Response, parse: (String) -> T): ApiResult<T> {
        val outcome = classifyRawResponse(
            code = response.code,
            contentType = response.header("Content-Type"),
            body = response.body?.string() ?: "",
            // response.request.url reflects the *final* URL after OkHttp's
            // default redirect-following, so a Cloudflare Access challenge
            // that redirected to its own login host is visible here.
            finalUrlHost = response.request.url.host,
            retryAfterHeader = response.header("Retry-After"),
        )
        return when (outcome) {
            is RawOutcome.Unauthenticated -> ApiResult.Unauthenticated
            is RawOutcome.RateLimited -> ApiResult.RateLimited(outcome.retryAfter)
            is RawOutcome.AccessChallenge -> ApiResult.AccessChallenge
            is RawOutcome.ServerError -> ApiResult.ServerError(outcome.code, outcome.body)
            is RawOutcome.Success -> runCatching { ApiResult.Success(parse(outcome.body)) }
                .getOrElse { ApiResult.ServerError(response.code, "unparseable response") }
        }
    }
}
