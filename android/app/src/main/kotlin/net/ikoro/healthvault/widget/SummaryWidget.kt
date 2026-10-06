package net.ikoro.healthvault.widget

import android.app.ActivityOptions
import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.os.Build
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.Image
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
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.color.ColorProvider as DayNightColorProvider
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
import androidx.glance.semantics.contentDescription
import androidx.glance.semantics.semantics
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

private val MICRO_SIZE = DpSize(48.dp, 48.dp)
private val SHORT_SIZE = DpSize(109.dp, 48.dp)
private val WIDE_SHORT_SIZE = DpSize(230.dp, 48.dp)
private val TALL_NARROW_SIZE = DpSize(48.dp, 110.dp)
private val COMPACT_SIZE = DpSize(110.dp, 110.dp)
private val WIDE_SIZE = DpSize(230.dp, 110.dp)

private val PACE_GOOD: ColorProvider = DayNightColorProvider(
    day = Color(0xFF2E7D32),
    night = Color(0xFF69D68B),
)
private val PACE_WARNING: ColorProvider = DayNightColorProvider(
    day = Color(0xFF8A5A00),
    night = Color(0xFFF3C75D),
)
private val PACE_BAD: ColorProvider = DayNightColorProvider(
    day = Color(0xFFD93025),
    night = Color(0xFFFF453A),
)

internal enum class SummaryWidgetLayout {
    MICRO,
    SHORT,
    WIDE_SHORT,
    TALL,
    COMPACT,
    WIDE,
}

internal enum class WidgetTapTarget {
    LOG_FOOD,
    OPEN_APP,
}

internal fun summaryWidgetLayout(size: DpSize): SummaryWidgetLayout = when {
    size.width >= WIDE_SIZE.width && size.height >= WIDE_SIZE.height -> SummaryWidgetLayout.WIDE
    size.width >= COMPACT_SIZE.width && size.height >= COMPACT_SIZE.height -> SummaryWidgetLayout.COMPACT
    size.height >= TALL_NARROW_SIZE.height -> SummaryWidgetLayout.TALL
    size.width >= WIDE_SHORT_SIZE.width -> SummaryWidgetLayout.WIDE_SHORT
    size.width >= SHORT_SIZE.width -> SummaryWidgetLayout.SHORT
    else -> SummaryWidgetLayout.MICRO
}

internal fun useReducedWidgetContent(fontScale: Float, layout: SummaryWidgetLayout): Boolean = when (layout) {
    SummaryWidgetLayout.MICRO -> fontScale >= 1.1f
    else -> fontScale >= 1.3f
}

internal fun useMinimalMicroContent(fontScale: Float): Boolean = fontScale >= 1.4f

internal fun calorieHeroText(consumed: Int, isStale: Boolean, useReducedContent: Boolean): String =
    if (isStale && useReducedContent) "$consumed!" else consumed.toString()

internal fun widgetTapTarget(state: WidgetState): WidgetTapTarget = when (state) {
    is WidgetState.Loaded, is WidgetState.Stale -> WidgetTapTarget.LOG_FOOD
    is WidgetState.SignedOut, is WidgetState.Error -> WidgetTapTarget.OPEN_APP
}

/** Glance defaults Text to black; always pair widget text with an explicit theme foreground. */
@Composable
internal fun WidgetText(
    text: String,
    modifier: GlanceModifier = GlanceModifier,
    color: ColorProvider = GlanceTheme.colors.onSurface,
    fontSize: TextUnit? = null,
    fontWeight: FontWeight? = null,
    maxLines: Int = Int.MAX_VALUE,
) {
    Text(
        text = text,
        modifier = modifier,
        style = widgetTextStyle(color, fontSize, fontWeight),
        maxLines = maxLines,
    )
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

internal fun paceColor(level: PaceLevel): ColorProvider = when (level) {
    PaceLevel.GOOD -> PACE_GOOD
    PaceLevel.WARNING -> PACE_WARNING
    PaceLevel.BAD -> PACE_BAD
}

internal fun paceFor(summary: TodaySummary, consumed: Double, target: Int): PaceSignal? {
    val inputs = resolvePaceInputs(
        eatingOccasionsToday = summary.eatingOccasionsToday,
        usualMealsPerDay = summary.usualMealsPerDay,
        legacyMealCount = summary.mealCount,
    )
    return paceSignal(
        consumed = consumed,
        target = target,
        eatingOccasionsToday = inputs.eatingOccasionsToday,
        usualMealsPerDay = inputs.usualMealsPerDay,
    )
}

@Composable
internal fun CalorieValue(
    resourceContext: Context,
    consumed: Int,
    isStale: Boolean,
    useReducedContent: Boolean,
    valueSize: TextUnit,
    unitSize: TextUnit,
) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        WidgetText(
            text = calorieHeroText(consumed, isStale, useReducedContent),
            fontWeight = FontWeight.Bold,
            fontSize = valueSize,
            maxLines = 1,
        )
        Spacer(modifier = GlanceModifier.width(2.dp))
        WidgetText(
            text = resourceContext.getString(R.string.widget_calories_unit),
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = unitSize,
            fontWeight = FontWeight.Medium,
            maxLines = 1,
        )
    }
}

