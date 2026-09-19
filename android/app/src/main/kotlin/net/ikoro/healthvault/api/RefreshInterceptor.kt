package net.ikoro.healthvault.api

import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock
import okhttp3.Interceptor
import okhttp3.Response

private val authExemptPaths = setOf("/api/auth/login", "/api/auth/refresh")

/** Mirrors isAuthExemptPath in frontend/lib/api.ts. */
fun isAuthExemptPath(path: String): Boolean = path in authExemptPaths

/**
 * Reproduces frontend/lib/api.ts's coordinatedRefresh rule for OkHttp: a 401
 * for a request dispatched *before* the last successful refresh completed is
 * retried, not refreshed again.
 *
 * That rule exists because authdb.RotateRefreshToken (backend/pkg/server/auth.go)
 * consumes the refresh token it is given — two concurrent refreshes would
 * spend an already-rotated token and sign the device out. A ReentrantLock
 * plays the role the frontend's Web Lock + in-flight promise play together:
 * every request that hits a 401 blocks here until the current refresh (if
 * any) finishes, then re-checks [lastRefreshCompletedAt] before deciding
 * whether it still needs to refresh itself.
 */
class RefreshInterceptor(private val refresher: Refresher) : Interceptor {

    /** Performs POST /api/auth/refresh. Returns true on success. */
    fun interface Refresher {
        fun refresh(): Boolean
    }

    private val lock = ReentrantLock()

    @Volatile
    private var lastRefreshCompletedAt: Long = 0L

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val dispatchedAt = System.currentTimeMillis()
        val response = chain.proceed(request)

        if (response.code != 401 || isAuthExemptPath(request.url.encodedPath)) {
            return response
        }

        val refreshed = coordinatedRefresh(dispatchedAt)
        if (!refreshed) {
            return response
        }

        response.close()
        return chain.proceed(request)
    }

    private fun coordinatedRefresh(dispatchedAt: Long): Boolean {
        // Fast path without the lock: a refresh that already completed at or
        // after dispatchedAt means this 401 was caused by the pre-refresh
        // token, so retrying (not refreshing again) is correct.
        if (lastRefreshCompletedAt >= dispatchedAt) return true

        lock.withLock {
            // Re-check inside the lock: another thread may have refreshed
            // while this one waited for the lock — exactly the race this
            // lock exists to serialize.
            if (lastRefreshCompletedAt >= dispatchedAt) return true

            val ok = refresher.refresh()
            if (ok) lastRefreshCompletedAt = System.currentTimeMillis()
            return ok
        }
    }
}
