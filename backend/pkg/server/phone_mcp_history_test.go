package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func TestPhoneHistoryTotalsPaginationAndReadOnly(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(owner, family, `{"timezone":"Europe/Prague","usual_meals_per_day":3}`); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Prague")
	// March 29 is a 23-hour DST day. UTC March 28 23:30 belongs to it.
	logged := time.Date(2026, 3, 28, 23, 30, 0, 0, time.UTC)
	create := func(user uuid.UUID, status string, stamp time.Time, calories float64) database.FoodMeal {
		m := database.FoodMeal{UserID: user, Status: status, LoggedAt: stamp, Name: "Soup", Calories: calories, ProteinGrams: 10, SodiumGrams: 0.1}
		m.ID, m.FamilyID = uuid.New(), family
		if err := st.DB().Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		return m
	}
	m1 := create(owner, database.MealStatusConfirmed, logged, 200)
	m2 := create(owner, database.MealStatusConfirmed, logged, 300)
	create(owner, database.MealStatusPendingReview, logged, 999)
	create(uuid.New(), database.MealStatusConfirmed, logged, 9999)
	create(owner, database.MealStatusConfirmed, time.Date(2026, 3, 29, 22, 0, 0, 0, time.UTC), 777) // next local day
	deleted := create(owner, database.MealStatusConfirmed, logged, 555)
	if err := st.DB().Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{database.MacroSourceNone, database.MacroSourceEstimated} {
		it := database.FoodItem{UserID: owner, MealID: m1.ID, Name: "Ingredient", MacroSource: source}
		it.ID, it.FamilyID = uuid.New(), family
		if err := st.DB().Create(&it).Error; err != nil {
			t.Fatal(err)
		}
	}
	stale := database.FoodDayCompletion{UserID: owner, LocalDate: "2026-03-28", ConfirmedAt: time.Now()}
	stale.ID, stale.FamilyID = uuid.New(), family
	if err := st.DB().Create(&stale).Error; err != nil {
		t.Fatal(err)
	}
	food := server.NewFoodHandlers(st, nil, t.TempDir())
	h := food.PhoneMCPHandler("test-phone-token", owner.String(), nil)
	args := map[string]any{"period": "range", "start_date": "2026-03-29", "end_date": "2026-03-29", "limit": 1}
	seen := map[string]bool{}
	for {
		out := phoneSuccess(t, phoneCall(t, h, "list_food_meals", args))
		if out["timezone"] != loc.String() {
			t.Fatalf("wrong zone: %+v", out)
		}
		rows := out["meals"].([]any)
		if len(rows) != 1 {
			t.Fatalf("page rows: %+v", rows)
		}
		m := rows[0].(map[string]any)
		id := m["meal_id"].(string)
		if seen[id] {
			t.Fatal("duplicate tied-timestamp meal")
		}
		seen[id] = true
		if m["confirmed"] == false && len(m["known_totals"].(map[string]any)) != 0 {
			t.Fatal("draft included in known totals")
		}
		if out["has_more"] == false {
			if out["next_cursor"] != "" {
				t.Fatal("unexpected terminal cursor")
			}
			break
		}
		args["cursor"] = out["next_cursor"]
	}
	if len(seen) != 3 || !seen[m1.ID.String()] || !seen[m2.ID.String()] {
		t.Fatalf("lost or foreign meals: %+v", seen)
	}
	daily := phoneSuccess(t, phoneCall(t, h, "get_food_daily_totals", map[string]any{"period": "range", "start_date": "2026-03-28", "end_date": "2026-03-29"}))
	days := daily["days"].([]any)
	empty := days[0].(map[string]any)
	day := days[1].(map[string]any)
	if empty["recorded_meals"] != float64(0) || empty["completeness"] != database.DayStateIncomplete {
		t.Fatalf("empty day: %+v", empty)
	}
	emptyList := phoneSuccess(t, phoneCall(t, h, "list_food_meals", map[string]any{"period": "range", "start_date": "2026-03-28", "end_date": "2026-03-28"}))
	if len(emptyList["meals"].([]any)) != 0 || emptyList["has_more"] != false {
		t.Fatal("empty period not empty")
	}
	if day["calories"] != float64(500) || day["unconfirmed_meals"] != float64(1) || day["confirmed_meals"] != float64(2) || day["unknown_confirmed_items"] != float64(1) || day["estimated_confirmed_items"] != float64(1) {
		t.Fatalf("incorrect daily evidence: %+v", day)
	}
	if day["occasion_count"] != float64(1) || day["completeness_basis"] != "insufficient_logging" {
		t.Fatalf("incorrect completeness: %+v", day)
	}
	confirmation := database.FoodDayCompletion{UserID: owner, LocalDate: "2026-03-29", ConfirmedAt: time.Now()}
	confirmation.ID, confirmation.FamilyID = uuid.New(), family
	if err := st.DB().Create(&confirmation).Error; err != nil {
		t.Fatal(err)
	}
	confirmedDay := phoneSuccess(t, phoneCall(t, h, "get_food_daily_totals", map[string]any{"period": "range", "start_date": "2026-03-29", "end_date": "2026-03-29"}))["days"].([]any)[0].(map[string]any)
	if confirmedDay["completeness_basis"] != "owner_assertion" {
		t.Fatalf("owner assertion missing: %+v", confirmedDay)
	}
	if !strings.Contains(daily["notes"].([]any)[0].(string), "kcal") {
		t.Fatal("energy units missing")
	}
	// Reading an empty day must not trigger DayRange's historical cleanup.
	var count int64
	if err := st.DB().Model(&database.FoodDayCompletion{}).Where("id = ?", stale.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("read deleted stale confirmation")
	}
	create(owner, database.MealStatusConfirmed, time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC), 100)
	later := phoneSuccess(t, phoneCall(t, h, "get_food_daily_totals", map[string]any{"period": "range", "start_date": "2026-03-28", "end_date": "2026-03-28"}))["days"].([]any)[0].(map[string]any)
	if later["completeness_basis"] != "owner_assertion" {
		t.Fatal("stored owner assertion disagrees with established HealthVault semantics")
	}
	var stored database.FoodMeal
	if err := st.DB().First(&stored, "id = ?", m1.ID).Error; err != nil || stored.Calories != 200 || stored.Status != database.MealStatusConfirmed {
		t.Fatal("read changed meal")
	}
	// A changed period must reject a prior opaque cursor rather than omit records.
	args["end_date"] = "2026-03-30"
	if phoneCall(t, h, "list_food_meals", args)["isError"] != true {
		t.Fatal("changed-period cursor accepted")
	}
}

