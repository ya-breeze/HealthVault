package net.ikoro.healthvault.api

import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.*
import org.junit.Test

internal val HISTORY_JSON = """{
 "hours":[{"hour":"2026-10-06T10:00:00Z","temperature_c":0,"apparent_temperature_c":-1,
 "relative_humidity_percent":70,"surface_pressure_hpa":1000,"mean_sea_level_pressure_hpa":1010,
 "precipitation_mm":0,"wind_speed_kmh":5,"source":"open-meteo historical-forecast",
 "model":"ecmwf_ifs025","fetched_at":"2026-10-06T11:00:00Z"}],
 "gaps":[],"units":{"temperature_c":"°C","apparent_temperature_c":"°C","relative_humidity_percent":"%",
 "surface_pressure_hpa":"hPa","mean_sea_level_pressure_hpa":"hPa","precipitation_mm":"mm","wind_speed_kmh":"km/h"}}
"""

class WeatherHistoryTest {
    private fun history() = Json.decodeFromString<WeatherHistory>(HISTORY_JSON)
    private val now = Instant.parse("2026-10-06T11:45:00Z")

    @Test fun `latest selection uses timestamp not array order and excludes unfinished or old hours`() {
        val h = history().hours.single()
        val input = history().copy(hours = listOf(h.copy(hour = "2026-10-06T11:00:00Z"), h,
            h.copy(hour = "2026-10-06T09:00:00Z"), h.copy(hour = "2026-09-29T10:00:00Z")))
        assertEquals(h, latestSavedWeather(input, now))
        assertNull(latestSavedWeather(input.copy(hours = listOf(h.copy(hour = "bad"))), now))
    }

    @Test fun `exact seven day lower boundary is included`() {
        val boundary = history().hours.single().copy(hour = "2026-09-29T11:00:00Z")
        assertEquals(boundary, latestSavedWeather(history().copy(hours = listOf(boundary)), now))
    }

    @Test fun `empty coverage keeps explicit latest gap reason`() {
        val h = history().copy(hours = emptyList(), gaps = listOf(
            WeatherHistoryGap("2026-10-06T09:00:00Z", "2026-10-06T10:00:00Z", "moving"),
            WeatherHistoryGap("2026-10-06T10:00:00Z", "2026-10-06T11:00:00Z", "pending_weather")))
        assertNull(latestSavedWeather(h, now))
        assertEquals("pending_weather", latestWeatherGapReason(h))
        assertNull(latestWeatherGapReason(h.copy(gaps = emptyList())))
    }

    @Test fun `zero values remain valid but incorrect units and missing fields fail`() {
        assertEquals(0.0, history().requireValid().hours.single().temperatureC, 0.0)
        assertTrue(runCatching { history().copy(units = history().units + ("surface_pressure_hpa" to "Pa")).requireValid() }.isFailure)
        assertTrue(runCatching { Json.decodeFromString<WeatherHistory>(HISTORY_JSON.replace("\"temperature_c\":0,", "")) }.isFailure)
        assertTrue(runCatching { history().copy(hours = listOf(history().hours.single().copy(temperatureC = Double.NaN))).requireValid() }.isFailure)
    }

    @Test fun `phone zone controls day link across UTC midnight`() {
        val h = history().hours.single().copy(hour = "2026-10-06T00:00:00Z")
        val zone = ZoneId.of("America/Los_Angeles")
        val url = weatherDayUrl("https://example.test/", h, zone).toHttpUrl()
        assertEquals("/weather/day/", url.encodedPath)
        assertEquals("2026-10-05", url.queryParameter("date"))
        assertEquals(zone.id, url.queryParameter("timezone"))
        assertTrue(weatherHourLabel(h, zone).contains("2026-10-05 17:00 -07:00"))
        assertTrue(weatherHourLabel(h, zone).endsWith(zone.id))
        val emptyDay = weatherDayUrl("https://example.test", LocalDate.parse("2026-10-06"), zone).toHttpUrl()
        assertEquals("2026-10-06", emptyDay.queryParameter("date"))
        assertEquals(zone.id, emptyDay.queryParameter("timezone"))
    }

    @Test fun `DST repeated local hour labels distinguish offsets`() {
        val h = history().hours.single()
        val zone = ZoneId.of("Europe/Prague")
        val first = weatherHourLabel(h.copy(hour = "2026-10-25T00:00:00Z"), zone)
        val second = weatherHourLabel(h.copy(hour = "2026-10-25T01:00:00Z"), zone)
        assertTrue(first.contains("02:00 +02:00"))
        assertTrue(second.contains("02:00 +01:00"))
        assertNotEquals(first, second)
    }
}
