package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

const lowSaltJSON = `{"low_added_salt":true}`

func assertCookingGuidance(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, "homemade") {
		t.Fatal("model guidance still infers homemade food")
	}
	for _, part := range []string{"little salt was added", "intrinsic sodium", "label values", "user corrections"} {
		if !strings.Contains(text, part) {
			t.Errorf("missing %q in model guidance: %q", part, text)
		}
	}
}

func TestCookingContext_PhotoPersistsAcrossRetryClarifyAndReanalyze(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{RecognizeErr: context.DeadlineExceeded}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateMeal(w, withClaims(newMealUploadRequest(t, "meal.jpg", fakeJPEGBytes, "chicken", lowSaltJSON), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created database.FoodMeal
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	var stored database.FoodMeal
	if err := st.DB().First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CookingContext == nil || !stored.CookingContext.LowAddedSalt {
		t.Fatalf("context did not survive database round trip: %+v", stored.CookingContext)
	}
	assertCookingGuidance(t, fake.RecognizeCalls[0].Hint)
	if !strings.HasSuffix(fake.RecognizeCalls[0].Hint, "chicken") {
		t.Fatal("upload hint lost")
	}
	fake.RecognizeErr = nil
	fake.RecognizeResult = &vision.RecognizeResult{Items: []vision.Item{{Name: "Chicken", WeightGrams: 200}}, ClarificationQuestions: []string{"Which sauce?"}}
	r := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/retry", nil), map[string]string{"id": created.ID.String()})
	w = httptest.NewRecorder()
	h.RetryMeal(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	assertCookingGuidance(t, fake.RecognizeCalls[1].Hint)
	fake.ClarifyResult = &vision.RecognizeResult{Items: []vision.Item{{Name: "Chicken", WeightGrams: 200}}}
	w = httptest.NewRecorder()
	h.ClarifyMeal(w, withClaims(clarifyRequest(created.ID.String(), []string{"No sauce"}), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("clarify: %d %s", w.Code, w.Body.String())
	}
	call := fake.ClarifyCalls[0]
	if call.Description != "" {
		t.Fatal("photo clarification was converted to description framing")
	}
	if len(call.History) != 2 || call.History[1].Answer != "No sauce" {
		t.Fatalf("wrong history: %+v", call.History)
	}
	assertCookingGuidance(t, call.History[0].Answer)
	fake.RecognizeResult = &vision.RecognizeResult{Items: []vision.Item{{Name: "Chicken", WeightGrams: 200}}}
	w = httptest.NewRecorder()
	h.Reanalyze(w, withClaims(reanalyzeHTTPRequest(created.ID.String(), "Actually two grams of salt were added"), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("reanalyze: %d %s", w.Code, w.Body.String())
	}
	assertCookingGuidance(t, fake.RecognizeCalls[2].Hint)
	if !strings.HasSuffix(fake.RecognizeCalls[2].Hint, "Actually two grams of salt were added") {
		t.Fatal("explicit correction lost")
	}
	if err := st.DB().First(&stored, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CookingContext == nil || !stored.CookingContext.LowAddedSalt {
		t.Fatal("lifecycle lost persisted context")
	}
}

func TestCookingContext_DescriptionPersistsAndRetryUsesContext(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, _ := seedFoodUser(t, st)
	fake := &vision.Fake{DescribeErr: context.DeadlineExceeded}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	w := httptest.NewRecorder()
	h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(map[string]any{"description": "  chicken  ", "cooking_context": json.RawMessage(lowSaltJSON)}), userID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var meal database.FoodMeal
	if err := json.Unmarshal(w.Body.Bytes(), &meal); err != nil {
		t.Fatal(err)
	}
	if meal.Description != "chicken" {
		t.Fatal("context altered original description")
	}
	assertCookingGuidance(t, fake.DescribeCalls[0].Description)
	fake.DescribeErr = nil
	r := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/retry", nil), map[string]string{"id": meal.ID.String()})
	w = httptest.NewRecorder()
	h.RetryMeal(w, withClaims(r, userID))
	if w.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	assertCookingGuidance(t, fake.DescribeCalls[1].Description)
	if err := st.DB().First(&meal, "id = ?", meal.ID).Error; err != nil {
		t.Fatal(err)
	}
	if meal.Description != "chicken" || meal.CookingContext == nil {
		t.Fatal("stored description/context changed")
	}
}

func TestCookingContext_InvalidInputHasNoSideEffects(t *testing.T) {
	for _, raw := range []string{`{`, `{}`, `true`, `{"low_added_salt":"yes"}`, `{"low_added_salt":null}`} {
		t.Run(raw, func(t *testing.T) {
			st := newFoodTestStorage(t)
			userID, _ := seedFoodUser(t, st)
			dir := t.TempDir()
			fake := &vision.Fake{}
			h := server.NewFoodHandlers(st, nil, dir).WithVision(fake, 10<<20, time.Second)
			w := httptest.NewRecorder()
			h.CreateMeal(w, withClaims(newMealUploadRequest(t, "meal.jpg", fakeJPEGBytes, "", raw), userID))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("upload accepted invalid context: %d", w.Code)
			}
			w = httptest.NewRecorder()
			h.CreateDescribedMeal(w, withClaims(describeMealRawRequest([]byte(`{"description":"chicken","cooking_context":`+raw+`}`)), userID))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("description accepted invalid context: %d", w.Code)
			}
			var count int64
			st.DB().Model(&database.FoodMeal{}).Count(&count)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 || len(entries) != 0 || len(fake.RecognizeCalls) != 0 || len(fake.DescribeCalls) != 0 {
				t.Fatal("invalid context caused side effects")
			}
		})
	}
}

func TestCookingContext_UnknownAndExplicitFalseRemainDistinct(t *testing.T) {
	for _, c := range []*database.MealCookingContext{nil, {LowAddedSalt: false}} {
		st := newFoodTestStorage(t)
		userID, _ := seedFoodUser(t, st)
		fake := &vision.Fake{}
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
		body := map[string]any{"description": "chicken"}
		if c != nil {
			body["cooking_context"] = c
		}
		w := httptest.NewRecorder()
		h.CreateDescribedMeal(w, withClaims(describeMealHTTPRequest(body), userID))
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var meal database.FoodMeal
		json.Unmarshal(w.Body.Bytes(), &meal)
		if err := st.DB().First(&meal, "id = ?", meal.ID).Error; err != nil {
			t.Fatal(err)
		}
		if c == nil {
			if meal.CookingContext != nil || fake.DescribeCalls[0].Description != "chicken" {
				t.Fatal("missing context was invented")
			}
		} else {
			if meal.CookingContext == nil || *meal.CookingContext != *c {
				t.Fatalf("false flags lost: %+v", meal.CookingContext)
			}
			if strings.Contains(fake.DescribeCalls[0].Description, "little salt was added") {
				t.Fatal("unchecked low salt was treated as low salt")
			}
		}
	}
}
