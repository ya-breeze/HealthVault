package weather

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := database.Open(slog.New(slog.NewTextHandler(io.Discard, nil)), filepath.Join(t.TempDir(), "weather.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { _ = sql.Close() })
	return &Store{DB: db}
}
func location(user, family uuid.UUID, h time.Time, lat float64) database.WeatherLocation {
	return database.WeatherLocation{ID: uuid.New(), UserID: user, FamilyID: family, ObservedAt: h, Latitude: lat, Longitude: 14.4, AccuracyM: 500}
}
func mustAccept(t *testing.T, s *Store, o database.WeatherLocation) {
	t.Helper()
	if err := s.Accept(o); err != nil {
		t.Fatal(err)
	}
}
func TestEvidenceRepairAndIdempotence(t *testing.T) {
	s := newStore(t)
	user, family := uuid.New(), uuid.New()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, b := location(user, family, base, 50.1), location(user, family, base.Add(4*time.Hour), 50.1)
	mustAccept(t, s, a)
	out, err := s.History(user, family, base, base.Add(time.Hour))
	if err != nil || len(out.Hours) != 0 || len(out.Gaps) != 1 || out.Gaps[0].Reason != "missing_location" {
		t.Fatalf("first observation %+v %v", out, err)
	}
	mustAccept(t, s, b)
	mustAccept(t, s, b)
	var count int64
	s.DB.Model(&database.WeatherHour{}).Count(&count)
	if count != 4 {
		t.Fatalf("hours %d", count)
	}
	bad := b
	bad.AccuracyM++
	if !errors.Is(s.Accept(bad), ErrConflict) {
		t.Fatal("accepted conflicting UUID")
	}
	bad = b
	bad.ID = uuid.New()
	if !errors.Is(s.Accept(bad), ErrConflict) {
		t.Fatal("accepted duplicate observation time")
	}
	// A late sea-trip observation must invalidate the previously compatible pair.
	c := location(user, family, base.Add(2*time.Hour), 42.1)
	mustAccept(t, s, c)
	s.DB.Model(&database.WeatherHour{}).Count(&count)
	if count != 0 {
		t.Fatalf("stale jobs survived %d", count)
	}
	out, err = s.History(user, family, base, base.Add(4*time.Hour))
	if err != nil || len(out.Gaps) != 1 || out.Gaps[0].Reason != "moving" {
		t.Fatalf("travel %+v %v", out, err)
	}
	out, err = s.History(uuid.New(), family, base, base.Add(4*time.Hour))
	if err != nil || len(out.Hours) != 0 || out.Gaps[0].Reason != "missing_location" {
		t.Fatal("isolation", out, err)
	}
}
func TestConservativeIntervals(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 15, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		duration time.Duration
		accuracy float64
		lat      float64
		reason   string
		hours    int
	}{
		{"partial hours", 2 * time.Hour, 500, 50.1, "", 1},
		{"accuracy", 2 * time.Hour, 10001, 50.1, "poor_accuracy", 0},
		{"uncertainty radii", 2 * time.Hour, 10000, 50.2, "moving", 0},
		{"long gap", 7 * time.Hour, 500, 50.1, "long_gap", 0},
		{"six hours", 6 * time.Hour, 500, 50.1, "", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			u, f := uuid.New(), uuid.New()
			a, b := location(u, f, base, 50.1), location(u, f, base.Add(tc.duration), tc.lat)
			a.AccuracyM = tc.accuracy
			b.AccuracyM = tc.accuracy
			mustAccept(t, s, a)
			mustAccept(t, s, b)
			var rows []database.WeatherHour
			s.DB.Find(&rows)
			if len(rows) != tc.hours {
				t.Fatalf("hours %d want %d", len(rows), tc.hours)
			}
			var span database.WeatherCoverage
			s.DB.First(&span)
			if span.Reason != tc.reason {
				t.Fatal(span.Reason)
			}
			for _, h := range rows {
				if h.Hour.Before(a.ObservedAt) || h.Hour.Add(time.Hour).After(b.ObservedAt) {
					t.Fatal("partial hour assigned")
				}
			}
		})
	}
}

type providerFunc func(context.Context, database.WeatherHour) (database.WeatherHour, error)

