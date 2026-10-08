package server

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func parseCookingContext(raw []byte) (*database.MealCookingContext, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var input struct {
		Homemade     *bool `json:"homemade"`
		LowAddedSalt *bool `json:"low_added_salt"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Homemade == nil || input.LowAddedSalt == nil {
		return nil, errors.New("invalid cooking context")
	}
	if !*input.Homemade && *input.LowAddedSalt {
		return nil, errors.New("low added salt requires homemade food")
	}
	return &database.MealCookingContext{Homemade: *input.Homemade, LowAddedSalt: *input.LowAddedSalt}, nil
}

func cookingContextGuidance(meal *database.FoodMeal) string {
	if meal.CookingContext == nil {
		return ""
	}
	context := "The user marked this meal as not homemade. Its cooking salt amount is unknown."
	if meal.CookingContext.Homemade {
		context = "The user marked this meal as homemade. Its added salt amount is unspecified."
		if meal.CookingContext.LowAddedSalt {
			context = "The user marked this meal as homemade with little salt added during cooking. Account for this when estimating sodium in homemade dishes; do not assume a normally salted restaurant or prepared-food recipe."
		}
	}
	return "Meal preparation context: " + context + " This is qualitative context, not a measured quantity or zero sodium. Retain intrinsic sodium and sodium from processed ingredients, bread, cheese, sauces, and labels. Explicit ingredient quantities, label values, and later user corrections override this general context."
}

func withCookingContext(meal *database.FoodMeal, text string) string {
	guidance := cookingContextGuidance(meal)
	if guidance == "" {
		return text
	}
	return guidance + "\n\n" + text
}
