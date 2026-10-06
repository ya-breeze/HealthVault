package net.ikoro.healthvault.widget

import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import org.junit.Assert.*
import org.junit.Test

class GreenWidgetModelTest {
    @Test
    fun fullDayOverrunIsStrictAndDoesNotDependOnMealPace() {
        assertFalse(WidgetMetric(NutritionMetric.PROTEIN, 127.99, 128).exceeded)
        assertFalse(WidgetMetric(NutritionMetric.PROTEIN, 128.0, 128).exceeded)
        assertTrue(WidgetMetric(NutritionMetric.PROTEIN, 128.01, 128).exceeded)
        assertTrue(WidgetMetric(NutritionMetric.CARBS, 1.0, 0).exceeded)
        assertFalse(WidgetMetric(NutritionMetric.FIBER, 18.0, null).exceeded)
        assertFalse(WidgetMetric(NutritionMetric.FIBER, null, null).exceeded)
        assertFalse(WidgetMetric(NutritionMetric.PROTEIN, 140.0, -1).exceeded)
    }

    @Test
    fun railsClampAndKeepMissingOrZeroTargetsUnknown() {
        assertEquals(.5f, WidgetMetric(NutritionMetric.FAT, 20.0, 40).fraction!!, 0f)
        assertEquals(1f, WidgetMetric(NutritionMetric.PROTEIN, 145.0, 128).fraction!!, 0f)
        assertEquals(0f, WidgetMetric(NutritionMetric.CARBS, -1.0, 100).fraction!!, 0f)
        assertEquals(1f, WidgetMetric(NutritionMetric.CARBS, 1.0, 0).fraction!!, 0f)
        assertEquals(0f, WidgetMetric(NutritionMetric.CARBS, 0.0, 0).fraction!!, 0f)
        assertFalse(WidgetMetric(NutritionMetric.CARBS, 0.0, 0).exceeded)
        assertNull(WidgetMetric(NutritionMetric.FIBER, 18.0, null).fraction)
        assertNull(WidgetMetric(NutritionMetric.FIBER, null, 25).fraction)
    }

    @Test
    fun tinySizesPrioritizeCaloriesAndNormalSquareKeepsAllFour() {
        assertFalse(greenWidgetSizing(DpSize(48.dp, 48.dp), 1f).showNutrients)
        assertFalse(greenWidgetSizing(DpSize(230.dp, 48.dp), 1f).showNutrients)
        val square = greenWidgetSizing(DpSize(180.dp, 180.dp), 1f)
        assertTrue(square.showNutrients)
        assertFalse(square.primaryOnly)
        assertFalse(square.grid)
        val wide = greenWidgetSizing(DpSize(300.dp, 110.dp), 1f)
        assertTrue(wide.showNutrients)
        assertTrue(wide.grid)
        assertFalse(wide.primaryOnly)
    }

    @Test
    fun actualLauncherBoundsGrowTypographyAndLargeFontsReduceDensity() {
        val small = greenWidgetSizing(DpSize(110.dp, 110.dp), 1f)
        val roomy = greenWidgetSizing(DpSize(220.dp, 220.dp), 1f)
        assertTrue(roomy.heroSp > small.heroSp)
        assertTrue(roomy.valueSp > small.valueSp)
        assertTrue(roomy.railDp > small.railDp)
        assertTrue(greenWidgetSizing(DpSize(180.dp, 180.dp), 1.5f).primaryOnly)
        assertFalse(greenWidgetSizing(DpSize(110.dp, 110.dp), 2f).showNutrients)
    }

    @Test
    fun launcherSizesKeepCalorieEmphasisAndBudgetInsetsForShortSurfaces() {
        for (size in listOf(DpSize(48.dp, 48.dp), DpSize(180.dp, 180.dp), DpSize(300.dp, 110.dp))) {
            val sizing = greenWidgetSizing(size, 1f)
            assertTrue(sizing.calorieRailDp > sizing.railDp)
            assertTrue(sizing.paddingDp * 2 + sizing.calorieRailDp + sizing.heroGapDp < size.height.value)
        }
        val square = greenWidgetSizing(DpSize(180.dp, 180.dp), 1f)
        val short = greenWidgetSizing(DpSize(300.dp, 110.dp), 1f)
        assertTrue(square.paddingDp > short.paddingDp)
        assertTrue(square.paddingDp >= 10)
        assertTrue(short.showNutrients)
    }

    @Test
    fun singleAndDoubleCellWidgetsUseLargerCaloriesAndThickerRails() {
        val single = greenWidgetSizing(DpSize(80.dp, 80.dp), 1f)
        val double = greenWidgetSizing(DpSize(170.dp, 80.dp), 1f)
        assertTrue(single.heroSp >= 20)
        assertTrue(double.heroSp >= 30)
        assertTrue(single.calorieRailDp >= 7)
        assertTrue(double.calorieRailDp >= 7)
        assertFalse(single.showNutrients)
        assertFalse(double.showNutrients)
        assertTrue(double.heroSp > single.heroSp)
    }

    @Test
    fun wideSurfaceSpendsExtraRoomOnNutrientNumbersAndIcons() {
        val square = greenWidgetSizing(DpSize(180.dp, 180.dp), 1f)
        val wide = greenWidgetSizing(DpSize(380.dp, 180.dp), 1f)
        assertTrue(wide.valueSp > square.valueSp)
        assertTrue(wide.labelSp > square.labelSp)
        assertTrue(wide.grid)
        val short = greenWidgetSizing(DpSize(300.dp, 110.dp), 1f)
        assertTrue(short.valueSp <= wide.valueSp)
        assertTrue(short.showNutrients)
    }

    @Test
    fun sharedNumberColumnHandlesDifferentDigitsUnknownValuesAndFontScale() {
        val metrics = listOf(
            WidgetMetric(NutritionMetric.PROTEIN, 145.0, 128),
            WidgetMetric(NutritionMetric.FAT, 9.0, 89),
            WidgetMetric(NutritionMetric.FIBER, null, null),
        )
        val width = nutrientNumberWidth(metrics, 22, 1f)
        assertEquals(width, nutrientNumberWidth(metrics.reversed(), 22, 1f))
        assertEquals(width, nutrientNumberWidth(listOf(metrics[1]), 22, 1f))
        assertTrue(nutrientNumberWidth(metrics, 22, 1.5f) > width)
        assertTrue(nutrientNumberWidth(metrics + WidgetMetric(NutritionMetric.CARBS, 12345.0, 200), 22, 1f) > width)
    }

    @Test
    fun tallNarrowGridAndEnlargedFontsKeepTheCompleteRowWithinItsColumn() {
        for (size in listOf(DpSize(230.dp, 250.dp), DpSize(230.dp, 220.dp), DpSize(110.dp, 220.dp))) {
            for (scale in listOf(1f, 2f)) {
                for (characters in listOf(3, 5, 10)) {
                    val sizing = greenWidgetSizing(size, scale, characters)
                    val available = if (sizing.grid) (size.width.value - sizing.paddingDp * 2 - 10f) / 2f
                        else size.width.value - sizing.paddingDp * 2
                    if (sizing.showNutrients) {
                        assertTrue(nutrientRowWidth(sizing.valueSp, sizing.grid, scale, characters) <= available)
                    }
                }
            }
        }
        assertFalse(greenWidgetSizing(DpSize(110.dp, 220.dp), 2f).showNutrients)
        assertTrue(greenWidgetSizing(DpSize(230.dp, 220.dp), 2f).showNutrients)
    }
}
