package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	kinmodels "github.com/ya-breeze/kin-core/models"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/server"
)

func persistAdviceRevision(t *testing.T, st database.Storage, userID, familyID uuid.UUID, day string, generatedAt time.Time) {
	t.Helper()
	row := database.FoodAdvice{
		UserID: userID, LoggedDay: day, Label: "fair", ReasonCodes: "protein_far",
		Language: "en", InputHash: uuid.NewString(), Lines: `["Add protein."]`, GeneratedAt: generatedAt.UTC(),
	}
	row.ID = uuid.New()
	row.FamilyID = familyID
	if err := st.DB().Create(&row).Error; err != nil {
		t.Fatalf("create advice revision: %v", err)
	}
}

func engagementRequest(t *testing.T, userID, familyID uuid.UUID, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal engagement body: %v", err)
	}
	return withClaimsFamily(
		httptest.NewRequest(http.MethodPost, "/api/food/advice/engagement", bytes.NewReader(b)),
		userID, familyID,
	)
}

func qualifiedViewBody(day string, generatedAt time.Time) map[string]any {
	return map[string]any{
		"event": "qualified_view", "logged_day": day, "generated_at": generatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func TestFoodAdviceEngagement_AuthenticationOriginAndInput(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	day := "2026-09-08"
	generatedAt := time.Date(2026, 9, 8, 12, 0, 0, 123000000, time.UTC)
	persistAdviceRevision(t, st, userID, familyID, day, generatedAt)
	assertNoEngagementRows := func(t *testing.T) {
		t.Helper()
		var count int64
		if err := st.DB().Model(&database.FoodAdviceEngagement{}).Count(&count).Error; err != nil {
			t.Fatalf("count engagement rows: %v", err)
		}
		if count != 0 {
			t.Fatalf("engagement rows = %d, want none", count)
		}
	}

	t.Run("authentication required", func(t *testing.T) {
		b, _ := json.Marshal(qualifiedViewBody(day, generatedAt))
		w := httptest.NewRecorder()
		h.RecordFoodAdviceEngagement(w, httptest.NewRequest(http.MethodPost, "/api/food/advice/engagement", bytes.NewReader(b)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
		assertNoEngagementRows(t)
	})

	for _, site := range []string{"same-site", "cross-site", "none"} {
		t.Run(site+" rejected", func(t *testing.T) {
			req := engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt))
			req.Header.Set("Sec-Fetch-Site", site)
			w := httptest.NewRecorder()
			h.RecordFoodAdviceEngagement(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", w.Code)
			}
			assertNoEngagementRows(t)
		})
	}

	for _, tc := range []struct {
		name string
		body any
	}{
		{"client cannot report refresh", map[string]any{"event": "refresh_success", "logged_day": day, "generated_at": generatedAt}},
		{"malformed date", qualifiedViewBody("2026-02-30", generatedAt)},
		{"missing timestamp", map[string]any{"event": "qualified_view", "logged_day": day}},
		{"unknown field", map[string]any{"event": "qualified_view", "logged_day": day, "generated_at": generatedAt, "advice": "private"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, tc.body))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			assertNoEngagementRows(t)
		})
	}

	t.Run("insert failure leaves no partial row", func(t *testing.T) {
		if err := st.DB().Exec(`CREATE TRIGGER fail_engagement_insert
			BEFORE INSERT ON food_advice_engagements
			BEGIN SELECT RAISE(FAIL, 'engagement unavailable'); END`).Error; err != nil {
			t.Fatalf("create failure trigger: %v", err)
		}
		w := httptest.NewRecorder()
		h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt)))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
		}
		assertNoEngagementRows(t)
	})
}

func TestFoodAdviceEngagement_RejectsAbsentStaleAndCrossUserRevisions(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	day := "2026-09-08"
	generatedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	assertNotFound := func(t *testing.T, at time.Time) {
		t.Helper()
		w := httptest.NewRecorder()
		h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, at)))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
		}
	}
	assertNotFound(t, generatedAt)

	persistAdviceRevision(t, st, userID, familyID, day, generatedAt)
	assertNotFound(t, generatedAt.Add(-time.Second))

	otherUser := uuid.New()
	otherFamily := uuid.New()
	if err := st.DB().Model(&database.FoodAdvice{}).Where("user_id = ?", userID).
		Updates(map[string]any{"user_id": otherUser, "family_id": otherFamily}).Error; err != nil {
		t.Fatalf("move advice to another owner: %v", err)
	}
	assertNotFound(t, generatedAt)

	var count int64
	if err := st.DB().Model(&database.FoodAdviceEngagement{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("engagement rows = %d, err=%v; want none", count, err)
	}
}

