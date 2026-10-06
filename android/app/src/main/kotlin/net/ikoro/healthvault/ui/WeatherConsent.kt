package net.ikoro.healthvault.ui

import android.Manifest
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.Button
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.R
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.weather.WeatherScheduler

@Composable
fun WeatherConsent(store: SecureStore) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val generation = remember { store.currentSessionGeneration }
    var enabled by remember { mutableStateOf(store.weatherConsent != null) }
    var foreground by remember { mutableStateOf(WeatherScheduler.foregroundAllowed(context)) }
    var denied by remember { mutableStateOf(false) }

    fun enableIfAllowed() {
        if (!WeatherScheduler.allowed(context)) { denied = true; return }
        scope.launch {
            val saved = withContext(Dispatchers.IO) {
                val saved = store.setWeatherEnabled(true, generation)
                WeatherScheduler.reconcile(context)
                saved
            }
            if (saved) { enabled = true; denied = false }
        }
    }

    val backgroundRequest = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        if (it) enableIfAllowed() else denied = true
    }
    val settingsRequest = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        if (WeatherScheduler.allowed(context)) enableIfAllowed() else denied = true
    }
    val foregroundRequest = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        foreground = it
        denied = !it
        if (it && Build.VERSION.SDK_INT < 29) enableIfAllowed()
        // On API 29+, a separate user action requests background access.
    }
    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                foreground = WeatherScheduler.foregroundAllowed(context)
                scope.launch {
                    withContext(Dispatchers.IO) { WeatherScheduler.reconcile(context) }
                    enabled = store.weatherConsent != null
                }
            }
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }

    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(stringResource(R.string.weather_title))
        Text(stringResource(R.string.weather_explanation))
        Text(stringResource(if (enabled) R.string.weather_enabled else R.string.weather_disabled))
        if (denied) Text(stringResource(R.string.weather_permission_missing))
        if (enabled) {
            Button(onClick = {
                scope.launch {
                    withContext(Dispatchers.IO) {
                        store.setWeatherEnabled(false, generation)
                        WeatherScheduler.reconcile(context)
                    }
                    enabled = false
                }
            }) { Text(stringResource(R.string.weather_disable)) }
        } else if (!foreground) {
            Button(onClick = { foregroundRequest.launch(Manifest.permission.ACCESS_COARSE_LOCATION) }) {
                Text(stringResource(R.string.weather_allow_location))
            }
            if (denied) {
                Button(onClick = {
                    settingsRequest.launch(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                        Uri.parse("package:" + context.packageName)))
                }) { Text(stringResource(R.string.weather_open_settings)) }
            }
        } else {
            Text(stringResource(R.string.weather_background_explanation))
            Button(onClick = {
                when {
                    WeatherScheduler.allowed(context) -> enableIfAllowed()
                    Build.VERSION.SDK_INT == 29 -> backgroundRequest.launch(Manifest.permission.ACCESS_BACKGROUND_LOCATION)
                    else -> settingsRequest.launch(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                        Uri.parse("package:" + context.packageName)))
                }
            }) { Text(stringResource(R.string.weather_enable_background)) }
        }
    }
}
