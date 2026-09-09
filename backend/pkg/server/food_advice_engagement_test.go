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

	t.Run("authentication required", func(t *testing.T) {
		b, _ := json.Marshal(qualifiedViewBody(day, generatedAt))
		w := httptest.NewRecorder()
		h.RecordFoodAdviceEngagement(w, httptest.NewRequest(http.MethodPost, "/api/food/advice/engagement", bytes.NewReader(b)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
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
		})
	}
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

	first := got.FirstQualifiedViewAt.Add(-time.Hour)
	last := got.LastQualifiedViewAt.Add(time.Hour)
	if err := st.DB().Model(&got).Updates(map[string]any{
		"first_qualified_view_at": first, "last_qualified_view_at": last,
	}).Error; err != nil {
		t.Fatalf("seed wider bounds: %v", err)
	}
	w := httptest.NewRecorder()
	h.RecordFoodAdviceEngagement(w, engagementRequest(t, userID, familyID, qualifiedViewBody(day, generatedAt)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("bounded update status = %d", w.Code)
	}
	if err := st.DB().First(&got, "id = ?", got.ID).Error; err != nil {
		t.Fatalf("reload aggregate: %v", err)
	}
	if got.FirstQualifiedViewAt == nil || !got.FirstQualifiedViewAt.Equal(first) ||
		got.LastQualifiedViewAt == nil || !got.LastQualifiedViewAt.Equal(last) {
		t.Fatalf("bounds moved backwards: first=%v last=%v, want %v/%v", got.FirstQualifiedViewAt, got.LastQualifiedViewAt, first, last)
	}
}
