package net.ikoro.healthvault.widget

import android.app.ActivityOptions
import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.net.Uri
import android.os.Build
import androidx.browser.customtabs.CustomTabsIntent
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

private val PACE_GOOD = ColorProvider(Color(0xFF69D68B))
private val PACE_WARNING = ColorProvider(Color(0xFFF3C75D))
private val PACE_BAD = ColorProvider(Color(0xFFFF453A))

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

internal fun paceFor(summary: TodaySummary, consumed: Double, target: Int): PaceSignal? = paceSignal(
    consumed = consumed,
    target = target,
    eatingOccasionsToday = summary.eatingOccasionsToday,
    usualMealsPerDay = summary.usualMealsPerDay,
)

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
private fun PaceGlyph(summary: TodaySummary, consumed: Double, target: Int, fontSize: TextUnit) {
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
    val signal = paceFor(summary, consumed, target) ?: return
    LinearProgressIndicator(
        progress = progressFraction(consumed.toInt(), target),
        modifier = modifier.height(height.dp),
        color = paceColor(signal.level),
        backgroundColor = GlanceTheme.colors.surfaceVariant,
    )
}

@Composable
private fun MiniMacroRails(resourceContext: Context, summary: TodaySummary, showValues: Boolean = false) {
    val target = summary.target.takeIf { it.available } ?: return
    Row(modifier = GlanceModifier.fillMaxWidth()) {
        MiniMacroRail(
            resourceContext,
            summary,
            R.string.widget_protein_short,
            summary.proteinGramsConsumed,
            target.proteinGrams,
            showValues,
        )
        Spacer(modifier = GlanceModifier.width(3.dp))
        MiniMacroRail(
            resourceContext,
            summary,
            R.string.widget_carbs_short,
            summary.carbsGramsConsumed,
            target.carbsGrams,
            showValues,
        )
        Spacer(modifier = GlanceModifier.width(3.dp))
        MiniMacroRail(
            resourceContext,
            summary,
            R.string.widget_fat_short,
            summary.fatGramsConsumed,
            target.fatGrams,
            showValues,
        )
    }
}

@Composable
private fun androidx.glance.layout.RowScope.MiniMacroRail(
    resourceContext: Context,
    summary: TodaySummary,
    labelRes: Int,
    consumed: Double,
    target: Int,
    showValue: Boolean,
) {
    val signal = paceFor(summary, consumed, target)
    Column(modifier = GlanceModifier.defaultWeight()) {
        Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            WidgetText(
                text = if (showValue) {
                    resourceContext.getString(R.string.widget_macro_compact_value, resourceContext.getString(labelRes), consumed.toInt())
                } else {
                    resourceContext.getString(labelRes)
                },
                color = GlanceTheme.colors.onSurfaceVariant,
                fontSize = 7.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
            )
            if (signal != null) {
                Spacer(modifier = GlanceModifier.defaultWeight())
                WidgetText(
                    text = signal.direction.glyph,
                    color = paceColor(signal.level),
                    fontSize = 7.sp,
                    fontWeight = FontWeight.Bold,
                    maxLines = 1,
                )
            }
        }
        if (signal != null) {
            PaceProgress(summary, consumed, target, height = 2)
        }
    }
}

/**
 * Single Glance widget with deliberate 1x1, 2x1, 2x2, 4x1 and 4x2 breakpoints,
 * rather than separate pickable widgets — see the spec's "The widget" section.
 */
class SummaryWidget : GlanceAppWidget() {

    override val sizeMode = SizeMode.Responsive(
        setOf(MICRO_SIZE, SHORT_SIZE, WIDE_SHORT_SIZE, TALL_NARROW_SIZE, COMPACT_SIZE, WIDE_SIZE),
    )

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
    val layout = summaryWidgetLayout(size)
    val fontScale = resourceContext.resources.configuration.fontScale
    val useReducedContent = useReducedWidgetContent(fontScale, layout)
    val useMinimalMicroContent = useMinimalMicroContent(fontScale)

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

    val accessibleCardModifier = cardModifier.semantics {
        contentDescription = widgetContentDescription(resourceContext, state)
    }

    val interactiveCardModifier = when (widgetTapTarget(state)) {
        WidgetTapTarget.LOG_FOOD ->
            accessibleCardModifier.clickable(actionRunCallback<LogFoodAction>())
        WidgetTapTarget.OPEN_APP ->
            // The reified actionStartActivity<T>() lives in androidx.glance.action; this file
            // imports androidx.glance.appwidget.action, whose actionStartActivity only takes an
            // Intent — build it explicitly rather than switching import packages.
            accessibleCardModifier.clickable(actionStartActivity(Intent(resourceContext, MainActivity::class.java)))
    }

