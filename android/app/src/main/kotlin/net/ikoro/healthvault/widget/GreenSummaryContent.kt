package net.ikoro.healthvault.widget

import android.content.Context
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.layout.ContentScale
import androidx.glance.ColorFilter
import androidx.glance.GlanceModifier
import androidx.glance.GlanceTheme
import androidx.glance.Image
import androidx.glance.ImageProvider
import androidx.glance.LocalSize
import androidx.glance.appwidget.LinearProgressIndicator
import androidx.glance.appwidget.action.actionRunCallback
import androidx.glance.appwidget.components.CircleIconButton
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
import androidx.glance.material3.ColorProviders
import androidx.glance.semantics.contentDescription
import androidx.glance.semantics.semantics
import androidx.glance.text.FontWeight
import net.ikoro.healthvault.R
import net.ikoro.healthvault.api.TodaySummary

// Fixed green identity; the ColorProviders resolve Android's current day/night configuration.
internal val greenWidgetColors = ColorProviders(
    light = lightColorScheme(
        background = Color(0xFFDFEDDD), onBackground = Color(0xFF203D2C),
        surface = Color(0xFFDFEDDD), onSurface = Color(0xFF203D2C),
        onSurfaceVariant = Color(0xFF526F58), surfaceVariant = Color(0xFFBFD1BB),
        primary = Color(0xFF477C48), onPrimary = Color.White, error = Color(0xFFB72D29),
    ),
    dark = darkColorScheme(
        background = Color(0xFF173C31), onBackground = Color(0xFFEFF9E7),
        surface = Color(0xFF173C31), onSurface = Color(0xFFEFF9E7),
        onSurfaceVariant = Color(0xFFB4CFBB), surfaceVariant = Color(0xFF3C6051),
        primary = Color(0xFFA9D98E), onPrimary = Color(0xFF173C31), error = Color(0xFFFF8C82),
    ),
)

