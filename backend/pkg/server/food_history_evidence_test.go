package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

type copiedNutritionEvidenceClient struct {
	vision.Fake
	day                   string
	contributors, details []byte
}

func (c *copiedNutritionEvidenceClient) NutritionChat(ctx context.Context, in vision.NutritionChatInput) (*vision.NutritionChatResult, error) {
	var err error
	c.contributors, err = in.HistoryTools.Execute(ctx, "explain_nutrition_signal", json.RawMessage(`{"signal":"fiber"}`))
	if err != nil {
		return nil, err
	}
	args, _ := json.Marshal(map[string]string{"date": c.day})
	c.details, err = in.HistoryTools.Execute(ctx, "get_day_details", args)
	if err != nil {
		return nil, err
	}
	return &vision.NutritionChatResult{Answer: "Recorded nutrition remains an estimate when its source was estimated."}, nil
}

func TestHistoryNutritionEvidence_PreservesOriginalUncertaintyAfterAcceptAndRepeat(t *testing.T) {
	for _, operation := range []string{"accept", "repeat"} {
		for _, origin := range []string{database.MacroSourceEstimated, database.MacroSourceManual} {
			t.Run(operation+"/"+origin, func(t *testing.T) {
				st := newFoodTestStorage(t)
				user, family := seedFoodUser(t, st)
				configureAdviceTarget(t, st, user, "en")
				source := seedHistorySource(t, st, user, family)
				if err := st.DB().Model(&database.FoodItem{}).Where("id = ?", source.Items[0].ID).Updates(map[string]any{"macro_source": origin, "confidence": .3}).Error; err != nil {
					t.Fatal(err)
				}
				yesterday := time.Now().UTC().AddDate(0, 0, -1)
				day := yesterday.Format("2006-01-02")
				client := &copiedNutritionEvidenceClient{day: day}
				client.RecognizeResult = &vision.RecognizeResult{Items: []vision.Item{{Name: "Vegetables", WeightGrams: 200, Confidence: .95, EstimatedProfile: &database.NutrientProfile{CaloriesPer100g: 100, DietaryFiberPer100g: 3}}}, HistoryMatch: &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}}
				h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(client, 10<<20, time.Second)
				w := httptest.NewRecorder()
				var target database.FoodMeal
				if operation == "accept" {
					h.CreateMeal(w, withClaims(newMealUploadRequest(t, "meal.jpg", fakeJPEGBytes), user))
					target = historyMeal(t, w, 201)
					w = httptest.NewRecorder()
					h.ResolveMealHistory(w, withClaims(historyRequest(target.ID, map[string]string{"action": "apply", "expected_version": target.HistoryVersion}), user))
					target = historyMeal(t, w, 200)
				} else {
					h.RepeatMeal(w, withClaims(historyRequest(source.ID, map[string]string{"request_id": uuid.NewString()}), user))
					target = historyMeal(t, w, 201)
				}
				w = httptest.NewRecorder()
				h.ConfirmMeal(w, withClaims(historyRequest(target.ID, map[string]any{"logged_at": yesterday}), user))
				target = historyMeal(t, w, http.StatusOK)
				// Make the single copied meal eligible evidence for yesterday.
				completion := database.FoodDayCompletion{UserID: user, LocalDate: day, ConfirmedAt: time.Now().UTC()}
				completion.ID = uuid.New()
				completion.FamilyID = family
				if err := st.DB().Create(&completion).Error; err != nil {
					t.Fatal(err)
				}
				phone := phoneSuccess(t, phoneCall(t, h.PhoneMCPHandler("test-phone-token", user.String(), nil), "get_food_meal", map[string]any{"meal_id": target.ID.String()}))
				item := phone["items"].([]any)[0].(map[string]any)
				if item["macro_source"] != origin || item["confidence"] != .3 || item["history_reused"] != true {
					t.Fatalf("personal MCP dropped original nutrition evidence: %+v", item)
				}
				body := chatBody("Which foods contributed fiber?", nil)
				w, response := callChat(t, h, newChatRequest(t, user, family, body))
				if w.Code != http.StatusOK || !response.Available {
					t.Fatalf("chat=%d %s", w.Code, w.Body.String())
				}
				if len(response.Sources) != 1 || response.Sources[0].MacroSource != origin || response.Sources[0].Confidence != .3 {
					t.Fatalf("Actionable Source promoted copied nutrition: %+v", response.Sources)
				}
				var contributors struct {
					Contributors []struct {
						MacroSource   string  `json:"macro_source"`
						Confidence    float64 `json:"confidence"`
						HistoryReused bool    `json:"history_reused"`
					}
				}
				if err := json.Unmarshal(client.contributors, &contributors); err != nil {
					t.Fatal(err)
				}
				if len(contributors.Contributors) != 1 || contributors.Contributors[0].MacroSource != origin || contributors.Contributors[0].Confidence != .3 || !contributors.Contributors[0].HistoryReused {
					t.Fatalf("model contributors lost origin: %s", client.contributors)
				}
				var details struct {
					Meals []struct {
						Items []struct {
							MacroSource   string  `json:"macro_source"`
							Confidence    float64 `json:"confidence"`
							HistoryReused bool    `json:"history_reused"`
						}
					}
				}
				if err := json.Unmarshal(client.details, &details); err != nil {
					t.Fatal(err)
				}
				if len(details.Meals) != 1 || len(details.Meals[0].Items) != 1 || details.Meals[0].Items[0].MacroSource != origin || details.Meals[0].Items[0].Confidence != .3 || !details.Meals[0].Items[0].HistoryReused {
					t.Fatalf("model day details lost origin: %s", client.details)
				}
			})
		}
	}
}
