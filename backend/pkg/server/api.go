package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	kinmodels "github.com/ya-breeze/kin-core/models"
	"github.com/ya-breeze/healthvault/pkg/database"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"gorm.io/gorm"
)

// typeInfo maps URL type names to (table name, primary time column,
// aggregation family, value column). family and valueCol are the zero value
// ("") for food_meal, which never accepts ?bucket=, and valueCol is also ""
// for blood_pressure and nutrition, whose multiple value columns need the
// dedicated QueryAggregateBloodPressure / QueryAggregateNutrition queries
// instead of the single-valueCol QueryAggregate path.
type typeInfo struct {
	table    string
	timeCol  string
	family   database.AggFamily
	valueCol string
}

var typeRegistry = map[string]typeInfo{
	"steps":                  {table: "steps", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "count"},
	"heart_rate":             {table: "heart_rates", timeCol: "time", family: database.AggFamilyPoint, valueCol: "bpm"},
	"heart_rate_variability": {table: "heart_rate_variabilities", timeCol: "time", family: database.AggFamilyPoint, valueCol: "rmssd_millis"},
	// sleep is bucketed by start_time (the type's night anchor), which is
	// unindexed — Sleep's unique index covers session_end_time instead. See
	// design.md: a full scan is fine given sleep's row volume.
	"sleep":                {table: "sleeps", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "duration_seconds"},
	"distance":             {table: "distances", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "meters"},
	"active_calories":      {table: "active_calories", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "calories"},
	"total_calories":       {table: "total_calories", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "calories"},
	"weight":               {table: "weights", timeCol: "time", family: database.AggFamilyPoint, valueCol: "kilograms"},
	"height":               {table: "heights", timeCol: "time", family: database.AggFamilyPoint, valueCol: "meters"},
	"weight_goal":          {table: "weight_goals", timeCol: "time", family: database.AggFamilyPoint, valueCol: "kilograms"},
	"blood_pressure":       {table: "blood_pressures", timeCol: "time", family: database.AggFamilyPoint}, // multi-column: see QueryAggregateBloodPressure
	"blood_glucose":        {table: "blood_glucoses", timeCol: "time", family: database.AggFamilyPoint, valueCol: "mmol_per_liter"},
	"oxygen_saturation":    {table: "oxygen_saturations", timeCol: "time", family: database.AggFamilyPoint, valueCol: "percentage"},
	"body_temperature":     {table: "body_temperatures", timeCol: "time", family: database.AggFamilyPoint, valueCol: "celsius"},
	"skin_temperature":     {table: "skin_temperatures", timeCol: "time", family: database.AggFamilyPoint, valueCol: "delta_celsius"},
	"respiratory_rate":     {table: "respiratory_rates", timeCol: "time", family: database.AggFamilyPoint, valueCol: "rate"},
	"resting_heart_rate":   {table: "resting_heart_rates", timeCol: "time", family: database.AggFamilyPoint, valueCol: "bpm"},
	"exercise":             {table: "exercises", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "duration_seconds"},
	"hydration":            {table: "hydrations", timeCol: "start_time", family: database.AggFamilyCumulative, valueCol: "liters"},
	"nutrition":            {table: "nutritions", timeCol: "start_time", family: database.AggFamilyCumulative}, // multi-column: see QueryAggregateNutrition
	"basal_metabolic_rate": {table: "basal_metabolic_rates", timeCol: "time", family: database.AggFamilyPoint, valueCol: "watts"},
	"body_fat":             {table: "body_fats", timeCol: "time", family: database.AggFamilyPoint, valueCol: "percentage"},
	"lean_body_mass":       {table: "lean_body_masses", timeCol: "time", family: database.AggFamilyPoint, valueCol: "kilograms"},
	"vo2_max":              {table: "vo2_maxes", timeCol: "time", family: database.AggFamilyPoint, valueCol: "ml_per_kg_per_min"},
	"bone_mass":            {table: "bone_masses", timeCol: "time", family: database.AggFamilyPoint, valueCol: "kilograms"},
	"speed":                {table: "speeds", timeCol: "time", family: database.AggFamilyPoint, valueCol: "meters_per_second"},
	// food_meal is user-authored, not ingested telemetry, and is exposed here
	// read-only: metadata and macros only (see columnAllowlist in
	// pkg/database/storage_impl.go), never the photo or clarify_log. Every
	// mutation stays under /api/food/*, owner-only. It never accepts
	// ?bucket= (see DataHandler), so it carries no aggregation family.
	"food_meal": {table: "food_meals", timeCol: "logged_at"},
}

// errInvalidBucket is returned (as HTTP 400) for an unrecognized ?bucket=
// value or a bucketed request against food_meal, which never aggregates.
var errInvalidBucket = errors.New("bucket must be 'day' or 'month'")

