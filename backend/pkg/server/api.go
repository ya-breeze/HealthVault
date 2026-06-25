package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"github.com/ya-breeze/healthvault/pkg/database"
)

// typeInfo maps URL type names to (table name, primary time column).
type typeInfo struct{ table, timeCol string }

var typeRegistry = map[string]typeInfo{
	"steps":                  {"steps", "start_time"},
	"heart_rate":             {"heart_rates", "time"},
	"heart_rate_variability": {"heart_rate_variabilities", "time"},
	"sleep":                  {"sleeps", "start_time"},
	"distance":               {"distances", "start_time"},
	"active_calories":        {"active_calories", "start_time"},
	"total_calories":         {"total_calories", "start_time"},
	"weight":                 {"weights", "time"},
	"height":                 {"heights", "time"},
	"blood_pressure":         {"blood_pressures", "time"},
	"blood_glucose":          {"blood_glucoses", "time"},
	"oxygen_saturation":      {"oxygen_saturations", "time"},
	"body_temperature":       {"body_temperatures", "time"},
	"skin_temperature":       {"skin_temperatures", "time"},
	"respiratory_rate":       {"respiratory_rates", "time"},
	"resting_heart_rate":     {"resting_heart_rates", "time"},
	"exercise":               {"exercises", "start_time"},
	"hydration":              {"hydrations", "start_time"},
	"nutrition":              {"nutritions", "start_time"},
	"basal_metabolic_rate":   {"basal_metabolic_rates", "time"},
	"body_fat":               {"body_fats", "time"},
	"lean_body_mass":         {"lean_body_masses", "time"},
	"vo2_max":                {"vo2_maxes", "time"},
	"bone_mass":              {"bone_masses", "time"},
}

// meHandler returns the authenticated user's profile.
func meHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := storage.FindUserByID(claims.UserID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"id":        user.ID,
			"username":  user.Username,
			"family_id": user.FamilyID,
		})
	}
}

// dataHandler returns health records for a given type within a time range.
// The {type} URL param is validated against typeRegistry before use in SQL to
// prevent SQL injection through user-controlled table/column names.
func dataHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		typeName := mux.Vars(r)["type"]
		info, ok := typeRegistry[typeName]
		if !ok {
			http.Error(w, "unknown type", http.StatusNotFound)
			return
		}

		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		familyID := FamilyIDFromCtx(r)

		targetUser, err := resolveUser(r, storage, claims.UserID, familyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		from, to := parseTimeRange(r)
		records, err := storage.QueryRecords(info.table, info.timeCol, targetUser.ID, database.TimeRange{From: from, To: to})
		if err != nil {
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}
		// Normalize nil to empty slice so clients always get a JSON array.
		if records == nil {
			records = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records) //nolint:errcheck
	}
}

// summaryHandler returns aggregate health stats (steps, avg heart rate, sleep) for a time range.
func summaryHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		familyID := FamilyIDFromCtx(r)

		targetUser, err := resolveUser(r, storage, claims.UserID, familyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		from, to := parseTimeRange(r)
		tr := database.TimeRange{From: from, To: to}

		steps, _ := storage.SummarySteps(targetUser.ID, tr)
		avgHR, _ := storage.SummaryAvgHeartRate(targetUser.ID, tr)
		sleepSec, _ := storage.SummarySleepSeconds(targetUser.ID, tr)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"steps":          steps,
			"avg_heart_rate": avgHR,
			"sleep_seconds":  sleepSec,
		})
	}
}

// parseTimeRange extracts ?from= and ?to= query params as time.Time values.
// Defaults: from = 7 days ago, to = now.
func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	q := r.URL.Query()
	from, _ := time.Parse(time.RFC3339, q.Get("from"))
	to, _ := time.Parse(time.RFC3339, q.Get("to"))
	if from.IsZero() {
		from = time.Now().UTC().AddDate(0, 0, -7)
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	return from, to
}

// resolveUser returns the target user: the caller themselves, or a named
// family member (from ?user= query param). Returns an error if the named user
// is not in the caller's family.
func resolveUser(r *http.Request, storage database.Storage, callerID uuid.UUID, familyID uuid.UUID) (*kinmodels.User, error) {
	username := r.URL.Query().Get("user")
	if username == "" {
		return storage.FindUserByID(callerID)
	}
	target, err := storage.FindUserByName(username)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	if target.FamilyID != familyID {
		return nil, fmt.Errorf("access denied")
	}
	return target, nil
}