@Composable
internal fun PaceGlyph(summary: TodaySummary, consumed: Double, target: Int, fontSize: TextUnit) {
    val signal = paceFor(summary, consumed, target) ?: return
    WidgetText(
        text = signal.direction.glyph,
        color = paceColor(signal.level),
        fontSize = fontSize,
        fontWeight = FontWeight.Bold,
        maxLines = 1,
    )
}

@Composable
internal fun PaceProgress(
    summary: TodaySummary,
    consumed: Double,
    target: Int,
    height: Int,
    modifier: GlanceModifier = GlanceModifier.fillMaxWidth(),
) {
    if (target <= 0) return
    val signal = paceFor(summary, consumed, target)
    LinearProgressIndicator(
        progress = progressFraction(consumed.toInt(), target),
        modifier = modifier.height(height.dp),
        color = signal?.let { paceColor(it.level) } ?: GlanceTheme.colors.primary,
        backgroundColor = GlanceTheme.colors.surfaceVariant,
    )
}

/**
 * One resizable home widget. Exact launcher bounds avoid stretching a small
 * responsive bucket into unused space; typography follows the actual size.
 */
class SummaryWidget : GlanceAppWidget() {

    override val sizeMode = SizeMode.Exact

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
            GlanceTheme(colors = greenWidgetColors) {
                WidgetContent(state, resourceContext)
            }
        }
    }
}

@Composable
private fun WidgetContent(state: WidgetState, resourceContext: Context) {
    val size = LocalSize.current
    val layout = summaryWidgetLayout(size)
    val fontScale = resourceContext.resources.configuration.fontScale
    val useMinimalMicroContent = useMinimalMicroContent(fontScale)

    val backgroundModifier = GlanceModifier
        .fillMaxSize()
        .appWidgetBackground()

    val cardModifier = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
        backgroundModifier
            // widgetBackground derives from secondaryContainer, not our fixed green background.
            .background(GlanceTheme.colors.background)
            .cornerRadius(R.dimen.widget_corner_radius)
    } else {
        backgroundModifier.background(ImageProvider(R.drawable.green_widget_background))
    }

    val accessibleCardModifier = cardModifier.semantics {
        contentDescription = greenWidgetContentDescription(resourceContext, state)
    }

    val interactiveCardModifier = when (widgetTapTarget(state)) {
        WidgetTapTarget.LOG_FOOD ->
            accessibleCardModifier.clickable(actionStartActivity(widgetLogFoodIntent(resourceContext)))
        WidgetTapTarget.OPEN_APP ->
            // The reified actionStartActivity<T>() lives in androidx.glance.action; this file
            // imports androidx.glance.appwidget.action, whose actionStartActivity only takes an
            // Intent — build it explicitly rather than switching import packages.
            accessibleCardModifier.clickable(actionStartActivity(Intent(resourceContext, MainActivity::class.java)))
    }

    val contentPadding = if (layout == SummaryWidgetLayout.MICRO && useMinimalMicroContent) 1.dp
        else greenWidgetSizing(size, fontScale).paddingDp.dp

    Box(modifier = interactiveCardModifier.padding(contentPadding)) {
        when (state) {
            is WidgetState.SignedOut -> SignedOutBody(resourceContext, layout)
            is WidgetState.Error -> ErrorBody(resourceContext, layout)
            is WidgetState.Loaded -> GreenSummaryBody(
                resourceContext,
                state.summary,
                layout,
                isStale = false,
            )
            is WidgetState.Stale -> GreenSummaryBody(
                resourceContext,
                state.summary,
                layout,
                isStale = true,
            )
        }
    }
}

internal fun widgetContentDescription(resourceContext: Context, state: WidgetState): String = when (state) {
    is WidgetState.Loaded -> resourceContext.getString(
        R.string.widget_loaded_action_description,
        resourceContext.getString(R.string.app_name),
        state.summary.caloriesConsumed.toInt(),
        caloriePaceDescription(resourceContext, state.summary),
    )
    is WidgetState.Stale -> resourceContext.getString(
        R.string.widget_stale_action_description,
        resourceContext.getString(R.string.app_name),
        state.summary.caloriesConsumed.toInt(),
        caloriePaceDescription(resourceContext, state.summary),
    )
    is WidgetState.SignedOut -> resourceContext.getString(R.string.widget_open_to_sign_in)
    is WidgetState.Error -> resourceContext.getString(R.string.widget_no_data)
}

