package net.ikoro.healthvault.api

import kotlin.time.Duration

/**
 * Classifies every outcome HealthVaultApi's calls can produce, so callers
 * (SetupScreen's validation, TodayScreen's refresh, RefreshWorker) branch on
 * a closed set of cases instead of parsing exceptions or status codes
 * themselves.
 */
sealed class ApiResult<out T> {
    data class Success<T>(val value: T) : ApiResult<T>()

    /** 401 that survived a refresh attempt (and, for summaryToday, a re-login attempt too). */
    data object Unauthenticated : ApiResult<Nothing>()

    /** 429 from the login limiter (backend/pkg/server/login_limiter.go); retryAfter is the Retry-After header. */
    data class RateLimited(val retryAfter: Duration) : ApiResult<Nothing>()

    /**
     * The response was a Cloudflare Access challenge, not the API: a redirect
     * to *.cloudflareaccess.com, or an HTML body where JSON was expected.
     * Reported distinctly so the UI says "this server needs an Access bypass
     * on /api/*" rather than "invalid credentials".
     */
    data object AccessChallenge : ApiResult<Nothing>()

    /** The request never got a response: DNS failure, connection refused, timeout, etc. */
    data class NetworkFailure(val cause: Throwable) : ApiResult<Nothing>()

    /** Any other non-2xx response. */
    data class ServerError(val code: Int, val message: String) : ApiResult<Nothing>()
}
