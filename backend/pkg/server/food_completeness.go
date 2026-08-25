package server

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

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

	settingsJSON, err := h.storage.GetUserSettings(claims.UserID)
	if err != nil {
		// "No settings row yet" is the ordinary case for a user who has
		// never opened settings — same treatment as DisplayLanguage
		// (display_language.go): fall back to defaults rather than 500.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("GetCompleteness: read user settings", "err", err, "user_id", claims.UserID)
			http.Error(w, "query error", http.StatusInternalServerError)
			return
		}
		settingsJSON = ""
	}
	loc := database.ResolveTimezone(settingsJSON)
	threshold := database.ResolveUsualMealsPerDay(settingsJSON)

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
