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
 * for field. `lastLoggedAt` and `recommendation` are nullable because the
 * backend serializes them as JSON `null` (a day with no logged meals yet, and
 * every response today, respectively) — see SummaryTodayHandler.
 */
@Serializable
data class TodaySummary(
    val date: String,
    @SerialName("calories_consumed") val caloriesConsumed: Double,
    @SerialName("protein_grams_consumed") val proteinGramsConsumed: Double,
    @SerialName("carbs_grams_consumed") val carbsGramsConsumed: Double,
    @SerialName("fat_grams_consumed") val fatGramsConsumed: Double,
    @SerialName("meal_count") val mealCount: Int,
    @SerialName("last_logged_at") val lastLoggedAt: String? = null,
    @SerialName("display_language") val displayLanguage: String,
    val target: TodaySummaryTarget,
    val recommendation: String? = null,
)
