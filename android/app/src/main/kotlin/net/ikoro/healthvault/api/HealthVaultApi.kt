package net.ikoro.healthvault.api

import java.io.IOException
import kotlin.time.Duration.Companion.seconds
import kotlinx.serialization.Serializable
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
        val contentType = response.header("Content-Type") ?: ""
        val body = response.body?.string() ?: ""

        if (looksLikeAccessChallenge(response, contentType, body)) {
            return ApiResult.AccessChallenge
        }
        return when {
            response.code == 401 -> ApiResult.Unauthenticated
            response.code == 429 -> {
                val retrySeconds = response.header("Retry-After")?.toLongOrNull() ?: 1L
                ApiResult.RateLimited(retrySeconds.seconds)
            }
            response.isSuccessful -> runCatching { ApiResult.Success(parse(body)) }
                .getOrElse { ApiResult.ServerError(response.code, "unparseable response") }
            else -> ApiResult.ServerError(response.code, body)
        }
    }

    /**
     * A Cloudflare Access challenge can arrive as a redirect (whose final
     * response, after OkHttp's default redirect-following, has
     * response.request.url pointed at *.cloudflareaccess.com) or as an HTML
     * login page returned in place of the JSON this client expects. Checked
     * before every other status branch, since Access can return any status
     * code, including 200, for its own login page.
     */
    private fun looksLikeAccessChallenge(response: Response, contentType: String, body: String): Boolean {
        if (response.request.url.host.endsWith("cloudflareaccess.com")) return true
        return contentType.contains("text/html", ignoreCase = true) && body.contains("<html", ignoreCase = true)
    }
}
