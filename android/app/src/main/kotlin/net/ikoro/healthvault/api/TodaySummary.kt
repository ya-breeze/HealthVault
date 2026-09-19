package net.ikoro.healthvault.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * Mirrors backend/pkg/server/summary_today.go's summaryTargetPayload field
 * for field. The four numeric fields are required (no default), matching the
 * backend's deliberate absence of `omitempty` on them: a target with
 * `available == false` still sends them as 0, and a client that made them
 * optional would let a *present* zero-valued target look partial. Only
 * `reason` is nullable, since the backend only sends it when unavailable.
 */
@Serializable
data class TodaySummaryTarget(
    val available: Boolean,
    val reason: String? = null,
    @SerialName("calories") val calories: Int,
    @SerialName("protein_grams") val proteinGrams: Int,
    @SerialName("carbs_grams") val carbsGrams: Int,
    @SerialName("fat_grams") val fatGrams: Int,
)

/**
 * Mirrors backend/pkg/server/summary_today.go's summaryTodayResponse field
 * for the fields this client consumes. `lastLoggedAt` is nullable because the
 * backend serializes it as JSON `null` on a day with no logged meals yet.
 */
@Serializable
data class TodaySummary(
    val date: String,
    @SerialName("calories_consumed") val caloriesConsumed: Double,
    @SerialName("protein_grams_consumed") val proteinGramsConsumed: Double,
    @SerialName("carbs_grams_consumed") val carbsGramsConsumed: Double,
    @SerialName("fat_grams_consumed") val fatGramsConsumed: Double,
    @SerialName("meal_count") val mealCount: Int,
    // Defaults keep an acceptance APK usable against an older server and let
    // an already-cached pre-pacing snapshot survive an app upgrade. A zero
    // usual-meal count deliberately disables pace judgement while retaining
    // neutral progress rails until the upgraded API is available.
    @SerialName("eating_occasions_today") val eatingOccasionsToday: Int = 0,
    @SerialName("usual_meals_per_day") val usualMealsPerDay: Int = 0,
    @SerialName("last_logged_at") val lastLoggedAt: String? = null,
    @SerialName("display_language") val displayLanguage: String,
    val target: TodaySummaryTarget,
)
