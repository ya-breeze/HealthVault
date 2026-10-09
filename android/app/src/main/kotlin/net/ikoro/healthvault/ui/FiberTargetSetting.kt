package net.ikoro.healthvault.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.store.SummarySnapshot
import net.ikoro.healthvault.widget.WidgetUpdater
import androidx.compose.ui.platform.LocalContext

@Composable
fun FiberTargetSetting(api: HealthVaultApi, store: SecureStore) {
    var grams by remember { mutableStateOf(store.loadSnapshot()?.summary?.target?.dietaryFiberGrams?.toString() ?: "") }
    var busy by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<Int?>(null) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    fun save(value: Int?) {
        busy = true
        scope.launch {
            message = withContext(Dispatchers.IO) {
                val generation = store.currentSessionGeneration
                val result = api.setFiberTarget(value)
                if (result is ApiResult.Success) {
                    val summary = api.summaryToday()
                    if (summary is ApiResult.Success && store.saveSnapshot(SummarySnapshot(summary.value, System.currentTimeMillis()), generation)) {
                        withContext(Dispatchers.Main) { grams = summary.value.target.dietaryFiberGrams?.toString() ?: "" }
                        WidgetUpdater.updateAll(context.applicationContext)
                        R.string.fiber_saved
                    } else R.string.fiber_saved_refresh_needed
                } else R.string.fiber_save_failed
            }
            busy = false
        }
    }
    Column {
        Text(stringResource(R.string.fiber_setting_title), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.fiber_setting_help))
        OutlinedTextField(value = grams, onValueChange = { grams = it }, enabled = !busy,
            label = { Text(stringResource(R.string.fiber_setting_grams)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), singleLine = true)
        TextButton(enabled = !busy && grams.toIntOrNull() in 1..200, onClick = { save(grams.toInt()) }) {
            Text(stringResource(R.string.fiber_setting_save))
        }
        TextButton(enabled = !busy, onClick = { save(null) }) { Text(stringResource(R.string.fiber_setting_reset)) }
        message?.let { Text(stringResource(it)) }
    }
}
