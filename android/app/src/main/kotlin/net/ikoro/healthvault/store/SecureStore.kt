package net.ikoro.healthvault.store

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
 */
class SecureStore(private val prefs: SharedPreferences) {

    private val json = Json { ignoreUnknownKeys = true }

    var serverUrl: String?
        get() = prefs.getString(KEY_SERVER_URL, null)
        set(value) {
            prefs.edit().putString(KEY_SERVER_URL, value).apply()
        }

    var username: String?
        get() = prefs.getString(KEY_USERNAME, null)
        set(value) {
            prefs.edit().putString(KEY_USERNAME, value).apply()
        }

    var password: String?
        get() = prefs.getString(KEY_PASSWORD, null)
        set(value) {
            prefs.edit().putString(KEY_PASSWORD, value).apply()
        }

    fun hasSession(): Boolean = username != null && password != null

    fun loadCookies(): List<PersistedCookie> {
        val raw = prefs.getString(KEY_COOKIES, null) ?: return emptyList()
        return runCatching { json.decodeFromString<List<PersistedCookie>>(raw) }.getOrDefault(emptyList())
    }

    fun saveCookies(cookies: List<PersistedCookie>) {
        prefs.edit().putString(KEY_COOKIES, json.encodeToString(cookies)).apply()
    }

    fun loadSnapshot(): SummarySnapshot? {
        val raw = prefs.getString(KEY_SNAPSHOT, null) ?: return null
        return runCatching { json.decodeFromString<SummarySnapshot>(raw) }.getOrNull()
    }

    fun saveSnapshot(snapshot: SummarySnapshot) {
        prefs.edit().putString(KEY_SNAPSHOT, json.encodeToString(snapshot)).apply()
    }

    /**
     * Sign-out: clears credentials, cookies and the cached snapshot.
     * `serverUrl` is intentionally kept — it is the address the widget and
     * the setup screen should keep pointing at, not part of the session.
     */
    fun clearSession() {
        prefs.edit()
            .remove(KEY_USERNAME)
            .remove(KEY_PASSWORD)
            .remove(KEY_COOKIES)
            .remove(KEY_SNAPSHOT)
            .apply()
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
