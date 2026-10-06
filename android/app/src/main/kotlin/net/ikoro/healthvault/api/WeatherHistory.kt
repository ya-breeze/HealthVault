package net.ikoro.healthvault.api

import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.temporal.ChronoUnit
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import okhttp3.HttpUrl.Companion.toHttpUrl

@Serializable
data class SavedWeatherHour(
    val hour: String,
    @SerialName("temperature_c") val temperatureC: Double,
    @SerialName("apparent_temperature_c") val apparentTemperatureC: Double,
    @SerialName("relative_humidity_percent") val relativeHumidityPercent: Double,
    @SerialName("surface_pressure_hpa") val surfacePressureHpa: Double,
    @SerialName("mean_sea_level_pressure_hpa") val meanSeaLevelPressureHpa: Double,
    @SerialName("precipitation_mm") val precipitationMm: Double,
    @SerialName("wind_speed_kmh") val windSpeedKmh: Double,
    val source: String,
    val model: String,
    @SerialName("fetched_at") val fetchedAt: String,
)

@Serializable
data class WeatherHistoryGap(val from: String, val to: String, val reason: String)

@Serializable
data class WeatherHistory(
    val hours: List<SavedWeatherHour>,
    val gaps: List<WeatherHistoryGap>,
    val units: Map<String, String>,
) {
    fun requireValid(): WeatherHistory {
        val expected = mapOf("temperature_c" to "°C", "apparent_temperature_c" to "°C",
            "relative_humidity_percent" to "%", "surface_pressure_hpa" to "hPa",
            "mean_sea_level_pressure_hpa" to "hPa", "precipitation_mm" to "mm", "wind_speed_kmh" to "km/h")
        require(expected.all { units[it.key] == it.value })
        for (h in hours) {
            val instant = Instant.parse(h.hour)
            require(instant == instant.truncatedTo(ChronoUnit.HOURS))
            require(listOf(h.temperatureC, h.apparentTemperatureC, h.relativeHumidityPercent,
                h.surfacePressureHpa, h.meanSeaLevelPressureHpa, h.precipitationMm, h.windSpeedKmh).all { it.isFinite() })
            require(h.source.isNotBlank() && h.model.isNotBlank())
            Instant.parse(h.fetchedAt)
        }
        for (g in gaps) require(Instant.parse(g.from).isBefore(Instant.parse(g.to)))
        return this
    }
}

/** A complete saved hour in the query window; never a claim about conditions now. */
fun latestSavedWeather(history: WeatherHistory, now: Instant): SavedWeatherHour? {
    val end = now.truncatedTo(ChronoUnit.HOURS)
    val start = end.minus(7, ChronoUnit.DAYS)
    return history.hours.mapNotNull { h -> runCatching { h to Instant.parse(h.hour) }.getOrNull() }
        .filter { (_, time) -> !time.isBefore(start) && !time.plus(1, ChronoUnit.HOURS).isAfter(end) }
        .maxByOrNull { it.second }?.first
}

fun latestWeatherGapReason(history: WeatherHistory): String? = history.gaps.mapNotNull { gap ->
    runCatching { gap.reason to Instant.parse(gap.to) }.getOrNull()
}.maxByOrNull { it.second }?.first

fun weatherHourLabel(hour: SavedWeatherHour, zone: ZoneId): String =
    Instant.parse(hour.hour).atZone(zone).format(DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm XXX")) + " · " + zone.id

fun weatherDayUrl(serverUrl: String, hour: SavedWeatherHour, zone: ZoneId): String =
    weatherDayUrl(serverUrl, Instant.parse(hour.hour).atZone(zone).toLocalDate(), zone)

fun weatherDayUrl(serverUrl: String, day: LocalDate, zone: ZoneId): String =
    (serverUrl.trimEnd('/') + "/weather/day/").toHttpUrl().newBuilder()
        .addQueryParameter("date", day.toString())
        .addQueryParameter("timezone", zone.id).build().toString()
