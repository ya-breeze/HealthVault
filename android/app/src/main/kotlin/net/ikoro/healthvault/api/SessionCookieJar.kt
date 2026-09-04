package net.ikoro.healthvault.api

import kotlinx.serialization.Serializable
import net.ikoro.healthvault.store.SecureStore
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl

/**
 * A plain-data mirror of the fields of okhttp3.Cookie that matter for
 * persistence and re-matching. kotlinx.serialization can't serialize
 * okhttp3.Cookie directly, and round-tripping through Cookie.toString() /
 * Cookie.parse() is fragile (parse() needs a base URL to default an
 * unspecified Domain attribute, and toString()'s format is not a documented
 * contract) — so this stores the fields explicitly and rebuilds an
 * okhttp3.Cookie with Cookie.Builder on load.
 */
@Serializable
data class PersistedCookie(
    val name: String,
    val value: String,
    val expiresAtMillis: Long,
    val domain: String,
    val hostOnly: Boolean,
    val path: String,
    val secure: Boolean,
    val httpOnly: Boolean,
)

fun Cookie.toPersisted(): PersistedCookie = PersistedCookie(
    name = name,
    value = value,
    expiresAtMillis = expiresAt,
    domain = domain,
    hostOnly = hostOnly,
    path = path,
    secure = secure,
    httpOnly = httpOnly,
)

fun PersistedCookie.toCookie(): Cookie {
    val builder = Cookie.Builder()
        .name(name)
        .value(value)
        .expiresAt(expiresAtMillis)
        .path(path)
    if (hostOnly) builder.hostOnlyDomain(domain) else builder.domain(domain)
    if (secure) builder.secure()
    if (httpOnly) builder.httpOnly()
    return builder.build()
}

/**
 * A persistent CookieJar backed by SecureStore. Domain, path and expiry
 * matching is delegated to okhttp3.Cookie.matches(url), which already
 * implements the RFC 6265 semantics this needs — in particular, kin-core's
 * SetRefreshCookie scopes kin_refresh to Path=/api/auth/refresh
 * (backend/pkg/server/auth.go), and matches() is what keeps that cookie off
 * every other request, including GET /api/summary/today.
 *
 * Every mutation writes through to SecureStore synchronously, before this
 * call returns — a widget update runs in a freshly started process almost
 * every time, so a rotated cookie that isn't durably written before the
 * process is killed is gone forever.
 */
class SessionCookieJar(private val secureStore: SecureStore) : CookieJar {

    private val lock = Any()
    private val cookies: MutableList<Cookie> =
        secureStore.loadCookies().map { it.toCookie() }.toMutableList()

    override fun saveFromResponse(url: HttpUrl, newCookies: List<Cookie>) {
        if (newCookies.isEmpty()) return
        synchronized(lock) {
            for (cookie in newCookies) {
                cookies.removeAll { existing ->
                    existing.name == cookie.name && existing.domain == cookie.domain && existing.path == cookie.path
                }
                // expiresAt <= now is okhttp's own convention for "delete
                // this cookie" (a Set-Cookie with Max-Age=0 or a past
                // Expires), so honour it as a removal rather than storing an
                // already-dead cookie.
                if (cookie.expiresAt > System.currentTimeMillis()) {
                    cookies.add(cookie)
                }
            }
            persistLocked()
        }
    }

    override fun loadForRequest(url: HttpUrl): List<Cookie> {
        synchronized(lock) {
            val now = System.currentTimeMillis()
            val hadExpired = cookies.removeAll { it.expiresAt <= now }
            if (hadExpired) persistLocked()
            return cookies.filter { it.matches(url) }
        }
    }

    /** Used by sign-out: drops every cookie, including the refresh token. */
    fun clear() {
        synchronized(lock) {
            cookies.clear()
            persistLocked()
        }
    }

    private fun persistLocked() {
        secureStore.saveCookies(cookies.map { it.toPersisted() })
    }
}
