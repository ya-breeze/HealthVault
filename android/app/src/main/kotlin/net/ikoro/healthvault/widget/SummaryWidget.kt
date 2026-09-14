package net.ikoro.healthvault.widget

import android.content.Context
import android.content.Intent
import android.content.res.Configuration
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
import java.util.Locale
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.ui.MainActivity
import net.ikoro.healthvault.ui.shippedDisplayLanguage

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
        val resourceContext = localizedResourceContext(context, snapshot?.summary?.displayLanguage)
        val state = widgetState(
            summary = snapshot?.summary,
            fetchedAtMillis = snapshot?.fetchedAtMillis,
            nowMillis = System.currentTimeMillis(),
            hasSession = app.secureStore.hasSession(),
        )

        provideContent {
            GlanceTheme {
                WidgetContent(state, resourceContext)
            }
        }
    }
}

@Composable
private fun WidgetContent(state: WidgetState, resourceContext: Context) {
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
            is WidgetState.SignedOut -> SignedOutBody(resourceContext)
            is WidgetState.Error -> ErrorBody(resourceContext)
            is WidgetState.Loaded -> SummaryBody(resourceContext, state.summary, isWide, isStale = false)
            is WidgetState.Stale -> SummaryBody(resourceContext, state.summary, isWide, isStale = true)
        }
    }
}

@Composable
private fun SignedOutBody(resourceContext: Context) {
    Column {
        Text(text = resourceContext.getString(R.string.widget_sign_in), style = TextStyle(fontWeight = FontWeight.Bold))
        Text(text = resourceContext.getString(R.string.widget_open_to_sign_in), style = TextStyle(fontSize = 11.sp))
    }
}

@Composable
private fun ErrorBody(resourceContext: Context) {
    Text(text = resourceContext.getString(R.string.widget_no_data))
}

@Composable
private fun SummaryBody(resourceContext: Context, summary: TodaySummary, isWide: Boolean, isStale: Boolean) {
    Column(modifier = GlanceModifier.fillMaxSize()) {
        val target = summary.target
        val caloriesLine = if (target.available) {
            resourceContext.getString(
                R.string.widget_calories_of_target,
                summary.caloriesConsumed.toInt(),
                target.calories,
            )
        } else {
            resourceContext.getString(R.string.widget_calories_consumed, summary.caloriesConsumed.toInt())
        }
        Text(text = caloriesLine, style = TextStyle(fontWeight = FontWeight.Bold, fontSize = if (isWide) 20.sp else 16.sp))

        if (isStale) {
            Text(text = resourceContext.getString(R.string.widget_stale), style = TextStyle(fontSize = 10.sp))
        }

        if (isWide) {
            Spacer(modifier = GlanceModifier.height(4.dp))
            MacroBar(resourceContext, R.string.widget_protein_short, summary.proteinGramsConsumed.toInt(), target.proteinGrams)
            MacroBar(resourceContext, R.string.widget_carbs_short, summary.carbsGramsConsumed.toInt(), target.carbsGrams)
            MacroBar(resourceContext, R.string.widget_fat_short, summary.fatGramsConsumed.toInt(), target.fatGrams)

            Spacer(modifier = GlanceModifier.height(4.dp))
            Row {
                Box(
                    modifier = GlanceModifier
                        .background(GlanceTheme.colors.primary)
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                        .clickable(actionRunCallback<LogFoodAction>()),
                ) {
                    Text(
                        text = resourceContext.getString(R.string.widget_log_food),
                        style = TextStyle(color = GlanceTheme.colors.onPrimary),
                    )
                }
                Spacer(modifier = GlanceModifier.width(8.dp))
                Box(
                    modifier = GlanceModifier
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                        .clickable(actionRunCallback<RefreshAction>()),
                ) {
                    Text(text = resourceContext.getString(R.string.widget_refresh))
                }
            }
        }
    }
}

@Composable
private fun MacroBar(resourceContext: Context, labelRes: Int, consumedGrams: Int, targetGrams: Int) {
    val fraction = if (targetGrams > 0) (consumedGrams.toFloat() / targetGrams).coerceIn(0f, 1f) else 0f
    Column(modifier = GlanceModifier.padding(vertical = 1.dp)) {
        val label = resourceContext.getString(labelRes)
        val text = if (targetGrams > 0) {
            resourceContext.getString(R.string.widget_macro_of_target, label, consumedGrams, targetGrams)
        } else {
            resourceContext.getString(R.string.widget_macro_consumed, label, consumedGrams)
        }
        Text(
            text = text,
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

private fun localizedResourceContext(context: Context, displayLanguage: String?): Context {
    val language = displayLanguage?.let(::shippedDisplayLanguage) ?: return context
    val configuration = Configuration(context.resources.configuration).apply {
        setLocale(Locale.forLanguageTag(language))
    }
    return context.createConfigurationContext(configuration)
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
