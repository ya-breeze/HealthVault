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

internal data class PaceInputs(
    val eatingOccasionsToday: Int,
    val usualMealsPerDay: Int,
)

private const val LEGACY_USUAL_MEALS_PER_DAY = 3

/**
 * Prefers the exact server-side Eating Occasion inputs. A pre-pacing server or
 * cache has no usable usual-meal value, so approximate with its raw meal count
 * and the domain default until the upgraded contract is available.
 */
internal fun resolvePaceInputs(
    eatingOccasionsToday: Int,
    usualMealsPerDay: Int,
    legacyMealCount: Int,
): PaceInputs = if (usualMealsPerDay > 0) {
    PaceInputs(eatingOccasionsToday.coerceAtLeast(0), usualMealsPerDay)
} else {
    PaceInputs(legacyMealCount.coerceAtLeast(0), LEGACY_USUAL_MEALS_PER_DAY)
}

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
