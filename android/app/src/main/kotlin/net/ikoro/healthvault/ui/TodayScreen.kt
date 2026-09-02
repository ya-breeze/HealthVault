package net.ikoro.healthvault.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.PullToRefreshBox
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.browser.customtabs.CustomTabsIntent
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.store.SummarySnapshot

/** Matches the four reason codes computeUserNutritionTarget can send (see TodaySummaryTarget). */
@Composable
private fun unmetReasonMessage(reason: String?): String = when (reason) {
    "missing_profile" -> stringResource(R.string.target_reason_missing_profile)
    "missing_measurements" -> stringResource(R.string.target_reason_missing_measurements)
    "missing_goal_weight" -> stringResource(R.string.target_reason_missing_goal_weight)
    "insufficient_activity_data" -> stringResource(R.string.target_reason_insufficient_activity_data)
    else -> stringResource(R.string.target_reason_unknown)
}

/** A coarse "N min/hours/days ago" string; no dependency on a date library. */
@Composable
private fun relativeTimeText(iso8601: String?, nowMillis: Long): String {
    if (iso8601 == null) return stringResource(R.string.today_no_meals_logged)
    val loggedAtMillis = runCatching { java.time.Instant.parse(iso8601).toEpochMilli() }.getOrNull()
        ?: return stringResource(R.string.today_no_meals_logged)
    val minutes = (nowMillis - loggedAtMillis) / 60_000
    return when {
        minutes < 1 -> stringResource(R.string.today_last_logged_just_now)
        minutes < 60 -> stringResource(R.string.today_last_logged_minutes_ago, minutes)
        minutes < 60 * 24 -> stringResource(R.string.today_last_logged_hours_ago, minutes / 60)
        else -> stringResource(R.string.today_last_logged_days_ago, minutes / (60 * 24))
    }
}

/** 6 hours, matching the widget's own staleness threshold (widget/WidgetState.kt). */
private const val STALE_AFTER_MILLIS = 6L * 60 * 60 * 1000

@Composable
fun TodayScreen(
    api: HealthVaultApi,
    secureStore: SecureStore,
    onSignedOut: () -> Unit,
) {
    var snapshot by remember { mutableStateOf(secureStore.loadSnapshot()) }
    var refreshing by remember { mutableStateOf(false) }
    var refreshFailed by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    suspend fun refresh() {
        refreshing = true
        val result = withContext(Dispatchers.IO) { api.summaryToday() }
        refreshing = false
        when (result) {
            is ApiResult.Success -> {
                refreshFailed = false
                val fresh = SummarySnapshot(result.value, System.currentTimeMillis())
                secureStore.saveSnapshot(fresh)
                snapshot = fresh
                applyDisplayLanguage(result.value.displayLanguage)
            }
            is ApiResult.Unauthenticated -> onSignedOut()
            else -> refreshFailed = true
        }
    }

    LaunchedEffect(Unit) { refresh() }

    val now = System.currentTimeMillis()
    val current = snapshot
    val isStale = current != null && now - current.fetchedAtMillis > STALE_AFTER_MILLIS

    Surface(modifier = Modifier.fillMaxSize()) {
        PullToRefreshBox(
            isRefreshing = refreshing,
            onRefresh = { scope.launch { refresh() } },
            modifier = Modifier.fillMaxSize(),
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Text(text = stringResource(R.string.app_name), style = MaterialTheme.typography.headlineSmall)
                    TextButton(onClick = onSignedOut) { Text(stringResource(R.string.today_sign_out)) }
                }

                if (current == null) {
                    Text(text = stringResource(R.string.today_loading))
                } else {
                    val summary = current.summary
                    if (isStale || refreshFailed) {
                        Text(
                            text = stringResource(R.string.today_stale_snapshot),
                            color = MaterialTheme.colorScheme.error,
                        )
                    }

                    TodayContent(summary)
                    Text(
                        text = stringResource(R.string.today_meal_count, summary.mealCount) + " · " +
                            relativeTimeText(summary.lastLoggedAt, now),
                    )

                    Button(onClick = { openLogFood(context, secureStore.serverUrl) }) {
                        Text(stringResource(R.string.today_log_food))
                    }
                }
            }
        }
    }
}

@Composable
private fun TodayContent(summary: TodaySummary) {
    val target = summary.target
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (target.available) {
            Text(
                text = stringResource(
                    R.string.today_calories_of_target,
                    summary.caloriesConsumed.toInt(),
                    target.calories,
                ),
                style = MaterialTheme.typography.headlineMedium,
            )
            MacroBar(stringResource(R.string.today_macro_protein), summary.proteinGramsConsumed, target.proteinGrams)
            MacroBar(stringResource(R.string.today_macro_carbs), summary.carbsGramsConsumed, target.carbsGrams)
            MacroBar(stringResource(R.string.today_macro_fat), summary.fatGramsConsumed, target.fatGrams)
        } else {
            // No fabricated denominator: show what was actually consumed,
            // and the specific reason a target isn't available yet.
            Text(
                text = stringResource(R.string.today_calories_no_target, summary.caloriesConsumed.toInt()),
                style = MaterialTheme.typography.headlineMedium,
            )
            Text(text = unmetReasonMessage(target.reason))
        }
    }
}

@Composable
private fun MacroBar(label: String, consumedGrams: Double, targetGrams: Int) {
    Column {
        Text(text = "$label: ${consumedGrams.toInt()}g" + if (targetGrams > 0) " / ${targetGrams}g" else "")
        if (targetGrams > 0) {
            LinearProgressIndicator(
                progress = { (consumedGrams / targetGrams).toFloat().coerceIn(0f, 1f) },
                modifier = Modifier.fillMaxWidth().height(6.dp),
            )
        }
        Spacer(modifier = Modifier.height(2.dp))
    }
}

/**
 * Opens `<server>/food/upload/` in a Chrome Custom Tab. The URL is always
 * derived from the stored server URL, never from an intent extra, so no
 * other app can drive this to an arbitrary page. The trailing slash is
 * required — frontend/next.config.ts sets trailingSlash: true on the static
 * export, so the un-slashed path 404s.
 */
private fun openLogFood(context: android.content.Context, serverUrl: String?) {
    val base = serverUrl ?: return
    val uri = android.net.Uri.parse(base.trimEnd('/') + "/food/upload/")
    CustomTabsIntent.Builder().build().launchUrl(context, uri)
}
