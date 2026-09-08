package net.ikoro.healthvault.api

import kotlin.time.Duration.Companion.seconds

/** [classifyRawResponse]'s result, before the response body (if any) is parsed into a specific type. */
internal sealed class RawOutcome {
    data class Success(val body: String) : RawOutcome()
    data object Unauthenticated : RawOutcome()
    data class RateLimited(val retryAfter: kotlin.time.Duration) : RawOutcome()
    data object AccessChallenge : RawOutcome()
    data class ServerError(val code: Int, val body: String) : RawOutcome()
}

/**
 * Pure classification of an HTTP response's shape into a [RawOutcome] —
 * split out from HealthVaultApi so it needs no OkHttpClient/MockWebServer to
 * unit test, in particular the Cloudflare Access redirect case, which would
 * otherwise require actually following a redirect to a real external host.
 *
 * Checked in this order: an Access challenge can arrive with any status code
 * (including 200, for its own login page), so it is checked first, ahead of
 * 401/429/success.
 */
internal fun classifyRawResponse(
    code: Int,
    contentType: String?,
    body: String,
    finalUrlHost: String,
    retryAfterHeader: String?,
): RawOutcome {
    val looksLikeAccessChallenge = finalUrlHost.endsWith("cloudflareaccess.com") ||
        (contentType?.contains("text/html", ignoreCase = true) == true && body.contains("<html", ignoreCase = true))
    if (looksLikeAccessChallenge) return RawOutcome.AccessChallenge

    return when {
        code == 401 -> RawOutcome.Unauthenticated
        code == 429 -> RawOutcome.RateLimited((retryAfterHeader?.toLongOrNull() ?: 1L).seconds)
        code in 200..299 -> RawOutcome.Success(body)
        else -> RawOutcome.ServerError(code, body)
    }
}
