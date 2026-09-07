package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

// stepsDiagnosticDay is one UTC calendar day's row in the
// GET /api/data/steps/diagnostics response — the six numbers the
// check-the-health-data spec uses to attribute an inflated steps total to a
// specific layer: duplicate intervals in the database (RawSum >
// CollapsedSum), more than one sync writing the same day (PayloadCount > 1),
// or a day-boundary mismatch between the chart's UTC day and the caller's
// local day (LocalDaySum != CollapsedSum).
type stepsDiagnosticDay struct {
	BucketStart    string `json:"bucket_start"`
	RawCount       int    `json:"raw_count"`
	RawSum         int    `json:"raw_sum"`
	CollapsedSum   int    `json:"collapsed_sum"`
	DroppedRecords int    `json:"dropped_records"`
	PayloadCount   int    `json:"payload_count"`
	LocalDaySum    int    `json:"local_day_sum"`
}

// StepsDiagnosticsHandler returns, per UTC day in the requested range, the
// raw vs. collapsed step totals for the resolved user (self, or a named
// family member — see resolveUser). Self/family-only and read-only: it
// never writes, and it applies the same access check DataHandler does.
// Exported for use in tests.
func StepsDiagnosticsHandler(storage database.Storage) http.HandlerFunc {
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

		var rows []database.Steps
		if err := storage.DB().
			Where("user_id = ? AND start_time >= ? AND start_time <= ?", targetUser.ID, from, to).
			Order("start_time, end_time").
			Find(&rows).Error; err != nil {
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}

		settingsJSON, err := storage.GetUserSettings(targetUser.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}
		// ResolveTimezone falls back to UTC on a missing or invalid zone. When
		// the resolved zone is UTC, local_day_sum always equals collapsed_sum
		// (the local and UTC bucket keys below are then computed from the same
		// day boundary) — that equality is expected, not a bug.
		loc := database.ResolveTimezone(settingsJSON)

		writeJSON(w, computeStepsDiagnostics(rows, loc))
	}
}

// computeStepsDiagnostics folds rows (already ordered by StartTime, then
// EndTime) into one stepsDiagnosticDay per UTC day that has at least one
// record.
//
// It applies the same watermark rule as CollapseOverlappingSteps
// (steps_overlap.go) — a record whose EndTime doesn't extend past the
// running watermark is fully covered by already-counted time and is
// dropped; every other record is kept whole and advances the watermark —
// duplicated here rather than shared, because CollapseOverlappingSteps
// takes an unexported type and this walk needs to accumulate several
// per-day tallies (raw, collapsed, dropped, payloads, local-day) alongside
// the keep/drop decision in the same single pass.
//
// A kept record is credited to two bucket keys at once: its own UTC day
// (for CollapsedSum) and its own day in loc (for LocalDaySum). Both keys
// come from the one shared watermark, so the two totals differ only by
// which day boundary a record falls on either side of — never by a
// different collapse outcome.
func computeStepsDiagnostics(rows []database.Steps, loc *time.Location) []stepsDiagnosticDay {
	type dayAccum struct {
		rawCount     int
		rawSum       int
		collapsedSum int
		dropped      int
		payloads     map[uuid.UUID]struct{}
	}
	byUTCDay := make(map[string]*dayAccum)
	var utcDayOrder []string
	localDaySum := make(map[string]int)

	var watermark time.Time
	for _, row := range rows {
		utcKey := row.StartTime.UTC().Format("2006-01-02")
		acc, ok := byUTCDay[utcKey]
		if !ok {
			acc = &dayAccum{payloads: make(map[uuid.UUID]struct{})}
			byUTCDay[utcKey] = acc
			utcDayOrder = append(utcDayOrder, utcKey)
		}
		acc.rawCount++
		acc.rawSum += row.Count
		acc.payloads[row.SourcePayloadID] = struct{}{}

		if !row.EndTime.After(watermark) {
			acc.dropped++
			continue
		}
		watermark = row.EndTime
		acc.collapsedSum += row.Count
		localDaySum[row.StartTime.In(loc).Format("2006-01-02")] += row.Count
	}

	days := make([]stepsDiagnosticDay, len(utcDayOrder))
	for i, key := range utcDayOrder {
		acc := byUTCDay[key]
		days[i] = stepsDiagnosticDay{
			BucketStart:    key + "T00:00:00Z",
			RawCount:       acc.rawCount,
			RawSum:         acc.rawSum,
			CollapsedSum:   acc.collapsedSum,
			DroppedRecords: acc.dropped,
			PayloadCount:   len(acc.payloads),
			LocalDaySum:    localDaySum[key],
		}
	}
	return days
}
