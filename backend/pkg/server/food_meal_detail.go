package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// loadOwnedMeal loads a meal with its items, scoped to userID. Returns
// database.ErrNotFound if it does not exist or belongs to someone else.
func (h *foodHandlers) loadOwnedMeal(id, userID uuid.UUID) (*database.FoodMeal, error) {
	var meal database.FoodMeal
	err := h.storage.DB().Preload("Items").
		Where("id = ? AND user_id = ?", id, userID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &meal, nil
}

// GetMeal handles GET /api/food/meals/{id}: the meal with its items, for the
// review UI. Owner-only — unlike GET /api/data/food_meal, which is
// family-visible but omits items, photo, and raw_response.
func (h *foodHandlers) GetMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	meal, err := h.loadOwnedMeal(id, claims.UserID)
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, meal)
}

type confirmMealRequest struct {
	LoggedAt *time.Time `json:"logged_at,omitempty"`
}

// ConfirmMeal handles PUT /api/food/meals/{id}/confirm: aggregates the 7
// macros across items whose macro_source is reference or manual, stores the
// aggregate on the meal, optionally corrects logged_at, and sets status
// confirmed. Never writes to the Nutrition table.
func (h *foodHandlers) ConfirmMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	meal, err := h.loadOwnedMeal(id, claims.UserID)
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	// Only pending_review makes sense to confirm: processing is a live or
	// stale analysis (racing its own status write), failed has no usable
	// items, pending_clarification hasn't finished resolving items, and
	// confirmed has already run once — PATCH .../items rejects edits past
	// that point, so a second confirm could only recompute the same
	// aggregate, not a meaningful re-confirmation.
	if meal.Status != database.MealStatusPendingReview {
		http.Error(w, "meal is not ready to confirm", http.StatusConflict)
		return
	}

	var req confirmMealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.LoggedAt != nil {
		meal.LoggedAt = *req.LoggedAt
	}
	meal.Aggregate(meal.Items)
	meal.Status = database.MealStatusConfirmed

	updates := map[string]any{
		"status":              meal.Status,
		"logged_at":           meal.LoggedAt,
		"calories":            meal.Calories,
		"protein_grams":       meal.ProteinGrams,
		"carbs_grams":         meal.CarbsGrams,
		"fat_grams":           meal.FatGrams,
		"sugar_grams":         meal.SugarGrams,
		"sodium_grams":        meal.SodiumGrams,
		"dietary_fiber_grams": meal.DietaryFiberGrams,
	}
	// Conditional on status still being pending_review, not a blind write:
	// Reanalyze is now eligible from pending_review too, so a concurrent
	// reanalysis could have already moved this meal to processing/pending_*
	// between the read above and this write. Without this guard, confirm
	// would blindly stomp the status back to confirmed — and persist an
	// aggregate computed from the item set as it stood before this request
	// even loaded, ignoring whatever reanalyze is doing to it — regardless
	// of what else has happened to the meal in between.
	res := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status = ?", meal.ID, database.MealStatusPendingReview).
		Updates(updates)
	if res.Error != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	if res.RowsAffected == 0 {
		http.Error(w, "meal is not ready to confirm", http.StatusConflict)
		return
	}
	writeJSON(w, meal)
}

// MealSummary is the response shape for GET /api/food/meals: enough for a
// history list. Deliberately narrower than the full FoodMeal returned by
// GetMeal — it excludes photo_path (a server-internal path), raw_response
// (the full model JSON blob), clarify_log, and tenant metadata, none of
// which the list view needs.
type MealSummary struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	LoggedAt time.Time `json:"logged_at"`
	Status   string    `json:"status"`
	Calories float64   `json:"calories"`
}

const (
	defaultMealListLimit = 50
	maxMealListLimit     = 200
)

