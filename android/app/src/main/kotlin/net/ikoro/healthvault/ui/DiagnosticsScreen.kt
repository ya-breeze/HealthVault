package net.ikoro.healthvault.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.diagnostics.DiagnosticJournal
import net.ikoro.healthvault.store.SecureStore

@Composable
fun DiagnosticsScreen(api: HealthVaultApi, store: SecureStore, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    var journal by remember { mutableStateOf(DiagnosticJournal()) }
    var sending by remember { mutableStateOf(false) }
    var sent by remember { mutableStateOf<Boolean?>(null) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) { journal = withContext(Dispatchers.IO) { store.diagnosticJournal() } }
    Surface(modifier = Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize().safeDrawingPadding().verticalScroll(rememberScrollState()).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.diagnostics_title), style = MaterialTheme.typography.headlineSmall)
            TextButton(onClick = onBack) { Text(stringResource(R.string.settings_back)) }
            Text(stringResource(R.string.diagnostics_privacy))
            Text(stringResource(R.string.diagnostics_last_success, journal.lastSummarySuccess ?: "—"))
            val last = journal.events.lastOrNull { it.operation == "summary" }
            Text(stringResource(R.string.diagnostics_current,
                last?.let { diagnosticCategory(it.category) } ?: stringResource(R.string.diagnostics_unknown)))
            Text(stringResource(R.string.diagnostics_pending, journal.pendingIds.size))
            Button(enabled = !sending && journal.pendingIds.isNotEmpty(), onClick = {
                sending = true
                scope.launch {
                    sent = withContext(Dispatchers.IO) { runCatching { api.sendDiagnostics() is ApiResult.Success }.getOrDefault(false) }
                    journal = withContext(Dispatchers.IO) { store.diagnosticJournal() }
                    sending = false
                }
            }) { Text(stringResource(R.string.diagnostics_send)) }
            sent?.let { Text(stringResource(if (it) R.string.diagnostics_sent else R.string.diagnostics_failed)) }
            HorizontalDivider()
            Text(stringResource(R.string.diagnostics_history), style = MaterialTheme.typography.titleMedium)
            if (journal.events.isEmpty()) Text(stringResource(R.string.diagnostics_empty))
            journal.events.asReversed().forEach { event ->
                Text("${event.occurredAt} · ${diagnosticOperation(event.operation)}", style = MaterialTheme.typography.labelLarge)
                Text("${diagnosticCategory(event.category)} · HTTP ${event.httpCode.takeIf { it > 0 } ?: "—"} · ${event.durationMillis} ms")
                Text(stringResource(R.string.diagnostics_attempt, event.attempt, event.appVersion, event.androidApi))
                Text("ID: ${event.requestId}", style = MaterialTheme.typography.bodySmall)
                if (event.workId.isNotEmpty()) Text(stringResource(R.string.diagnostics_work_id, event.workId), style = MaterialTheme.typography.bodySmall)
                if (event.trigger.isNotEmpty()) Text(stringResource(R.string.diagnostics_trigger, diagnosticTrigger(event.trigger)), style = MaterialTheme.typography.bodySmall)
                if (event.provider.isNotEmpty()) Text(stringResource(R.string.diagnostics_provider, diagnosticProvider(event.provider)), style = MaterialTheme.typography.bodySmall)
                event.snapshotAgeMillis?.let { Text(stringResource(R.string.diagnostics_snapshot_age, it / 1000), style = MaterialTheme.typography.bodySmall) }
                event.snapshotChanged?.let { Text(stringResource(if (it) R.string.diagnostics_snapshot_changed else R.string.diagnostics_snapshot_unchanged), style = MaterialTheme.typography.bodySmall) }
                HorizontalDivider()
            }
        }
    }
}

@Composable
private fun diagnosticOperation(operation: String): String = stringResource(when (operation) {
    "widget_browser" -> R.string.diagnostics_widget_browser
    "widget_work" -> R.string.diagnostics_widget_work
    "widget_fetch" -> R.string.diagnostics_widget_fetch
    "widget_snapshot" -> R.string.diagnostics_widget_snapshot
    "widget_redraw" -> R.string.diagnostics_widget_redraw
    "summary" -> R.string.diagnostics_summary
    "weather" -> R.string.diagnostics_weather
    "auth_login" -> R.string.diagnostics_login
    else -> R.string.diagnostics_auth
})

@Composable
private fun diagnosticCategory(category: String): String = stringResource(when (category) {
    "launched" -> R.string.diagnostics_launched
    "returned" -> R.string.diagnostics_returned
    "replacement_requested" -> R.string.diagnostics_replacement_requested
    "enqueue_requested" -> R.string.diagnostics_enqueue_requested
    "started" -> R.string.diagnostics_started
    "deferred" -> R.string.diagnostics_deferred
    "cancelled" -> R.string.diagnostics_cancelled
    "failed" -> R.string.diagnostics_stage_failed
    "rejected" -> R.string.diagnostics_rejected
    "success" -> R.string.diagnostics_ok
    "recovered" -> R.string.diagnostics_recovered
    "unauthenticated" -> R.string.diagnostics_unauthenticated
    "rate_limited" -> R.string.diagnostics_rate_limited
    "access_challenge" -> R.string.diagnostics_access
    "dns" -> R.string.diagnostics_dns
    "timeout" -> R.string.diagnostics_timeout
    "tls" -> R.string.diagnostics_tls
    "network" -> R.string.diagnostics_network
    "server" -> R.string.diagnostics_server
    else -> R.string.diagnostics_invalid
})

@Composable
private fun diagnosticTrigger(trigger: String): String = stringResource(when (trigger) {
    "food_entry" -> R.string.diagnostics_trigger_food
    "manual" -> R.string.diagnostics_trigger_manual
    "app_resume" -> R.string.diagnostics_trigger_resume
    "placement" -> R.string.diagnostics_trigger_placement
    "periodic" -> R.string.diagnostics_trigger_periodic
    "retry" -> R.string.diagnostics_trigger_retry
    else -> R.string.diagnostics_unknown
})

@Composable
private fun diagnosticProvider(provider: String): String = stringResource(when (provider) {
    "home" -> R.string.diagnostics_provider_home
    "flex_window" -> R.string.diagnostics_provider_flex
    else -> R.string.diagnostics_unknown
})
