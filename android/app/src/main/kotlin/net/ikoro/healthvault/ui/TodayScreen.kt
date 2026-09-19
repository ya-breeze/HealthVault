package net.ikoro.healthvault.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
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
import kotlin.time.Duration.Companion.milliseconds
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.ApiResult
import net.ikoro.healthvault.api.HealthVaultApi
import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.store.SecureStore
import net.ikoro.healthvault.store.SummarySnapshot
import net.ikoro.healthvault.widget.WIDGET_STALE_AFTER_MILLIS
import net.ikoro.healthvault.widget.WidgetUpdater

/**
 * Why the last refresh failed. Mirrors SetupScreen's SetupError: every
 * recoverable outcome names its own cause, because "we couldn't reach the
 * server", "the rate limiter is holding us off" and "Access is challenging
 * the public API" call for three different reactions from the owner. A dead session is
 * deliberately absent — [ApiResult.Unauthenticated] routes to `onSignedOut()`
 * instead, and after HealthVaultApi.summaryToday's re-login fallback that is
 * the only outcome that really means the session ended.
 */
private sealed class RefreshProblem {
    data class LockedOut(val retrySeconds: Long) : RefreshProblem()
    data object AccessChallenge : RefreshProblem()
    data object Unreachable : RefreshProblem()
    data class Server(val code: Int) : RefreshProblem()
}

