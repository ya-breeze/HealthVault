package server

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/weather"
)

func WeatherLocationHandler(store *weather.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		var req struct {
			ID         string    `json:"id"`
			ObservedAt time.Time `json:"observed_at"`
			Latitude   *float64  `json:"latitude"`
			Longitude  *float64  `json:"longitude"`
			AccuracyM  *float64  `json:"accuracy_m"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		var trailing any
		if dec.Decode(&trailing) != io.EOF {
			http.Error(w, "invalid body", 400)
			return
		}
		id, err := uuid.Parse(req.ID)
		now := time.Now().UTC()
		finite := func(p *float64) bool { return p != nil && !math.IsNaN(*p) && !math.IsInf(*p, 0) }
		rounded := func(p *float64) bool { return math.Abs(*p*10-math.Round(*p*10)) < 1e-7 }
		if err != nil || id == uuid.Nil || req.ObservedAt.IsZero() || req.ObservedAt.Before(now.Add(-14*24*time.Hour)) || req.ObservedAt.After(now.Add(5*time.Minute)) || !finite(req.Latitude) || !finite(req.Longitude) || !finite(req.AccuracyM) {
			http.Error(w, "invalid observation", 400)
			return
		}
		if *req.Latitude < -90 || *req.Latitude > 90 || *req.Longitude < -180 || *req.Longitude > 180 || *req.AccuracyM <= 0 || *req.AccuracyM > 20000000 || !rounded(req.Latitude) || !rounded(req.Longitude) {
			http.Error(w, "invalid approximate location", 400)
			return
		}
		o := database.WeatherLocation{ID: id, UserID: claims.UserID, FamilyID: FamilyIDFromCtx(r), ObservedAt: req.ObservedAt.UTC(), Latitude: math.Round(*req.Latitude*10) / 10, Longitude: math.Round(*req.Longitude*10) / 10, AccuracyM: *req.AccuracyM}
		if err := store.Accept(o); err != nil {
			if errors.Is(err, weather.ErrConflict) {
				http.Error(w, "observation conflict", 409)
			} else {
				http.Error(w, "storage error", 500)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "accepted"})
	}
}

func WeatherHistoryHandler(store *weather.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		from, e1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		to, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if e1 != nil || e2 != nil || !from.Before(to) || to.Sub(from) > 31*24*time.Hour || to.After(time.Now().UTC().Add(5*time.Minute)) {
			http.Error(w, "invalid time range (maximum 31 days)", 400)
			return
		}
		out, err := store.History(claims.UserID, FamilyIDFromCtx(r), from.UTC(), to.UTC())
		if err != nil {
			http.Error(w, "query error", 500)
			return
		}
		writeJSON(w, out)
	}
}
