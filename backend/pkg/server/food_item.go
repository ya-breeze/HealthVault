package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// patchItemRequest is the JSON body for PATCH /api/food/meals/{id}/items/{item_id}.
// Exactly one of Manual/FdcID-or-CustomFoodID/WeightGrams-alone applies, in
// this precedence:
//  1. Manual: the 7 macro fields are stored as given, macro_source = manual.
//  2. FdcID or CustomFoodID: bound to that reference food, macro_source =
//     reference, macros scaled from its profile by WeightGrams.
//  3. WeightGrams alone: rescales the item from its existing binding, if
//     any — a reference food's profile, or (for macro_source = estimated) its
//     own persisted per-100g estimate, the same way a reference item does.
//
// Name is independent of the above and may be sent alongside any of them, or
// alone: it corrects the item's displayed description (e.g. the vision
// model's guess was "dark berries" but it's actually cherries) without
// implying anything about macro_source.
//
// SaveAsCustomFood, alongside Manual, additionally creates a CustomFood
// owned by the caller from the item's (possibly just-corrected) name and the
// supplied per-100g-converted macro values — see design.md decision 5. It has
// no effect without Manual: there is no other supplied per-100g profile to
// save.
type patchItemRequest struct {
	Manual           bool       `json:"manual,omitempty"`
	FdcID            *int64     `json:"fdc_id,omitempty"`
	CustomFoodID     *uuid.UUID `json:"custom_food_id,omitempty"`
	OffCode          *string    `json:"off_code,omitempty"`
	WeightGrams      *float64   `json:"weight_grams,omitempty"`
	Name             *string    `json:"name,omitempty"`
	SaveAsCustomFood bool       `json:"save_as_custom_food,omitempty"`

	Calories          float64 `json:"calories,omitempty"`
	ProteinGrams      float64 `json:"protein_grams,omitempty"`
	CarbsGrams        float64 `json:"carbs_grams,omitempty"`
	FatGrams          float64 `json:"fat_grams,omitempty"`
	SugarGrams        float64 `json:"sugar_grams,omitempty"`
	SodiumGrams       float64 `json:"sodium_grams,omitempty"`
	DietaryFiberGrams float64 `json:"dietary_fiber_grams,omitempty"`
}

// editableMealStatus reports whether a meal's items (and its name/logged_at)
// can be mutated. pending_review and confirmed both have a stable, reviewable
// item set; processing/pending_clarification/failed do not.
func editableMealStatus(status string) bool {
	return status == database.MealStatusPendingReview || status == database.MealStatusConfirmed
}

// errMealNoLongerEditable is returned by applyItemMutation when the meal's
// status has moved out of pending_review/confirmed between the handler's
// initial status check and the transaction actually running — e.g. a
// concurrent Reanalyze claimed the meal (and, on success, replaced its items
// and zeroed its aggregate) in that window. Callers map it to HTTP 409.
var errMealNoLongerEditable = errors.New("meal is no longer editable")

// errItemMutationConflictExhausted is returned by applyItemMutation when
// every bounded retry attempt still hit SQLITE_BUSY_SNAPSHOT while the meal
// itself remained eligible on every re-check — i.e. this was not a one-off
// race a retry could clear, but sustained conflicting writes (most often to
// the very row this request is trying to change; see
// TestPatchMealItem_PersistentBusySnapshotReturns409AfterBoundedRetries).
// The item-resolution requirement's contract is explicit that this
// outcome — a write still based on a stale read after every retry attempt
// is exhausted — is HTTP 409, not a generic server error: distinct from
// errMealNoLongerEditable, whose 409 is about the *meal's* status rather
// than a specific write repeatedly losing a race.
var errItemMutationConflictExhausted = errors.New(
	"write repeatedly conflicted with concurrent activity and was not applied")

// itemMutationError carries an HTTP status/message pair out of an
// applyItemMutation closure for a validation failure that can only be
// determined once the item's current row has been loaded inside the
// transaction — e.g. PatchMealItem's weight/macro-source checks, which
// depend on the item's state at the moment of the write, not at request
// start. Distinct from errMealNoLongerEditable, which every applyItemMutation
// caller maps to 409 uniformly regardless of the specific status/message.
type itemMutationError struct {
	status int
	msg    string
}

func (e *itemMutationError) Error() string { return e.msg }

