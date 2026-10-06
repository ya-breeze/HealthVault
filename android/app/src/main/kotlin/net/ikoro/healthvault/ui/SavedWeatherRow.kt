package net.ikoro.healthvault.ui

import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.api.WeatherHistory
import net.ikoro.healthvault.api.latestSavedWeather
import net.ikoro.healthvault.api.latestWeatherGapReason
import net.ikoro.healthvault.api.weatherDayUrl
import net.ikoro.healthvault.api.weatherHourLabel
import net.ikoro.healthvault.store.SecureStore

@Composable
fun SavedWeatherRow(api: HealthVaultApi, store: SecureStore, refreshToken: Int) {
    var result by remember { mutableStateOf<ApiResult<WeatherHistory>?>(null) }
    var loading by remember { mutableStateOf(true) }
    var queriedAt by remember { mutableStateOf(Instant.now()) }
    val context = LocalContext.current
    LaunchedEffect(refreshToken) {
        loading = true
        val generation = store.currentSessionGeneration
        val now = Instant.now()
        val fresh = withContext(Dispatchers.IO) { api.weatherHistory(now, generation) }
        if (!store.isCurrentSession(generation)) return@LaunchedEffect
        queriedAt = now
        result = fresh
        loading = false
    }
    val response = result
    val history = (response as? ApiResult.Success<WeatherHistory>)?.value
    val latest = history?.let { latestSavedWeather(it, queriedAt) }
    val zone = ZoneId.systemDefault()
    Column {
        Text(stringResource(R.string.saved_weather_title), style = MaterialTheme.typography.titleSmall)
        when {
            loading -> Text(stringResource(R.string.saved_weather_loading))
            latest != null -> {
                val base = store.serverUrl
                TextButton(modifier = Modifier.fillMaxWidth(), onClick = {
                    if (base != null) CustomTabsIntent.Builder().build()
                        .launchUrl(context, Uri.parse(weatherDayUrl(base, latest, zone)))
                }) {
                    Column {
                        Text(stringResource(R.string.saved_weather_values, latest.temperatureC, latest.surfacePressureHpa))
                        Text(weatherHourLabel(latest, zone), style = MaterialTheme.typography.bodySmall)
                        Text(stringResource(R.string.saved_weather_day), style = MaterialTheme.typography.labelSmall)
                    }
                }
            }
            history != null -> Column {
                Text(stringResource(when (latestWeatherGapReason(history)) {
                    "pending_weather" -> R.string.saved_weather_pending
                    "moving" -> R.string.saved_weather_moving
                    "poor_accuracy" -> R.string.saved_weather_accuracy
                    "long_gap", "missing_location", "incomplete_hour" -> R.string.saved_weather_missing_location
                    else -> R.string.saved_weather_empty
                }))
                TextButton(onClick = {
                    store.serverUrl?.let { base -> CustomTabsIntent.Builder().build()
                        .launchUrl(context, Uri.parse(weatherDayUrl(base, LocalDate.now(zone), zone))) }
                }) { Text(stringResource(R.string.saved_weather_today)) }
            }
            response is ApiResult.RateLimited -> Text(stringResource(R.string.saved_weather_rate_limited))
            response is ApiResult.AccessChallenge -> Text(stringResource(R.string.saved_weather_access))
            response is ApiResult.Unauthenticated -> Text(stringResource(R.string.saved_weather_sign_in))
            else -> Text(stringResource(R.string.saved_weather_error))
        }
    }
}
