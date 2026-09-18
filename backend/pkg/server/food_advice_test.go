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

func adviceBody(label string, reasons []string, protein float64) map[string]any {
	return map[string]any{
		"label": label, "reasons": reasons,
		"window": map[string]any{
			"mean_calories": 1820.5, "mean_protein_grams": protein,
			"mean_carbs_grams": 210.25, "mean_fat_grams": 62.5,
			"mean_sugar_grams": 88.75, "mean_sodium_grams": 3.1,
			"mean_dietary_fiber_grams": 24.5, "mean_saturated_fat_grams": 5.5,
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
	body := adviceBody("fair", []string{"protein_far", "sugar_off"}, 74.25)

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
	if got := generatedFake.AdviseCalls[0].HealthContext; got.ActivitySource != "profile_override" ||
		got.MeanDailySteps != nil || got.MeanSleepHours != nil || got.WeightTrend != nil {
		t.Fatalf("sparse health context was not safely omitted: %+v", got)
	}
	if generatedFake.AdviseCalls[0].MeanDietaryFiberGrams != 24.5 {
		t.Fatalf("fiber mean did not reach advice input: %+v", generatedFake.AdviseCalls[0])
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

func createAdviceHealthDay(
	t *testing.T, st database.Storage, userID, familyID uuid.UUID, day time.Time,
	steps, sleepSeconds int, weightKg *float64,
) {
	t.Helper()
	payloadID := uuid.New()
	step := database.Steps{
		UserID: userID, SourcePayloadID: payloadID,
		StartTime: day.Add(7 * time.Hour), EndTime: day.Add(8 * time.Hour), Count: steps,
	}
	step.ID, step.FamilyID = uuid.New(), familyID
	if err := st.DB().Create(&step).Error; err != nil {
		t.Fatalf("create advice steps: %v", err)
	}
	sleep := database.Sleep{
		UserID: userID, SourcePayloadID: payloadID,
		StartTime: day, SessionEndTime: day.Add(time.Duration(sleepSeconds) * time.Second),
		DurationSeconds: sleepSeconds,
	}
	sleep.ID, sleep.FamilyID = uuid.New(), familyID
	if err := st.DB().Create(&sleep).Error; err != nil {
		t.Fatalf("create advice sleep: %v", err)
	}
	if weightKg == nil {
		return
	}
	weight := database.Weight{
		UserID: userID, SourcePayloadID: &payloadID,
		Time: day.Add(9 * time.Hour), Kilograms: *weightKg,
	}
	weight.ID, weight.FamilyID = uuid.New(), familyID
	if err := st.DB().Create(&weight).Error; err != nil {
		t.Fatalf("create advice weight: %v", err)
	}
}

func TestFoodAdvice_UsesCoveredCallerHealthContextAndHashesChanges(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID, otherFamilyID := seedSecondFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weights := map[int]float64{3: 78, 2: 79, 1: 80}
	for i := 7; i >= 1; i-- {
		var weight *float64
		if value, ok := weights[i]; ok {
			valueCopy := value
			weight = &valueCopy
		}
		createAdviceHealthDay(t, st, userID, familyID, today.AddDate(0, 0, -i), 7000, 8*3600, weight)
	}
	otherWeight := 190.0
	createAdviceHealthDay(
		t, st, otherUserID, otherFamilyID, today.AddDate(0, 0, -1), 99000, 2*3600, &otherWeight,
	)

	fake := &vision.Fake{AdviseResult: []string{"Keep the adjustment practical."}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	body := adviceBody("fair", []string{"sodium_far"}, 70)
	w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
	if w.Code != http.StatusOK || !response.Available || len(fake.AdviseCalls) != 1 {
		t.Fatalf("generate contextual advice: status=%d calls=%d body=%s", w.Code, len(fake.AdviseCalls), w.Body.String())
	}
	context := fake.AdviseCalls[0].HealthContext
	if context.WindowDays != 28 || context.WindowEnds != today.AddDate(0, 0, -1).Format("2006-01-02") {
		t.Errorf("health context window = %+v", context)
	}
	if context.ActivitySource != "profile_override" || context.ActivityTier != "Moderately active" {
		t.Errorf("activity provenance = %+v", context)
	}
	if context.MeanDailySteps == nil || context.MeanDailySteps.Value != 7000 || context.MeanDailySteps.RecordedDays != 7 {
		t.Errorf("step context = %+v", context.MeanDailySteps)
	}
	if context.MeanSleepHours == nil || context.MeanSleepHours.Value != 8 || context.MeanSleepHours.RecordedDays != 7 {
		t.Errorf("sleep context = %+v", context.MeanSleepHours)
	}
	if context.WeightTrend == nil || context.WeightTrend.FirstDailyAverageKg != 78 ||
		context.WeightTrend.LatestDailyAverageKg != 80 || context.WeightTrend.RecordedDays != 3 {
		t.Errorf("weight context = %+v", context.WeightTrend)
	}

	// A newly synced caller-owned day changes AdviceInput and therefore the
	// cache hash. The other user's extreme records above do not enter it.
	createAdviceHealthDay(t, st, userID, familyID, today.AddDate(0, 0, -8), 7000, 8*3600, nil)
	w, response = callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
	if w.Code != http.StatusOK || !response.Available || len(fake.AdviseCalls) != 2 {
		t.Fatalf("changed health context did not regenerate: status=%d calls=%d body=%s", w.Code, len(fake.AdviseCalls), w.Body.String())
	}
	if got := fake.AdviseCalls[1].HealthContext.MeanSleepHours; got == nil || got.RecordedDays != 8 || got.Value != 8 {
		t.Errorf("updated sleep context = %+v", got)
	}

	setProfile(t, st, userID, `{"birthdate":"1990-01-01","sex":"male","display_language":"en"}`)
	w, response = callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
	if w.Code != http.StatusOK || !response.Available || len(fake.AdviseCalls) != 3 {
		t.Fatalf("inferred activity did not regenerate: status=%d calls=%d body=%s", w.Code, len(fake.AdviseCalls), w.Body.String())
	}
	inferred := fake.AdviseCalls[2].HealthContext
	if inferred.ActivitySource != "inferred_from_steps" || inferred.ActivityTier != "Lightly active" {
		t.Errorf("inferred activity provenance = %+v", inferred)
	}
}

func TestFoodAdvice_HealthContextReadFailureDoesNotGenerateOrCache(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	if err := st.DB().Exec("DROP TABLE sleeps").Error; err != nil {
		t.Fatalf("drop sleeps table: %v", err)
	}
	fake := &vision.Fake{AdviseResult: []string{"Must not be generated."}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, 70)))
	if w.Code != http.StatusOK || response.Available || response.Reason != "unavailable" {
		t.Fatalf("health read failure: status=%d body=%s", w.Code, w.Body.String())
	}
	if len(fake.AdviseCalls) != 0 {
		t.Fatalf("health read failure made %d model calls", len(fake.AdviseCalls))
	}
	var count int64
	if err := st.DB().Model(&database.FoodAdvice{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("health read failure cached %d rows (err=%v)", count, err)
	}
}

// What survives the refresh control's removal: advice the model produced but
// the cache could not keep is not served as if it had been stored. The engagement
// half of the old refresh test went with the events it measured.
func TestFoodAdvice_CacheWriteFailureIsUnavailableAndPersistsNothing(t *testing.T) {
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

	w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, 70)))
	if w.Code != http.StatusOK || response.Available || response.Reason != "unavailable" {
		t.Fatalf("cache failure: %d %s", w.Code, w.Body.String())
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
	var engagementCount int64
	if err := st.DB().Model(&database.FoodAdviceEngagement{}).Count(&engagementCount).Error; err != nil {
		t.Fatalf("count engagement rows: %v", err)
	}
	if engagementCount != 0 {
		t.Fatalf("engagement rows = %d, want none: the advice path records no engagement of its own", engagementCount)
	}
}

// A client still posting the retired flag must fail loudly. The decoder
// disallows unknown fields, so an old bundle cannot silently get advice it
// believes was regenerated.
func TestFoodAdvice_RetiredRefreshFlagIsRejected(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	fake := &vision.Fake{AdviseResult: []string{"Should never be reached."}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)

	body := adviceBody("fair", nil, 70)
	body["refresh"] = true
	w, _ := callAdvice(t, h, newAdviceRequest(t, userID, familyID, body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if len(fake.AdviseCalls) != 0 {
		t.Fatal("a request carrying the retired refresh flag reached the model")
	}
}

func TestFoodAdvice_MissPersistsNormalizedInputAndEveryChangeRegenerates(t *testing.T) {
	st := newFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	fake := &vision.Fake{AdviseResult: []string{"Add protein."}}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(fake, 10<<20, time.Second)
	body := adviceBody("fair", []string{"protein_far", "protein_far", "sugar_off"}, 74.125)

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
	assertAdditionalCall("label", adviceBody("needs_attention", []string{"protein_far", "sugar_off"}, 74.125))
	assertAdditionalCall("reason codes", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, 74.125))
	assertAdditionalCall("mean figure", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, 74.5))

	configureAdviceTarget(t, st, userID, "ru")
	assertAdditionalCall("language", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, 74.5))

	goalTime := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	recordHandler := server.CreateRecordHandler(st)
	goalWriter := httptest.NewRecorder()
	recordHandler.ServeHTTP(goalWriter, newCreateRequest("weight_goal", map[string]any{"value": 70, "time": goalTime}, userID))
	if goalWriter.Code != http.StatusCreated {
		t.Fatalf("change goal: %d: %s", goalWriter.Code, goalWriter.Body.String())
	}
	assertAdditionalCall("target figure", adviceBody("needs_attention", []string{"protein_far", "sodium_off"}, 74.5))
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

func TestFoodAdvice_ConcurrentMissesCoalesce(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	configureAdviceTarget(t, st, userID, "en")
	client := &blockingAdviceClient{
		Fake:    &vision.Fake{AdviseResult: []string{"First."}},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	h := server.NewFoodHandlers(st, nil, t.TempDir()).WithVision(client, 10<<20, time.Second)
	body := adviceBody("good", []string{"balanced"}, 100.5)

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
		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, 70)))
		if w.Code != http.StatusOK || response.Available || response.Reason != "unconfigured" {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("model error writes no row", func(t *testing.T) {
		fake := &vision.Fake{AdviseErr: errors.New("model down")}
		st, userID, familyID, h := setup(t, fake)
		w, response := callAdvice(t, h, newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, 70)))
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
		{"rejected label", adviceBody("excellent", nil, 70)},
		{"malformed reason", adviceBody("fair", []string{"Protein-Low"}, 70)},
		{"missing existing window figure", func() map[string]any {
			body := adviceBody("fair", nil, 70)
			delete(body["window"].(map[string]any), "mean_sodium_grams")
			return body
		}()},
		{"missing fiber window figure", func() map[string]any {
			body := adviceBody("fair", nil, 70)
			delete(body["window"].(map[string]any), "mean_dietary_fiber_grams")
			return body
		}()},
		{"missing saturated fat window figure", func() map[string]any {
			body := adviceBody("fair", nil, 70)
			delete(body["window"].(map[string]any), "mean_saturated_fat_grams")
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
			req := newAdviceRequest(t, userID, familyID, adviceBody("fair", nil, 70))
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
		b, _ := json.Marshal(adviceBody("fair", nil, 70))
		w, _ := callAdvice(t, h, httptest.NewRequest(http.MethodPost, "/api/food/advice", bytes.NewReader(b)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
}