// ListMeals handles GET /api/food/meals?limit=&before=&before_id=: the
// caller's own meals of any status, ordered most-recent-first. Owner-scoped
// by claims.UserID — unlike GET /api/data/food_meal, which is family-visible
// — so every summary returned is guaranteed openable via GetMeal. `before`
// (paired with `before_id`) lets the caller page past `limit`, since a hard
// cap with no way past it would make older meals permanently unreachable.
//
// Ordering and paging use (logged_at, id) as a keyset cursor, not a
// timestamp-only one. `id` alone is already a complete, collision-free
// tie-break (it's the primary key), so the pair fully determines a total
// order with no third field needed. A timestamp-only cursor cannot: two
// meals can share the exact logged_at (plausible — logged_at is frequently
// corrected via a minute-granularity datetime picker), and if a plain
// `LIMIT` split such a tied group across a page boundary, the next page's
// `logged_at < before` filter would exclude the rest of that group forever,
// since they're not strictly before the cursor either. Keying the cursor on
// the exact (logged_at, id) of the last row returned — `(logged_at, id) <
// (before, before_id)` in the same DESC order the list uses — always resumes
// at exactly the next row, tied group or not, so nothing can be dropped.
func (h *foodHandlers) ListMeals(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := defaultMealListLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > maxMealListLimit {
			http.Error(w, "limit must be a positive integer up to 200", http.StatusBadRequest)
			return
		}
		limit = n
	}

	query := h.storage.DB().Model(&database.FoodMeal{}).
		Where("user_id = ?", claims.UserID).
		Order("logged_at DESC, id DESC").
		Limit(limit)

	beforeStr := r.URL.Query().Get("before")
	beforeIDStr := r.URL.Query().Get("before_id")
	switch {
	case beforeStr != "" && beforeIDStr != "":
		before, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			http.Error(w, "before must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		beforeID, err := uuid.Parse(beforeIDStr)
		if err != nil {
			http.Error(w, "before_id must be a UUID", http.StatusBadRequest)
			return
		}
		// Exact keyset continuation: strictly after (in DESC order) the row
		// identified by (before, before_id), regardless of any ties at that
		// logged_at.
		query = query.Where("(logged_at < ? OR (logged_at = ? AND id < ?))", before, before, beforeID)
	case beforeStr != "":
		// before without before_id: a plain "meals logged before this
		// instant" filter, not a guaranteed-lossless continuation of a prior
		// page. The history page always supplies both together; this form
		// exists for a caller that just wants a date cutoff.
		before, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			http.Error(w, "before must be an RFC3339 timestamp", http.StatusBadRequest)
			return
		}
		query = query.Where("logged_at < ?", before)
	case beforeIDStr != "":
		http.Error(w, "before_id requires before", http.StatusBadRequest)
		return
	}

	var meals []database.FoodMeal
	if err := query.Find(&meals).Error; err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	summaries := make([]MealSummary, len(meals))
	for i, m := range meals {
		summaries[i] = MealSummary{ID: m.ID, Name: m.Name, LoggedAt: m.LoggedAt, Status: m.Status, Calories: m.Calories}
	}
	writeJSON(w, summaries)
}

// patchMealRequest is the JSON body for PATCH /api/food/meals/{id}: correct a
// meal's name and/or logged_at independently of confirming it.
type patchMealRequest struct {
	Name     *string    `json:"name,omitempty"`
	LoggedAt *time.Time `json:"logged_at,omitempty"`
}

// PatchMeal handles PATCH /api/food/meals/{id}. Permitted while the meal's
// status is pending_review or confirmed, same as item edits — see
// editableMealStatus in food_item.go. Rejects a zero-value logged_at to
// preserve the existing invariant that every meal carries a non-zero one
// (see the Meal Logged Time requirement).
func (h *foodHandlers) PatchMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Select("id", "status").
		Where("id = ? AND user_id = ?", id, claims.UserID).First(&meal).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	if !editableMealStatus(meal.Status) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}

	var req patchMealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.LoggedAt != nil && req.LoggedAt.IsZero() {
		http.Error(w, "logged_at must not be zero", http.StatusBadRequest)
		return
	}

	hasName := req.Name != nil && strings.TrimSpace(*req.Name) != ""
	hasLoggedAt := req.LoggedAt != nil
	if !hasName && !hasLoggedAt {
		http.Error(w, "at least one of name or logged_at is required", http.StatusBadRequest)
		return
	}

	updates := map[string]any{}
	if hasName {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if hasLoggedAt {
		updates["logged_at"] = *req.LoggedAt
	}
	// Conditional on status still being editable, not a blind write — same
	// reasoning as ConfirmMeal: the status could have moved between the
	// check above and this write (e.g. a concurrent Reanalyze claimed the
	// meal), and this write must not silently apply against a meal that's
	// no longer in a state this handler is meant to touch.
	res := h.storage.DB().Model(&database.FoodMeal{}).
		Where("id = ? AND status IN ?", id, []string{database.MealStatusPendingReview, database.MealStatusConfirmed}).
		Updates(updates)
	if res.Error != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	if res.RowsAffected == 0 {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}

	updated, err := h.loadOwnedMeal(id, claims.UserID)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, updated)
}
