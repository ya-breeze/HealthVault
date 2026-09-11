package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

type adviceTestResponse struct {
	Available   bool       `json:"available"`
	Reason      string     `json:"reason"`
	Lines       []string   `json:"lines"`
	LoggedDay   string     `json:"logged_day"`
	GeneratedAt *time.Time `json:"generated_at"`
	Context     struct {
		TargetCalories     int    `json:"target_calories"`
		TargetProteinGrams int    `json:"target_protein_grams"`
		TargetCarbsGrams   int    `json:"target_carbs_grams"`
		TargetFatGrams     int    `json:"target_fat_grams"`
		DisplayLanguage    string `json:"display_language"`
	} `json:"context"`
}

func adviceBody(label string, reasons []string, refresh bool, protein float64) map[string]any {
	return map[string]any{
		"label": label, "reasons": reasons, "refresh": refresh,
		"window": map[string]any{
			"mean_calories": 1820.5, "mean_protein_grams": protein,
			"mean_carbs_grams": 210.25, "mean_fat_grams": 62.5,
			"mean_sugar_grams": 88.75, "mean_sodium_grams": 3.1,
		},
	}
}

func newAdviceRequest(t *testing.T, userID, familyID uuid.UUID, body map[string]any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal advice body: %v", err)
	}
	return withClaimsFamily(httptest.NewRequest(http.MethodPost, "/api/food/advice", bytes.NewReader(b)), userID, familyID)
}

func callAdvice(t *testing.T, h interface {
	PostFoodAdvice(http.ResponseWriter, *http.Request)
}, req *http.Request) (*httptest.ResponseRecorder, adviceTestResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	h.PostFoodAdvice(w, req)
	var response adviceTestResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode advice response: %v", err)
		}
	}
	return w, response
}

func configureAdviceTarget(t *testing.T, st database.Storage, userID uuid.UUID, language string) {
	t.Helper()
	settings := `{"birthdate":"1990-01-01","sex":"male","activity_override":"moderate","display_language":"` + language + `"}`
	setProfile(t, st, userID, settings)
	base := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	for kind, value := range map[string]float64{"weight": 80, "height": 1.8, "weight_goal": 75} {
		h := server.CreateRecordHandler(st)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, newCreateRequest(kind, map[string]any{"value": value, "time": base}, userID))
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d: %s", kind, w.Code, w.Body.String())
		}
	}
}

func TestFoodAdvice_CacheHitSkipsAdviseAndCarriesServerContext(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "ru-RU")
	body := adviceBody("fair", []string{"protein_far", "sugar_off"}, false, 74.25)

	generatedFake := &vision.Fake{AdviseResult: []string{"Добавьте белок."}}
	generatedHandler := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(generatedFake, 10<<20, time.Second)
	w1, generated := callAdvice(t, generatedHandler, newAdviceRequest(t, userID, familyID, body))
	if w1.Code != http.StatusOK || !generated.Available {
		t.Fatalf("generate: %d: %s", w1.Code, w1.Body.String())
	}
	if generated.Context.DisplayLanguage != "ru" || generated.Context.TargetCalories == 0 || generated.GeneratedAt == nil {
		t.Fatalf("generated response missing effective context: %+v", generated)
	}
	if generated.LoggedDay == "" {
		t.Fatalf("generated response missing Logged Day: %+v", generated)
	}

	hitFake := &vision.Fake{AdviseErr: errors.New("Advise must not be called on a cache hit")}
	hitHandler := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(hitFake, 10<<20, time.Second)
	w2, cached := callAdvice(t, hitHandler, newAdviceRequest(t, userID, familyID, body))
	if w2.Code != http.StatusOK || !cached.Available || len(hitFake.AdviseCalls) != 0 {
		t.Fatalf("cache hit: %d calls=%d body=%s", w2.Code, len(hitFake.AdviseCalls), w2.Body.String())
	}
	if cached.Context != generated.Context || cached.GeneratedAt == nil || !cached.GeneratedAt.Equal(*generated.GeneratedAt) {
		t.Errorf("cache context/time = %+v/%v, want %+v/%v", cached.Context, cached.GeneratedAt, generated.Context, generated.GeneratedAt)
	}
	if cached.LoggedDay != generated.LoggedDay {
		t.Errorf("cached Logged Day = %q, want %q", cached.LoggedDay, generated.LoggedDay)
	}
}