    val contentPadding = when (layout) {
        SummaryWidgetLayout.MICRO -> if (useMinimalMicroContent) 1.dp else 3.dp
        SummaryWidgetLayout.SHORT -> 4.dp
        SummaryWidgetLayout.WIDE_SHORT -> 5.dp
        SummaryWidgetLayout.TALL -> 4.dp
        SummaryWidgetLayout.COMPACT -> 10.dp
        SummaryWidgetLayout.WIDE -> 5.dp
    }

    Box(modifier = interactiveCardModifier.padding(contentPadding)) {
        when (state) {
            is WidgetState.SignedOut -> SignedOutBody(resourceContext, layout)
            is WidgetState.Error -> ErrorBody(resourceContext, layout)
            is WidgetState.Loaded -> SummaryBody(
                resourceContext,
                state.summary,
                layout,
                isStale = false,
                useReducedContent = useReducedContent,
                useMinimalMicroContent = useMinimalMicroContent,
            )
            is WidgetState.Stale -> SummaryBody(
                resourceContext,
                state.summary,
                layout,
                isStale = true,
                useReducedContent = useReducedContent,
                useMinimalMicroContent = useMinimalMicroContent,
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
private fun SummaryBody(
    resourceContext: Context,
    summary: TodaySummary,
    layout: SummaryWidgetLayout,
    isStale: Boolean,
    useReducedContent: Boolean,
    useMinimalMicroContent: Boolean,
) {
    when (layout) {
        SummaryWidgetLayout.MICRO -> MicroSummary(
            resourceContext,
            summary,
            isStale,
            useReducedContent,
            useMinimalMicroContent,
        )
        SummaryWidgetLayout.SHORT -> ShortSummary(resourceContext, summary, isStale, useReducedContent)
        SummaryWidgetLayout.WIDE_SHORT -> WideShortSummary(resourceContext, summary, isStale, useReducedContent)
        SummaryWidgetLayout.TALL -> TallSummary(resourceContext, summary, isStale, useReducedContent)
        SummaryWidgetLayout.COMPACT -> CompactSummary(resourceContext, summary, isStale, useReducedContent)
        SummaryWidgetLayout.WIDE -> WideSummary(resourceContext, summary, isStale, useReducedContent)
    }
}

@Composable
private fun MicroSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
    useReducedContent: Boolean,
    useMinimalContent: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }

    if (useMinimalContent) {
        // At >= 1.4x, secondary rails no longer fit the launcher's 48dp cell
        // with a safe vertical margin. Keep the value and explicit unit visible;
        // the card semantics retain HealthVault identity and the Log food action.
        Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CalorieValue(
                resourceContext,
                consumed,
                isStale,
                useReducedContent,
                valueSize = 9.sp,
                unitSize = 5.sp,
            )
        }
        return
    }

    Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(modifier = GlanceModifier.fillMaxWidth()) {
            Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                CalorieValue(
                    resourceContext,
                    consumed,
                    isStale,
                    useReducedContent,
                    valueSize = if (useReducedContent) 10.sp else 12.sp,
                    unitSize = 6.sp,
                )
                if (target != null) {
                    Spacer(modifier = GlanceModifier.defaultWeight())
                    PaceGlyph(summary, summary.caloriesConsumed, target.calories, fontSize = 8.sp)
                }
            }
            if (!useReducedContent && target != null) {
                Spacer(modifier = GlanceModifier.height(2.dp))
                MiniMacroRails(resourceContext, summary)
            }
        }
    }
}

@Composable
private fun ShortSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
    useReducedContent: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }
    val caloriePace = target?.let { paceFor(summary, summary.caloriesConsumed, it.calories) }

    Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(modifier = GlanceModifier.fillMaxWidth()) {
            Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                WidgetBrandMark(size = 20)
                Spacer(modifier = GlanceModifier.width(4.dp))
                CalorieValue(
                    resourceContext,
                    consumed,
                    isStale,
                    useReducedContent,
                    valueSize = if (useReducedContent) 12.sp else 16.sp,
                    unitSize = if (useReducedContent) 6.sp else 8.sp,
                )
                if (!useReducedContent) {
                    Spacer(modifier = GlanceModifier.defaultWeight())
                    WidgetText(
                        text = when {
                            isStale -> resourceContext.getString(R.string.widget_stale_short)
                            caloriePace != null -> "${caloriePace.direction.glyph} ${caloriePace.actualPercent}%"
                            else -> resourceContext.getString(R.string.widget_calories_unit)
                        },
                        color = caloriePace?.let { paceColor(it.level) } ?: GlanceTheme.colors.onSurfaceVariant,
                        fontSize = 11.sp,
                        maxLines = 1,
                    )
                }
            }
            if (target != null) {
                Spacer(modifier = GlanceModifier.height(3.dp))
                PaceProgress(summary, summary.caloriesConsumed, target.calories, height = 4)
                if (!useReducedContent) {
                    Spacer(modifier = GlanceModifier.height(2.dp))
                    MiniMacroRails(resourceContext, summary)
                }
            }
        }
    }
}