// writeAllowlist is the set of types POST /api/data/{type} accepts manual
// writes for. Deliberately narrow to single-value-column point types with a
// plausible manual-entry use case: blood_pressure/nutrition are multi-column
// and have no single `value` field to hang this contract on, and shipping a
// fully generic POST across every registered type with an unexplained
// exception for those two is worse than an explicit allowlist (see
// design.md).
var writeAllowlist = map[string]bool{
	"weight":      true,
	"height":      true,
	"weight_goal": true,
}

// writeBounds are per-type plausibility ranges in the type's stored unit
// (kilograms for weight/weight_goal, metres for height). A bare `> 0` check
// is not enough when one generic form serves units of wildly different
// magnitude: entering the natural "178" for a height stores 178 *metres*,
// which is accepted, and BMI then renders as 0.0 with category bands drawn
// around 586,000 kg. Rejecting it at the API keeps the bad value out of the
// database no matter which client wrote it.
var writeBounds = map[string]struct{ min, max float64 }{
	"weight":      {min: 20, max: 500},
	"weight_goal": {min: 20, max: 500},
	"height":      {min: 0.5, max: 2.5},
}

// clockSkewAllowance is how far ahead of the server a client's timestamp may
// be before it counts as future-dated. Small enough that a genuinely wrong
// date is still rejected, large enough that an unsynced client clock isn't.
const clockSkewAllowance = time.Minute

// createRecordRequest is the POST /api/data/{type} request body. Value is a
// pointer so a missing key is distinguishable from a present-but-zero value.
type createRecordRequest struct {
	Value *float64   `json:"value"`
	Time  *time.Time `json:"time"`
}

// CreateRecordHandler creates one manually-authored record for an
// allowlisted type, scoped to the caller (claims.UserID) — it does not
// honor ?user=, unlike the sibling GET, matching DeleteRecordHandler's
// convention that mutations on this path family are always caller-scoped.
// Exported for use in tests.
func CreateRecordHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		typeName := mux.Vars(r)["type"]
		info, ok := typeRegistry[typeName]
		if !ok {
			http.Error(w, "unknown type", http.StatusNotFound)
			return
		}
		if !writeAllowlist[typeName] {
			http.Error(w, "type is not writable", http.StatusForbidden)
			return
		}

		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		familyID := FamilyIDFromCtx(r)

		var req createRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.Value == nil || *req.Value <= 0 {
			http.Error(w, "value must be a positive number", http.StatusBadRequest)
			return
		}
		if b, ok := writeBounds[typeName]; ok && (*req.Value < b.min || *req.Value > b.max) {
			http.Error(w, fmt.Sprintf("value must be between %g and %g", b.min, b.max), http.StatusBadRequest)
			return
		}

		t := time.Now().UTC()
		if req.Time != nil {
			t = req.Time.UTC()
			// Every read path caps its upper bound at now, so a future-dated
			// row is created but never returned: it appears in no table, no
			// chart, no goal line and no BMI readout, which also means it has
			// no delete button. The unique (user_id, time) index then makes
			// re-entering that timestamp fail with 409 forever, describing a
			// record the user cannot see. Reject it at the door instead.
			// The skew allowance keeps a client clock a few seconds fast from
			// being an error.
			if t.After(time.Now().UTC().Add(clockSkewAllowance)) {
				http.Error(w, "time must not be in the future", http.StatusBadRequest)
				return
			}
		}

		record, err := storage.InsertRecord(info.table, info.timeCol, info.valueCol, familyID, claims.UserID, t, *req.Value)
		if err != nil {
			if errors.Is(err, database.ErrConflict) {
				http.Error(w, "record already exists for this time", http.StatusConflict)
				return
			}
			http.Error(w, "insert error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(record) //nolint:errcheck
	}
}

// queryBucketed dispatches a bucketed aggregation query to the right storage
// method: the two multi-value-column special cases, or the generic
// single-valueCol path for every other type.
func queryBucketed(
	storage database.Storage, typeName string, info typeInfo, bucket database.Bucket, userID uuid.UUID, tr database.TimeRange,
) ([]map[string]any, error) {
	if typeName == "food_meal" {
		return nil, errInvalidBucket
	}
	if bucket != database.BucketDay && bucket != database.BucketMonth {
		return nil, errInvalidBucket
	}
	switch typeName {
	case "blood_pressure":
		return storage.QueryAggregateBloodPressure(bucket, userID, tr)
	case "nutrition":
		return storage.QueryAggregateNutrition(bucket, userID, tr)
	default:
		return storage.QueryAggregate(info.table, info.timeCol, info.valueCol, info.family, bucket, userID, tr)
	}
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

// GetUserSettingsHandler returns the authenticated user's settings object, or
// {} if they have never saved one — a missing row is not an error. Exported
// for use in tests.
func GetUserSettingsHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		settingsJSON, err := storage.GetUserSettings(claims.UserID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				settingsJSON = "{}"
			} else {
				http.Error(w, "query error", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(settingsJSON)) //nolint:errcheck
	}
}