func TestFoodAdvice_RefreshEngagementOutcomesAndTelemetryIsolation(t *testing.T) {
	t.Run("success increments request and success", func(t *testing.T) {
		st := newFileFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		fake := &vision.Fake{AdviseResult: []string{"Refreshed."}}
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, true, 70)))
		if w.Code != http.StatusOK || !response.Available {
			t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
		}
		var got database.FoodAdviceEngagement
		if err := st.DB().Where("user_id = ? AND logged_day = ?", userID, response.LoggedDay).First(&got).Error; err != nil {
			t.Fatalf("load engagement: %v", err)
		}
		if got.RefreshRequestCount != 1 || got.RefreshSuccessCount != 1 ||
			got.FirstRefreshRequestAt == nil || got.LastRefreshRequestAt == nil ||
			got.FirstRefreshSuccessAt == nil || got.LastRefreshSuccessAt == nil {
			t.Fatalf("refresh aggregate = %+v", got)
		}
	})

	t.Run("model failure increments request only", func(t *testing.T) {
		st := newFileFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(
			&vision.Fake{AdviseErr: errors.New("model down")}, 10<<20, time.Second,
		)

		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, true, 70)))
		if w.Code != http.StatusOK || response.Available {
			t.Fatalf("refresh failure: %d %s", w.Code, w.Body.String())
		}
		var got database.FoodAdviceEngagement
		if err := st.DB().Where("user_id = ?", userID).First(&got).Error; err != nil {
			t.Fatalf("load engagement: %v", err)
		}
		if got.RefreshRequestCount != 1 || got.RefreshSuccessCount != 0 {
			t.Fatalf("refresh counts = %d/%d, want 1/0", got.RefreshRequestCount, got.RefreshSuccessCount)
		}
	})

	t.Run("cache write failure increments request only", func(t *testing.T) {
		st := newFileFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		if err := st.DB().Exec(`CREATE TRIGGER fail_food_advice_insert
			BEFORE INSERT ON food_advices
			BEGIN SELECT RAISE(FAIL, 'advice cache unavailable'); END`).Error; err != nil {
			t.Fatalf("create cache failure trigger: %v", err)
		}
		fake := &vision.Fake{AdviseResult: []string{"Valid but uncacheable."}}
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, true, 70)))
		if w.Code != http.StatusOK || response.Available || response.Reason != "unavailable" {
			t.Fatalf("refresh with cache failure: %d %s", w.Code, w.Body.String())
		}
		if len(fake.AdviseCalls) != 1 {
			t.Fatalf("Advise calls = %d, want 1", len(fake.AdviseCalls))
		}
		var adviceCount int64
		if err := st.DB().Model(&database.FoodAdvice{}).Where("user_id = ?", userID).Count(&adviceCount).Error; err != nil {
			t.Fatalf("count cached advice: %v", err)
		}
		if adviceCount != 0 {
			t.Fatalf("cached advice rows = %d, want none", adviceCount)
		}
		var got database.FoodAdviceEngagement
		if err := st.DB().Where("user_id = ?", userID).First(&got).Error; err != nil {
			t.Fatalf("load engagement: %v", err)
		}
		if got.RefreshRequestCount != 1 || got.RefreshSuccessCount != 0 ||
			got.FirstRefreshRequestAt == nil || got.LastRefreshRequestAt == nil ||
			got.FirstRefreshSuccessAt != nil || got.LastRefreshSuccessAt != nil {
			t.Fatalf("refresh aggregate = %+v, want one request and no success", got)
		}
	})

	t.Run("cached request increments neither", func(t *testing.T) {
		st := newFileFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		fake := &vision.Fake{AdviseResult: []string{"Cached."}}
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
		body := adviceBody("fair", nil, false, 70)
		for range 2 {
			w, _ := callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
			if w.Code != http.StatusOK {
				t.Fatalf("advice: %d %s", w.Code, w.Body.String())
			}
		}
		var count int64
		if err := st.DB().Model(&database.FoodAdviceEngagement{}).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("engagement rows = %d, err=%v; want none", count, err)
		}
	})

	t.Run("telemetry failure does not fail delivery", func(t *testing.T) {
		st := newFileFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		if err := st.DB().Exec(`CREATE TRIGGER fail_advice_engagement
			BEFORE INSERT ON food_advice_engagements
			BEGIN SELECT RAISE(FAIL, 'telemetry unavailable'); END`).Error; err != nil {
			t.Fatalf("create failure trigger: %v", err)
		}
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(
			&vision.Fake{AdviseResult: []string{"Still delivered."}}, 10<<20, time.Second,
		)
		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, true, 70)))
		if w.Code != http.StatusOK || !response.Available || len(response.Lines) != 1 {
			t.Fatalf("refresh with telemetry failure: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("request telemetry failure cannot create success-only aggregate", func(t *testing.T) {
		st := newFileFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		if err := st.DB().Exec(`CREATE TRIGGER fail_refresh_request
			BEFORE INSERT ON food_advice_engagements
			WHEN NEW.refresh_request_count = 1
			BEGIN SELECT RAISE(FAIL, 'refresh request telemetry unavailable'); END`).Error; err != nil {
			t.Fatalf("create selective failure trigger: %v", err)
		}
		h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(
			&vision.Fake{AdviseResult: []string{"Still delivered and cached."}}, 10<<20, time.Second,
		)

		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, true, 70)))
		if w.Code != http.StatusOK || !response.Available || len(response.Lines) != 1 {
			t.Fatalf("refresh with request telemetry failure: %d %s", w.Code, w.Body.String())
		}
		var advice database.FoodAdvice
		if err := st.DB().Where("user_id = ?", userID).First(&advice).Error; err != nil {
			t.Fatalf("load cached advice: %v", err)
		}
		var count int64
		if err := st.DB().Model(&database.FoodAdviceEngagement{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			t.Fatalf("count engagement rows: %v", err)
		}
		if count != 0 {
			t.Fatalf("engagement rows = %d, want none", count)
		}
	})
}

