package net.ikoro.healthvault.widget

import androidx.compose.ui.unit.DpSize
import net.ikoro.healthvault.api.TodaySummary

internal enum class NutritionMetric { PROTEIN, FAT, CARBS, FIBER }

internal data class WidgetMetric(val metric: NutritionMetric, val consumed: Double?, val target: Int?) {
    val exceeded: Boolean get() = target != null && target >= 0 && consumed != null && consumed > target
    val fraction: Float? get() = when {
        target == null || target < 0 || consumed == null -> null
        target == 0 -> if (consumed > 0) 1f else 0f
        else -> (consumed.coerceAtLeast(0.0) / target).coerceIn(0.0, 1.0).toFloat()
    }
}

internal fun widgetMetrics(summary: TodaySummary): List<WidgetMetric> {
    val target = summary.target.takeIf { it.available }
    return listOf(
        WidgetMetric(NutritionMetric.PROTEIN, summary.proteinGramsConsumed, target?.proteinGrams),
        WidgetMetric(NutritionMetric.FAT, summary.fatGramsConsumed, target?.fatGrams),
        WidgetMetric(NutritionMetric.CARBS, summary.carbsGramsConsumed, target?.carbsGrams),
        WidgetMetric(NutritionMetric.FIBER, summary.dietaryFiberGramsConsumed, null),
    )
}

internal data class GreenWidgetSizing(
    val heroSp: Int,
    val valueSp: Int,
    val labelSp: Int,
    val railDp: Int,
    val showNutrients: Boolean,
    val grid: Boolean,
    val primaryOnly: Boolean,
)

/** Budget against actual dp bounds and scaled text, never a launcher cell count. */
internal fun greenWidgetSizing(size: DpSize, fontScale: Float): GreenWidgetSizing {
    val width = size.width.value
    val height = size.height.value
    val scale = fontScale.coerceAtLeast(1f)
    val grid = width >= 230f && height >= 110f
    val supportsRows = width >= 110f && height >= 110f
    val primaryOnly = supportsRows && scale > 1.15f && height / scale < 145f
    val showNutrients = supportsRows && height / scale >= 95f
    val contentHeight = (height - 12f).coerceAtLeast(0f)
    val heroWidth = if (grid) (width - 108f).coerceAtLeast(48f) else width
    val hero = when {
        supportsRows -> (contentHeight * .23f / scale).toInt().coerceIn(20, 48)
        else -> (height * .38f / scale).toInt().coerceIn(9, 28)
    }.coerceAtMost((heroWidth * .21f / scale).toInt().coerceAtLeast(9))
    val value = (contentHeight * .1f / scale).toInt().coerceIn(11, 22)
    return GreenWidgetSizing(
        heroSp = hero,
        valueSp = value,
        labelSp = (value * .7f).toInt().coerceAtLeast(9),
        railDp = (height * .04f).toInt().coerceIn(4, 10),
        showNutrients = showNutrients,
        grid = grid,
        primaryOnly = primaryOnly,
    )
}
