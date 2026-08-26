package server

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// maxCompletenessRangeDays is the inclusive-day cap on a single
// GET /api/food/completeness request (design.md §5 "API").
const maxCompletenessRangeDays = 92

// GetCompleteness handles GET /api/food/completeness?from=YYYY-MM-DD&to=YYYY-MM-DD:
// the caller's per-day Day Completeness state across an inclusive Logged-Day
// range (design.md §5 "API"). Scoped strictly to the caller — no ?user=
// override, matching the manual-write endpoint's convention from ADR-005:
// this is a personal assertion, not shared family data.
func (h *foodHandlers) GetCompleteness(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}
	fromDate, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		http.Error(w, "invalid from date", http.StatusBadRequest)
		return
	}
	toDate, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		http.Error(w, "invalid to date", http.StatusBadRequest)
		return
	}

	loc, threshold, err := h.callerTimezoneAndThreshold(claims.UserID)
	if err != nil {
		slog.Error("GetCompleteness: read user settings", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	// Clamp `to` to yesterday in the caller's zone *first* — only then is
	// `from > to` (using the now-possibly-clamped `to`) validated. This
	// order matters: a `from` naming today or a future date must fail the
	// from > to check post-clamp, not resolve to an inverted/empty range
	// (design.md §5 "API").
	todayStr := database.LocalDate(time.Now(), loc)
	todayDate, err := time.Parse("2006-01-02", todayStr)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	yesterdayDate := todayDate.AddDate(0, 0, -1)
	if toDate.After(yesterdayDate) {
		toDate = yesterdayDate
		toStr = toDate.Format("2006-01-02")
	}

	if toDate.Before(fromDate) {
		http.Error(w, "from must not be after to", http.StatusBadRequest)
		return
	}
	// fromDate/toDate are both UTC midnights parsed from plain YYYY-MM-DD
	// strings, so the difference is always an exact multiple of 24h —
	// no DST ambiguity here, unlike computing this from loc-based times.
	days := int(toDate.Sub(fromDate).Hours()/24) + 1
	if days > maxCompletenessRangeDays {
		http.Error(w, "range must not exceed 92 days", http.StatusBadRequest)
		return
	}

	result, err := database.DayRange(h.storage.DB(), claims.UserID, loc, threshold, fromStr, toStr)
	if err != nil {
		slog.Error("GetCompleteness: compute day range", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}

// callerTimezoneAndThreshold resolves the caller's timezone and
// usual_meals_per_day threshold from their stored settings. "No settings
// row yet" is the ordinary case for a user who has never opened settings —
// same treatment as DisplayLanguage (display_language.go): fall back to
// defaults rather than surfacing an error. Any other read error is
// propagated to the caller to turn into a 500.
func (h *foodHandlers) callerTimezoneAndThreshold(userID uuid.UUID) (*time.Location, int, error) {
	settingsJSON, err := h.callerSettingsJSON(userID)
	if err != nil {
		return nil, 0, err
	}
	return database.ResolveTimezone(settingsJSON), database.ResolveUsualMealsPerDay(settingsJSON), nil
}

// callerTimezone resolves just the caller's timezone from their stored
// settings — the half of callerTimezoneAndThreshold that GetFoodDailyTotals
// needs, which has no usual_meals_per_day threshold to resolve.
func (h *foodHandlers) callerTimezone(userID uuid.UUID) (*time.Location, error) {
	settingsJSON, err := h.callerSettingsJSON(userID)
	if err != nil {
		return nil, err
	}
	return database.ResolveTimezone(settingsJSON), nil
}

// callerSettingsJSON reads the caller's raw UserSettings JSON blob, treating
// "no settings row yet" as the ordinary case for a user who has never opened
// settings (same fail-open precedent as DisplayLanguage) rather than an
// error.
func (h *foodHandlers) callerSettingsJSON(userID uuid.UUID) (string, error) {
	settingsJSON, err := h.storage.GetUserSettings(userID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		return "", nil
	}
	return settingsJSON, nil
}

// parseCompletenessDate validates the {date} path parameter shared by
// ConfirmDay and UnconfirmDay: well-formed YYYY-MM-DD, and not naming today
// or a future date in the caller's zone (design.md §5 "API" — confirmation
// only applies to a day whose Logged Day has already closed).
func parseCompletenessDate(r *http.Request, loc *time.Location) (string, bool) {
	dateStr := mux.Vars(r)["date"]
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		return "", false
	}
	if dateStr >= database.LocalDate(time.Now(), loc) {
		return "", false
	}
	return dateStr, true
}

// confirmDayResponse is the body of a successful ConfirmDay response
// (design.md §5 "API": "A fresh confirmation returns 201 with
// `{date, state, confirmed_at}`") — deliberately not the raw
// database.FoodDayCompletion persistence model, whose Go field names
// (LocalDate, no State at all) don't match that contract.
type confirmDayResponse struct {
	Date        string    `json:"date"`
	State       string    `json:"state"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

func newConfirmDayResponse(row database.FoodDayCompletion) confirmDayResponse {
	return confirmDayResponse{
		Date:        row.LocalDate,
		State:       database.DayStateConfirmedComplete,
		ConfirmedAt: row.ConfirmedAt,
	}
}

// ConfirmDay handles POST /api/food/completeness/{date}/confirm: the
// caller's assertion that a below-threshold day (Unconfirmed) is
// nonetheless complete (design.md §5 "API"). Rejects a day that is already
// Complete (threshold met — nothing to assert) or Incomplete (zero
// occasions — nothing to confirm). Idempotent: confirming an
// already-Confirmed-Complete day returns 200 with the existing row rather
// than erroring or creating a duplicate.
func (h *foodHandlers) ConfirmDay(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	loc, threshold, err := h.callerTimezoneAndThreshold(claims.UserID)
	if err != nil {
		slog.Error("ConfirmDay: read user settings", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	dateStr, ok := parseCompletenessDate(r, loc)
	if !ok {
		http.Error(w, "invalid or ineligible date", http.StatusBadRequest)
		return
	}

	entries, err := database.DayRange(h.storage.DB(), claims.UserID, loc, threshold, dateStr, dateStr)
	if err != nil {
		slog.Error("ConfirmDay: compute day state", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if len(entries) != 1 {
		slog.Error("ConfirmDay: unexpected day range length", "len", len(entries), "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	switch entries[0].State {
	case database.DayStateIncomplete, database.DayStateComplete:
		http.Error(w, "day is not eligible for confirmation", http.StatusBadRequest)
		return
	}

	var existing database.FoodDayCompletion
	err = h.storage.DB().
		Where("user_id = ? AND local_date = ?", claims.UserID, dateStr).
		First(&existing).Error
	switch {
	case err == nil:
		writeJSONStatus(w, http.StatusOK, newConfirmDayResponse(existing))
	case errors.Is(err, gorm.ErrRecordNotFound):
		row := database.FoodDayCompletion{
			UserID:      claims.UserID,
			LocalDate:   dateStr,
			ConfirmedAt: time.Now().UTC(),
		}
		row.ID = uuid.New()
		row.FamilyID = FamilyIDFromCtx(r)
		if err := h.storage.DB().Create(&row).Error; err != nil {
			if isUniqueViolation(err) {
				// Lost a concurrent create race for the same (user_id,
				// local_date) pair; the winner's row is the assertion,
				// so treat this the same as the err == nil branch above.
				if reErr := h.storage.DB().
					Where("user_id = ? AND local_date = ?", claims.UserID, dateStr).
					First(&existing).Error; reErr == nil {
					writeJSONStatus(w, http.StatusOK, newConfirmDayResponse(existing))
					return
				}
			}
			slog.Error("ConfirmDay: create confirmation", "err", err, "user_id", claims.UserID)
			http.Error(w, "create error", http.StatusInternalServerError)
			return
		}
		writeJSONStatus(w, http.StatusCreated, newConfirmDayResponse(row))
	default:
		slog.Error("ConfirmDay: query confirmation", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
	}
}

// UnconfirmDay handles DELETE /api/food/completeness/{date}/confirm:
// retracts a confirmation (design.md §5 "API"). Always 204, including when
// no confirmation exists for that date — retracting a non-assertion is a
// no-op, not an error.
//
// Unscoped: FoodDayCompletion embeds TenantModel, so a plain Delete
// soft-deletes (sets deleted_at) rather than removing the row, and the
// (user_id, local_date) unique index has no deleted_at clause — a
// soft-deleted row would permanently block re-confirming that date, the
// same trap DeleteCustomFood (food_custom.go) already guards against.
func (h *foodHandlers) UnconfirmDay(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	loc, _, err := h.callerTimezoneAndThreshold(claims.UserID)
	if err != nil {
		slog.Error("UnconfirmDay: read user settings", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	dateStr, ok := parseCompletenessDate(r, loc)
	if !ok {
		http.Error(w, "invalid or ineligible date", http.StatusBadRequest)
		return
	}

	if err := h.storage.DB().Unscoped().
		Where("user_id = ? AND local_date = ?", claims.UserID, dateStr).
		Delete(&database.FoodDayCompletion{}).Error; err != nil {
		slog.Error("UnconfirmDay: delete confirmation", "err", err, "user_id", claims.UserID)
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
