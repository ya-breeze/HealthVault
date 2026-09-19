package net.ikoro.healthvault

import android.app.Application
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.api.SessionCookieJar
import net.ikoro.healthvault.store.SecureStore

/**
 * Owns the process-wide singletons every entry point needs: the today screen,
 * the setup screen, and RefreshWorker (which runs in its own freshly started
 * process almost every time, so this construction must be cheap and cannot
 * assume anything survived from a previous run). Deliberately no DI
 * framework — three objects with one dependency edge each doesn't need one.
 */
class HealthVaultApp : Application() {

    val secureStore: SecureStore by lazy { SecureStore.create(this) }
    val cookieJar: SessionCookieJar by lazy { SessionCookieJar(secureStore) }
    val api: HealthVaultApi by lazy { HealthVaultApi(secureStore, cookieJar) }
}
