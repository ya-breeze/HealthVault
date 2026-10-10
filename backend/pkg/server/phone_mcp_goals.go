package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
)

type phoneGoalsInput struct{}

type phoneMacroGoals struct {
	Available         bool                   `json:"available"`
	UnavailableReason string                 `json:"unavailable_reason,omitempty"`
	Source            string                 `json:"source"`
	Values            *nutritionTargetValues `json:"values"`
	ActivitySource    string                 `json:"activity_source,omitempty"`
}

type phoneFiberGoal struct {
	Grams  *int   `json:"grams"`
	Source string `json:"source"`
}

type phoneGoalsResult struct {
	AsOf             time.Time       `json:"as_of"`
	LocalDate        string          `json:"local_date"`
	Timezone         string          `json:"timezone"`
	TimezoneFallback bool            `json:"timezone_fallback"`
	Nutrition        phoneMacroGoals `json:"nutrition"`
	Fiber            phoneFiberGoal  `json:"fiber"`
	Notes            []string        `json:"notes"`
}

var phoneGoalsNotes = []string{
	"These are current HealthVault application targets calculated on read, not historical targets. A comparison with past intake uses today's reference and cannot establish the target that applied then.",
	"Calories are BMR times the activity multiplier, not a prescribed calorie deficit. Goal weight determines protein and the fat floor. Activity inference uses the existing nutrition-target algorithm, not the activity MCP period totals.",
	"Calories and BMR are kcal per day; protein, carbs, fat and fiber are grams per day. Basis weights are kg, height is meters, age is completed years, and activity multiplier is dimensionless.",
	"Unavailable nutrition values are null, not zero. Fiber is independent: configured is an explicit setting, default is the application's 25 grams (also with a missing profile), and unavailable_under_18 has no default. Do not describe a default as the user's configured target.",
	"This tool exposes no goals for sodium, sugar or saturated fat. Do not invent saved goals or assume today's targets applied to past days.",
	"Timezone follows the existing nutrition-target endpoint. Legacy Local uses the server's local timezone; with timezone_fallback true, configure an IANA timezone before assuming its date matches the food/activity MCP dates.",
}

func (h *foodHandlers) addPhoneGoalsTool(s *mcp.Server, id uuid.UUID, barrier *sync.RWMutex) {
	closed := false
	read := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}
	mcp.AddTool(s, &mcp.Tool{Name: "get_nutrition_goals", Description: "Read the connected user's current HealthVault calorie, protein, carbohydrate, fat and independent fiber targets. No parameters. Returns calculation basis, units, local date/timezone, configured/default fiber source and explicit missing-data reasons. Current targets are not historical targets or a prescribed calorie deficit; read notes before comparing with food history.", Annotations: read}, func(_ context.Context, _ *mcp.CallToolRequest, _ phoneGoalsInput) (*mcp.CallToolResult, phoneGoalsResult, error) {
		barrier.RLock()
		defer barrier.RUnlock()
		if _, err := h.storage.FindUserByID(id); err != nil {
			return phoneReadError(errors.New("connected user unavailable")), phoneGoalsResult{}, nil
		}
		settings, err := readUserSettingsJSON(h.storage, id)
		if err != nil {
			return phoneReadError(errors.New("connected user settings unavailable")), phoneGoalsResult{}, nil
		}
		now := time.Now().UTC()
		// Keep the HTTP target's timezone semantics, including legacy Local.
		loc := database.ResolveTimezone(settings)
		storedZone := database.SettingsRawString(settings, "timezone")
		_, zoneErr := time.LoadLocation(storedZone)
		profile := parseUserProfile(settings)
		values, reason, err := computeNutritionTargetForProfile(h.storage, id, now, loc, profile)
		if err != nil {
			return phoneReadError(errors.New("nutrition goals unavailable")), phoneGoalsResult{}, nil
		}
		out := phoneGoalsResult{
			AsOf: now, LocalDate: database.LocalDate(now, loc), Timezone: loc.String(),
			TimezoneFallback: storedZone == "" || storedZone == "Local" || zoneErr != nil,
			Nutrition:        phoneMacroGoals{Available: reason == "", UnavailableReason: reason, Source: "computed_current"},
			Notes:            phoneGoalsNotes,
		}
		if reason == "" {
			out.Nutrition.Values = &values
			out.Nutrition.ActivitySource = "inferred_steps"
			if profile.HasActivityOverride {
				out.Nutrition.ActivitySource = "configured_override"
			}
		}
		out.Fiber.Grams, out.Fiber.Source = resolveFiberTarget(settings, now)
		return nil, out, nil
	})
}
