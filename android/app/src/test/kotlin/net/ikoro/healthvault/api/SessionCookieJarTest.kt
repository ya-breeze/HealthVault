package net.ikoro.healthvault.api

import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import okhttp3.Cookie
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

private const val HOST = "hcw.example.com"

private fun accessCookie(expiresAtMillis: Long = Long.MAX_VALUE, value: String = "access-token") = Cookie.Builder()
    .name("kin_access")
    .value(value)
    .hostOnlyDomain(HOST)
    .path("/api")
    .expiresAt(expiresAtMillis)
    .build()

private fun refreshCookie(expiresAtMillis: Long = Long.MAX_VALUE) = Cookie.Builder()
    .name("kin_refresh")
    .value("refresh-token")
    .hostOnlyDomain(HOST)
    .path("/api/auth/refresh")
    .expiresAt(expiresAtMillis)
    .build()

class SessionCookieJarTest {

    private fun url(path: String) = "https://$HOST$path".toHttpUrl()

    @Test
    fun `kin_refresh is only sent to the refresh path`() {
        val jar = SessionCookieJar(SecureStore(FakeSharedPreferences()))
        jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie(), refreshCookie()))

        val forSummary = jar.loadForRequest(url("/api/summary/today")).map { it.name }
        assertEquals(listOf("kin_access"), forSummary)

        val forRefresh = jar.loadForRequest(url("/api/auth/refresh")).map { it.name }.toSet()
        assertEquals(setOf("kin_access", "kin_refresh"), forRefresh)
    }

    @Test
    fun `expired cookies are dropped on load`() {
        val store = SecureStore(FakeSharedPreferences())
        val past = System.currentTimeMillis() - 1_000
        // Seed persistence directly: saveFromResponse deliberately discards an
        // already-expired cookie, which would never exercise loadForRequest's
        // expiry-removal branch.
        store.saveCookies(listOf(accessCookie(expiresAtMillis = past).toPersisted()))
        val jar = SessionCookieJar(store)

        assertTrue(jar.loadForRequest(url("/api/summary/today")).isEmpty())
        assertTrue("expired cookie must also be removed from persistence", store.loadCookies().isEmpty())
    }

    @Test
    fun `saveFromResponse does not persist an already-expired cookie`() {
        val prefs = FakeSharedPreferences()
        val jar = SessionCookieJar(SecureStore(prefs))
        val past = System.currentTimeMillis() - 1_000
        jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie(expiresAtMillis = past)))

        val reloaded = SessionCookieJar(SecureStore(prefs))
        assertTrue(reloaded.loadForRequest(url("/api/summary/today")).isEmpty())
    }

    @Test
    fun `cookies survive a store round trip`() {
        val prefs = FakeSharedPreferences()
        val jar = SessionCookieJar(SecureStore(prefs))
        jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie(), refreshCookie()))

        // A fresh jar over the same backing store simulates a new process —
        // a widget update runs in one almost every time.
        val reloaded = SessionCookieJar(SecureStore(prefs))
        val forRefresh = reloaded.loadForRequest(url("/api/auth/refresh")).map { it.name to it.value }.toSet()
        assertEquals(setOf("kin_access" to "access-token", "kin_refresh" to "refresh-token"), forRefresh)
    }

    @Test
    fun `a new cookie of the same name, domain and path replaces the old value`() {
        val jar = SessionCookieJar(SecureStore(FakeSharedPreferences()))
        jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie()))
        val rotated = Cookie.Builder()
            .name("kin_access")
            .value("rotated-token")
            .hostOnlyDomain(HOST)
            .path("/api")
            .expiresAt(Long.MAX_VALUE)
            .build()
        jar.saveFromResponse(url("/api/auth/refresh"), listOf(rotated))

        val values = jar.loadForRequest(url("/api/summary/today")).map { it.value }
        assertEquals(listOf("rotated-token"), values)
    }

    @Test
    fun `clear removes every cookie including the refresh token`() {
        val jar = SessionCookieJar(SecureStore(FakeSharedPreferences()))
        jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie(), refreshCookie()))
        jar.clear()

        assertFalse(jar.loadForRequest(url("/api/auth/refresh")).isNotEmpty())
    }

    @Test
    fun `atomic session clear followed by in-memory clear cannot restore the cookie jar`() {
        val prefs = FakeSharedPreferences()
        val store = SecureStore(prefs).apply {
            username = "alice"
            password = "secret"
        }
        val jar = SessionCookieJar(store)
        jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie(), refreshCookie()))

        store.clearSession()
        jar.clearInMemory()

        assertFalse(store.hasSession())
        assertTrue(store.loadCookies().isEmpty())
        assertTrue(jar.loadForRequest(url("/api/auth/refresh")).isEmpty())
    }

    @Test
    fun `a late response from an old session cannot replace the new session cookies`() {
        val store = SecureStore(FakeSharedPreferences())
        val jar = SessionCookieJar(store)
        store.saveSession("https://$HOST", "alice", "old-secret")
        val aliceGeneration = store.currentSessionGeneration

        store.clearSession()
        jar.clearInMemory()
        store.saveSession("https://$HOST", "bob", "new-secret")
        val bobGeneration = store.currentSessionGeneration
        jar.withSessionGeneration(bobGeneration) {
            jar.saveFromResponse(url("/api/auth/login"), listOf(accessCookie(value = "bob-token")))
        }

        jar.withSessionGeneration(aliceGeneration) {
            jar.saveFromResponse(url("/api/auth/refresh"), listOf(accessCookie(value = "late-alice-token")))
        }

        assertEquals(listOf("bob-token"), jar.loadForRequest(url("/api/summary/today")).map { it.value })
        assertEquals(listOf("bob-token"), store.loadCookies().map { it.value })
    }
}
