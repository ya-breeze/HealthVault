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
		LowAddedSalt *bool `json:"low_added_salt"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.LowAddedSalt == nil {
		return nil, errors.New("invalid cooking context")
	}
	return &database.MealCookingContext{LowAddedSalt: *input.LowAddedSalt}, nil
}

func cookingContextGuidance(meal *database.FoodMeal) string {
	if meal.CookingContext == nil {
		return ""
	}
	context := "The amount of salt added during preparation is unspecified."
	if meal.CookingContext.LowAddedSalt {
		context = "The user says little salt was added during preparation. Account for this when estimating sodium from added salt; do not assume a normally salted recipe. This does not specify where the food was prepared."
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
