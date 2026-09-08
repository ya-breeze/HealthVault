package net.ikoro.healthvault.api

import kotlin.time.Duration.Companion.seconds
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ResponseClassifierTest {

    @Test
    fun `a redirect to cloudflareaccess-com is an Access challenge, not Unauthenticated`() {
        val outcome = classifyRawResponse(
            code = 200,
            contentType = "text/html; charset=utf-8",
            body = "<html><body>Sign in with Google</body></html>",
            finalUrlHost = "healthvault.cloudflareaccess.com",
            retryAfterHeader = null,
        )
        assertTrue(outcome is RawOutcome.AccessChallenge)
    }

    @Test
    fun `an HTML body where JSON was expected is an Access challenge`() {
        val outcome = classifyRawResponse(
            code = 200,
            contentType = "text/html; charset=utf-8",
            body = "<html><head></head><body>login</body></html>",
            finalUrlHost = "hcw.example.com",
            retryAfterHeader = null,
        )
        assertTrue(outcome is RawOutcome.AccessChallenge)
    }

    @Test
    fun `a plain JSON 401 is Unauthenticated, not an Access challenge`() {
        val outcome = classifyRawResponse(
            code = 401,
            contentType = "application/json",
            body = "unauthorized",
            finalUrlHost = "hcw.example.com",
            retryAfterHeader = null,
        )
        assertEquals(RawOutcome.Unauthenticated, outcome)
    }

    @Test
    fun `429 carries Retry-After as a Duration`() {
        val outcome = classifyRawResponse(
            code = 429,
            contentType = "application/json",
            body = """{"error":"too_many_attempts","retry_after_seconds":42}""",
            finalUrlHost = "hcw.example.com",
            retryAfterHeader = "42",
        )
        assertEquals(RawOutcome.RateLimited(42.seconds), outcome)
    }

    @Test
    fun `429 without a Retry-After header still classifies, defaulting to 1 second`() {
        val outcome = classifyRawResponse(
            code = 429,
            contentType = "application/json",
            body = "{}",
            finalUrlHost = "hcw.example.com",
            retryAfterHeader = null,
        )
        assertEquals(RawOutcome.RateLimited(1.seconds), outcome)
    }

    @Test
    fun `a 2xx JSON body is Success`() {
        val outcome = classifyRawResponse(
            code = 200,
            contentType = "application/json",
            body = """{"status":"ok"}""",
            finalUrlHost = "hcw.example.com",
            retryAfterHeader = null,
        )
        assertEquals(RawOutcome.Success("""{"status":"ok"}"""), outcome)
    }

    @Test
    fun `any other non-2xx is a ServerError`() {
        val outcome = classifyRawResponse(
            code = 500,
            contentType = "application/json",
            body = "internal error",
            finalUrlHost = "hcw.example.com",
            retryAfterHeader = null,
        )
        assertEquals(RawOutcome.ServerError(500, "internal error"), outcome)
    }
}
