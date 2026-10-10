package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func TestPhoneNutritionGoalsParityIsolationAndFreshSettings(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	profile := `{"birthdate":"1990-01-01","sex":"male","activity_override":"moderate","timezone":"Europe/Prague","fiber_target_grams":31}`
	setProfile(t, st, owner, profile)
	for _, r := range []struct {
		kind  string
		value float64
	}{{"weight", 80}, {"height", 1.8}, {"weight_goal", 75}} {
		createRecord(t, st, owner, r.kind, r.value)
	}
	for _, user := range []uuid.UUID{owner, uuid.New()} {
		ts := time.Now().UTC()
		if user == owner {
			ts = ts.AddDate(0, 1, 0)
		}
		insertFutureRecord(t, st, user, family, "weights", "time", "kilograms", ts, 999)
		insertFutureRecord(t, st, user, family, "heights", "time", "meters", ts, 9)
		insertFutureRecord(t, st, user, family, "weight_goals", "time", "kilograms", ts, 999)
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	assertParity := func() map[string]any {
		t.Helper()
		out := phoneSuccess(t, phoneCall(t, h, "get_nutrition_goals", map[string]any{}))
		nutrition := out["nutrition"].(map[string]any)
		w := httptest.NewRecorder()
		server.NutritionTargetHandler(st).ServeHTTP(w, newNutritionTargetRequest(owner))
		if w.Code != http.StatusOK {
			t.Fatalf("target: %d %s", w.Code, w.Body.String())
		}
		var httpValues map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &httpValues); err != nil {
			t.Fatal(err)
		}
		if nutrition["available"] != true || !reflect.DeepEqual(httpValues, nutrition["values"]) {
			t.Fatalf("HTTP/MCP mismatch: %+v / %+v", httpValues, nutrition)
		}
		if nutrition["activity_source"] != "configured_override" || httpValues["measured_weight_kg"] != float64(80) || httpValues["goal_weight_kg"] != float64(75) {
			t.Fatalf("owner/future isolation: %+v", nutrition)
		}
		stamp, err := time.Parse(time.RFC3339Nano, out["as_of"].(string))
		if err != nil {
			t.Fatal(err)
		}
		loc, _ := time.LoadLocation("Europe/Prague")
		if out["local_date"] != stamp.In(loc).Format("2006-01-02") || out["timezone"] != "Europe/Prague" || out["timezone_fallback"] != false {
			t.Fatalf("timezone: %+v", out)
		}
		body, _ := json.Marshal(out)
		for _, private := range []string{"birthdate", "user_id", "family_id", owner.String(), "1990-01-01"} {
			if strings.Contains(string(body), private) {
				t.Fatalf("private field %s: %s", private, body)
			}
		}
		return out
	}
	before, _ := st.GetUserSettings(owner)
	snapshot := func() string {
		t.Helper()
		all := map[string][]map[string]any{}
		for _, table := range []string{"weights", "heights", "weight_goals", "food_advices", "food_advice_engagements", "food_meals"} {
			var rows []map[string]any
			if err := st.DB().Table(table).Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			all[table] = rows
		}
		body, err := json.Marshal(all)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	rowsBefore := snapshot()
	first := assertParity()
	if first["fiber"].(map[string]any)["grams"] != float64(31) || first["fiber"].(map[string]any)["source"] != "configured" {
		t.Fatalf("fiber: %+v", first)
	}
	after, _ := st.GetUserSettings(owner)
	if before != after {
		t.Fatal("read changed settings")
	}
	if rowsBefore != snapshot() {
		t.Fatal("read changed records or advice caches")
	}
	setProfile(t, st, owner, strings.ReplaceAll(strings.ReplaceAll(profile, "moderate", "light"), "31", "40"))
	second := assertParity()
	if reflect.DeepEqual(first["nutrition"], second["nutrition"]) || second["fiber"].(map[string]any)["grams"] != float64(40) {
		t.Fatal("stale cached goals")
	}
}

func TestPhoneNutritionGoalDeletionAndOwnerRecheck(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	setProfile(t, st, owner, `{"birthdate":"1990-01-01","sex":"male","activity_override":"moderate"}`)
	createRecord(t, st, owner, "weight", 80)
	createRecord(t, st, owner, "height", 1.8)
	old := time.Now().UTC().Add(-time.Hour)
	previous := database.WeightGoal{UserID: owner, Time: old, Kilograms: 75}
	latest := database.WeightGoal{UserID: owner, Time: old.Add(time.Minute), Kilograms: 70}
	for _, row := range []*database.WeightGoal{&previous, &latest} {
		row.ID, row.FamilyID = uuid.New(), family
		if err := st.DB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	get := func() map[string]any {
		return phoneSuccess(t, phoneCall(t, h, "get_nutrition_goals", map[string]any{}))["nutrition"].(map[string]any)
	}
	if get()["values"].(map[string]any)["goal_weight_kg"] != float64(70) {
		t.Fatal("latest goal missing")
	}
	if err := st.DeleteRecord("weight_goals", latest.ID, owner); err != nil {
		t.Fatal(err)
	}
	if get()["values"].(map[string]any)["goal_weight_kg"] != float64(75) {
		t.Fatal("deleted goal still applied")
	}
	if err := st.DeleteRecord("weight_goals", previous.ID, owner); err != nil {
		t.Fatal(err)
	}
	if get()["unavailable_reason"] != "missing_goal_weight" {
		t.Fatal("deleted goals retained")
	}
	if err := st.DB().Exec("DELETE FROM users WHERE id = ?", owner).Error; err != nil {
		t.Fatal(err)
	}
	if phoneCall(t, h, "get_nutrition_goals", map[string]any{})["isError"] != true {
		t.Fatal("deleted owner accepted by existing handler")
	}
}

func TestPhoneNutritionGoalsUnavailableReasonsAndIndependentFiber(t *testing.T) {
	for _, reason := range []string{"missing_profile", "missing_measurements", "missing_goal_weight", "insufficient_activity_data"} {
		t.Run(reason, func(t *testing.T) {
			st := newFoodTestStorage(t)
			owner, _ := seedFoodUser(t, st)
			if reason != "missing_profile" {
				setProfile(t, st, owner, `{"birthdate":"1990-01-01","sex":"male"}`)
			}
			if reason == "missing_goal_weight" || reason == "insufficient_activity_data" {
				createRecord(t, st, owner, "weight", 80)
				createRecord(t, st, owner, "height", 1.8)
			}
			if reason == "insufficient_activity_data" {
				createRecord(t, st, owner, "weight_goal", 75)
			}
			h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
			out := phoneSuccess(t, phoneCall(t, h, "get_nutrition_goals", map[string]any{}))
			n := out["nutrition"].(map[string]any)
			if n["available"] != false || n["unavailable_reason"] != reason || n["values"] != nil || n["activity_source"] != nil {
				t.Fatalf("missing goals: %+v", out)
			}
			f := out["fiber"].(map[string]any)
			if f["grams"] != float64(25) || f["source"] != "default" || out["timezone_fallback"] != true {
				t.Fatalf("independent default fiber: %+v", out)
			}
			w := httptest.NewRecorder()
			server.NutritionTargetHandler(st).ServeHTTP(w, newNutritionTargetRequest(owner))
			if w.Code != 422 || decodeUnprocessableReason(t, w) != reason {
				t.Fatal("reason differs from HTTP")
			}
		})
	}
}

func TestPhoneNutritionFiberSources(t *testing.T) {
	minor := time.Now().UTC().AddDate(-12, 0, 0).Format("2006-01-02")
	for _, tc := range []struct {
		settings, source string
		grams            any
	}{
		{`{}`, "default", float64(25)},
		{`{"fiber_target_grams":1}`, "configured", float64(1)},
		{`{"fiber_target_grams":200}`, "configured", float64(200)},
		{`{"fiber_target_grams":201}`, "default", float64(25)},
		{`{"fiber_target_grams":"30"}`, "default", float64(25)},
		{`{"birthdate":"` + minor + `"}`, "unavailable_under_18", nil},
		{`{"birthdate":"` + minor + `","fiber_target_grams":30}`, "configured", float64(30)},
	} {
		t.Run(tc.settings, func(t *testing.T) {
			st := newFoodTestStorage(t)
			owner, _ := seedFoodUser(t, st)
			setProfile(t, st, owner, tc.settings)
			h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
			out := phoneSuccess(t, phoneCall(t, h, "get_nutrition_goals", map[string]any{}))
			f := out["fiber"].(map[string]any)
			if f["source"] != tc.source || f["grams"] != tc.grams {
				t.Fatalf("fiber: %+v", f)
			}
		})
	}
}

func TestPhoneNutritionGoalsOwnerAndDatabaseErrors(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, _ := seedFoodUser(t, st)
	food := server.NewFoodHandlers(st, nil, t.TempDir())
	missing := food.PhoneMCPHandler("test-phone-token", uuid.NewString(), nil)
	if phoneCall(t, missing, "get_nutrition_goals", map[string]any{})["isError"] != true {
		t.Fatal("missing owner accepted")
	}
	h := food.PhoneMCPHandler("test-phone-token", owner.String(), nil)
	if err := st.DB().Migrator().DropTable(&database.UserSettings{}); err != nil {
		t.Fatal(err)
	}
	if phoneCall(t, h, "get_nutrition_goals", map[string]any{})["isError"] != true {
		t.Fatal("database error treated as missing profile")
	}
}

func TestPhoneNutritionGoalsDisabledIdentityAndComputeFailure(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, _ := seedFoodUser(t, st)
	food := server.NewFoodHandlers(st, nil, t.TempDir())
	for _, identity := range []string{"", "bad-id", uuid.Nil.String()} {
		r := httptest.NewRequest(http.MethodPost, "/phone-mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_nutrition_goals","arguments":{}}}`))
		r.Header.Set("Authorization", "Bearer test-phone-token")
		w := httptest.NewRecorder()
		food.PhoneMCPHandler("test-phone-token", identity, nil).ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatalf("invalid identity %q accepted: HTTP%d", identity, w.Code)
		}
	}
	setProfile(t, st, owner, `{"birthdate":"1990-01-01","sex":"male"}`)
	if err := st.DB().Migrator().DropTable(&database.Weight{}); err != nil {
		t.Fatal(err)
	}
	h := food.PhoneMCPHandler("test-phone-token", owner.String(), nil)
	if phoneCall(t, h, "get_nutrition_goals", map[string]any{})["isError"] != true {
		t.Fatal("compute DB failure reported as ordinary unavailable target")
	}
}

func TestPhoneNutritionGoalsBackupBarrier(t *testing.T) {
	st := newFoodTestStorage(t)
	owner, _ := seedFoodUser(t, st)
	barrier := &sync.RWMutex{}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), barrier)
	barrier.Lock()
	done := make(chan struct{})
	go func() { defer close(done); phoneCall(t, h, "get_nutrition_goals", map[string]any{}) }()
	select {
	case <-done:
		barrier.Unlock()
		t.Fatal("goals bypassed backup barrier")
	case <-time.After(30 * time.Millisecond):
	}
	barrier.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("goals remained blocked after backup")
	}
}

// Legacy Local must retain HTTP target semantics, rather than the history
// tools' explicit-UTC fallback. No parallel test may replace time.Local.
func TestPhoneNutritionGoalsLegacyLocalAndInferredActivityParity(t *testing.T) {
	previousLocal := time.Local
	offset := -12 * 3600
	if time.Now().UTC().Hour() >= 10 {
		offset = 14 * 3600
	}
	time.Local = time.FixedZone("LegacyServerZone", offset)
	t.Cleanup(func() { time.Local = previousLocal })
	st := newFoodTestStorage(t)
	owner, family := seedFoodUser(t, st)
	setProfile(t, st, owner, `{"birthdate":"1990-01-01","sex":"male","timezone":"Local"}`)
	createRecord(t, st, owner, "weight", 80)
	createRecord(t, st, owner, "height", 1.8)
	createRecord(t, st, owner, "weight_goal", 75)
	now := time.Now().UTC()
	for i := 2; i <= 10; i++ {
		start := now.AddDate(0, 0, -i)
		row := database.Steps{UserID: owner, StartTime: start, EndTime: start.Add(time.Minute), Count: 8000}
		row.ID, row.FamilyID = uuid.New(), family
		if err := st.DB().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).PhoneMCPHandler("test-phone-token", owner.String(), nil)
	out := phoneSuccess(t, phoneCall(t, h, "get_nutrition_goals", map[string]any{}))
	n := out["nutrition"].(map[string]any)
	w := httptest.NewRecorder()
	server.NutritionTargetHandler(st).ServeHTTP(w, newNutritionTargetRequest(owner))
	var httpValues map[string]any
	if w.Code != 200 {
		t.Fatalf("HTTP target %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &httpValues); err != nil {
		t.Fatal(err)
	}
	if n["available"] != true || n["activity_source"] != "inferred_steps" || !reflect.DeepEqual(n["values"], httpValues) {
		t.Fatalf("inferred parity: %+v / %+v", n, httpValues)
	}
	stamp, err := time.Parse(time.RFC3339Nano, out["as_of"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if out["timezone"] != time.Local.String() || out["timezone_fallback"] != true || out["local_date"] != stamp.In(time.Local).Format("2006-01-02") || out["local_date"] == stamp.UTC().Format("2006-01-02") {
		t.Fatalf("Local fallback drift: %+v", out)
	}
}