@Composable
private fun WideShortSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
    useReducedContent: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }
    val caloriePace = target?.let { paceFor(summary, summary.caloriesConsumed, it.calories) }

    Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(modifier = GlanceModifier.fillMaxWidth()) {
            Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                WidgetHeader(resourceContext, isStale)
                Spacer(modifier = GlanceModifier.defaultWeight())
                CalorieValue(
                    resourceContext,
                    consumed,
                    isStale,
                    useReducedContent,
                    valueSize = if (useReducedContent) 13.sp else 17.sp,
                    unitSize = if (useReducedContent) 6.sp else 8.sp,
                )
                if (!useReducedContent) {
                    Spacer(modifier = GlanceModifier.width(8.dp))
                    WidgetText(
                        text = caloriePace?.let { "${it.direction.glyph} ${it.actualPercent}%" }
                            ?: resourceContext.getString(R.string.widget_calories_unit),
                        color = caloriePace?.let { paceColor(it.level) } ?: GlanceTheme.colors.onSurfaceVariant,
                        fontSize = 11.sp,
                        maxLines = 1,
                    )
                }
            }
            if (target != null) {
                Spacer(modifier = GlanceModifier.height(3.dp))
                PaceProgress(summary, summary.caloriesConsumed, target.calories, height = 4)
                if (!useReducedContent) {
                    Spacer(modifier = GlanceModifier.height(2.dp))
                    MiniMacroRails(resourceContext, summary)
                }
            }
        }
    }
}

@Composable
private fun TallSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
    useReducedContent: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }

    Column(modifier = GlanceModifier.fillMaxSize()) {
        WidgetBrandMark(size = 20, isStale = isStale)
        Spacer(modifier = GlanceModifier.defaultWeight())
        CalorieValue(
            resourceContext,
            consumed,
            isStale,
            useReducedContent,
            valueSize = if (useReducedContent) 12.sp else 15.sp,
            unitSize = 6.sp,
        )
        Spacer(modifier = GlanceModifier.defaultWeight())
        if (!useReducedContent && target != null) {
            WidgetText(
                text = paceFor(summary, summary.caloriesConsumed, target.calories)?.let {
                    "${it.direction.glyph} ${it.actualPercent}%"
                } ?: "",
                color = paceFor(summary, summary.caloriesConsumed, target.calories)?.let { paceColor(it.level) }
                    ?: GlanceTheme.colors.onSurfaceVariant,
                fontSize = 11.sp,
                maxLines = 1,
            )
        }
        if (target != null) {
            Spacer(modifier = GlanceModifier.defaultWeight())
            PaceProgress(summary, summary.caloriesConsumed, target.calories, height = 4)
        }
    }
}

@Composable
private fun CompactSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
    useReducedContent: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }
    val caloriePace = target?.let { paceFor(summary, summary.caloriesConsumed, it.calories) }

    Column(modifier = GlanceModifier.fillMaxSize()) {
        WidgetHeader(resourceContext, isStale)
        Box(
            modifier = GlanceModifier.fillMaxWidth().defaultWeight(),
            contentAlignment = Alignment.CenterStart,
        ) {
            Column(modifier = GlanceModifier.fillMaxWidth()) {
                CalorieValue(
                    resourceContext,
                    consumed,
                    isStale,
                    useReducedContent,
                    valueSize = if (useReducedContent) 17.sp else 26.sp,
                    unitSize = if (useReducedContent) 8.sp else 11.sp,
                )
                if (!useReducedContent) {
                    Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
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
                        if (caloriePace != null) {
                            Spacer(modifier = GlanceModifier.defaultWeight())
                            WidgetText(
                                text = caloriePace.direction.glyph,
                                color = paceColor(caloriePace.level),
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Bold,
                                maxLines = 1,
                            )
                        }
                    }
                }
                if (target != null) {
                    Spacer(modifier = GlanceModifier.height(3.dp))
                    PaceProgress(summary, summary.caloriesConsumed, target.calories, height = 6)
                    if (!useReducedContent) {
                        Spacer(modifier = GlanceModifier.height(4.dp))
                        MiniMacroRails(resourceContext, summary, showValues = true)
                    }
                }
            }
        }
    }
}

