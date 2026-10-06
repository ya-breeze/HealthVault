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
import net.ikoro.healthvault.weather.WeatherObservation
import java.time.Instant
import java.util.UUID

private const val PREFS_NAME = "healthvault_secure_store"
private const val KEY_WEATHER_CONSENT = "weather_consent"
private const val KEY_WEATHER_QUEUE = "weather_queue"
private const val KEY_SERVER_URL = "server_url"
private const val KEY_USERNAME = "username"
private const val KEY_PASSWORD = "password"
private const val KEY_COOKIES = "cookies"
private const val KEY_SNAPSHOT = "snapshot"
private const val KEY_REFRESH_FAILED = "refresh_failed"
private const val KEY_NEXT_REFRESH_AT = "next_refresh_at"

@Serializable
data class SummarySnapshot(val summary: TodaySummary, val fetchedAtMillis: Long)

/**
 * Keystore-backed AES-GCM encrypted store for the session: server URL,
 * credentials, the persistent cookie jar, refresh status/holdoff, and the last
 * summary snapshot with its fetch time. Credentials are stored (not just cookies) so a refresh
 * failure can re-log-in once from them (see HealthVaultApi) — a home-screen
 * widget that silently stops updating until the owner opens the app is worse
 * than one that recovers itself. The residual risk of an attacker holding the
 * unlocked, rooted device is accepted; see docs/adr/ADR-015.
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
 * threads, and UI storage work inside a Dispatchers.IO block — which is the
 * only concern lint's `ApplySharedPref` check ("prefer apply()") actually
 * raises, hence the suppression on the class.
 */
@SuppressLint("ApplySharedPref")
class SecureStore(private val prefs: SharedPreferences) {

    private val json = Json { ignoreUnknownKeys = true }
    private val refreshStateLock = Any()
    private var sessionGeneration = 0L

    var serverUrl: String?
        get() = prefs.getString(KEY_SERVER_URL, null)
        set(value) {
            commitOrThrow(prefs.edit().putString(KEY_SERVER_URL, value), "server URL")
        }

    var username: String?
        get() = prefs.getString(KEY_USERNAME, null)
        set(value) {
            commitOrThrow(prefs.edit().putString(KEY_USERNAME, value), "username")
        }

    var password: String?
        get() = prefs.getString(KEY_PASSWORD, null)
        set(value) {
            commitOrThrow(prefs.edit().putString(KEY_PASSWORD, value), "password")
        }

    val refreshFailed: Boolean
        get() = prefs.getBoolean(KEY_REFRESH_FAILED, false)

    val nextRefreshAtMillis: Long
        get() = prefs.getLong(KEY_NEXT_REFRESH_AT, 0L)

    val currentSessionGeneration: Long
        get() = synchronized(refreshStateLock) { sessionGeneration }

    fun isCurrentSession(generation: Long): Boolean =
        synchronized(refreshStateLock) { generation == sessionGeneration }

    fun saveRefreshState(
        failed: Boolean,
        nextAttemptAtMillis: Long,
        nowMillis: Long = System.currentTimeMillis(),
        expectedSessionGeneration: Long? = null,
    ): Boolean {
        synchronized(refreshStateLock) {
            if (expectedSessionGeneration != null && expectedSessionGeneration != sessionGeneration) return false
            // Foreground and worker refreshes can overlap. A response that
            // started before a 429 must not erase the newer server holdoff
            // when it finishes afterward.
            val activeDeadline = prefs.getLong(KEY_NEXT_REFRESH_AT, 0L).takeIf { it > nowMillis } ?: 0L
            val effectiveDeadline = maxOf(nextAttemptAtMillis, activeDeadline)
            commitOrThrow(
                prefs.edit()
                    .putBoolean(KEY_REFRESH_FAILED, failed || effectiveDeadline > nowMillis)
                    .putLong(KEY_NEXT_REFRESH_AT, effectiveDeadline),
                "refresh state",
            )
            return true
        }
    }

    /** Installs a newly authenticated session and invalidates writes from requests for the old one. */
    fun saveSession(serverUrl: String, username: String, password: String) {
        synchronized(refreshStateLock) {
            commitOrThrow(
                prefs.edit()
                    .putString(KEY_SERVER_URL, serverUrl)
                    .putString(KEY_USERNAME, username)
                    .putString(KEY_PASSWORD, password)
                    .remove(KEY_SNAPSHOT)
                    .remove(KEY_REFRESH_FAILED)
                    .remove(KEY_NEXT_REFRESH_AT)
                    .remove(KEY_WEATHER_CONSENT)
                    .remove(KEY_WEATHER_QUEUE),
                "session",
            )
            sessionGeneration++
        }
    }

    fun hasSession(): Boolean = username != null && password != null

    fun loadCookies(): List<PersistedCookie> {
        val raw = prefs.getString(KEY_COOKIES, null) ?: return emptyList()
        return runCatching { json.decodeFromString<List<PersistedCookie>>(raw) }.getOrDefault(emptyList())
    }