@Composable
private fun RefreshProblem.message(): String = when (this) {
    is RefreshProblem.LockedOut -> stringResource(R.string.today_refresh_locked_out, retrySeconds)
    is RefreshProblem.AccessChallenge -> stringResource(R.string.today_refresh_access_challenge)
    is RefreshProblem.Unreachable -> stringResource(R.string.today_refresh_unreachable)
    is RefreshProblem.Server -> stringResource(R.string.today_refresh_server, code)
}

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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TodayScreen(
    api: HealthVaultApi,
    secureStore: SecureStore,
    onSignedOut: () -> Unit,
) {
    var snapshot by remember { mutableStateOf(secureStore.loadSnapshot()) }
    var refreshing by remember { mutableStateOf(false) }
    var problem by remember { mutableStateOf<RefreshProblem?>(null) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    suspend fun refresh() {
        refreshing = true
        val (result, sessionGeneration) = withContext(Dispatchers.IO) {
            val generation = secureStore.currentSessionGeneration
            val now = System.currentTimeMillis()
            val heldUntil = secureStore.nextRefreshAtMillis
            val isHeld = heldUntil > now
            val outcome = if (isHeld) {
                ApiResult.RateLimited((heldUntil - now).milliseconds)
            } else {
                api.summaryToday()
            }

            when (outcome) {
                is ApiResult.Success -> {
                    secureStore.saveRefreshState(
                        failed = false,
                        nextAttemptAtMillis = 0L,
                        expectedSessionGeneration = generation,
                    )
                }
                is ApiResult.RateLimited -> {
                    val receivedAt = System.currentTimeMillis()
                    val delayMillis = outcome.retryAfter.inWholeMilliseconds.coerceAtLeast(0L)
                    val nextAttemptAt = if (isHeld) {
                        heldUntil
                    } else if (delayMillis > Long.MAX_VALUE - receivedAt) {
                        Long.MAX_VALUE
                    } else {
                        receivedAt + delayMillis
                    }
                    secureStore.saveRefreshState(
                        failed = true,
                        nextAttemptAtMillis = nextAttemptAt,
                        expectedSessionGeneration = generation,
                    )
                }
                is ApiResult.NetworkFailure,
                is ApiResult.AccessChallenge,
                is ApiResult.ServerError,
                -> {
                    secureStore.saveRefreshState(
                        failed = true,
                        nextAttemptAtMillis = 0L,
                        expectedSessionGeneration = generation,
                    )
                }
                is ApiResult.Unauthenticated -> Unit
            }
            outcome to generation
        }
        refreshing = false
        if (!secureStore.isCurrentSession(sessionGeneration)) return
        when (result) {
            is ApiResult.Success -> {
                problem = null
                val fresh = SummarySnapshot(result.value, System.currentTimeMillis())
                secureStore.saveSnapshot(fresh, sessionGeneration)
                snapshot = fresh
                applyDisplayLanguage(result.value.displayLanguage)
                WidgetUpdater.updateAll(context.applicationContext)
            }
            // Only a 401 that survived both the refresh and the re-login
            // fallback means the session is really gone.
            is ApiResult.Unauthenticated -> onSignedOut()
            is ApiResult.RateLimited -> {
                problem = RefreshProblem.LockedOut(result.retryAfter.inWholeSeconds)
                WidgetUpdater.updateAll(context.applicationContext)
            }
            is ApiResult.AccessChallenge -> {
                problem = RefreshProblem.AccessChallenge
                WidgetUpdater.updateAll(context.applicationContext)
            }
            is ApiResult.NetworkFailure -> {
                problem = RefreshProblem.Unreachable
                WidgetUpdater.updateAll(context.applicationContext)
            }
            is ApiResult.ServerError -> {
                problem = RefreshProblem.Server(result.code)
                WidgetUpdater.updateAll(context.applicationContext)
            }
        }
    }

    LaunchedEffect(Unit) {
        // A background widget refresh may have cached a newer Display
        // Language while the activity was not running. Apply that cached
        // value even if this foreground refresh is currently offline.
        snapshot?.summary?.displayLanguage?.let(::applyDisplayLanguage)
        refresh()
    }

    val now = System.currentTimeMillis()
    val current = snapshot
    val isStale = current != null && now - current.fetchedAtMillis > WIDGET_STALE_AFTER_MILLIS

    Surface(modifier = Modifier.fillMaxSize()) {
        PullToRefreshBox(
            isRefreshing = refreshing,
            onRefresh = { scope.launch { refresh() } },
            modifier = Modifier.fillMaxSize(),
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
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

                // A failed refresh names its cause; a snapshot that is merely
                // old says only that. The cause is shown even with nothing
                // cached, so a first run against an unreachable server reports
                // why rather than sitting on "Loading…" forever.
                val currentProblem = problem
                if (currentProblem != null) {
                    Text(text = currentProblem.message(), color = MaterialTheme.colorScheme.error)
                } else if (isStale) {
                    Text(
                        text = stringResource(R.string.today_stale_snapshot),
                        color = MaterialTheme.colorScheme.error,
                    )
                }

                if (current != null) {
                    val summary = current.summary
                    TodayContent(summary)
                    Text(
                        text = stringResource(R.string.today_meal_count, summary.mealCount) + " · " +
                            relativeTimeText(summary.lastLoggedAt, now),
                    )

                    Button(onClick = { openLogFood(context, secureStore.serverUrl) }) {
                        Text(stringResource(R.string.today_log_food))
                    }
                } else if (currentProblem == null) {
                    // Nothing cached and nothing wrong: the first fetch is
                    // still in flight. With a problem, the banner above has
                    // already said what happened, so don't claim to be loading.
                    Text(text = stringResource(R.string.today_loading))
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
            MacroBar(stringResource(R.string.today_macro_protein), summary.proteinGramsConsumed, 0)
            MacroBar(stringResource(R.string.today_macro_carbs), summary.carbsGramsConsumed, 0)
            MacroBar(stringResource(R.string.today_macro_fat), summary.fatGramsConsumed, 0)
        }
    }
}

@Composable
private fun MacroBar(label: String, consumedGrams: Double, targetGrams: Int) {
    Column {
        val text = if (targetGrams > 0) {
            stringResource(R.string.today_macro_of_target, label, consumedGrams.toInt(), targetGrams)
        } else {
            stringResource(R.string.today_macro_consumed, label, consumedGrams.toInt())
        }
        Text(text = text)
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
