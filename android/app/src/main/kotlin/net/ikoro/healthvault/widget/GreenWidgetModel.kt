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
    val calorieRailDp: Int,
    val paddingDp: Int,
    val heroGapDp: Int,
    val showNutrients: Boolean,
    val grid: Boolean,
    val primaryOnly: Boolean,
)

/** Budget against actual dp bounds and scaled text, never a launcher cell count. */
internal fun greenWidgetSizing(size: DpSize, fontScale: Float, numberCharacters: Int = 3): GreenWidgetSizing {
    val width = size.width.value
    val height = size.height.value
    val scale = fontScale.coerceAtLeast(1f)
    val grid = width >= 230f && height >= 110f
    val supportsRows = width >= 110f && height >= 110f
    val primaryOnly = supportsRows && scale > 1.15f && height / scale < 145f
    val padding = if (supportsRows) (minOf(width, height) * .068f).toInt().coerceIn(7, 17)
        else (minOf(width, height) * .06f).toInt().coerceIn(2, 6)
    val contentHeight = (height - padding * 2).coerceAtLeast(0f)
    val heroWidth = if (grid) (width - padding * 2 - 96f).coerceAtLeast(48f) else width - padding * 2
    val hero = when {
        supportsRows -> (contentHeight * .23f / scale).toInt().coerceIn(20, 48)
        else -> (contentHeight * .48f / scale).toInt().coerceIn(12, 38)
    }.coerceAtMost((heroWidth * (if (supportsRows) .21f else .29f) / scale).toInt().coerceAtLeast(9))
    var value = (contentHeight * (if (grid) .16f else .1f) / scale).toInt()
        .coerceIn(11, if (grid) 30 else 22)
    val availableNutrientWidth = if (grid) (width - padding * 2 - 10f) / 2f else width - padding * 2
    while (value > 11 && nutrientRowWidth(value, grid, scale, numberCharacters) > availableNutrientWidth) value--
    val showNutrients = supportsRows && height / scale >= 95f &&
        nutrientRowWidth(value, grid, scale, numberCharacters) <= availableNutrientWidth
    return GreenWidgetSizing(
        heroSp = hero,
        valueSp = value,
        labelSp = nutrientLabelSize(value, grid),
        railDp = (height * .04f).toInt().coerceIn(4, 10),
        calorieRailDp = (height * (if (supportsRows) .055f else .09f)).toInt().coerceIn(6, 14),
        paddingDp = padding,
        heroGapDp = (height * .025f).toInt().coerceIn(3, 8),
        showNutrients = showNutrients,
        grid = grid,
        primaryOnly = primaryOnly,
    )
}

/** Share the widest displayed number across every row, including an unknown fiber value. */
internal fun nutrientNumberWidth(metrics: List<WidgetMetric>, valueSp: Int, fontScale: Float): Int {
    return numberColumnWidth(nutrientNumberCharacters(metrics), valueSp, fontScale)
}

internal fun nutrientNumberCharacters(metrics: List<WidgetMetric>): Int =
    (metrics.maxOfOrNull { it.consumed?.toInt()?.toString()?.length ?: 1 } ?: 3).coerceAtLeast(3)

private fun nutrientLabelSize(valueSp: Int, grid: Boolean): Int =
    (valueSp * (if (grid) .8f else .7f)).toInt().coerceAtLeast(9)

private fun numberColumnWidth(characters: Int, valueSp: Int, fontScale: Float): Int =
    kotlin.math.ceil(characters.coerceAtLeast(3) * valueSp * fontScale.coerceAtLeast(1f) * .7f).toInt()

/** Include every fixed child and keep a usable rail when amounts share a compact row. */
internal fun nutrientRowWidth(valueSp: Int, grid: Boolean, fontScale: Float, characters: Int): Float {
    val scale = fontScale.coerceAtLeast(1f)
    return nutrientLabelSize(valueSp, grid) * scale * 1.25f + 5f +
        numberColumnWidth(characters, valueSp, scale) + 2f + (valueSp / 2).coerceAtLeast(7) * scale +
        if (grid) 0f else 6f + 24f
}