func (f providerFunc) Fetch(c context.Context, h database.WeatherHour) (database.WeatherHour, error) {
	return f(c, h)
}
func TestDurableRetryAndInflightInvalidation(t *testing.T) {
	s := newStore(t)
	u, f := uuid.New(), uuid.New()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mustAccept(t, s, location(u, f, base, 50.1))
	mustAccept(t, s, location(u, f, base.Add(2*time.Hour), 50.1))
	now := time.Now().UTC()
	barrier := &sync.RWMutex{}
	if err := s.ProcessBatch(context.Background(), providerFunc(func(_ context.Context, h database.WeatherHour) (database.WeatherHour, error) {
		return h, errors.New("offline")
	}), barrier, now); err != nil {
		t.Fatal(err)
	}
	var jobs []database.WeatherHour
	s.DB.Find(&jobs)
	if len(jobs) != 2 || jobs[0].Attempts != 1 || !jobs[0].NextAttempt.After(now) {
		t.Fatal("retry not persisted", jobs)
	}
	// A reconstructed Store sees persisted retry state and can finish it.
	restarted := &Store{DB: s.DB}
	calls := 0
	p := providerFunc(func(_ context.Context, h database.WeatherHour) (database.WeatherHour, error) {
		calls++
		h.Status = "ready"
		h.TemperatureC = 0
		h.Source = "test"
		h.Model = "test"
		h.FetchedAt = now
		return h, nil
	})
	if err := restarted.ProcessBatch(context.Background(), p, barrier, now); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("ignored backoff")
	}
	if err := restarted.ProcessBatch(context.Background(), p, barrier, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	out, err := s.History(u, f, base, base.Add(2*time.Hour))
	if err != nil || len(out.Hours) != 2 || len(out.Gaps) != 0 || out.Hours[0].TemperatureC != 0 {
		t.Fatal("ready/zero", out, err)
	}
	// New isolated pair, replace evidence while the provider request is running.
	s2 := newStore(t)
	mustAccept(t, s2, location(u, f, base, 50.1))
	mustAccept(t, s2, location(u, f, base.Add(2*time.Hour), 50.1))
	once := sync.Once{}
	p = providerFunc(func(_ context.Context, h database.WeatherHour) (database.WeatherHour, error) {
		once.Do(func() { mustAccept(t, s2, location(u, f, base.Add(time.Hour), 42.1)) })
		h.Status = "ready"
		return h, nil
	})
	if err := s2.ProcessBatch(context.Background(), p, barrier, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var n int64
	s2.DB.Model(&database.WeatherHour{}).Count(&n)
	if n != 0 {
		t.Fatal("resurrected invalidated weather")
	}
}
func TestBackupBarrierBlocksProviderPersistence(t *testing.T) {
	s := newStore(t)
	u, f := uuid.New(), uuid.New()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mustAccept(t, s, location(u, f, base, 50.1))
	mustAccept(t, s, location(u, f, base.Add(time.Hour), 50.1))
	barrier := &sync.RWMutex{}
	barrier.Lock()
	fetched := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.ProcessBatch(context.Background(), providerFunc(func(_ context.Context, h database.WeatherHour) (database.WeatherHour, error) {
			close(fetched)
			return h, nil
		}), barrier, time.Now().UTC())
	}()
	<-fetched
	select {
	case <-done:
		t.Fatal("persisted during capture")
	case <-time.After(20 * time.Millisecond):
	}
	barrier.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func providerResponse(hour time.Time) map[string]any {
	times := []string{}
	for h := hour.Truncate(24 * time.Hour); h.Before(hour.Truncate(24 * time.Hour).Add(24 * time.Hour)); h = h.Add(time.Hour) {
		times = append(times, h.Format("2006-01-02T15:04"))
	}
	hourly := map[string]any{"time": times}
	units := map[string]string{"time": "iso8601"}
	names := []string{"°C", "°C", "%", "hPa", "hPa", "mm", "km/h"}
	for j, v := range variables {
		values := make([]float64, len(times))
		for i := range values {
			values[i] = float64(i + j)
		}
		hourly[v] = values
		units[v] = names[j]
	}
	return map[string]any{"hourly": hourly, "hourly_units": units, "utc_offset_seconds": 0}
}
func TestProviderValidatesArraysAndAccumulation(t *testing.T) {
	hour := time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		ok     bool
	}{
		{"valid", func(map[string]any) {}, true},
		{"null temperature at hour start", func(d map[string]any) {
			raw := d["hourly"].(map[string]any)["temperature_2m"].([]float64)
			values := make([]any, len(raw))
			for i, v := range raw {
				values[i] = v
			}
			values[hour.Hour()] = nil
			d["hourly"].(map[string]any)["temperature_2m"] = values
		}, false},
		{"null precipitation at hour end", func(d map[string]any) {
			raw := d["hourly"].(map[string]any)["precipitation"].([]float64)
			values := make([]any, len(raw))
			for i, v := range raw {
				values[i] = v
			}
			values[hour.Hour()+1] = nil
			d["hourly"].(map[string]any)["precipitation"] = values
		}, false},
		{"misalignment", func(d map[string]any) { d["hourly"].(map[string]any)["wind_speed_10m"] = []float64{1} }, false},
		{"unit", func(d map[string]any) { d["hourly_units"].(map[string]string)["surface_pressure"] = "Pa" }, false},
		{"time", func(d map[string]any) { ts := d["hourly"].(map[string]any)["time"].([]string); ts[4] = ts[3] }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := providerResponse(hour)
			tc.mutate(d)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("models") != "ecmwf_ifs025" || r.URL.Query().Get("timezone") != "UTC" {
					t.Error("wrong provider selection")
				}
				_ = json.NewEncoder(w).Encode(d)
			}))
			defer srv.Close()
			p := &OpenMeteo{Client: srv.Client(), URL: srv.URL}
			h, err := p.Fetch(context.Background(), database.WeatherHour{Hour: hour, Latitude: 50.1, Longitude: 14.4})
			if (err == nil) != tc.ok {
				t.Fatalf("error %v", err)
			}
			if tc.ok && (h.TemperatureC != 3 || h.PrecipitationMm != 9 || h.Source != "open-meteo historical-forecast" || h.Status != "ready") {
				t.Fatal(h)
			}
		})
	}
}

