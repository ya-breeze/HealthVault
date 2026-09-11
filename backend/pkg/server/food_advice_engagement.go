package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// The one engagement event. Named rather than written as a literal at each
// call site, because an unrecognized event is only caught at run time by
// recordFoodAdviceEngagement's default branch.
const qualifiedViewEvent = "qualified_view"

type foodAdviceEngagementRequest struct {
	Event       string    `json:"event"`
	LoggedDay   string    `json:"logged_day"`
	GeneratedAt time.Time `json:"generated_at"`
}

// RecordFoodAdviceEngagement records a qualified view of an exact advice
// revision, which is the only engagement event left: the refresh events went
// with the refresh control on 2026-09-11.
func (h *foodHandlers) RecordFoodAdviceEngagement(w http.ResponseWriter, r *http.Request) {
	if !isSameOriginRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req foodAdviceEngagementRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Event != qualifiedViewEvent || !validLoggedDay(req.LoggedDay) || req.GeneratedAt.IsZero() {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var advice database.FoodAdvice
	err := h.storage.DB().Where(
		"user_id = ? AND logged_day = ? AND generated_at = ?",
		claims.UserID, req.LoggedDay, req.GeneratedAt.UTC(),
	).First(&advice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "advice revision not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "engagement unavailable", http.StatusInternalServerError)
		return
	}

	if err := recordFoodAdviceEngagement(
		h.storage.DB(), claims.UserID, FamilyIDFromCtx(r), req.LoggedDay, qualifiedViewEvent, time.Now().UTC(),
	); err != nil {
		http.Error(w, "engagement unavailable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validLoggedDay(day string) bool {
	parsed, err := time.Parse("2006-01-02", day)
	return err == nil && parsed.Format("2006-01-02") == day
}

func recordFoodAdviceEngagement(
	db *gorm.DB, userID, familyID uuid.UUID, loggedDay, event string, at time.Time,
) error {
	row := database.FoodAdviceEngagement{UserID: userID, LoggedDay: loggedDay}
	row.ID = uuid.New()
	row.FamilyID = familyID
	at = at.UTC()

	var countColumn, firstColumn, lastColumn string
	switch event {
	case qualifiedViewEvent:
		row.QualifiedViewCount = 1
		row.FirstQualifiedViewAt = &at
		row.LastQualifiedViewAt = &at
		countColumn, firstColumn, lastColumn = "qualified_view_count", "first_qualified_view_at", "last_qualified_view_at"
	default:
		return errors.New("unknown food advice engagement event")
	}

	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "logged_day"}},
		DoUpdates: clause.Assignments(map[string]any{
			countColumn: gorm.Expr(countColumn + " + 1"),
			firstColumn: gorm.Expr(
				"CASE WHEN " + firstColumn + " IS NULL OR " + firstColumn + " > excluded." + firstColumn +
					" THEN excluded." + firstColumn + " ELSE " + firstColumn + " END",
			),
			lastColumn: gorm.Expr(
				"CASE WHEN " + lastColumn + " IS NULL OR " + lastColumn + " < excluded." + lastColumn +
					" THEN excluded." + lastColumn + " ELSE " + lastColumn + " END",
			),
			"updated_at": at,
		}),
	}).Create(&row).Error
}