// isBusySnapshot reports whether err is specifically SQLITE_BUSY_SNAPSHOT —
// a transaction's read snapshot going stale because some other transaction
// committed first. sqlite3.Error.Code is ErrBusy for plain SQLITE_BUSY (a
// lock-contention retry situation) *and* every one of its extended variants;
// only ExtendedCode distinguishes the snapshot-staleness case specifically,
// so checking Code alone would also catch plain lock contention that has
// nothing to do with a stale read.
func isBusySnapshot(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrBusySnapshot
}

// maxBusySnapshotRetries bounds applyItemMutation's retry of a transaction
// that hit SQLITE_BUSY_SNAPSHOT for a reason unrelated to this meal — see
// its doc comment. Small and fixed rather than exponential backoff: this is
// a single-node app under light concurrency, and each retry starts an
// entirely fresh transaction (a fresh read snapshot), so genuinely
// unrelated staleness is expected to clear within one or two attempts.
const maxBusySnapshotRetries = 2

// applyItemMutation runs fn (an item create/update/delete) inside a
// transaction. It re-reads the meal's status *inside* that transaction and
// aborts with errMealNoLongerEditable if it's no longer pending_review or
// confirmed, before fn runs — closing the gap between the handler's earlier
// status check and this write. Without that re-check, a concurrent Reanalyze
// could replace/delete the meal's items and zero its aggregate in between,
// and this transaction would then blindly resurrect a since-deleted item
// (GORM's Save inserts if the primary key no longer exists) and overwrite
// the freshly-zeroed aggregate using the stale status captured before the
// race. If the meal is still confirmed, it also reloads its current items
// and recomputes+persists its macro aggregate within the same transaction
// before committing, so "the item change" and "the meal's stored total
// reflects it" commit or roll back together, and two concurrent edits to the
// same meal can't compute their aggregate from different snapshots. Returns
// the fully updated meal (current items + aggregate) for the response.
// pending_review meals are left untouched, matching today's behavior
// (aggregate computed only at confirm).
func (h *foodHandlers) applyItemMutation(
	mealID, userID uuid.UUID, fn func(tx *gorm.DB) error,
) (*database.FoodMeal, error) {
	var lastBusyErr error
	for attempt := 0; attempt <= maxBusySnapshotRetries; attempt++ {
		err := h.storage.DB().Transaction(func(tx *gorm.DB) error {
			var meal database.FoodMeal
			if err := tx.Select("id", "status").Where("id = ?", mealID).First(&meal).Error; err != nil {
				// Every caller already verified ownership before opening this
				// transaction, but that check ran on a separate, earlier read —
				// if the meal was deleted in between (e.g. via the generic
				// DELETE /api/data/food_meal endpoint), this re-check is where
				// that's actually discovered. Translate to the same sentinel
				// loadOwnedMeal uses so callers can map it to 404 like any other
				// "this meal doesn't exist" case, instead of falling through to
				// a generic 500 for a resource that simply isn't there anymore.
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return database.ErrNotFound
				}
				return err
			}
			if !editableMealStatus(meal.Status) {
				return errMealNoLongerEditable
			}
			if err := fn(tx); err != nil {
				return err
			}
			if meal.Status != database.MealStatusConfirmed {
				return nil
			}
			var items []database.FoodItem
			if err := tx.Where("meal_id = ?", mealID).Find(&items).Error; err != nil {
				return err
			}
			var agg database.FoodMeal
			agg.Aggregate(items)
			return tx.Model(&database.FoodMeal{}).Where("id = ?", mealID).Updates(map[string]any{
				"calories":            agg.Calories,
				"protein_grams":       agg.ProteinGrams,
				"carbs_grams":         agg.CarbsGrams,
				"fat_grams":           agg.FatGrams,
				"sugar_grams":         agg.SugarGrams,
				"sodium_grams":        agg.SodiumGrams,
				"dietary_fiber_grams": agg.DietaryFiberGrams,
			}).Error
		})
		if err == nil {
			return h.loadOwnedMeal(mealID, userID)
		}
		if !isBusySnapshot(err) {
			return nil, err
		}
		// This transaction's read snapshot went stale because *some*
		// transaction committed before this one's first write — that alone
		// doesn't prove this meal became ineligible: an edit to a different
		// meal, or any other unrelated write, triggers the same
		// database-wide condition. This transaction has already rolled
		// back, so re-read the meal's actual current status fresh: if it's
		// genuinely no longer editable (or gone), report that; otherwise
		// this was unrelated activity, and a retry gets a fresh snapshot
		// that no longer conflicts with whatever caused this one to.
		lastBusyErr = err
		var meal database.FoodMeal
		reErr := h.storage.DB().Select("id", "status").Where("id = ?", mealID).First(&meal).Error
		if errors.Is(reErr, gorm.ErrRecordNotFound) {
			return nil, database.ErrNotFound
		}
		if reErr != nil {
			return nil, reErr
		}
		if !editableMealStatus(meal.Status) {
			return nil, errMealNoLongerEditable
		}
		// Still eligible — retry with a fresh snapshot.
	}
	return nil, fmt.Errorf("%w (last: %v)", errItemMutationConflictExhausted, lastBusyErr)
}