func TestJitteredHourlyChainProducesWeatherAndLateTravelInvalidates(t *testing.T) {
	s := newStore(t)
	u, f := uuid.New(), uuid.New()
	base := time.Date(2026, 1, 1, 9, 5, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		mustAccept(t, s, location(u, f, base.Add(time.Duration(i)*time.Hour), 50.1))
	}
	var jobs []database.WeatherHour
	if err := s.DB.Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || !jobs[0].Hour.Equal(base.Truncate(time.Hour).Add(time.Hour)) {
		t.Fatalf("jittered hourly samples did not enclose complete hour: %+v", jobs)
	}
	// Appending compatible evidence must preserve previously established jobs.
	firstID := jobs[0].ID
	mustAccept(t, s, location(u, f, base.Add(3*time.Hour), 50.1))
	jobs = nil
	s.DB.Order("hour").Find(&jobs)
	if len(jobs) != 2 || jobs[0].ID != firstID {
		t.Fatal("unchanged job replaced")
	}
	mustAccept(t, s, location(u, f, base.Add(90*time.Minute), 42.1))
	jobs = nil
	s.DB.Find(&jobs)
	if len(jobs) != 0 {
		t.Fatalf("late travel retained chain jobs: %+v", jobs)
	}
}

func TestGapReasonsUseActualObservationBoundaries(t *testing.T) {
	s := newStore(t)
	u, f := uuid.New(), uuid.New()
	base := time.Date(2026, 1, 1, 7, 1, 0, 0, time.UTC)
	mustAccept(t, s, location(u, f, base, 50.1))
	mustAccept(t, s, location(u, f, base.Add(3*time.Hour), 50.1))
	mustAccept(t, s, location(u, f, base.Add(4*time.Hour), 42.1))
	out, err := s.History(u, f, base.Truncate(time.Hour), base.Add(4*time.Hour+30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range out.Gaps {
		if g.Reason == "moving" {
			found = true
			if !g.From.Equal(base.Add(3*time.Hour)) || !g.To.Equal(base.Add(4*time.Hour)) {
				t.Fatal("travel boundary shifted", g)
			}
		}
	}
	if !found {
		t.Fatal("travel gap missing", out)
	}
	if !out.Gaps[0].To.Equal(base) || out.Gaps[0].Reason != "missing_location" || out.Gaps[len(out.Gaps)-1].Reason != "missing_location" {
		t.Fatal("missing evidence boundary", out)
	}
}

func TestChainDoesNotAssignStartWeatherAcrossAccumulatedMovement(t *testing.T) {
	s := newStore(t)
	u, f := uuid.New(), uuid.New()
	base := time.Date(2026, 1, 1, 9, 5, 0, 0, time.UTC)
	for i, lat := range []float64{50.1, 50.3, 50.5} {
		mustAccept(t, s, location(u, f, base.Add(time.Duration(i)*time.Hour), lat))
	}
	var n int64
	s.DB.Model(&database.WeatherHour{}).Count(&n)
	if n != 0 {
		t.Fatal("weather assigned across known wider movement")
	}
}
