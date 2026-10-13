package server

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"net/http"
	"regexp"
	"time"
)

const diagnosticRetention = 365 * 24 * time.Hour

var diagnosticVersion = regexp.MustCompile(`^[a-zA-Z0-9.+_-]{1,80}$`)
var errDiagnosticConflict = errors.New("diagnostic id conflict")

func decodeBoundedJSON(w http.ResponseWriter, r *http.Request, max int64, value any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func validDiagnostic(e database.ClientDiagnosticEvent) bool {
	id, err := uuid.Parse(e.ID)
	rid, re := uuid.Parse(e.RequestID)
	if err != nil || re != nil || id == uuid.Nil || rid == uuid.Nil || e.ID != id.String() || e.RequestID != rid.String() {
		return false
	}
	if !validDiagnosticStage(e.Operation, e.Category) {
		return false
	}
	if e.WorkID != "" {
		workID, err := uuid.Parse(e.WorkID)
		if err != nil || workID == uuid.Nil || e.WorkID != workID.String() {
			return false
		}
	}
	switch e.Trigger {
	case "", "food_entry", "manual", "app_resume", "placement", "periodic", "retry":
	default:
		return false
	}
	switch e.Provider {
	case "", "home", "flex_window":
	default:
		return false
	}
	if e.SnapshotAgeMillis != nil && *e.SnapshotAgeMillis < 0 {
		return false
	}
	// Phone clocks can be skewed. Retention uses the server receipt time.
	return !e.OccurredAt.IsZero() &&
		e.HTTPCode >= 0 && e.HTTPCode <= 599 && (e.HTTPCode == 0 || e.HTTPCode >= 100) && e.DurationMillis >= 0 && e.DurationMillis <= 24*60*60*1000 &&
		e.Attempt >= 1 && e.Attempt <= 1000000 && diagnosticVersion.MatchString(e.AppVersion) && e.AndroidAPI >= 26 && e.AndroidAPI <= 100
}

// Widget stages use a closed vocabulary; arbitrary client text is never persisted.
func validDiagnosticStage(operation, category string) bool {
	allowed := map[string][]string{
		"widget_browser":  {"launched", "returned", "cancelled", "failed"},
		"widget_work":     {"enqueue_requested", "replacement_requested", "started", "deferred", "cancelled", "failed", "success"},
		"widget_snapshot": {"success", "rejected"},
		"widget_redraw":   {"started", "success", "failed", "cancelled"},
	}
	if categories, widget := allowed[operation]; widget {
		for _, c := range categories {
			if category == c {
				return true
			}
		}
		return false
	}
	switch operation {
	case "summary", "weather", "auth_login", "auth_refresh", "widget_fetch":
	default:
		return false
	}
	switch category {
	case "success", "recovered", "unauthenticated", "rate_limited", "access_challenge", "dns", "timeout", "tls", "network", "server", "invalid_response":
		return true
	default:
		return false
	}
}

func sameOptional[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// DiagnosticsHandler is self-only. Receipts are emitted only after the transaction commits.
func DiagnosticsHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		now := time.Now().UTC()
		if r.Method == http.MethodGet {
			events := []database.ClientDiagnosticEvent{}
			if err := storage.DB().Where("user_id = ? AND received_at >= ?", claims.UserID, now.Add(-diagnosticRetention)).Order("received_at DESC, id DESC").Limit(100).Find(&events).Error; err != nil {
				http.Error(w, "query error", 500)
				return
			}
			writeJSON(w, map[string]any{"events": events})
			return
		}
		var input struct {
			Events []database.ClientDiagnosticEvent `json:"events"`
		}
		if !decodeBoundedJSON(w, r, 32768, &input) {
			return
		}
		if len(input.Events) == 0 || len(input.Events) > 50 {
			http.Error(w, "invalid batch", 400)
			return
		}
		ids := []string{}
		for i := range input.Events {
			e := &input.Events[i]
			if !validDiagnostic(*e) {
				http.Error(w, "invalid event", 400)
				return
			}
			e.UserID = claims.UserID
			e.OccurredAt = e.OccurredAt.UTC()
			e.ReceivedAt = now
			ids = append(ids, e.ID)
		}
		err := storage.DB().Transaction(func(tx *gorm.DB) error {
			for _, e := range input.Events {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&e).Error; err != nil {
					return err
				}
				var old database.ClientDiagnosticEvent
				if err := tx.Where("user_id = ? AND id = ?", claims.UserID, e.ID).First(&old).Error; err != nil {
					return err
				}
				old.ReceivedAt = e.ReceivedAt
				if !sameOptional(old.SnapshotAgeMillis, e.SnapshotAgeMillis) || !sameOptional(old.SnapshotChanged, e.SnapshotChanged) {
					return errDiagnosticConflict
				}
				// Compare optional metadata by value, not decoder/ORM pointer identity.
				old.SnapshotAgeMillis = e.SnapshotAgeMillis
				old.SnapshotChanged = e.SnapshotChanged
				if old != e {
					return errDiagnosticConflict
				}
			}
			if err := tx.Where("user_id = ? AND received_at < ?", claims.UserID, now.Add(-diagnosticRetention)).Delete(&database.ClientDiagnosticEvent{}).Error; err != nil {
				return err
			}
			return nil
		})
		if errors.Is(err, errDiagnosticConflict) {
			http.Error(w, "event id conflict", 409)
			return
		}
		if err != nil {
			http.Error(w, "save error", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted_ids": ids})
	}
}
