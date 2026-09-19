package net.ikoro.healthvault.widget

import kotlin.math.abs
import kotlin.math.roundToInt

internal enum class PaceLevel {
    GOOD,
    WARNING,
    BAD,
}

internal enum class PaceDirection(val glyph: String) {
    ON_PACE("✓"),
    BELOW("↓"),
    ABOVE("↑"),
}

internal data class PaceSignal(
    val actualPercent: Int,
    val expectedPercent: Int,
    val level: PaceLevel,
    val direction: PaceDirection,
)

/**
 * Compares one consumed daily metric with the share expected after the
 * caller's current eating occasion. Ten percentage points is on pace,
 * twenty is nearby, and anything further away is deliberately conspicuous.
 */
internal fun paceSignal(
    consumed: Double,
    target: Int,
    eatingOccasionsToday: Int,
    usualMealsPerDay: Int,
): PaceSignal? {
    if (target <= 0 || usualMealsPerDay <= 0) return null

    val actual = consumed.coerceAtLeast(0.0) * 100.0 / target
    val expected = eatingOccasionsToday.coerceAtLeast(0)
        .toDouble()
        .div(usualMealsPerDay)
        .coerceIn(0.0, 1.0) * 100.0
    val deviation = actual - expected
    val level = when {
        abs(deviation) <= 10.0 -> PaceLevel.GOOD
        abs(deviation) <= 20.0 -> PaceLevel.WARNING
        else -> PaceLevel.BAD
    }
    val direction = when {
        level == PaceLevel.GOOD -> PaceDirection.ON_PACE
        deviation < 0.0 -> PaceDirection.BELOW
        else -> PaceDirection.ABOVE
    }

    return PaceSignal(
        actualPercent = actual.roundToInt(),
        expectedPercent = expected.roundToInt(),
        level = level,
        direction = direction,
    )
}