// maxSettingsBodyBytes bounds the request body read before decoding — the
// settings document is a small per-user preferences blob, not user-supplied
// data of unbounded size.
const maxSettingsBodyBytes = 64 * 1024

// PutUserSettingsHandler replaces the authenticated user's settings with the
// full JSON object in the request body — a full-document upsert, not a
// merge. The body must be a JSON object; anything else (malformed JSON, the
// literal `null`, or a bare array/scalar) is rejected with 400 and leaves
// the stored settings untouched. Exported for use in tests.
func PutUserSettingsHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		familyID := FamilyIDFromCtx(r)

		r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBodyBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
			http.Error(w, "body must be a JSON object", http.StatusBadRequest)
			return
		}

		// A timezone change invalidates any existing FoodDayCompletion rows
		// (their LocalDate was computed under the old zone) — see design.md
		// §4 "Storage" under openspec/changes/food-day-completeness. The
		// comparison is on the raw stored string, not a resolved zone: see
		// database.SettingsRawString.
		oldSettingsJSON, err := storage.GetUserSettings(claims.UserID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}
		timezoneChanged := database.SettingsRawString(oldSettingsJSON, "timezone") !=
			database.SettingsRawString(string(body), "timezone")

		if timezoneChanged {
			err = storage.UpsertUserSettingsClearingFoodDayCompletions(claims.UserID, familyID, string(body))
		} else {
			err = storage.UpsertUserSettings(claims.UserID, familyID, string(body))
		}
		if err != nil {
			http.Error(w, "save error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) //nolint:errcheck
	}
}

// DataHandler returns health records for a given type within a time range.
// The {type} URL param is validated against typeRegistry before use in SQL to
// prevent SQL injection through user-controlled table/column names.
// Exported for use in tests.
func DataHandler(storage database.Storage) http.HandlerFunc {
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
		tr := database.TimeRange{From: from, To: to}

		var records []map[string]any
		if bucketParam := r.URL.Query().Get("bucket"); bucketParam != "" {
			records, err = queryBucketed(storage, typeName, info, database.Bucket(bucketParam), targetUser.ID, tr)
			if err != nil {
				if errors.Is(err, errInvalidBucket) {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				http.Error(w, "query error", http.StatusInternalServerError)
				return
			}
		} else {
			records, err = storage.QueryRecords(info.table, info.timeCol, targetUser.ID, tr)
			if err != nil {
				http.Error(w, "query error", http.StatusInternalServerError)
				return
			}
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

// DataTypesPresenceHandler returns, for every type in typeRegistry, whether
// the resolved user has ever recorded at least one row of it — the signal the
// dashboard uses to hide types with no data at all (as opposed to the
// existing 7-day recency window, which is unrelated). One indexed COUNT
// round-trip per type against storage.DB() directly, mirroring
// NeedsAttentionCount's precedent; see design.md for why this is fine at this
// project's scale. Exported for use in tests.
func DataTypesPresenceHandler(storage database.Storage) http.HandlerFunc {
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

		presence := make(map[string]bool, len(typeRegistry))
		for name, info := range typeRegistry {
			var count int64
			if err := storage.DB().Table(info.table).
				Where("user_id = ?", targetUser.ID).
				Count(&count).Error; err != nil {
				http.Error(w, "query error", http.StatusInternalServerError)
				return
			}
			presence[name] = count > 0
		}

		writeJSON(w, presence)
	}
}

// DeleteRecordHandler hard-deletes a single health record owned by the authenticated user.
// photos may be nil; it is only consulted for the food_meal type, whose photo
// file must be removed alongside the row. Exported for use in tests.
func DeleteRecordHandler(storage database.Storage, photos *photostorage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		typeName := vars["type"]
		info, ok := typeRegistry[typeName]
		if !ok {
			http.Error(w, "unknown type", http.StatusNotFound)
			return
		}

		id, err := uuid.Parse(vars["id"])
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// The photo path must be read before the row is deleted; DeleteRecord
		// removes it, and food_meal's own column allowlist blocks reading
		// photo_path back out through the generic query path.
		var photoPath string
		if typeName == "food_meal" {
			storage.DB().Table(info.table).
				Where("id = ? AND user_id = ?", id, claims.UserID).
				Limit(1).Pluck("photo_path", &photoPath) //nolint:errcheck // best-effort; absence just skips cleanup
		}

		if err := storage.DeleteRecord(info.table, id, claims.UserID); err != nil {
			if errors.Is(err, database.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, "delete error", http.StatusInternalServerError)
			return
		}

		if photoPath != "" && photos != nil {
			photos.Remove(photoPath) //nolint:errcheck // best-effort; the row is already gone either way
		}
		w.WriteHeader(http.StatusNoContent)
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
