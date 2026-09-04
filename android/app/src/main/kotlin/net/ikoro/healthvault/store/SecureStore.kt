package net.ikoro.healthvault.store

import android.annotation.SuppressLint
import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import net.ikoro.healthvault.api.PersistedCookie
import net.ikoro.healthvault.api.TodaySummary

private const val PREFS_NAME = "healthvault_secure_store"
private const val KEY_SERVER_URL = "server_url"
private const val KEY_USERNAME = "username"
private const val KEY_PASSWORD = "password"
private const val KEY_COOKIES = "cookies"
private const val KEY_SNAPSHOT = "snapshot"

@Serializable
data class SummarySnapshot(val summary: TodaySummary, val fetchedAtMillis: Long)

/**
 * Keystore-backed AES-GCM encrypted store for the session: server URL,
 * credentials, the persistent cookie jar, and the last summary snapshot with
 * its fetch time. Credentials are stored (not just cookies) so a refresh
 * failure can re-log-in once from them (see HealthVaultApi) — a home-screen
 * widget that silently stops updating until the owner opens the app is worse
 * than one that recovers itself. The residual risk of an attacker holding the
 * unlocked, rooted device is accepted; see docs/adr/ADR-013.
 *
 * Takes a SharedPreferences instance rather than a Context so unit tests can
 * inject a plain in-memory fake: android.content.SharedPreferences is just an
 * interface at compile time and needs no Android runtime to implement,
 * unlike EncryptedSharedPreferences itself. [create] wires the real
 * Keystore-backed instance for production use.
 *
 * **commit() versus apply(), deliberately split.** `apply()` updates the
 * in-memory map at once but writes the file on a background thread, and a
 * process the system kills outright — which is the normal end of a widget
 * refresh — never runs that write. Everything whose loss cannot be undone by
 * fetching again is therefore written with `commit()`:
 *
 *  - cookies, because `authdb.RotateRefreshToken` *consumes* the token it
 *    rotates. A refresh token the server has already retired and this device
 *    never persisted is gone for good, and the session with it;
 *  - credentials and the server URL, because they are the only thing the
 *    unattended re-login fallback has to work from;
 *  - [clearSession], because a sign-out whose write is dropped brings the
 *    credentials back on the next launch.
 *
 * The summary snapshot is the one exception: it is a display cache, the next
 * refresh replaces it, and it is written from the UI thread (TodayScreen), so
 * it keeps `apply()` rather than doing disk I/O in a frame. Every `commit()`
 * caller here runs off the main thread — the cookie jar on OkHttp's network
 * threads, sign-in and sign-out inside a Dispatchers.IO block — which is the
 * only concern lint's `ApplySharedPref` check ("prefer apply()") actually
 * raises, hence the suppression on the class.
 */
@SuppressLint("ApplySharedPref")
class SecureStore(private val prefs: SharedPreferences) {

    private val json = Json { ignoreUnknownKeys = true }

    var serverUrl: String?
        get() = prefs.getString(KEY_SERVER_URL, null)
        set(value) {
            prefs.edit().putString(KEY_SERVER_URL, value).commit()
        }

    var username: String?
        get() = prefs.getString(KEY_USERNAME, null)
        set(value) {
            prefs.edit().putString(KEY_USERNAME, value).commit()
        }

    var password: String?
        get() = prefs.getString(KEY_PASSWORD, null)
        set(value) {
            prefs.edit().putString(KEY_PASSWORD, value).commit()
        }

    fun hasSession(): Boolean = username != null && password != null

    fun loadCookies(): List<PersistedCookie> {
        val raw = prefs.getString(KEY_COOKIES, null) ?: return emptyList()
        return runCatching { json.decodeFromString<List<PersistedCookie>>(raw) }.getOrDefault(emptyList())
    }

    /** Durable before it returns — SessionCookieJar's contract depends on it; see the class doc. */
    fun saveCookies(cookies: List<PersistedCookie>) {
        prefs.edit().putString(KEY_COOKIES, json.encodeToString(cookies)).commit()
    }

    fun loadSnapshot(): SummarySnapshot? {
        val raw = prefs.getString(KEY_SNAPSHOT, null) ?: return null
        return runCatching { json.decodeFromString<SummarySnapshot>(raw) }.getOrNull()
    }

    /** Cache only, so `apply()`: losing it costs one re-fetch, and TodayScreen writes it on the UI thread. */
    fun saveSnapshot(snapshot: SummarySnapshot) {
        prefs.edit().putString(KEY_SNAPSHOT, json.encodeToString(snapshot)).apply()
    }

    /**
     * Sign-out: clears credentials, cookies and the cached snapshot.
     * `serverUrl` is intentionally kept — it is the address the widget and
     * the setup screen should keep pointing at, not part of the session.
     *
     * Committed, not applied: a dropped removal would restore the credentials
     * on the next launch, which is the opposite of what was asked for.
     */
    fun clearSession() {
        prefs.edit()
            .remove(KEY_USERNAME)
            .remove(KEY_PASSWORD)
            .remove(KEY_COOKIES)
            .remove(KEY_SNAPSHOT)
            .commit()
    }

    companion object {
        fun create(context: Context): SecureStore {
            val masterKey = MasterKey.Builder(context)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build()
            val prefs = EncryptedSharedPreferences.create(
                context,
                PREFS_NAME,
                masterKey,
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
            )
            return SecureStore(prefs)
        }
    }
}
