package net.ikoro.healthvault.api

import java.io.IOException
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import net.ikoro.healthvault.store.SecureStore
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.internal.EMPTY_REQUEST

private val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()

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
    cookieJar: SessionCookieJar,
    private val json: Json = Json { ignoreUnknownKeys = true },
) {
    private val plainClient = OkHttpClient.Builder().cookieJar(cookieJar).build()

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
            .post(EMPTY_REQUEST)
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
     */
    fun summaryToday(): ApiResult<TodaySummary> {
        val serverUrl = secureStore.serverUrl ?: return ApiResult.Unauthenticated
        val request = Request.Builder().url(serverUrl.trimEnd('/') + "/api/summary/today").get().build()

        val result = execute(request)
        if (result !is ApiResult.Unauthenticated) return result

        val username = secureStore.username
        val password = secureStore.password
        if (username == null || password == null) return result

        val reLogin = login(serverUrl, username, password)
        if (reLogin !is ApiResult.Success) return result

        return execute(request)
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
