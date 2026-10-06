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
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.TextButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.NonCancellable
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
    var expanded by rememberSaveable { mutableStateOf(false) }

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
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(stringResource(R.string.weather_title), style = MaterialTheme.typography.titleMedium)
                Text(
                    stringResource(if (enabled) R.string.weather_enabled else R.string.weather_disabled),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (enabled) {
                val switchLabel = stringResource(R.string.weather_title)
                Switch(modifier = Modifier.semantics { contentDescription = switchLabel }, checked = true, onCheckedChange = {
                    // Enter the durable operation before Back/rotation can dispose this screen.
                    scope.launch(start = CoroutineStart.UNDISPATCHED) {
                        val saved = withContext(NonCancellable + Dispatchers.IO) {
                            val saved = store.setWeatherEnabled(false, generation)
                            WeatherScheduler.reconcile(context)
                            saved
                        }
                        if (saved) enabled = false
                    }
                })
            }
        }
        TextButton(onClick = { expanded = !expanded }) {
            Text(stringResource(if (expanded) R.string.weather_less else R.string.weather_more))
        }
        if (expanded) {
            Text(stringResource(R.string.weather_explanation), style = MaterialTheme.typography.bodyMedium)
            Text(stringResource(R.string.weather_disable_details), style = MaterialTheme.typography.bodyMedium)
        }
        if (!enabled) Text(stringResource(R.string.weather_consent_summary))
        if (denied) Text(stringResource(R.string.weather_permission_missing), color = MaterialTheme.colorScheme.error)
        if (!enabled && !foreground) {
            Button(onClick = { foregroundRequest.launch(Manifest.permission.ACCESS_COARSE_LOCATION) }) {
                Text(stringResource(R.string.weather_allow_location))
            }
            if (denied) {
                Button(onClick = {
                    settingsRequest.launch(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                        Uri.parse("package:" + context.packageName)))
                }) { Text(stringResource(R.string.weather_open_settings)) }
            }
        } else if (!enabled) {
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
