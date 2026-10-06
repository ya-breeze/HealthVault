package net.ikoro.healthvault.weather

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Build
import android.os.Bundle
import android.os.Looper
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.api.ApiResult
import kotlin.coroutines.resume

object WeatherScheduler {
    private const val NAME = "weather-location-hourly"
    fun foregroundAllowed(context: Context): Boolean = ContextCompat.checkSelfPermission(context,
        Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED
    fun allowed(context: Context): Boolean = foregroundAllowed(context) && (Build.VERSION.SDK_INT < 29 ||
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_BACKGROUND_LOCATION) == PackageManager.PERMISSION_GRANTED)
    @Synchronized
    fun reconcile(context: Context) {
        val store = (context.applicationContext as HealthVaultApp).secureStore
        if (store.weatherConsent != null && !allowed(context)) {
            store.setWeatherEnabled(false, store.currentSessionGeneration)
        }
        if (store.weatherConsent != null && store.hasSession() && allowed(context)) {
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(NAME, ExistingPeriodicWorkPolicy.KEEP,
                PeriodicWorkRequestBuilder<WeatherWorker>(1, TimeUnit.HOURS).build())
        } else {
            WorkManager.getInstance(context).cancelUniqueWork(NAME)
        }
    }
}

class WeatherWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val app = applicationContext as HealthVaultApp
        val store = app.secureStore
        val generation = store.currentSessionGeneration
        val consent = store.weatherConsent ?: return@withContext Result.success()
        fun active() = store.weatherActive(consent, generation) && WeatherScheduler.allowed(applicationContext)
        if (!active()) {
            if (!WeatherScheduler.allowed(applicationContext)) WeatherScheduler.reconcile(applicationContext)
            return@withContext Result.success()
        }
        val fix = freshLocation()
        if (fix != null && active()) store.enqueueWeather(fix, consent, generation)
        for (observation in store.weatherQueue(consent, generation).take(8)) {
            if (!active()) break
            if (System.currentTimeMillis() - Instant.parse(observation.observedAt).toEpochMilli() > WeatherObservation.QUEUE_MAX_AGE_MILLIS) {
                store.removeWeather(observation.id, consent, generation)
                continue
            }
            when (val result = app.api.uploadWeather(observation, consent, generation) { WeatherScheduler.allowed(applicationContext) }) {
                is ApiResult.Success -> store.removeWeather(observation.id, consent, generation)
                is ApiResult.Unauthenticated -> {
                    if (store.clearSession(generation)) app.cookieJar.clearInMemory()
                    WeatherScheduler.reconcile(applicationContext)
                    return@withContext Result.success()
                }
                is ApiResult.ServerError -> {
                    // Permanent invalid/conflicting observations must not block newer evidence.
                    if (result.code == 400 || result.code == 409 || result.code == 422) {
                        store.removeWeather(observation.id, consent, generation)
                    } else break
                }
                else -> break // Preserve identity/content for the next hourly offline-safe run.
            }
        }
        if (!WeatherScheduler.allowed(applicationContext)) WeatherScheduler.reconcile(applicationContext)
        Result.success()
    }

    @SuppressLint("MissingPermission")
    private suspend fun freshLocation(): WeatherObservation? {
        val manager = applicationContext.getSystemService(Context.LOCATION_SERVICE) as LocationManager
        fun observation(location: Location): WeatherObservation? = if (!location.hasAccuracy()) null else
            WeatherObservation.fromFix(location.latitude, location.longitude, location.accuracy.toDouble(),
                location.time, location.elapsedRealtimeNanos, System.currentTimeMillis(), SystemClock.elapsedRealtimeNanos())
        // Network location works with coarse-only permission; never request GPS/fine location.
        return try {
            if (!manager.isProviderEnabled(LocationManager.NETWORK_PROVIDER)) return null
            manager.getLastKnownLocation(LocationManager.NETWORK_PROVIDER)?.let { observation(it) }?.let { return it }
            withContext(Dispatchers.Main) {
                withTimeoutOrNull(30_000) {
                    suspendCancellableCoroutine { continuation ->
                        val listener = object : LocationListener {
                            override fun onLocationChanged(location: Location) {
                                val rounded = observation(location) ?: return
                                if (continuation.isActive) {
                                    runCatching { manager.removeUpdates(this) }
                                    continuation.resume(rounded)
                                }
                            }
                            override fun onProviderEnabled(provider: String) = Unit
                            override fun onProviderDisabled(provider: String) = Unit
                            @Suppress("OVERRIDE_DEPRECATION")
                            override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) = Unit
                        }
                        continuation.invokeOnCancellation { runCatching { manager.removeUpdates(listener) } }
                        try {
                            manager.requestLocationUpdates(LocationManager.NETWORK_PROVIDER, 0L, 0f, listener, Looper.getMainLooper())
                        } catch (_: SecurityException) {
                            if (continuation.isActive) continuation.resume(null)
                        } catch (_: IllegalArgumentException) {
                            if (continuation.isActive) continuation.resume(null)
                        }
                    }
                }
            }
        } catch (_: SecurityException) { null } catch (_: IllegalArgumentException) { null }
    }
}
