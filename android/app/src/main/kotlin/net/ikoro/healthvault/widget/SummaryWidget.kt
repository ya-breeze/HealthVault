package net.ikoro.healthvault.widget

import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.net.Uri
import android.os.Build
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.ImageProvider
import androidx.glance.GlanceModifier
import androidx.glance.GlanceTheme
import androidx.glance.LocalSize
import androidx.glance.action.ActionParameters
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.LinearProgressIndicator
import androidx.glance.appwidget.SizeMode
import androidx.glance.appwidget.action.ActionCallback
import androidx.glance.appwidget.action.actionRunCallback
import androidx.glance.appwidget.action.actionStartActivity
import androidx.glance.appwidget.appWidgetBackground
import androidx.glance.appwidget.components.CircleIconButton
import androidx.glance.appwidget.components.SquareIconButton
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.height
import androidx.glance.layout.padding
import androidx.glance.layout.width
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextStyle
import androidx.glance.unit.ColorProvider
import java.util.Locale
import kotlin.math.roundToInt
import net.ikoro.healthvault.HealthVaultApp
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.TodaySummary
import net.ikoro.healthvault.ui.MainActivity
import net.ikoro.healthvault.ui.shippedDisplayLanguage

private val COMPACT_SIZE = DpSize(110.dp, 110.dp)
private val WIDE_SIZE = DpSize(230.dp, 110.dp)

/** Glance defaults Text to black; always pair widget text with an explicit theme foreground. */
@Composable
internal fun WidgetText(
    text: String,
    color: ColorProvider = GlanceTheme.colors.onSurface,
    fontSize: TextUnit? = null,
    fontWeight: FontWeight? = null,
    maxLines: Int = Int.MAX_VALUE,
) {
    Text(text = text, style = widgetTextStyle(color, fontSize, fontWeight), maxLines = maxLines)
}

internal fun widgetTextStyle(
    color: ColorProvider,
    fontSize: TextUnit? = null,
    fontWeight: FontWeight? = null,
) = TextStyle(color = color, fontSize = fontSize, fontWeight = fontWeight)

internal fun progressFraction(consumed: Int, target: Int): Float {
    if (target <= 0) return 0f
    return (consumed.coerceAtLeast(0).toFloat() / target).coerceIn(0f, 1f)
}

internal fun progressPercent(consumed: Int, target: Int): Int? {
    if (target <= 0) return null
    return (consumed.coerceAtLeast(0).toDouble() * 100 / target).roundToInt()
}

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
            refreshFailed = app.secureStore.refreshFailed,
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

    val backgroundModifier = GlanceModifier
        .fillMaxSize()
        .appWidgetBackground()

    val cardModifier = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        backgroundModifier
            .background(GlanceTheme.colors.widgetBackground)
            .cornerRadius(R.dimen.widget_corner_radius)
    } else {
        backgroundModifier.background(ImageProvider(R.drawable.widget_background))
    }

    Box(
        modifier = cardModifier
            .padding(if (isWide) 7.dp else 10.dp)
            // The reified actionStartActivity<T>() lives in androidx.glance.action; this file
            // imports androidx.glance.appwidget.action, whose actionStartActivity only takes an
            // Intent — build it explicitly rather than switching import packages.
            .clickable(actionStartActivity(Intent(resourceContext, MainActivity::class.java))),
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
        WidgetText(text = resourceContext.getString(R.string.widget_sign_in), fontWeight = FontWeight.Bold)
        WidgetText(text = resourceContext.getString(R.string.widget_open_to_sign_in), fontSize = 11.sp)
    }
}

@Composable
private fun ErrorBody(resourceContext: Context) {
    WidgetText(text = resourceContext.getString(R.string.widget_no_data))
}

@Composable
private fun SummaryBody(resourceContext: Context, summary: TodaySummary, isWide: Boolean, isStale: Boolean) {
    if (isWide) {
        WideSummary(resourceContext, summary, isStale)
    } else {
        CompactSummary(resourceContext, summary, isStale)
    }
}

@Composable
private fun CompactSummary(resourceContext: Context, summary: TodaySummary, isStale: Boolean) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }

    Column(modifier = GlanceModifier.fillMaxSize()) {
        WidgetHeader(resourceContext, isStale)
        Spacer(modifier = GlanceModifier.height(2.dp))
        WidgetText(text = consumed.toString(), fontWeight = FontWeight.Bold, fontSize = 30.sp, maxLines = 1)
        WidgetText(
            text = target?.let {
                resourceContext.getString(
                    R.string.widget_calorie_target_progress,
                    it.calories,
                    progressPercent(consumed, it.calories),
                )
            } ?: resourceContext.getString(R.string.widget_calories_today),
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = 11.sp,
            maxLines = 1,
        )
        if (target != null) {
            Spacer(modifier = GlanceModifier.height(5.dp))
            CalorieProgress(consumed, target.calories)
        }
    }
}

