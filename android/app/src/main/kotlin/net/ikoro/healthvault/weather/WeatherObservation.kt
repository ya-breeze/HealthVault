package net.ikoro.healthvault.weather

import java.time.Instant
import java.util.UUID
import kotlin.math.round
import kotlin.math.sin
import kotlin.math.cos
import kotlin.math.sqrt
import kotlin.math.asin
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class WeatherObservation(
    val id: String,
    @SerialName("observed_at") val observedAt: String,
    val latitude: Double,
    val longitude: Double,
    @SerialName("accuracy_m") val accuracyM: Double,
) {
    companion object {
        const val MAX_AGE_MILLIS = 15 * 60 * 1000L
        const val QUEUE_MAX_AGE_MILLIS = 14 * 24 * 60 * 60 * 1000L
        const val MAX_QUEUE_SIZE = 336

        fun fromFix(latitude: Double, longitude: Double, accuracyM: Double, timeMillis: Long,
                    elapsedNanos: Long, nowMillis: Long, nowElapsedNanos: Long): WeatherObservation? {
            if (!latitude.isFinite() || latitude !in -90.0..90.0 ||
                !longitude.isFinite() || longitude !in -180.0..180.0 ||
                !accuracyM.isFinite() || accuracyM < 0 || elapsedNanos <= 0 || elapsedNanos > nowElapsedNanos) return null
            val wallAge = nowMillis - timeMillis
            val monotonicAge = (nowElapsedNanos - elapsedNanos) / 1_000_000
            if (wallAge !in 0..MAX_AGE_MILLIS || monotonicAge > MAX_AGE_MILLIS) return null
            val roundedLatitude = round(latitude * 10) / 10
            val roundedLongitude = round(longitude * 10) / 10
            val latitudeDelta = Math.toRadians(roundedLatitude - latitude)
            val longitudeDelta = Math.toRadians(roundedLongitude - longitude)
            val haversine = sin(latitudeDelta / 2) * sin(latitudeDelta / 2) +
                cos(Math.toRadians(latitude)) * cos(Math.toRadians(roundedLatitude)) *
                sin(longitudeDelta / 2) * sin(longitudeDelta / 2)
            val roundingDistance = 2 * 6_371_000 * asin(sqrt(haversine.coerceIn(0.0, 1.0)))
            return WeatherObservation(UUID.randomUUID().toString(), Instant.ofEpochMilli(timeMillis).toString(),
                roundedLatitude, roundedLongitude, accuracyM + roundingDistance)
        }
    }
}