func TestFoodAdviceEngagement_AtomicAggregationAndTimeBounds(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())
	day := "2026-09-08"
	generatedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	persistAdviceRevision(t, st, userID, familyID, day, generatedAt)

	const calls = 24
	statuses := make(chan int, calls)
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt)))
			statuses <- w.Code
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusNoContent {
			t.Fatalf("concurrent status = %d, want 204", status)
		}
	}

	var got database.FoodAdviceEngagement
	if err := st.DB().Where("user_id = ? AND logged_day = ?", userID, day).First(&got).Error; err != nil {
		t.Fatalf("load aggregate: %v", err)
	}
	if got.QualifiedViewCount != calls || got.FirstQualifiedViewAt == nil || got.LastQualifiedViewAt == nil {
		t.Fatalf("aggregate = %+v, want %d views and both bounds", got, calls)
	}
	if got.FirstQualifiedViewAt.After(*got.LastQualifiedViewAt) {
		t.Fatalf("first view %v is after last view %v", got.FirstQualifiedViewAt, got.LastQualifiedViewAt)
	}

	// A later event must expand the last bound without changing an earlier
	// first bound.
	pastFirst := time.Now().UTC().Add(-2 * time.Hour)
	pastLast := time.Now().UTC().Add(-time.Hour)
	if err := st.DB().Model(&got).Updates(map[string]any{
		"first_qualified_view_at": pastFirst, "last_qualified_view_at": pastLast,
	}).Error; err != nil {
		t.Fatalf("seed past bounds: %v", err)
	}
	w := httptest.NewRecorder()
	h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("later update status = %d", w.Code)
	}
	if err := st.DB().First(&got, "id = ?", got.ID).Error; err != nil {
		t.Fatalf("reload aggregate: %v", err)
	}
	if got.FirstQualifiedViewAt == nil || !got.FirstQualifiedViewAt.Equal(pastFirst) ||
		got.LastQualifiedViewAt == nil || !got.LastQualifiedViewAt.After(pastLast) {
		t.Fatalf("later event bounds = %v/%v, want first %v and last after %v",
			got.FirstQualifiedViewAt, got.LastQualifiedViewAt, pastFirst, pastLast)
	}

	// An earlier event must expand the first bound without changing a later
	// last bound.
	futureFirst := time.Now().UTC().Add(time.Hour)
	futureLast := time.Now().UTC().Add(2 * time.Hour)
	if err := st.DB().Model(&got).Updates(map[string]any{
		"first_qualified_view_at": futureFirst, "last_qualified_view_at": futureLast,
	}).Error; err != nil {
		t.Fatalf("seed future bounds: %v", err)
	}
	w = httptest.NewRecorder()
	h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("earlier update status = %d", w.Code)
	}
	if err := st.DB().First(&got, "id = ?", got.ID).Error; err != nil {
		t.Fatalf("reload aggregate: %v", err)
	}
	if got.FirstQualifiedViewAt == nil || !got.FirstQualifiedViewAt.Before(futureFirst) ||
		got.LastQualifiedViewAt == nil || !got.LastQualifiedViewAt.Equal(futureLast) {
		t.Fatalf("earlier event bounds = %v/%v, want first before %v and last %v",
			got.FirstQualifiedViewAt, got.LastQualifiedViewAt, futureFirst, futureLast)
	}

	// An event already bracketed by the stored range must not shrink either
	// bound.
	wideFirst := time.Now().UTC().Add(-time.Hour)
	wideLast := time.Now().UTC().Add(time.Hour)
	if err := st.DB().Model(&got).Updates(map[string]any{
		"first_qualified_view_at": wideFirst, "last_qualified_view_at": wideLast,
	}).Error; err != nil {
		t.Fatalf("seed wide bounds: %v", err)
	}
	w = httptest.NewRecorder()
	h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("bounded update status = %d", w.Code)
	}
	if err := st.DB().First(&got, "id = ?", got.ID).Error; err != nil {
		t.Fatalf("reload aggregate: %v", err)
	}
	if got.FirstQualifiedViewAt == nil || !got.FirstQualifiedViewAt.Equal(wideFirst) ||
		got.LastQualifiedViewAt == nil || !got.LastQualifiedViewAt.Equal(wideLast) {
		t.Fatalf("bracketing bounds changed: first=%v last=%v, want %v/%v",
			got.FirstQualifiedViewAt, got.LastQualifiedViewAt, wideFirst, wideLast)
	}
}