func TestPhoneHistoryTodayCompletenessAndValidation(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	if err := st.UpsertUserSettings(owner, family, `{"timezone":"UTC","usual_meals_per_day":1}`); err != nil {
		t.Fatal(err)
	}
	m := database.FoodMeal{UserID: owner, Status: database.MealStatusConfirmed, LoggedAt: time.Now().UTC(), Calories: 123}
	m.ID, m.FamilyID = uuid.New(), family
	if err := st.DB().Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	out := phoneSuccess(t, phoneCall(t, h, "get_food_daily_totals", map[string]any{"period": "today"}))
	day := out["days"].([]any)[0].(map[string]any)
	if day["partial_today"] != true || day["calories"] != float64(123) || day["completeness_basis"] != "not_evaluated_today" || day["completeness"] != nil {
		t.Fatalf("today silently clamped or wrong: %+v", out)
	}
	for _, tool := range []string{"get_food_daily_totals", "list_food_meals"} {
		for _, in := range []map[string]any{
			{"period": "range", "start_date": "2026-01-01", "end_date": "2026-05-01"},
			{"period": "range", "start_date": "2026-03-30", "end_date": "2026-03-29"},
			{"period": "range", "start_date": "invalid", "end_date": "2026-03-29"},
			{"period": "range", "start_date": "2099-01-01", "end_date": "2099-01-02"},
			{"period": "today", "start_date": "2026-01-01"},
			{"period": "bad"},
		} {
			if phoneCall(t, h, tool, in)["isError"] != true {
				t.Fatalf("%s accepted %+v", tool, in)
			}
		}
	}
	if phoneCall(t, h, "list_food_meals", map[string]any{"limit": 21})["isError"] != true {
		t.Fatal("unbounded list")
	}
	if phoneCall(t, h, "list_food_meals", map[string]any{"cursor": "bad"})["isError"] != true {
		t.Fatal("invalid cursor")
	}
	// Missing configured identity must also block the newly added read tools.
	missing := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", uuid.NewString(), nil)
	for _, tool := range []string{"get_food_daily_totals", "list_food_meals", "get_activity_daily_totals", "list_activity_exercises", "get_nutrition_goals"} {
		if phoneCall(t, missing, tool, map[string]any{})["isError"] != true {
			t.Fatal("missing identity accepted")
		}
	}
}

func TestPhoneHistoryDiscoveryAndSelectorSchema(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, _ := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	for _, method := range []string{"initialize", "tools/list"} {
		payload := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{}}`
		if method == "initialize" {
			payload = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
		}
		r := httptest.NewRequest(http.MethodPost, "/phone-mcp", strings.NewReader(payload))
		r.Header.Set("Authorization", "Bearer test-phone-token")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s HTTP%d: %s", method, w.Code, w.Body.String())
		}
		if method == "tools/list" {
			body := w.Body.String()
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "data: ") {
					body = strings.TrimPrefix(line, "data: ")
					break
				}
			}
			var reply struct {
				Result struct {
					Tools []struct {
						Name        string         `json:"name"`
						InputSchema map[string]any `json:"inputSchema"`
						Annotations map[string]any `json:"annotations"`
					} `json:"tools"`
				} `json:"result"`
			}
			if err := json.Unmarshal([]byte(body), &reply); err != nil {
				t.Fatal(err)
			}
			if len(reply.Result.Tools) != 10 {
				t.Fatalf("tools: %s", body)
			}
			for _, tool := range reply.Result.Tools {
				if tool.Name == "get_nutrition_goals" {
					props, _ := tool.InputSchema["properties"].(map[string]any)
					if len(props) != 0 || tool.Annotations["readOnlyHint"] != true || tool.Annotations["idempotentHint"] != true || tool.Annotations["openWorldHint"] != false {
						t.Fatalf("goals schema/annotations: %+v", tool)
					}
				}
				if tool.Name == "list_food_meals" || tool.Name == "get_food_daily_totals" || tool.Name == "get_activity_daily_totals" || tool.Name == "list_activity_exercises" {
					props := tool.InputSchema["properties"].(map[string]any)
					if _, ok := props["period"]; !ok {
						t.Fatal("embedded period absent from schema")
					}
					for _, key := range []string{"user_id", "family_id"} {
						if _, ok := props[key]; ok {
							t.Fatal("selector in schema")
						}
					}
					if tool.Annotations["readOnlyHint"] != true {
						t.Fatal("read-only annotation missing")
					}
				}
			}
		}
	}
}
