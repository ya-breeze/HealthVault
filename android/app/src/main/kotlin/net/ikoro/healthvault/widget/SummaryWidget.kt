package net.ikoro.healthvault.widget

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.LocalSize
import androidx.glance.action.ActionParameters
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.SizeMode
import androidx.glance.appwidget.action.ActionCallback
import androidx.glance.appwidget.action.actionRunCallback
import androidx.glance.appwidget.action.actionStartActivity
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.height
import androidx.glance.layout.padding
import androidx.glance.layout.width
import androidx.glance.material3.GlanceTheme
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextStyle
import androidx.glance.unit.ColorProvider
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.ui.MainActivity

private val COMPACT_SIZE = DpSize(110.dp, 110.dp)
private val WIDE_SIZE = DpSize(250.dp, 110.dp)
private val MACRO_BAR_WIDTH = 90.dp

/**
 * Single Glance widget, one placement resized between a compact (~110x110dp,
 * a 2x2 cell) and a wide (~250x110dp, a 4x2 cell) layout, rather than two
 * separate pickable widgets — see the spec's "The widget" section for why.
 */
class SummaryWidget : GlanceAppWidget() {

    override val sizeMode = SizeMode.Responsive(setOf(COMPACT_SIZE, WIDE_SIZE))

    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val app = context.applicationContext as HealthVaultApp
        val snapshot = app.secureStore.loadSnapshot()
        val state = widgetState(
            summary = snapshot?.summary,
            fetchedAtMillis = snapshot?.fetchedAtMillis,
            nowMillis = System.currentTimeMillis(),
            hasSession = app.secureStore.hasSession(),
        )

        provideContent {
            GlanceTheme {
                WidgetContent(state)
            }
        }
    }
}

@Composable
private fun WidgetContent(state: WidgetState) {
    val size = LocalSize.current
    val isWide = size.width >= WIDE_SIZE.width

    Box(
        modifier = GlanceModifier
            .fillMaxSize()
            .background(GlanceTheme.colors.background)
            .padding(8.dp)
            .clickable(actionStartActivity<MainActivity>()),
    ) {
        when (state) {
            is WidgetState.SignedOut -> SignedOutBody()
            is WidgetState.Error -> ErrorBody()
            is WidgetState.Loaded -> SummaryBody(state.summary, isWide, isStale = false)
            is WidgetState.Stale -> SummaryBody(state.summary, isWide, isStale = true)
        }
    }
}

@Composable
private fun SignedOutBody() {
    Column {
        Text(text = "Sign in", style = TextStyle(fontWeight = FontWeight.Bold))
        Text(text = "Open HealthVault to sign in", style = TextStyle(fontSize = 11.sp))
    }
}

@Composable
private fun ErrorBody() {
    Text(text = "No data yet — tap to open HealthVault")
}

@Composable
private fun SummaryBody(summary: TodaySummary, isWide: Boolean, isStale: Boolean) {
    Column(modifier = GlanceModifier.fillMaxSize()) {
        val target = summary.target
        val caloriesLine = if (target.available) {
            "${summary.caloriesConsumed.toInt()} / ${target.calories} kcal"
        } else {
            "${summary.caloriesConsumed.toInt()} kcal"
        }
        Text(text = caloriesLine, style = TextStyle(fontWeight = FontWeight.Bold, fontSize = if (isWide) 20.sp else 16.sp))

        if (isStale) {
            Text(text = "Last updated a while ago", style = TextStyle(fontSize = 10.sp))
        }

        if (isWide) {
            Spacer(modifier = GlanceModifier.height(4.dp))
            MacroBar("P", summary.proteinGramsConsumed.toInt(), target.proteinGrams)
            MacroBar("C", summary.carbsGramsConsumed.toInt(), target.carbsGrams)
            MacroBar("F", summary.fatGramsConsumed.toInt(), target.fatGrams)

            // Reserved slot: renders only when the backend actually sends a
            // recommendation (always null today, per SummaryTodayHandler),
            // so shipping it later needs no re-layout here.
            summary.recommendation?.let { recommendation ->
                Text(text = recommendation, style = TextStyle(fontSize = 10.sp))
            }

            Spacer(modifier = GlanceModifier.height(4.dp))
            Row {
                Box(
                    modifier = GlanceModifier
                        .background(GlanceTheme.colors.primary)
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                        .clickable(actionRunCallback<LogFoodAction>()),
                ) {
                    Text(text = "Log food", style = TextStyle(color = GlanceTheme.colors.onPrimary))
                }
                Spacer(modifier = GlanceModifier.width(8.dp))
                Box(
                    modifier = GlanceModifier
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                        .clickable(actionRunCallback<RefreshAction>()),
                ) {
                    Text(text = "Refresh")
                }
            }
        }
    }
}

@Composable
private fun MacroBar(label: String, consumedGrams: Int, targetGrams: Int) {
    val fraction = if (targetGrams > 0) (consumedGrams.toFloat() / targetGrams).coerceIn(0f, 1f) else 0f
    Column(modifier = GlanceModifier.padding(vertical = 1.dp)) {
        Text(
            text = "$label ${consumedGrams}g" + if (targetGrams > 0) "/${targetGrams}g" else "",
            style = TextStyle(fontSize = 9.sp),
        )
        Box(
            modifier = GlanceModifier
                .width(MACRO_BAR_WIDTH)
                .height(4.dp)
                .background(ColorProvider(Color(0xFFE0E0E0))),
        ) {
            if (fraction > 0f) {
                Box(
                    modifier = GlanceModifier
                        .width(MACRO_BAR_WIDTH * fraction)
                        .height(4.dp)
                        .background(ColorProvider(Color(0xFF4CAF50))),
                ) {}
            }
        }
    }
}

/**
 * Always derives the Log food URL from the stored server URL, never from an
 * intent extra, so no other app can drive this to an arbitrary page. Mirrors
 * TodayScreen's openLogFood.
 */
class LogFoodAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        val app = context.applicationContext as HealthVaultApp
        val serverUrl = app.secureStore.serverUrl ?: return
        val intent = CustomTabsIntent.Builder().build().intent.apply {
            data = Uri.parse(serverUrl.trimEnd('/') + "/food/upload/")
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        context.startActivity(intent)
    }
}

/** The widget's own refresh affordance: enqueues an immediate one-off update (work/RefreshScheduler.kt). */
class RefreshAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        net.ikoro.healthvault.work.RefreshScheduler.enqueueOneOff(context)
    }
}