func TestFoodAdvice_MissPersistsNormalizedInputAndEveryChangeRegenerates(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	fake := &vision.Fake{AdviseResult: []string{"Add protein."}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	body := adviceBody("fair", []string{"protein_far", "protein_far", "sugar_off"}, false, 74.125)

	w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
	if w.Code != http.StatusOK || !response.Available {
		t.Fatalf("initial miss: %d: %s", w.Code, w.Body.String())
	}
	var row database.FoodAdvice
	if err := st.DB().Where("user_id = ?", userID).First(&row).Error; err != nil {
		t.Fatalf("persisted advice: %v", err)
	}
	if row.ReasonCodes != "protein_far,sugar_off" || row.Language != "en" || row.InputHash == "" {
		t.Errorf("unexpected persisted row: %+v", row)
	}
	if got := fake.AdviseCalls[0]; len(got.Reasons) != 2 || got.Reasons[0] != "protein_far" || got.Reasons[1] != "sugar_off" || got.MeanProteinGrams != 74.125 {
		t.Errorf("priority, deduplication, or fractional mean lost: %+v", got)
	}

	assertAdditionalCall := func(name string, changed map[string]any) {
		t.Helper()
		before := len(fake.AdviseCalls)
		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, changed))
		if w.Code != http.StatusOK || !response.Available || len(fake.AdviseCalls) != before+1 {
			t.Fatalf("%s did not regenerate: status=%d calls=%d body=%s", name, w.Code, len(fake.AdviseCalls), w.Body.String())
		}
	}

	if err := st.DB().Model(&database.FoodAdvice{}).Where("user_id = ?", userID).Update("logged_day", "2000-01-01").Error; err != nil {
		t.Fatal(err)
	}
	assertAdditionalCall("logged day", body)
	assertAdditionalCall("label", adviceBody("needs_attention", []string{"protein_far", "sugar_off"}, false, 74.125))
	assertAdditionalCall("reason codes", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, false, 74.125))
	assertAdditionalCall("mean figure", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, false, 74.5))

	configureAdviceTarget(t, st, userID, "ru")
	assertAdditionalCall("language", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, false, 74.5))

	goalTime := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	recordHandler := server.CreateRecordHandler(st)
	goalWriter := httptest.NewRecorder()
	recordHandler.ServeHTTP(goalWriter, newCreateRequest("weight_goal", map[string]any{"value": 70, "time": goalTime}, userID))
	if goalWriter.Code != http.StatusCreated {
		t.Fatalf("change goal: %d: %s", goalWriter.Code, goalWriter.Body.String())
	}
	assertAdditionalCall("target figure", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, false, 74.5))
}

