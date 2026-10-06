package net.ikoro.healthvault.weather

import java.time.Instant
import net.ikoro.healthvault.store.FakeSharedPreferences
import net.ikoro.healthvault.store.SecureStore
import org.junit.Assert.*
import org.junit.Test

class WeatherTest {
    private val now = 1_780_000_000_000L
    private fun fix(lat: Double = 50.123456, lon: Double = 14.456789, age: Long = 0,
                    monotonicAge: Long = age): WeatherObservation? = WeatherObservation.fromFix(
        lat, lon, 150.0, now - age, 10_000_000_000_000L - monotonicAge * 1_000_000,
        now, 10_000_000_000_000L)

    @Test fun `fix rounds before persistence and rejects wall or monotonic staleness`() {
        assertEquals(50.1, fix()!!.latitude, 0.0)
        assertEquals(14.5, fix()!!.longitude, 0.0)
        assertTrue(fix()!!.accuracyM > 150.0)
        assertEquals(150.0, fix(lat = 50.1, lon = 14.5)!!.accuracyM, 0.001)
        assertTrue(fix(lat = 0.049, lon = 0.049)!!.accuracyM > 7_700.0)
        assertNull(fix(age = WeatherObservation.MAX_AGE_MILLIS + 1))
        assertNull(fix(monotonicAge = WeatherObservation.MAX_AGE_MILLIS + 1))
        assertNull(fix(lat = Double.NaN))
        assertNull(fix(lon = 181.0))
        assertNull(fix(age = -1))
    }

    @Test fun `queue survives restart and stops old work after disable and reenable`() {
        val prefs = FakeSharedPreferences()
        val store = SecureStore(prefs)
        store.saveSession("https://example.test", "alice", "password")
        val generation = store.currentSessionGeneration
        store.setWeatherEnabled(true, generation)
        val consent = store.weatherConsent!!
        val observation = fix()!!
        assertTrue(store.enqueueWeather(observation, consent, generation, now))
        val restarted = SecureStore(prefs)
        assertEquals(listOf(observation), restarted.weatherQueue(consent, restarted.currentSessionGeneration))
        store.setWeatherEnabled(false, generation)
        store.setWeatherEnabled(true, generation)
        assertFalse(store.enqueueWeather(observation, consent, generation, now))
        assertFalse(store.removeWeather(observation.id, consent, generation))
        assertTrue(store.weatherQueue(store.weatherConsent!!, generation).isEmpty())
    }

    @Test fun `logout and replacement clear consent and queue`() {
        val store = SecureStore(FakeSharedPreferences())
        store.saveSession("https://example.test", "alice", "password")
        val gen = store.currentSessionGeneration
        store.setWeatherEnabled(true, gen)
        val token = store.weatherConsent!!
        store.enqueueWeather(fix()!!, token, gen, now)
        store.saveSession("https://other.test", "bob", "password")
        assertNull(store.weatherConsent)
        assertFalse(store.weatherActive(token, gen))
        assertTrue(store.weatherQueue(token, gen).isEmpty())
        store.setWeatherEnabled(true, store.currentSessionGeneration)
        store.clearSession()
        assertNull(store.weatherConsent)
    }

    @Test fun `queue is bounded and drops expired observations`() {
        val store = SecureStore(FakeSharedPreferences())
        store.saveSession("https://example.test", "alice", "password")
        val gen = store.currentSessionGeneration
        store.setWeatherEnabled(true, gen)
        val token = store.weatherConsent!!
        repeat(WeatherObservation.MAX_QUEUE_SIZE + 3) { store.enqueueWeather(fix()!!, token, gen, now) }
        assertEquals(WeatherObservation.MAX_QUEUE_SIZE, store.weatherQueue(token, gen).size)
        val old = fix()!!.copy(observedAt = Instant.ofEpochMilli(now - WeatherObservation.QUEUE_MAX_AGE_MILLIS - 1).toString())
        store.enqueueWeather(old, token, gen, now)
        assertFalse(store.weatherQueue(token, gen).any { it.id == old.id })
        val head = store.weatherQueue(token, gen).first()
        store.removeWeather(head.id, token, gen)
        assertFalse(store.weatherQueue(token, gen).any { it.id == head.id })
    }
}