// PatchMealItem handles PATCH /api/food/meals/{id}/items/{item_id}: resolve
// one item by binding it to a reference food, supplying macros directly, or
// changing its weight. Permitted for pending_review and confirmed meals —
// see editableMealStatus. The response is the full updated meal, not just
// the item, so a confirmed meal's freshly recomputed aggregate is visible in
// the same response (see applyItemMutation).
func (h *foodHandlers) PatchMealItem(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	vars := mux.Vars(r)
	mealID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "invalid meal id", http.StatusBadRequest)
		return
	}
	itemID, err := uuid.Parse(vars["item_id"])
	if err != nil {
		http.Error(w, "invalid item id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Select("id", "status").
		Where("id = ? AND user_id = ?", mealID, claims.UserID).First(&meal).Error
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

	var req patchItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// A name that's empty or whitespace-only carries nothing to apply — treat
	// it as absent here so it can't alone satisfy "something to update" and
	// then silently no-op past the guard below.
	hasName := req.Name != nil && strings.TrimSpace(*req.Name) != ""

	if !req.Manual && req.FdcID == nil && req.CustomFoodID == nil && req.OffCode == nil && req.WeightGrams == nil && !hasName {
		http.Error(w, "nothing to update: specify manual, fdc_id, off_code, custom_food_id, weight_grams, or name", http.StatusBadRequest)
		return
	}
	if refSourcesSet(req.FdcID, req.OffCode, req.CustomFoodID) > 1 {
		http.Error(w, "specify at most one of fdc_id, off_code, or custom_food_id", http.StatusBadRequest)
		return
	}
	if req.Manual && (req.FdcID != nil || req.CustomFoodID != nil || req.OffCode != nil) {
		http.Error(w, "specify manual macros or a food reference, not both", http.StatusBadRequest)
		return
	}

	// Everything above depends only on the request body, so it can be
	// validated before opening a transaction. Everything below depends on
	// the item's current row (its existing weight/macro_source), so it must
	// be loaded and mutated *inside* applyItemMutation's transaction, not
	// loaded here and mutated in memory — see itemMutationError's doc
	// comment for the race this closes: the shipped UI can fire a
	// weight-only PATCH (weight input's onBlur) and a binding PATCH (search
	// result click) for the same item close together, and a stale
	// outside-the-transaction snapshot lets whichever request's Save commits
	// second silently overwrite the other's already-committed change with
	// its own stale copy of every other column.
	//
	// Loading inside the transaction alone only narrows that window — it
	// doesn't close it, since resolveReferenceProfile below still does I/O
	// between this load and the eventual write, during which a concurrent
	// request's own transaction can fully commit. The final write is
	// therefore also conditioned on the item's updated_at still matching
	// what was just observed (the same optimistic-concurrency pattern used
	// throughout this codebase for meals — see analyzeMeal's lease-token doc
	// comment in food_upload.go): if it no longer matches, something else
	// changed this item in between and this write is rejected as a conflict
	// rather than blindly overwriting it.
	updated, err := h.applyItemMutation(mealID, claims.UserID, func(tx *gorm.DB) error {
		var item database.FoodItem
		if err := tx.Where("id = ? AND meal_id = ? AND user_id = ?", itemID, mealID, claims.UserID).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &itemMutationError{status: http.StatusNotFound, msg: "not found"}
			}
			return err
		}
		observedUpdatedAt := item.UpdatedAt

		switch {
		case req.Manual:
			item.FdcID = nil
			item.CustomFoodID = nil
			item.OffCode = nil
			item.MacroSource = database.MacroSourceManual
			if req.WeightGrams != nil {
				item.WeightGrams = *req.WeightGrams
			}
			item.Calories = req.Calories
			item.ProteinGrams = req.ProteinGrams
			item.CarbsGrams = req.CarbsGrams
			item.FatGrams = req.FatGrams
			item.SugarGrams = req.SugarGrams
			item.SodiumGrams = req.SodiumGrams
			item.DietaryFiberGrams = req.DietaryFiberGrams
		case req.FdcID != nil || req.CustomFoodID != nil || req.OffCode != nil:
			if req.WeightGrams != nil {
				item.WeightGrams = *req.WeightGrams
			}
			// item.WeightGrams may already be positive from a prior vision
			// recognition or edit even when this request omits weight_grams —
			// but it can also be zero (an unresolved vision item) or, per
			// this request, explicitly non-positive. Either way, binding a
			// reference food scales its profile by this weight (ApplyProfile
			// below), so a non-positive value here would persist negative or
			// zero item macros and, via applyItemMutation's recompute, a
			// negative meal aggregate.
			if item.WeightGrams <= 0 {
				return &itemMutationError{status: http.StatusBadRequest, msg: "weight_grams must be positive to bind a reference food"}
			}
			profile, status, err := h.resolveReferenceProfile(claims.UserID, req.FdcID, req.OffCode, req.CustomFoodID)
			if err != nil {
				return &itemMutationError{status: status, msg: err.Error()}
			}
			item.FdcID = req.FdcID
			item.CustomFoodID = req.CustomFoodID
			item.OffCode = req.OffCode
			item.ApplyProfile(profile)
		case req.WeightGrams != nil:
			item.WeightGrams = *req.WeightGrams
			switch item.MacroSource {
			case database.MacroSourceReference:
				if item.WeightGrams <= 0 {
					return &itemMutationError{status: http.StatusBadRequest, msg: "weight_grams must be positive for a reference item"}
				}
				profile, status, err := h.resolveReferenceProfile(claims.UserID, item.FdcID, item.OffCode, item.CustomFoodID)
				if err != nil {
					return &itemMutationError{status: status, msg: err.Error()}
				}
				item.ApplyProfile(profile)
			case database.MacroSourceEstimated:
				if item.WeightGrams <= 0 {
					return &itemMutationError{status: http.StatusBadRequest, msg: "weight_grams must be positive for an estimated item"}
				}
				item.ApplyEstimatedProfile()
			}
		}

		if req.Name != nil {
			if name := strings.TrimSpace(*req.Name); name != "" {
				item.Name = name
			}
		}

		if req.Manual && req.SaveAsCustomFood {
			if item.WeightGrams <= 0 {
				return &itemMutationError{status: http.StatusBadRequest, msg: "weight_grams must be positive to save as a reusable food"}
			}
			// The manual-macro form defaults every field to 0, so an accidental
			// save before entering any values would otherwise persist a
			// permanent, reusable CustomFood with every macro at zero — future
			// candidate matching could then offer it and silently zero out a
			// real meal. Calories is used as a simple required-signal check;
			// a genuinely calorie-free item (e.g. black coffee) isn't a
			// realistic candidate for "save as reusable food" in the first
			// place.
			if req.Calories <= 0 {
				return &itemMutationError{status: http.StatusBadRequest, msg: "calories must be positive to save as a reusable food"}
			}
			// req.Calories etc. are the item's total macros at its current
			// weight, not per-100g — CustomFood stores per-100g, matching
			// every other reference-food profile in this codebase. Reuses
			// customFoodRequest.applyTo (food_custom.go) rather than
			// hand-assigning each field a second time, so this "save my
			// correction as reusable food" path can't silently drift from
			// direct custom-food creation/update if a macro field is ever
			// added or renamed.
			scale := 100.0 / item.WeightGrams
			cfReq := customFoodRequest{
				Name:                item.Name,
				CaloriesPer100g:     item.Calories * scale,
				ProteinPer100g:      item.ProteinGrams * scale,
				CarbsPer100g:        item.CarbsGrams * scale,
				FatPer100g:          item.FatGrams * scale,
				SugarPer100g:        item.SugarGrams * scale,
				SodiumPer100g:       item.SodiumGrams * scale,
				DietaryFiberPer100g: item.DietaryFiberGrams * scale,
			}
			cf := database.CustomFood{UserID: claims.UserID}
			cf.ID = uuid.New()
			cf.FamilyID = FamilyIDFromCtx(r)
			cfReq.applyTo(&cf)
			if err := tx.Create(&cf).Error; err != nil {
				if isUniqueViolation(err) {
					return &itemMutationError{status: http.StatusConflict, msg: "a custom food with this name already exists"}
				}
				return err
			}
		}

		res := tx.Model(&database.FoodItem{}).
			Where("id = ? AND updated_at = ?", item.ID, observedUpdatedAt).
			Updates(map[string]any{
				"fdc_id":              item.FdcID,
				"custom_food_id":      item.CustomFoodID,
				"off_code":            item.OffCode,
				"macro_source":        item.MacroSource,
				"weight_grams":        item.WeightGrams,
				"name":                item.Name,
				"calories":            item.Calories,
				"protein_grams":       item.ProteinGrams,
				"carbs_grams":         item.CarbsGrams,
				"fat_grams":           item.FatGrams,
				"sugar_grams":         item.SugarGrams,
				"sodium_grams":        item.SodiumGrams,
				"dietary_fiber_grams": item.DietaryFiberGrams,
			})
		if res.Error != nil {
			// A concurrent write landing between this transaction's read and
			// this write doesn't always show up as RowsAffected == 0: under
			// SQLite's WAL-mode snapshot isolation, this transaction's
			// earlier read established a read snapshot, and a write against
			// a now-stale snapshot fails outright with SQLITE_BUSY_SNAPSHOT
			// rather than quietly matching zero rows. That alone doesn't
			// prove *this item* conflicted, though — the same
			// database-wide condition can be triggered by any unrelated
			// write elsewhere. Returned unclassified: applyItemMutation's
			// caller retries the whole transaction on this specific SQLite
			// condition (with a fresh snapshot and, since the item is
			// reloaded at the top of this closure, a fresh read of this
			// item too), so unrelated noise resolves silently on retry, and
			// only a write that's *still* stale after that surfaces as
			// RowsAffected == 0 below — the actual per-item conflict
			// signal.
			return res.Error
		}
		if res.RowsAffected == 0 {
			return &itemMutationError{
				status: http.StatusConflict,
				msg:    "item was modified by another request; reload and try again",
			}
		}
		return nil
	})
	if errors.Is(err, errMealNoLongerEditable) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}
	if errors.Is(err, errItemMutationConflictExhausted) {
		http.Error(w, "item could not be safely written after repeated concurrent conflicts; reload and try again", http.StatusConflict)
		return
	}
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var mutErr *itemMutationError
	if errors.As(err, &mutErr) {
		http.Error(w, mutErr.msg, mutErr.status)
		return
	}
	if err != nil {
		http.Error(w, "update error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, updated)
}

// createItemRequest is the JSON body for POST /api/food/meals/{id}/items.
// Unlike patchItemRequest, a newly created item has no existing state to
// fall back on: a bare weight_grams or name alone is only meaningful as an
// edit to something that already has a name/weight/macro source. Creation
// therefore requires a non-blank Name plus exactly one of Manual+macros or
// a reference (FdcID/CustomFoodID) with a positive WeightGrams.
type createItemRequest struct {
	Name         string     `json:"name"`
	Manual       bool       `json:"manual,omitempty"`
	FdcID        *int64     `json:"fdc_id,omitempty"`
	CustomFoodID *uuid.UUID `json:"custom_food_id,omitempty"`
	OffCode      *string    `json:"off_code,omitempty"`
	WeightGrams  *float64   `json:"weight_grams,omitempty"`

	Calories          float64 `json:"calories,omitempty"`
	ProteinGrams      float64 `json:"protein_grams,omitempty"`
	CarbsGrams        float64 `json:"carbs_grams,omitempty"`
	FatGrams          float64 `json:"fat_grams,omitempty"`
	SugarGrams        float64 `json:"sugar_grams,omitempty"`
	SodiumGrams       float64 `json:"sodium_grams,omitempty"`
	DietaryFiberGrams float64 `json:"dietary_fiber_grams,omitempty"`
}

// CreateMealItem handles POST /api/food/meals/{id}/items: add a new item to
// a meal whose status is pending_review or confirmed. Returns the full
// updated meal, same as PatchMealItem.
func (h *foodHandlers) CreateMealItem(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mealID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid meal id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Select("id", "status", "family_id").
		Where("id = ? AND user_id = ?", mealID, claims.UserID).First(&meal).Error
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

	var req createItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Reject ambiguous combinations before building anything: a request must
	// specify exactly one macro source. Accepting `manual` alongside a
	// reference would silently prefer manual and discard the reference the
	// caller also sent; accepting more than one of fdc_id/off_code/
	// custom_food_id would resolve only one of them
	// (resolveReferenceProfile's precedence) while persisting more than one
	// ID on the item, leaving it claiming a binding to a profile it was
	// never actually scaled from.
	hasReference := req.FdcID != nil || req.CustomFoodID != nil || req.OffCode != nil
	if refSourcesSet(req.FdcID, req.OffCode, req.CustomFoodID) > 1 {
		http.Error(w, "specify at most one of fdc_id, off_code, or custom_food_id", http.StatusBadRequest)
		return
	}
	if req.Manual && hasReference {
		http.Error(w, "specify manual macros or a food reference, not both", http.StatusBadRequest)
		return
	}

	item := database.FoodItem{UserID: claims.UserID, MealID: mealID, Name: name}
	item.ID = uuid.New()
	item.FamilyID = meal.FamilyID

	switch {
	case req.Manual:
		item.MacroSource = database.MacroSourceManual
		if req.WeightGrams != nil {
			item.WeightGrams = *req.WeightGrams
		}
		item.Calories = req.Calories
		item.ProteinGrams = req.ProteinGrams
		item.CarbsGrams = req.CarbsGrams
		item.FatGrams = req.FatGrams
		item.SugarGrams = req.SugarGrams
		item.SodiumGrams = req.SodiumGrams
		item.DietaryFiberGrams = req.DietaryFiberGrams
	case hasReference:
		if req.WeightGrams == nil || *req.WeightGrams <= 0 {
			http.Error(w, "weight_grams must be positive for a reference item", http.StatusBadRequest)
			return
		}
		profile, status, err := h.resolveReferenceProfile(claims.UserID, req.FdcID, req.OffCode, req.CustomFoodID)
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		item.FdcID = req.FdcID
		item.CustomFoodID = req.CustomFoodID
		item.OffCode = req.OffCode
		item.WeightGrams = *req.WeightGrams
		item.ApplyProfile(profile)
	default:
		http.Error(w, "specify manual macros or a food reference (fdc_id/off_code/custom_food_id) with weight_grams", http.StatusBadRequest)
		return
	}

	updated, err := h.applyItemMutation(mealID, claims.UserID, func(tx *gorm.DB) error {
		return tx.Create(&item).Error
	})
	if errors.Is(err, errMealNoLongerEditable) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}
	if errors.Is(err, errItemMutationConflictExhausted) {
		http.Error(w, "item could not be safely written after repeated concurrent conflicts; reload and try again", http.StatusConflict)
		return
	}
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusCreated, updated)
}