type blockingAdviceClient struct {
	*vision.Fake
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingAdviceClient) Advise(ctx context.Context, in vision.AdviceInput) ([]string, error) {
	c.once.Do(func() { close(c.entered) })
	select {
	case <-c.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return c.Fake.Advise(ctx, in)
}

func TestFoodAdvice_ConcurrentMissesCoalesceButRefreshAlwaysRegenerates(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	client := &blockingAdviceClient{
		Fake:    &vision.Fake{AdviseResult: []string{"First."}},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(client, 10<<20, time.Second)
	body := adviceBody("good", []string{"balanced"}, false, 100.5)

	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			w, _ := callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
			responses <- w
		}()
	}
	<-client.entered
	close(client.release)
	for range 2 {
		if w := <-responses; w.Code != http.StatusOK {
			t.Fatalf("concurrent response: %d: %s", w.Code, w.Body.String())
		}
	}
	if len(client.AdviseCalls) != 1 {
		t.Fatalf("concurrent misses made %d calls, want 1", len(client.AdviseCalls))
	}

	client.AdviseResult = []string{"Refreshed."}
	refreshBody := adviceBody("good", []string{"balanced"}, true, 100.5)
	_, refreshed := callAdvice(t, h, newAdviceRequest(t, userID, familyID, refreshBody))
	if len(client.AdviseCalls) != 2 || len(refreshed.Lines) != 1 || refreshed.Lines[0] != "Refreshed." {
		t.Fatalf("refresh did not force a second generation: calls=%d response=%+v", len(client.AdviseCalls), refreshed)
	}
}

func TestFoodAdvice_FailureValidationOriginAndAuthStates(t *testing.T) {
	setup := func(t *testing.T, client vision.Client) (database.Storage, uuid.UUID, uuid.UUID, interface {
		PostFoodAdvice(http.ResponseWriter, *http.Request)
	}) {
		t.Helper()
		st := newFoodTestStorage(t)
		userID, familyID := seedFoodUser(t, st)
		configureAdviceTarget(t, st, userID, "en")
		return st, userID, familyID, server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(client, 10<<20, time.Second)
	}

	t.Run("unconfigured", func(t *testing.T) {
		_, userID, familyID, h := setup(t, vision.Unconfigured{})
		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, false, 70)))
		if w.Code != http.StatusOK || response.Available || response.Reason != "unconfigured" {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("model error writes no row", func(t *testing.T) {
		fake := &vision.Fake{AdviseErr: errors.New("model down")}
		st, userID, familyID, h := setup(t, fake)
		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, false, 70)))
		var count int64
		st.DB().Model(&database.FoodAdvice{}).Where("user_id = ?", userID).Count(&count)
		if w.Code != http.StatusOK || response.Reason != "unavailable" || count != 0 {
			t.Fatalf("status=%d response=%+v rows=%d", w.Code, response, count)
		}
	})

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"rejected label", adviceBody("excellent", nil, false, 70)},
		{"malformed reason", adviceBody("fair", []string{"Protein-Low"}, false, 70)},
		{"missing window figure", func() map[string]any {
			body := adviceBody("fair", nil, false, 70)
			delete(body["window"].(map[string]any), "mean_sodium_grams")
			return body
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &vision.Fake{AdviseResult: []string{"unused"}}
			_, userID, familyID, h := setup(t, fake)
			w, _ := callAdvice(t, h, newAdviceRequest(t, userID, familyID, tc.body))
			if w.Code != http.StatusBadRequest || len(fake.AdviseCalls) != 0 {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, len(fake.AdviseCalls), w.Body.String())
			}
		})
	}

	for _, site := range []string{"same-site", "cross-site"} {
		t.Run(site+" rejected before side effects", func(t *testing.T) {
			fake := &vision.Fake{AdviseResult: []string{"unused"}}
			st, userID, familyID, h := setup(t, fake)
			req := newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, false, 70))
			req.Header.Set("Sec-Fetch-Site", site)
			w, _ := callAdvice(t, h, req)
			var count int64
			st.DB().Model(&database.FoodAdvice{}).Count(&count)
			if w.Code != http.StatusForbidden || len(fake.AdviseCalls) != 0 || count != 0 {
				t.Fatalf("status=%d calls=%d rows=%d", w.Code, len(fake.AdviseCalls), count)
			}
		})
	}

	t.Run("no claims", func(t *testing.T) {
		st := newFoodTestStorage(t)
		h := server.NewFoodHandlers(st, nil, t.TempDir())
		b, _ := json.Marshal(adviceBody("fair", nil, false, 70))
		w, _ := callAdvice(t, h, httptest.NewRequest(http.MethodPost, "/api/food/advice", bytes.NewReader(b)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
}