@Composable
internal fun GreenSummaryBody(
    resourceContext: Context,
    summary: TodaySummary,
    layout: SummaryWidgetLayout,
    isStale: Boolean,
) {
    val metrics = widgetMetrics(summary)
    val sizing = greenWidgetSizing(LocalSize.current, resourceContext.resources.configuration.fontScale, nutrientNumberCharacters(metrics))
    val calories = WidgetMetric(NutritionMetric.PROTEIN, summary.caloriesConsumed, summary.target.takeIf { it.available }?.calories)
    val numberWidth = nutrientNumberWidth(metrics, sizing.valueSp, resourceContext.resources.configuration.fontScale)
    val visible = if (sizing.primaryOnly) metrics.filter { it.metric in setOf(NutritionMetric.PROTEIN, NutritionMetric.FIBER) } else metrics
    Box(modifier = GlanceModifier.fillMaxSize(), contentAlignment = Alignment.TopEnd) {
        Column(
            modifier = GlanceModifier.fillMaxSize(),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalAlignment = if (sizing.showNutrients) Alignment.Top else Alignment.CenterVertically,
        ) {
            val heroModifier = if (sizing.grid) {
                val heroHeight = (sizing.heroSp * resourceContext.resources.configuration.fontScale * 1.25f + 4f)
                    .coerceIn(30f, 48f)
                GlanceModifier.fillMaxWidth().height(heroHeight.dp).padding(horizontal = 48.dp)
            } else GlanceModifier.fillMaxWidth()
            Box(modifier = heroModifier, contentAlignment = Alignment.Center) {
                if (LocalSize.current.width < 110.dp) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        CalorieNumber(summary, calories, sizing)
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            CalorieUnit(resourceContext, sizing)
                            StaleIndicator(resourceContext, isStale)
                        }
                    }
                } else {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        CalorieNumber(summary, calories, sizing)
                        Spacer(modifier = GlanceModifier.width(3.dp))
                        CalorieUnit(resourceContext, sizing)
                        StaleIndicator(resourceContext, isStale)
                    }
                }
            }

            Spacer(modifier = GlanceModifier.height(sizing.heroGapDp.dp))
            MetricRail(resourceContext, calories, sizing.calorieRailDp)
            if (sizing.showNutrients) {
                Spacer(modifier = GlanceModifier.height(sizing.heroGapDp.dp))
                if (sizing.grid) {
                    // Full-width widgets use two pairs with a rail for each metric.
                    val ordered = if (sizing.primaryOnly) visible else listOf(metrics[0], metrics[3], metrics[1], metrics[2])
                    ordered.chunked(2).forEach { pair ->
                        Row(
                            modifier = GlanceModifier.fillMaxWidth().defaultWeight(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            pair.forEachIndexed { index, metric ->
                                if (index > 0) Spacer(modifier = GlanceModifier.width(10.dp))
                                Column(modifier = GlanceModifier.defaultWeight()) {
                                    MetricValue(resourceContext, metric, sizing, numberWidth)
                                    Spacer(modifier = GlanceModifier.height(2.dp))
                                    MetricRail(resourceContext, metric, sizing.railDp)
                                }
                            }
                        }
                    }
                } else {
                    visible.forEach { metric ->
                        Row(
                            modifier = GlanceModifier.fillMaxWidth().defaultWeight(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            MetricLabelColumn(resourceContext, metric, sizing.labelSp)
                            Spacer(modifier = GlanceModifier.width(5.dp))
                            if (metric.metric != NutritionMetric.FIBER) {
                                MetricRail(resourceContext, metric, sizing.railDp, modifier = GlanceModifier.defaultWeight())
                                Spacer(modifier = GlanceModifier.width(6.dp))
                            }
                            MetricAmount(resourceContext, metric, sizing.valueSp, numberWidth)
                        }
                    }
                }
            }
        }
        if (layout == SummaryWidgetLayout.WIDE) {
            CircleIconButton(
                imageProvider = ImageProvider(R.drawable.ic_refresh_24),
                contentDescription = resourceContext.getString(R.string.widget_refresh),
                onClick = actionRunCallback<RefreshAction>(),
                backgroundColor = null, contentColor = GlanceTheme.colors.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun CalorieNumber(summary: TodaySummary, calories: WidgetMetric, sizing: GreenWidgetSizing) {
    WidgetText(
        text = summary.caloriesConsumed.toInt().toString(),
        color = if (calories.exceeded) GlanceTheme.colors.error else GlanceTheme.colors.onSurface,
        fontSize = sizing.heroSp.sp, fontWeight = FontWeight.Bold, maxLines = 1,
    )
}

@Composable
private fun CalorieUnit(context: Context, sizing: GreenWidgetSizing) {
    WidgetText(
        text = context.getString(R.string.widget_calories_unit),
        color = GlanceTheme.colors.onSurfaceVariant,
        fontSize = (sizing.heroSp / 3).coerceAtLeast(6).sp, maxLines = 1,
    )
}

@Composable
private fun StaleIndicator(context: Context, isStale: Boolean) {
    if (!isStale) return
    Spacer(modifier = GlanceModifier.width(2.dp))
    Image(
        provider = ImageProvider(R.drawable.ic_widget_stale),
        contentDescription = context.getString(R.string.widget_stale_short),
        colorFilter = ColorFilter.tint(GlanceTheme.colors.onSurfaceVariant),
        modifier = GlanceModifier.width(10.dp).height(10.dp),
    )
}

@Composable
private fun MetricValue(context: Context, metric: WidgetMetric, sizing: GreenWidgetSizing, numberWidth: Int) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        MetricLabelColumn(context, metric, sizing.labelSp)
        Spacer(modifier = GlanceModifier.width(5.dp))
        MetricAmount(context, metric, sizing.valueSp, numberWidth)
    }
}

@Composable
private fun MetricLabelColumn(context: Context, metric: WidgetMetric, size: Int) {
    val scale = context.resources.configuration.fontScale
    Box(
        modifier = GlanceModifier.width((size * scale * 1.25f).dp),
        contentAlignment = Alignment.CenterStart,
    ) {
        MetricLabel(context, metric, size)
    }
}

@Composable
private fun MetricLabel(context: Context, metric: WidgetMetric, size: Int) {
    if (metric.metric == NutritionMetric.FIBER) {
        Image(
            provider = ImageProvider(R.drawable.ic_widget_fiber),
            contentDescription = context.getString(R.string.widget_fiber),
            colorFilter = ColorFilter.tint(GlanceTheme.colors.onSurfaceVariant),
            modifier = GlanceModifier.width((size * context.resources.configuration.fontScale).dp)
                .height(((size + 2) * context.resources.configuration.fontScale).dp),
        )
    } else {
        WidgetText(
            text = context.getString(when (metric.metric) {
                NutritionMetric.PROTEIN -> R.string.widget_protein_short
                NutritionMetric.FAT -> R.string.widget_fat_short
                else -> R.string.widget_carbs_short
            }), color = GlanceTheme.colors.onSurfaceVariant, fontSize = size.sp, maxLines = 1,
        )
    }
}

@Composable
private fun MetricAmount(context: Context, metric: WidgetMetric, size: Int, numberWidth: Int) {
    Row(verticalAlignment = Alignment.Bottom) {
        Box(modifier = GlanceModifier.width(numberWidth.dp), contentAlignment = Alignment.CenterEnd) {
            WidgetText(
                text = metric.consumed?.toInt()?.toString() ?: "—",
                color = if (metric.exceeded) GlanceTheme.colors.error else GlanceTheme.colors.onSurface,
                fontSize = size.sp, fontWeight = FontWeight.Bold, maxLines = 1,
            )
        }
        Spacer(modifier = GlanceModifier.width(2.dp))
        val unitSize = (size / 2).coerceAtLeast(7)
        Box(modifier = GlanceModifier.width((unitSize * context.resources.configuration.fontScale).dp)) {
            if (metric.consumed != null) {
                WidgetText(
                    text = context.getString(R.string.widget_grams_unit),
                    color = if (metric.exceeded) GlanceTheme.colors.error else GlanceTheme.colors.onSurfaceVariant,
                    fontSize = unitSize.sp, maxLines = 1,
                    modifier = GlanceModifier.padding(bottom = 2.dp),
                )
            }
        }
    }
}

@Composable
private fun MetricRail(
    context: Context,
    metric: WidgetMetric,
    height: Int,
    modifier: GlanceModifier = GlanceModifier.fillMaxWidth(),
) {
    val accessible = modifier.height(height.dp).semantics {
        contentDescription = context.getString(when {
            metric.exceeded -> R.string.widget_goal_exceeded
            metric.fraction == null -> R.string.widget_target_unavailable
            else -> R.string.widget_goal_progress
        })
    }
    if (metric.fraction == null) {
        Image(
            provider = ImageProvider(R.drawable.widget_neutral_rail),
            contentScale = ContentScale.FillBounds,
            contentDescription = context.getString(R.string.widget_target_unavailable),
            modifier = accessible,
        )
    } else {
        LinearProgressIndicator(
            progress = metric.fraction!!, modifier = accessible,
            color = if (metric.exceeded) GlanceTheme.colors.error else GlanceTheme.colors.primary,
            backgroundColor = GlanceTheme.colors.surfaceVariant,
        )
    }
}


internal fun greenWidgetContentDescription(context: Context, state: WidgetState): String {
    val summary = when (state) {
        is WidgetState.Loaded -> state.summary
        is WidgetState.Stale -> state.summary
        else -> return widgetContentDescription(context, state)
    }
    val calories = WidgetMetric(NutritionMetric.PROTEIN, summary.caloriesConsumed, summary.target.takeIf { it.available }?.calories)
    val goal = calories.target?.let { context.getString(R.string.widget_calorie_goal, summary.caloriesConsumed.toInt(), it) }
        ?: context.getString(R.string.widget_target_unavailable)
    val nutrientDetails = widgetMetrics(summary).joinToString(". ") { metric ->
        val name = context.getString(when (metric.metric) {
            NutritionMetric.PROTEIN -> R.string.widget_protein_name
            NutritionMetric.FAT -> R.string.widget_fat_name
            NutritionMetric.CARBS -> R.string.widget_carbs_name
            NutritionMetric.FIBER -> R.string.widget_fiber
        })
        val amount = metric.consumed?.let { context.getString(R.string.widget_grams_value, it.toInt()) } ?: "—"
        val status = context.getString(when {
            metric.exceeded -> R.string.widget_goal_exceeded
            metric.target == null -> R.string.widget_target_unavailable
            else -> R.string.widget_goal_progress
        })
        context.getString(R.string.widget_metric_description, name, amount, status)
    }
    val calorieStatus = if (calories.exceeded) context.getString(R.string.widget_goal_exceeded) else goal
    return context.getString(
        if (state is WidgetState.Stale) R.string.widget_stale_action_description else R.string.widget_loaded_action_description,
        context.getString(R.string.app_name), summary.caloriesConsumed.toInt(),
        "$goal. $calorieStatus. $nutrientDetails",
    )
}
