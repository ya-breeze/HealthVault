package net.ikoro.healthvault.api

import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Mirrors backend/pkg/server/summary_today.go's summaryTodayResponse field for field. */
class TodaySummaryParsingTest {

    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `an available target parses with its four numbers`() {
        val body = """
            {
              "date": "2026-09-02",
              "calories_consumed": 1200.5,
              "protein_grams_consumed": 80.0,
              "carbs_grams_consumed": 120.0,
              "fat_grams_consumed": 40.0,
              "meal_count": 3,
              "last_logged_at": "2026-09-02T14:05:00Z",
              "display_language": "en",
              "target": {"available": true, "calories": 2000, "protein_grams": 150, "carbs_grams": 200, "fat_grams": 70},
              "recommendation": null
            }
        """.trimIndent()
        val summary = json.decodeFromString<TodaySummary>(body)

        assertTrue(summary.target.available)
        assertEquals(2000, summary.target.calories)
        assertEquals(150, summary.target.proteinGrams)
        assertEquals("2026-09-02T14:05:00Z", summary.lastLoggedAt)
        assertNull(summary.recommendation)
        assertEquals(3, summary.mealCount)
    }

    @Test
    fun `each unavailability reason parses with available false`() {
        val reasons = listOf(
            "missing_profile", "missing_measurements", "missing_goal_weight", "insufficient_activity_data",
        )
        for (reason in reasons) {
            val body = """
                {
                  "date": "2026-09-02", "calories_consumed": 0, "protein_grams_consumed": 0,
                  "carbs_grams_consumed": 0, "fat_grams_consumed": 0, "meal_count": 0,
                  "last_logged_at": null, "display_language": "en",
                  "target": {"available": false, "reason": "$reason", "calories": 0, "protein_grams": 0, "carbs_grams": 0, "fat_grams": 0},
                  "recommendation": null
                }
            """.trimIndent()
            val summary = json.decodeFromString<TodaySummary>(body)
            assertFalse("expected unavailable for $reason", summary.target.available)
            assertEquals(reason, summary.target.reason)
        }
    }

    @Test
    fun `a zero-valued but available target is not mistaken for absent`() {
        // computeNutritionTarget clamps carbs to zero whenever protein and
        // the fat floor already exhaust the calorie budget — see
        // summaryTargetPayload's own comment. This must decode as a present,
        // available target with carbsGrams == 0, not as missing data.
        val body = """
            {
              "date": "2026-09-02", "calories_consumed": 500, "protein_grams_consumed": 40,
              "carbs_grams_consumed": 0, "fat_grams_consumed": 30, "meal_count": 1,
              "last_logged_at": "2026-09-02T08:00:00Z", "display_language": "en",
              "target": {"available": true, "calories": 1800, "protein_grams": 160, "carbs_grams": 0, "fat_grams": 60},
              "recommendation": null
            }
        """.trimIndent()
        val summary = json.decodeFromString<TodaySummary>(body)

        assertTrue(summary.target.available)
        assertEquals(0, summary.target.carbsGrams)
    }

    @Test
    fun `a null last_logged_at parses as null, not a crash`() {
        val body = """
            {
              "date": "2026-09-02", "calories_consumed": 0, "protein_grams_consumed": 0,
              "carbs_grams_consumed": 0, "fat_grams_consumed": 0, "meal_count": 0,
              "last_logged_at": null, "display_language": "en",
              "target": {"available": false, "reason": "missing_profile", "calories": 0, "protein_grams": 0, "carbs_grams": 0, "fat_grams": 0},
              "recommendation": null
            }
        """.trimIndent()
        val summary = json.decodeFromString<TodaySummary>(body)
        assertNull(summary.lastLoggedAt)
    }
}