private fun caloriePaceDescription(resourceContext: Context, summary: TodaySummary): String {
    val target = summary.target.takeIf { it.available && it.calories > 0 }
        ?: return resourceContext.getString(R.string.widget_pace_unavailable)
    return when (paceFor(summary, summary.caloriesConsumed, target.calories)?.direction) {
        PaceDirection.ON_PACE -> resourceContext.getString(R.string.widget_pace_on)
        PaceDirection.BELOW -> resourceContext.getString(R.string.widget_pace_below)
        PaceDirection.ABOVE -> resourceContext.getString(R.string.widget_pace_above)
        null -> resourceContext.getString(R.string.widget_pace_unavailable)
    }
}

@Composable
private fun SignedOutBody(resourceContext: Context, layout: SummaryWidgetLayout) {
    if (layout in setOf(
            SummaryWidgetLayout.MICRO,
            SummaryWidgetLayout.SHORT,
            SummaryWidgetLayout.WIDE_SHORT,
        )
    ) {
        Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            WidgetText(
                text = resourceContext.getString(R.string.widget_sign_in),
                fontWeight = FontWeight.Bold,
                fontSize = 11.sp,
                maxLines = 1,
            )
        }
    } else {
        Column(modifier = GlanceModifier.fillMaxSize()) {
            Spacer(modifier = GlanceModifier.defaultWeight())
            WidgetText(text = resourceContext.getString(R.string.widget_sign_in), fontWeight = FontWeight.Bold)
            WidgetText(
                text = resourceContext.getString(R.string.widget_open_to_sign_in),
                fontSize = 11.sp,
                maxLines = 1,
            )
            Spacer(modifier = GlanceModifier.defaultWeight())
        }
    }
}

@Composable
private fun ErrorBody(resourceContext: Context, layout: SummaryWidgetLayout) {
    Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        WidgetText(
            text = resourceContext.getString(
                if (layout in setOf(
                        SummaryWidgetLayout.MICRO,
                        SummaryWidgetLayout.SHORT,
                        SummaryWidgetLayout.WIDE_SHORT,
                    )
                ) {
                    R.string.widget_no_data_short
                } else {
                    R.string.widget_no_data
                },
            ),
            fontSize = 11.sp,
            maxLines = if (layout in setOf(
                    SummaryWidgetLayout.MICRO,
                    SummaryWidgetLayout.SHORT,
                    SummaryWidgetLayout.WIDE_SHORT,
                )
            ) {
                1
            } else {
                2
            },
        )
    }
}

@Composable
internal fun WidgetBrandMark(size: Int, isStale: Boolean = false) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Image(
            provider = ImageProvider(R.drawable.healthvault_mark),
            contentDescription = null,
            modifier = GlanceModifier.width(size.dp).height(size.dp),
        )
        if (isStale) {
            Spacer(modifier = GlanceModifier.width(1.dp))
            WidgetText(
                text = "!",
                color = GlanceTheme.colors.onSurfaceVariant,
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
            )
        }
    }
}

@Composable
internal fun WidgetHeader(
    resourceContext: Context,
    isStale: Boolean,
    markSize: Int = 18,
    fontSize: TextUnit = 11.sp,
) {
    val appName = resourceContext.getString(R.string.app_name)
    Row(verticalAlignment = Alignment.CenterVertically) {
        WidgetBrandMark(markSize, isStale = isStale)
        Spacer(modifier = GlanceModifier.width(4.dp))
        WidgetText(
            text = if (isStale) {
                resourceContext.getString(R.string.widget_title_stale, appName)
            } else {
                appName
            },
            color = GlanceTheme.colors.onSurfaceVariant,
            fontSize = fontSize,
            fontWeight = FontWeight.Medium,
            maxLines = 1,
        )
    }
}

internal fun localizedResourceContext(context: Context, displayLanguage: String?): Context {
    val language = displayLanguage?.let(::shippedDisplayLanguage) ?: return context
    val configuration = Configuration(context.resources.configuration).apply {
        setLocale(Locale.forLanguageTag(language))
    }
    return context.createConfigurationContext(configuration)
}

private const val MAIN_DISPLAY_ID = 0

internal fun widgetLogFoodIntent(context: Context): Intent =
    Intent(context, WidgetLogFoodActivity::class.java).apply {
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    }

/** FlexWindow food entry belongs on the unfolded main display, not the cover display. */
internal fun flexWindowLogFoodActivityOptions() =
    ActivityOptions.makeBasic().apply { setLaunchDisplayId(MAIN_DISPLAY_ID) }.toBundle()

/** The widget's own refresh affordance: enqueues an immediate one-off update (work/RefreshScheduler.kt). */
class RefreshAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        net.ikoro.healthvault.work.RefreshScheduler.enqueueOneOff(context)
    }
}
