package server

import (
	"errors"
	"strings"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
)

func TestRedactQuestion(t *testing.T) {
	question := "why does it say my sodium is high?"

	t.Run("removes the question a provider echoed back", func(t *testing.T) {
		err := errors.New(`openai: 400 invalid request for prompt "` + question + `"`)
		got := redactQuestion(err, question)
		if strings.Contains(got, question) {
			t.Fatalf("the question survived redaction: %q", got)
		}
		if !strings.Contains(got, "<question>") || !strings.Contains(got, "400 invalid request") {
			t.Fatalf("redaction lost the diagnosable part of the error: %q", got)
		}
	})

	t.Run("bounds a provider error carrying a whole response body", func(t *testing.T) {
		got := redactQuestion(errors.New(strings.Repeat("я", 5000)), question)
		if len([]rune(got)) != nutritionChatMaxLoggedErrorRune+1 {
			t.Fatalf("expected a bounded error plus an ellipsis, got %d runes", len([]rune(got)))
		}
	})

	t.Run("leaves an unrelated error alone", func(t *testing.T) {
		if got := redactQuestion(errors.New("context deadline exceeded"), question); got != "context deadline exceeded" {
			t.Fatalf("an unrelated error was altered: %q", got)
		}
	})

	t.Run("does not blank the error when there is no question to redact", func(t *testing.T) {
		if got := redactQuestion(errors.New("model down"), ""); got != "model down" {
			t.Fatalf("empty question redacted the whole error: %q", got)
		}
	})
}

// saturated_fat must be wired into the same three seams fiber was: the
// explain_nutrition_signal allowlist, and both meal/item value lookups.
func TestNutritionSignalValue_SaturatedFat(t *testing.T) {
	if !nutritionHistorySignals["saturated_fat"] {
		t.Fatal("expected saturated_fat to be an accepted explain_nutrition_signal signal")
	}
	meal := database.FoodMeal{SaturatedFatGrams: 7.5}
	if got := nutritionSignalMealValue(meal, "saturated_fat"); got != 7.5 {
		t.Errorf("nutritionSignalMealValue(saturated_fat) = %v, want 7.5", got)
	}
	item := database.FoodItem{SaturatedFatGrams: 3.25}
	if got := nutritionSignalItemValue(item, "saturated_fat"); got != 3.25 {
		t.Errorf("nutritionSignalItemValue(saturated_fat) = %v, want 3.25", got)
	}
}