// seedSecondFoodUser adds a user in a family of its own, because seedFoodUser
// uses a fixed username and cannot be called twice against one database.
func seedSecondFoodUser(t *testing.T, s database.Storage) (userID, familyID uuid.UUID) {
	t.Helper()
	familyID = uuid.New()
	userID = uuid.New()
	if err := s.DB().Create(&kinmodels.Family{ID: familyID, Name: "OtherTestFamily"}).Error; err != nil {
		t.Fatalf("create second family: %v", err)
	}
	user := kinmodels.User{ID: userID, Username: "othertestuser", PasswordHash: "x", FamilyID: familyID}
	if err := s.DB().Create(&user).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}
	return userID, familyID
}

// TestFoodAdviceEngagement_IsolatesCallersAndDays proves at the handler layer
// that aggregates never merge across callers or Logged Days. The unique index
// is covered in the database package, but the upsert's conflict target is what
// decides which row a request lands in, and only the handler exercises that.
func TestFoodAdviceEngagement_IsolatesCallersAndDays(t *testing.T) {
	st := newFileFoodTestStorage(t)
	userID, familyID := seedFoodUser(t, st)
	otherUserID, otherFamilyID := seedSecondFoodUser(t, st)
	h := server.NewFoodHandlers(st, nil, t.TempDir())

	const firstDay = "2026-09-08"
	const secondDay = "2026-09-09"
	firstGeneratedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	secondGeneratedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	otherGeneratedAt := time.Date(2026, 9, 8, 13, 0, 0, 0, time.UTC)
	persistAdviceRevision(t, st, userID, familyID, firstDay, firstGeneratedAt)
	persistAdviceRevision(t, st, otherUserID, otherFamilyID, firstDay, otherGeneratedAt)

	record := func(t *testing.T, uid, fid uuid.UUID, day string, generatedAt time.Time) {
		t.Helper()
		w := httptest.NewRecorder()
		h.RecordFoodAdviceEngagement(w, engagementRequest(t, uid, fid, qualifiedViewBody(day, generatedAt)))
		if w.Code != http.StatusNoContent {
			t.Fatalf("record status = %d, want 204", w.Code)
		}
	}
	record(t, userID, familyID, firstDay, firstGeneratedAt)
	record(t, userID, familyID, firstDay, firstGeneratedAt)
	record(t, otherUserID, otherFamilyID, firstDay, otherGeneratedAt)

	// A caller holds at most one cached advice row, so the next day replaces
	// the previous revision rather than adding one beside it. The engagement
	// aggregate must still open a separate row for that day.
	if err := st.DB().Model(&database.FoodAdvice{}).Where("user_id = ?", userID).
		Updates(map[string]any{"logged_day": secondDay, "generated_at": secondGeneratedAt}).
		Error; err != nil {
		t.Fatalf("move advice revision to the next day: %v", err)
	}
	record(t, userID, familyID, secondDay, secondGeneratedAt)

	var rows int64
	if err := st.DB().Model(&database.FoodAdviceEngagement{}).Count(&rows).Error; err != nil {
		t.Fatalf("count engagement rows: %v", err)
	}
	if rows != 3 {
		t.Fatalf("engagement rows = %d, want 3 (two days for the caller, one for the other user)", rows)
	}

	for _, want := range []struct {
		userID uuid.UUID
		day    string
		count  uint64
	}{
		{userID, firstDay, 2},
		{userID, secondDay, 1},
		{otherUserID, firstDay, 1},
	} {
		var got database.FoodAdviceEngagement
		if err := st.DB().Where("user_id = ? AND logged_day = ?", want.userID, want.day).
			First(&got).Error; err != nil {
			t.Fatalf("load aggregate for %s on %s: %v", want.userID, want.day, err)
		}
		if got.QualifiedViewCount != want.count {
			t.Fatalf("aggregate for %s on %s has %d views, want %d",
				want.userID, want.day, got.QualifiedViewCount, want.count)
		}
		if got.RefreshRequestCount != 0 || got.RefreshSuccessCount != 0 {
			t.Fatalf("aggregate for %s on %s recorded refresh events: %+v", want.userID, want.day, got)
		}
	}
}
