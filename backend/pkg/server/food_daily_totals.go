package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// maxDailyTotalsRangeDays is the inclusive-day cap on a single
// GET /api/food/daily-totals request, matching GetCompleteness's cap
// (design.md §7 "Backend: GET /api/food/daily-totals").
const maxDailyTotalsRangeDays = 92

// GetFoodDailyTotals handles GET /api/food/daily-totals?from=YYYY-MM-DD&to=YYYY-MM-DD:
// the caller's daily logged-calorie totals across an inclusive Logged-Day
// range (design.md §7 "Backend: GET /api/food/daily-totals"). Scoped
// strictly to the caller — no ?user= override, same convention as
// GetCompleteness.
func (h *foodHandlers) GetFoodDailyTotals(w http.ResponseWriter, r *http.Request) {
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

	loc, err := h.callerTimezone(claims.UserID)
	if err != nil {
		slog.Error("GetFoodDailyTotals: read user settings", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	// Clamp `to` to yesterday in the caller's zone *first* — only then is
	// `from > to` (using the now-possibly-clamped `to`) validated. Same
	// order as GetCompleteness, and for the same reason: a `from` naming
	// today or a future date must fail the from > to check post-clamp, not
	// resolve to an inverted/empty range (design.md §7).
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
	// strings, so the difference is always an exact multiple of 24h — no DST
	// ambiguity here, unlike computing this from loc-based times.
	days := int(toDate.Sub(fromDate).Hours()/24) + 1
	if days > maxDailyTotalsRangeDays {
		http.Error(w, "range must not exceed 92 days", http.StatusBadRequest)
		return
	}

	result, err := database.DailyTotalsRange(h.storage.DB(), claims.UserID, loc, fromStr, toStr)
	if err != nil {
		slog.Error("GetFoodDailyTotals: compute daily totals", "err", err, "user_id", claims.UserID)
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