@Composable
private fun WideSummary(
    resourceContext: Context,
    summary: TodaySummary,
    isStale: Boolean,
    useReducedContent: Boolean,
) {
    val consumed = summary.caloriesConsumed.toInt()
    val target = summary.target.takeIf { it.available && it.calories > 0 }
    val caloriePace = target?.let { paceFor(summary, summary.caloriesConsumed, it.calories) }

    Column(modifier = GlanceModifier.fillMaxSize()) {
        Row(modifier = GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = GlanceModifier.defaultWeight()) {
                WidgetHeader(resourceContext, isStale)
                CalorieValue(
                    resourceContext,
                    consumed,
                    isStale,
                    useReducedContent,
                    valueSize = if (useReducedContent) 14.sp else 24.sp,
                    unitSize = if (useReducedContent) 7.sp else 10.sp,
                )
                if (!useReducedContent) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
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
                        if (caloriePace != null) {
                            Spacer(modifier = GlanceModifier.width(4.dp))
                            WidgetText(
                                text = caloriePace.direction.glyph,
                                color = paceColor(caloriePace.level),
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Bold,
                                maxLines = 1,
                            )
                        }
                    }
                }
            }
            CircleIconButton(
                imageProvider = ImageProvider(R.drawable.ic_refresh_24),
                contentDescription = resourceContext.getString(R.string.widget_refresh),
                onClick = actionRunCallback<RefreshAction>(),
                backgroundColor = null,
                contentColor = GlanceTheme.colors.onSurfaceVariant,
            )
        }

        if (target != null) PaceProgress(summary, summary.caloriesConsumed, target.calories, height = 6)

        if (useReducedContent) {
            Spacer(modifier = GlanceModifier.defaultWeight())
        } else {
            MacroRows(resourceContext, summary)
        }
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

@Composable
private fun androidx.glance.layout.ColumnScope.MacroRows(resourceContext: Context, summary: TodaySummary) {
    val target = summary.target.takeIf { it.available }
    Spacer(modifier = GlanceModifier.defaultWeight())
    MacroRow(resourceContext, summary, R.string.widget_protein_short, summary.proteinGramsConsumed, target?.proteinGrams ?: 0)
    Spacer(modifier = GlanceModifier.defaultWeight())
    MacroRow(resourceContext, summary, R.string.widget_carbs_short, summary.carbsGramsConsumed, target?.carbsGrams ?: 0)
    Spacer(modifier = GlanceModifier.defaultWeight())
    MacroRow(resourceContext, summary, R.string.widget_fat_short, summary.fatGramsConsumed, target?.fatGrams ?: 0)
}

@Composable
private fun MacroRow(
    resourceContext: Context,
    summary: TodaySummary,
    labelRes: Int,
    consumedGrams: Double,
    targetGrams: Int,
) {
    val consumed = consumedGrams.toInt()
    val signal = paceFor(summary, consumedGrams, targetGrams)
    Row(
        modifier = GlanceModifier.fillMaxWidth().padding(horizontal = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val label = resourceContext.getString(labelRes)
        Box(modifier = GlanceModifier.width(86.dp)) {
            WidgetText(
                text = if (targetGrams > 0) {
                    resourceContext.getString(R.string.widget_macro_of_target, label, consumed, targetGrams)
                } else {
                    resourceContext.getString(R.string.widget_macro_consumed, label, consumed)
                },
                color = GlanceTheme.colors.onSurfaceVariant,
                fontSize = 11.sp,
                maxLines = 1,
            )
        }
        if (targetGrams > 0) {
            Spacer(modifier = GlanceModifier.width(8.dp))
            PaceProgress(
                summary,
                consumedGrams,
                targetGrams,
                height = 5,
                modifier = GlanceModifier.defaultWeight(),
            )
            if (signal != null) {
                Spacer(modifier = GlanceModifier.width(6.dp))
                WidgetText(
                    text = signal.direction.glyph,
                    color = paceColor(signal.level),
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                    maxLines = 1,
                )
            }
        }
    }
}

internal fun localizedResourceContext(context: Context, displayLanguage: String?): Context {
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
private const val MAIN_DISPLAY_ID = 0

private fun launchLogFood(context: Context, displayId: Int? = null) {
    val app = context.applicationContext as HealthVaultApp
    val serverUrl = app.secureStore.serverUrl ?: return
    val intent = CustomTabsIntent.Builder().build().intent.apply {
        data = Uri.parse(serverUrl.trimEnd('/') + "/food/upload/")
        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    }
    if (displayId == null) {
        context.startActivity(intent)
    } else {
        val options = ActivityOptions.makeBasic().apply { setLaunchDisplayId(displayId) }
        context.startActivity(intent, options.toBundle())
    }
}

class LogFoodAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        launchLogFood(context)
    }
}

/** FlexWindow food entry belongs on the unfolded main display, not the cover display. */
class FlexWindowLogFoodAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        launchLogFood(context, MAIN_DISPLAY_ID)
    }
}

/** The widget's own refresh affordance: enqueues an immediate one-off update (work/RefreshScheduler.kt). */
class RefreshAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        net.ikoro.healthvault.work.RefreshScheduler.enqueueOneOff(context)
    }
}
