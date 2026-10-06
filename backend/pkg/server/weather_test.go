package server_test

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/server"
	"github.com/ya-breeze/healthvault/pkg/weather"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWeatherLocationValidationAndHistoryIsolation(t *testing.T) {
	st := newFoodTestStorage(t)
	u, f := seedFoodUser(t, st)
	store := &weather.Store{DB: st.DB()}
	h := server.WeatherLocationHandler(store)
	base := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Hour)
	id := uuid.New()
	body := func(i uuid.UUID, ts time.Time, lat, accuracy float64) string {
		return fmt.Sprintf(`{"id":"%s","observed_at":"%s","latitude":%g,"longitude":14.4,"accuracy_m":%g}`, i, ts.Format(time.RFC3339), lat, accuracy)
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"new", body(id, base, 50.1, 500), 202},
		{"retry", body(id, base, 50.1, 500), 202},
		{"conflict", body(id, base, 50.2, 500), 409},
		{"precise", body(uuid.New(), base.Add(time.Minute), 50.123, 500), 400},
		{"stale", body(uuid.New(), base.Add(-15*24*time.Hour), 50.1, 500), 400},
		{"future", body(uuid.New(), time.Now().Add(6*time.Minute), 50.1, 500), 400},
		{"negative accuracy", body(uuid.New(), base.Add(time.Minute), 50.1, -1), 400},
		{"missing coordinate", fmt.Sprintf(`{"id":"%s","observed_at":"%s","longitude":14.4,"accuracy_m":1}`, uuid.New(), base.Format(time.RFC3339)), 400},
		{"extra body", body(uuid.New(), base.Add(time.Minute), 50.1, 500) + ` {}`, 400},
		{"second compatible", body(uuid.New(), base.Add(2*time.Hour), 50.1, 500), 202},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := withClaimsFamily(httptest.NewRequest("POST", "/api/weather/locations", strings.NewReader(tc.body)), u, f)
			h(w, r)
			if w.Code != tc.status {
				t.Fatalf("status%d want%d %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("POST", "/api/weather/locations", strings.NewReader(body(id, base, 50.1, 500))))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	history := server.WeatherHistoryHandler(store)
	query := "?from=" + url.QueryEscape(base.Format(time.RFC3339)) + "&to=" + url.QueryEscape(base.Add(2*time.Hour).Format(time.RFC3339)) + "&user=someone"
	w = httptest.NewRecorder()
	history(w, withClaimsFamily(httptest.NewRequest("GET", "/api/weather/history"+query, nil), u, f))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	var out weather.History
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Hours) != 0 || len(out.Gaps) != 1 || out.Gaps[0].Reason != "pending_weather" {
		t.Fatal(out)
	}
	w = httptest.NewRecorder()
	history(w, withClaimsFamily(httptest.NewRequest("GET", "/api/weather/history"+query, nil), uuid.New(), f))
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Gaps[0].Reason != "missing_location" {
		t.Fatal("other user's data exposed", out)
	}
	for _, q := range []string{"", "?from=bad&to=bad", "?from=2026-01-01T00:00:00Z&to=2026-03-01T00:00:00Z"} {
		w = httptest.NewRecorder()
		history(w, withClaimsFamily(httptest.NewRequest("GET", "/api/weather/history"+q, nil), u, f))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}