// DeleteMealItem handles DELETE /api/food/meals/{id}/items/{item_id}: remove
// an item from a meal whose status is pending_review or confirmed. Returns
// the full updated meal, same as PatchMealItem/CreateMealItem.
func (h *foodHandlers) DeleteMealItem(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	vars := mux.Vars(r)
	mealID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "invalid meal id", http.StatusBadRequest)
		return
	}
	itemID, err := uuid.Parse(vars["item_id"])
	if err != nil {
		http.Error(w, "invalid item id", http.StatusBadRequest)
		return
	}

	var meal database.FoodMeal
	err = h.storage.DB().Select("id", "status").
		Where("id = ? AND user_id = ?", mealID, claims.UserID).First(&meal).Error
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

	var item database.FoodItem
	err = h.storage.DB().
		Where("id = ? AND meal_id = ? AND user_id = ?", itemID, mealID, claims.UserID).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	updated, err := h.applyItemMutation(mealID, claims.UserID, func(tx *gorm.DB) error {
		return tx.Delete(&item).Error
	})
	if errors.Is(err, errMealNoLongerEditable) {
		http.Error(w, "meal is not editable in its current status", http.StatusConflict)
		return
	}
	if errors.Is(err, errItemMutationConflictExhausted) {
		http.Error(w, "item could not be safely written after repeated concurrent conflicts; reload and try again", http.StatusConflict)
		return
	}
	if errors.Is(err, database.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "delete error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, updated)
}
