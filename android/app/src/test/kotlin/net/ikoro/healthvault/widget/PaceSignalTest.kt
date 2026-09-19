package net.ikoro.healthvault.widget

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PaceSignalTest {

    @Test
    fun `exact server pace inputs win over the legacy meal count`() {
        val inputs = resolvePaceInputs(eatingOccasionsToday = 2, usualMealsPerDay = 4, legacyMealCount = 7)

        assertEquals(2, inputs.eatingOccasionsToday)
        assertEquals(4, inputs.usualMealsPerDay)
    }

    @Test
    fun `legacy summary falls back to raw meal count and the three meal default`() {
        val inputs = resolvePaceInputs(eatingOccasionsToday = 0, usualMealsPerDay = 0, legacyMealCount = 2)

        assertEquals(2, inputs.eatingOccasionsToday)
        assertEquals(3, inputs.usualMealsPerDay)
    }

    @Test
    fun `three meals make the expected shares 33 67 and 100 percent`() {
        assertEquals(33, paceSignal(660.0, 2000, 1, 3)?.expectedPercent)
        assertEquals(67, paceSignal(1340.0, 2000, 2, 3)?.expectedPercent)
        assertEquals(100, paceSignal(2000.0, 2000, 3, 3)?.expectedPercent)
        assertEquals(100, paceSignal(2000.0, 2000, 4, 3)?.expectedPercent)
    }

    @Test
    fun `ten points is green and twenty points is amber`() {
        assertEquals(PaceLevel.GOOD, paceSignal(1200.0, 2000, 1, 2)?.level)
        assertEquals(PaceLevel.WARNING, paceSignal(1400.0, 2000, 1, 2)?.level)
        assertEquals(PaceLevel.WARNING, paceSignal(1200.2, 2000, 1, 2)?.level)
        assertEquals(PaceLevel.BAD, paceSignal(1400.2, 2000, 1, 2)?.level)
    }

    @Test
    fun `larger deviations are bright red with a direction`() {
        val below = paceSignal(200.0, 2000, 1, 3)
        val above = paceSignal(1200.0, 2000, 1, 3)

        assertEquals(PaceLevel.BAD, below?.level)
        assertEquals(PaceDirection.BELOW, below?.direction)
        assertEquals("↓", below?.direction?.glyph)
        assertEquals(PaceLevel.BAD, above?.level)
        assertEquals(PaceDirection.ABOVE, above?.direction)
        assertEquals("↑", above?.direction?.glyph)
    }

    @Test
    fun `over target percentage stays truthful while progress remains separately clamped`() {
        val signal = paceSignal(2198.0, 2139, 3, 3)

        assertEquals(103, signal?.actualPercent)
        assertEquals(1f, progressFraction(2198, 2139), 0f)
        assertEquals(PaceLevel.GOOD, signal?.level)
        assertEquals(PaceDirection.ON_PACE, signal?.direction)
    }

    @Test
    fun `missing targets cannot produce a pace judgment`() {
        assertNull(paceSignal(100.0, 0, 1, 3))
        assertNull(paceSignal(100.0, 2000, 1, 0))
    }
}
