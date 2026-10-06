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
}