@Composable
private fun WideSummary(resourceContext: Context, summary: TodaySummary, isStale: Boolean) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }

    Column(modifier = GlanceModifier.fillMaxSize()) {
        Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = GlanceModifier.defaultWeight()) {
                WidgetHeader(resourceContext, isStale)
                WidgetText(text = consumed.toString(), fontWeight = FontWeight.Bold, fontSize = 24.sp, maxLines = 1)
                WidgetText(
                    text = target?.let {
                        resourceContext.getString(
                            R.string.widget_calorie_target_progress,
                            it.calories,
                            progressPercent(consumed, it.calories),
                        )
                    } ?: resourceContext.getString(R.string.widget_calories_today),
                    color = GlanceTheme.colors.onSurfaceVariant,
                    fontSize = 11.sp,
                    maxLines = 1,
                )
            }
            SquareIconButton(
                imageProvider = ImageProvider(R.drawable.ic_add_24),
                contentDescription = resourceContext.getString(R.string.widget_log_food),
                onClick = actionRunCallback<LogFoodAction>(),
            )
            CircleIconButton(
                imageProvider = ImageProvider(R.drawable.ic_refresh_24),
                contentDescription = resourceContext.getString(R.string.widget_refresh),
                onClick = actionRunCallback<RefreshAction>(),
                backgroundColor = null,
                contentColor = GlanceTheme.colors.onSurfaceVariant,
            )
        }

        if (target != null) {
            CalorieProgress(consumed, target.calories)
            Spacer(modifier = GlanceModifier.height(3.dp))
        }

        MacroStrip(resourceContext, summary)
    }
}

@Composable
private fun WidgetHeader(resourceContext: Context, isStale: Boolean) {
    val appName = resourceContext.getString(R.string.app_name)
    WidgetText(
        text = if (isStale) {
            resourceContext.getString(R.string.widget_title_stale, appName)
        } else {
            appName
        },
        color = GlanceTheme.colors.onSurfaceVariant,
        fontSize = 11.sp,
        fontWeight = FontWeight.Medium,
        maxLines = 1,
    )
}

@Composable
private fun CalorieProgress(consumed: Int, target: Int) {
    LinearProgressIndicator(
        progress = progressFraction(consumed, target),
        modifier = GlanceModifier.fillMaxWidth().height(6.dp),
        color = GlanceTheme.colors.primary,
        backgroundColor = GlanceTheme.colors.surfaceVariant,
    )
}

@Composable
private fun MacroStrip(resourceContext: Context, summary: TodaySummary) {
    val target = summary.target.takeIf { it.available }
    Row(modifier = GlanceModifier.fillMaxWidth()) {
        MacroItem(
            resourceContext,
            R.string.widget_protein_short,
            summary.proteinGramsConsumed.toInt(),
            target?.proteinGrams ?: 0,
        )
        Spacer(modifier = GlanceModifier.width(6.dp))
        MacroItem(
            resourceContext,
            R.string.widget_carbs_short,
            summary.carbsGramsConsumed.toInt(),
            target?.carbsGrams ?: 0,
        )
        Spacer(modifier = GlanceModifier.width(6.dp))
        MacroItem(
            resourceContext,
            R.string.widget_fat_short,
            summary.fatGramsConsumed.toInt(),
            target?.fatGrams ?: 0,
        )
    }
}

@Composable
private fun androidx.glance.layout.RowScope.MacroItem(
    resourceContext: Context,
    labelRes: Int,
    consumedGrams: Int,
    targetGrams: Int,
) {
    Column(modifier = GlanceModifier.defaultWeight()) {
        val label = resourceContext.getString(labelRes)
        WidgetText(
            text = if (targetGrams > 0) {
                resourceContext.getString(R.string.widget_macro_of_target, label, consumedGrams, targetGrams)
            } else {
                resourceContext.getString(R.string.widget_macro_consumed, label, consumedGrams)
            },
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = 11.sp,
            maxLines = 1,
        )
        Spacer(modifier = GlanceModifier.height(2.dp))
        if (targetGrams > 0) {
            LinearProgressIndicator(
                progress = progressFraction(consumedGrams, targetGrams),
                modifier = GlanceModifier.fillMaxWidth().height(3.dp),
                color = GlanceTheme.colors.primary,
                backgroundColor = GlanceTheme.colors.surfaceVariant,
            )
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