    /** Durable before it returns — SessionCookieJar's contract depends on it; see the class doc. */
    fun saveCookies(cookies: List<PersistedCookie>, expectedSessionGeneration: Long? = null): Boolean {
        synchronized(refreshStateLock) {
            if (expectedSessionGeneration != null && expectedSessionGeneration != sessionGeneration) return false
            commitOrThrow(
                prefs.edit().putString(KEY_COOKIES, json.encodeToString(cookies)),
                "session cookies",
            )
            return true
        }
    }

    fun loadSnapshot(): SummarySnapshot? {
        val raw = prefs.getString(KEY_SNAPSHOT, null) ?: return null
        return runCatching { json.decodeFromString<SummarySnapshot>(raw) }.getOrNull()
    }

    /** Cache only, so `apply()`: losing it costs one re-fetch, and TodayScreen writes it on the UI thread. */
    fun saveSnapshot(snapshot: SummarySnapshot, expectedSessionGeneration: Long? = null): Boolean {
        synchronized(refreshStateLock) {
            if (expectedSessionGeneration != null && expectedSessionGeneration != sessionGeneration) return false
            prefs.edit().putString(KEY_SNAPSHOT, json.encodeToString(snapshot)).apply()
            return true
        }
    }

    /**
     * Sign-out: clears credentials, cookies and the cached snapshot.
     * `serverUrl` is intentionally kept — it is the address the widget and
     * the setup screen should keep pointing at, not part of the session.
     *
     * Committed, not applied: a dropped removal would restore the credentials
     * on the next launch, which is the opposite of what was asked for.
     */
    fun clearSession(expectedSessionGeneration: Long? = null): Boolean {
        synchronized(refreshStateLock) {
            if (expectedSessionGeneration != null && expectedSessionGeneration != sessionGeneration) return false
            val editor = prefs.edit()
                .remove(KEY_USERNAME)
                .remove(KEY_PASSWORD)
                .remove(KEY_COOKIES)
                .remove(KEY_SNAPSHOT)
                .remove(KEY_REFRESH_FAILED)
                .remove(KEY_NEXT_REFRESH_AT)
                .remove(KEY_WEATHER_CONSENT)
                .remove(KEY_WEATHER_QUEUE)
            commitOrThrow(editor, "session clear")
            sessionGeneration++
            return true
        }
    }

    /** Consent nonce scopes pending work to one opt-in and one authenticated session. */
    val weatherConsent: String?
        get() = synchronized(refreshStateLock) { prefs.getString(KEY_WEATHER_CONSENT, null) }

    fun setWeatherEnabled(enabled: Boolean, expectedGeneration: Long): Boolean = synchronized(refreshStateLock) {
        if (expectedGeneration != sessionGeneration) return false
        val editor = prefs.edit().remove(KEY_WEATHER_QUEUE)
        if (enabled && hasSession()) editor.putString(KEY_WEATHER_CONSENT, UUID.randomUUID().toString())
        else editor.remove(KEY_WEATHER_CONSENT)
        commitOrThrow(editor, "weather consent")
        true
    }

    fun weatherActive(consent: String, generation: Long): Boolean = synchronized(refreshStateLock) {
        generation == sessionGeneration && hasSession() && weatherConsent == consent
    }

    fun weatherQueue(consent: String, generation: Long): List<WeatherObservation> = synchronized(refreshStateLock) {
        if (!weatherActive(consent, generation)) return emptyList()
        val raw = prefs.getString(KEY_WEATHER_QUEUE, null) ?: return emptyList()
        runCatching { json.decodeFromString<List<WeatherObservation>>(raw) }.getOrDefault(emptyList())
    }

    fun enqueueWeather(observation: WeatherObservation, consent: String, generation: Long,
                       nowMillis: Long = System.currentTimeMillis()): Boolean = synchronized(refreshStateLock) {
        if (!weatherActive(consent, generation)) return false
        val queue = (weatherQueue(consent, generation) + observation).filter {
            val timestamp = runCatching { Instant.parse(it.observedAt).toEpochMilli() }.getOrNull()
            timestamp != null && nowMillis - timestamp <= WeatherObservation.QUEUE_MAX_AGE_MILLIS
        }.takeLast(WeatherObservation.MAX_QUEUE_SIZE)
        commitOrThrow(prefs.edit().putString(KEY_WEATHER_QUEUE, json.encodeToString(queue)), "weather queue")
        true
    }

    fun removeWeather(id: String, consent: String, generation: Long): Boolean = synchronized(refreshStateLock) {
        if (!weatherActive(consent, generation)) return false
        val remaining = weatherQueue(consent, generation).filterNot { it.id == id }
        commitOrThrow(prefs.edit().putString(KEY_WEATHER_QUEUE, json.encodeToString(remaining)), "weather queue")
        true
    }

    private fun commitOrThrow(editor: SharedPreferences.Editor, description: String) {
        check(editor.commit()) { "Failed to persist $description" }
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
