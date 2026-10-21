package server

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func historyProfileJSON(p *database.NutrientProfile) any {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	return string(b)
}
func historySuggestionJSON(p *database.MealHistorySuggestion) any {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	return string(b)
}
func historyStart(now time.Time, loc *time.Location) time.Time {
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -6).UTC()
}
func eligibleHistory(m *database.FoodMeal, start, now time.Time) bool {
	return m.Status == database.MealStatusConfirmed && !m.LoggedAt.Before(start) && !m.LoggedAt.After(now)
}
func loadHistoryMeal(tx *gorm.DB, id, userID uuid.UUID) (*database.FoodMeal, error) {
	var m database.FoodMeal
	err := tx.Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Where("user_id = ?", userID).Order("id") }).Where("id = ? AND user_id = ?", id, userID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, database.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.SetHistoryVersion()
	return &m, nil
}
func (h *foodHandlers) recentHistory(userID, exclude uuid.UUID, now time.Time) ([]database.FoodMeal, error) {
	loc, err := h.callerTimezone(userID)
	if err != nil {
		return nil, err
	}
	var meals []database.FoodMeal
	err = h.storage.DB().Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Where("user_id = ?", userID).Order("id") }).Where("user_id = ? AND id <> ? AND status = ? AND logged_at >= ? AND logged_at <= ?", userID, exclude, database.MealStatusConfirmed, historyStart(now, loc), now).Order("logged_at DESC, id DESC").Limit(20).Find(&meals).Error
	if err != nil {
		return nil, err
	}
	out := make([]database.FoodMeal, 0, len(meals))
	count := 0
	for _, m := range meals {
		m.SetHistoryVersion()
		items := make([]database.FoodItem, 0, len(m.Items))
		for _, it := range m.Items {
			if _, ok := it.CurrentProfile(); ok && strings.TrimSpace(it.Name) != "" {
				items = append(items, it)
				count++
				if count == 100 {
					break
				}
			}
		}
		if len(items) > 0 {
			m.Items = items
			out = append(out, m)
		}
		if count == 100 {
			break
		}
	}
	return out, nil
}
func visionHistory(meals []database.FoodMeal) []vision.HistoryMeal {
	out := make([]vision.HistoryMeal, len(meals))
	for i, m := range meals {
		out[i] = vision.HistoryMeal{Name: truncateWithEllipsis(m.Name, 200), LoggedAt: m.LoggedAt.UTC().Format(time.RFC3339), CookingContext: m.CookingContext}
		for _, it := range m.Items {
			p, _ := it.CurrentProfile()
			source, confidence := it.NutritionOrigin()
			out[i].Items = append(out[i].Items, vision.HistoryItem{Name: truncateWithEllipsis(it.Name, 200), CanonicalName: truncateWithEllipsis(it.CanonicalName, 200), Preparation: it.Preparation, State: it.State, Profile: p, MacroSource: source, Confidence: confidence})
		}
	}
	return out
}
func buildHistorySuggestion(match *vision.HistoryMatch, history []database.FoodMeal, items []database.FoodItem) *database.MealHistorySuggestion {
	if match == nil || match.SourceMealIndex < 0 || match.SourceMealIndex >= len(history) || len(match.Mappings) == 0 {
		return nil
	}
	source := history[match.SourceMealIndex]
	suggestion := &database.MealHistorySuggestion{SourceMealID: source.ID, SourceName: source.Name, SourceLoggedAt: source.LoggedAt, SourceVersion: source.HistoryVersion}
	targets := map[int]bool{}
	sources := map[int]bool{}
	for _, m := range match.Mappings {
		if m.ItemIndex < 0 || m.ItemIndex >= len(items) || m.SourceItemIndex < 0 || m.SourceItemIndex >= len(source.Items) || targets[m.ItemIndex] || sources[m.SourceItemIndex] || !isPositiveFinite(items[m.ItemIndex].WeightGrams) {
			return nil
		}
		targets[m.ItemIndex] = true
		sources[m.SourceItemIndex] = true
		it := source.Items[m.SourceItemIndex]
		p, ok := it.CurrentProfile()
		if !ok {
			return nil
		}
		origin, confidence := it.NutritionOrigin()
		suggestion.Mappings = append(suggestion.Mappings, database.MealHistoryMapping{ItemID: items[m.ItemIndex].ID, SourceItemID: it.ID, SourceName: it.Name, Profile: p, OriginalSource: origin, OriginalConfidence: confidence})
	}
	for _, it := range source.Items {
		origin, _ := it.NutritionOrigin()
		suggestion.SourceItems = append(suggestion.SourceItems, database.MealHistorySourceItem{Name: it.Name, WeightGrams: it.WeightGrams, MacroSource: origin})
	}
	return suggestion
}
func copyHistoryProfile(target *database.FoodItem, mapping database.MealHistoryMapping, sourceID uuid.UUID) {
	p := mapping.Profile
	target.HistoryProfile = &p
	target.HistorySourceMealID = &sourceID
	sid := mapping.SourceItemID
	target.HistorySourceItemID = &sid
	target.HistoryOriginalSource = mapping.OriginalSource
	target.HistoryOriginalConfidence = mapping.OriginalConfidence
	target.FdcID = nil
	target.CustomFoodID = nil
	target.OffCode = nil
	target.ApplyHistoryProfile()
}

var errHistoryConflict = errors.New("meal history changed; reload and try again")
var errHistoryUnavailable = errors.New("meal cannot be repeated: every item needs usable nutrition and a positive weight; use a meal with 1 to 100 items")

func (h *foodHandlers) historyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errHistoryUnavailable):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, database.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, errHistoryConflict) || isBusySnapshot(err):
		http.Error(w, errHistoryConflict.Error(), http.StatusConflict)
	default:
		http.Error(w, "history update error", http.StatusInternalServerError)
	}
}

// ResolveMealHistory explicitly accepts or dismisses the currently reviewed suggestion.
func (h *foodHandlers) ResolveMealHistory(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	var req struct {
		Action          string `json:"action"`
		ExpectedVersion string `json:"expected_version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Action != "apply" && req.Action != "dismiss") || req.ExpectedVersion == "" {
		http.Error(w, "action and expected_version are required", 400)
		return
	}
	loc, err := h.callerTimezone(claims.UserID)
	if err != nil {
		h.historyError(w, err)
		return
	}
	now := time.Now().UTC()
	err = h.storage.DB().Transaction(func(tx *gorm.DB) error {
		// Acquire SQLite's writer lock before reading target and source snapshots.
		claim := tx.Model(&database.FoodMeal{}).Where("id = ? AND user_id = ?", id, claims.UserID).UpdateColumn("status", gorm.Expr("status"))
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			return database.ErrNotFound
		}
		meal, err := loadHistoryMeal(tx, id, claims.UserID)
		if err != nil {
			return err
		}
		if meal.Status != database.MealStatusPendingReview || meal.HistoryVersion != req.ExpectedVersion || meal.HistorySuggestion == nil {
			return errHistoryConflict
		}
		if req.Action == "apply" {
			s := meal.HistorySuggestion
			source, err := loadHistoryMeal(tx, s.SourceMealID, claims.UserID)
			if err != nil {
				if errors.Is(err, database.ErrNotFound) {
					return errHistoryConflict
				}
				return err
			}
			if !eligibleHistory(source, historyStart(now, loc), now) || source.HistoryVersion != s.SourceVersion {
				return errHistoryConflict
			}
			targets := map[uuid.UUID]*database.FoodItem{}
			for i := range meal.Items {
				targets[meal.Items[i].ID] = &meal.Items[i]
			}
			for _, m := range s.Mappings {
				it := targets[m.ItemID]
				if it == nil || !isPositiveFinite(it.WeightGrams) || !database.ValidHistoryProfile(m.Profile) {
					return errHistoryConflict
				}
				copyHistoryProfile(it, m, s.SourceMealID)
				if err := tx.Omit("CreatedAt").Save(it).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&database.FoodMeal{}).Where("id = ?", id).Updates(map[string]any{"history_suggestion": nil}).Error
	})
	if err != nil {
		h.historyError(w, err)
		return
	}
	meal, err := h.loadOwnedMeal(id, claims.UserID)
	if err != nil {
		h.historyError(w, err)
		return
	}
	writeJSON(w, meal)
}

// RepeatMeal copies current saved nutrition into an independent meal awaiting review.
func (h *foodHandlers) RepeatMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	var req struct {
		RequestID string `json:"request_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.RequestID == "" || len(req.RequestID) > 128 {
		http.Error(w, "request_id is required and must be at most 128 bytes", 400)
		return
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte("repeat:"+id.String())))
	if h.replayDescribedMeal(w, claims.UserID, req.RequestID, fingerprint) {
		return
	}
	loc, err := h.callerTimezone(claims.UserID)
	if err != nil {
		h.historyError(w, err)
		return
	}
	now := time.Now().UTC()
	meal := database.FoodMeal{UserID: claims.UserID, Status: database.MealStatusPendingReview, LoggedAt: now, RequestID: &req.RequestID, RequestFingerprint: fingerprint}
	meal.ID = uuid.New()
	meal.FamilyID = FamilyIDFromCtx(r)
	err = h.storage.DB().Transaction(func(tx *gorm.DB) error {
		// The initial insert also reserves the replay key and acquires the writer lock.
		if err := tx.Create(&meal).Error; err != nil {
			return err
		}
		source, err := loadHistoryMeal(tx, id, claims.UserID)
		if err != nil {
			return err
		}
		if !eligibleHistory(source, historyStart(now, loc), now) {
			return errHistoryConflict
		}
		if len(source.Items) == 0 || len(source.Items) > 100 {
			return errHistoryUnavailable
		}
		meal.Name = source.Name
		meal.CookingContext = source.CookingContext
		for _, src := range source.Items {
			p, ok := src.CurrentProfile()
			if !ok {
				return errHistoryUnavailable
			}
			origin, confidence := src.NutritionOrigin()
			it := database.FoodItem{UserID: claims.UserID, MealID: meal.ID, Name: src.Name, CanonicalName: src.CanonicalName, Preparation: src.Preparation, State: src.State, Brand: src.Brand, WeightGrams: src.WeightGrams, Confidence: src.Confidence}
			it.ID = uuid.New()
			it.FamilyID = meal.FamilyID
			copyHistoryProfile(&it, database.MealHistoryMapping{SourceItemID: src.ID, Profile: p, OriginalSource: origin, OriginalConfidence: confidence}, source.ID)
			meal.Items = append(meal.Items, it)
		}
		if err := tx.Create(&meal.Items).Error; err != nil {
			return err
		}
		return tx.Model(&database.FoodMeal{}).Where("id = ?", meal.ID).Updates(map[string]any{"name": meal.Name, "cooking_context": historyContextJSON(meal.CookingContext)}).Error
	})
	if err != nil {
		if h.replayDescribedMeal(w, claims.UserID, req.RequestID, fingerprint) {
			return
		}
		h.historyError(w, err)
		return
	}
	meal.SetHistoryVersion()
	writeJSONStatus(w, http.StatusCreated, meal)
}
func historyContextJSON(p *database.MealCookingContext) any {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// repeatCapabilities checks the recent listed meals with one owner-scoped item
// query. History never advertises a repeat action that must reject its profiles.
func (h *foodHandlers) repeatCapabilities(userID uuid.UUID, meals []database.FoodMeal, now time.Time) (map[uuid.UUID]bool, error) {
	result := map[uuid.UUID]bool{}
	loc, err := h.callerTimezone(userID)
	if err != nil {
		return nil, err
	}
	start := historyStart(now, loc)
	ids := make([]uuid.UUID, 0, len(meals))
	for _, m := range meals {
		if eligibleHistory(&m, start, now) {
			ids = append(ids, m.ID)
			result[m.ID] = true
		}
	}
	if len(ids) == 0 {
		return result, nil
	}
	var items []database.FoodItem
	if err := h.storage.DB().Select("id", "meal_id", "macro_source", "weight_grams", "calories", "protein_grams", "carbs_grams", "fat_grams", "sugar_grams", "sodium_grams", "dietary_fiber_grams", "saturated_fat_grams").Where("user_id = ? AND meal_id IN ?", userID, ids).Find(&items).Error; err != nil {
		return nil, err
	}
	counts := map[uuid.UUID]int{}
	for _, it := range items {
		counts[it.MealID]++
		if _, ok := it.CurrentProfile(); !ok {
			result[it.MealID] = false
		}
	}
	for _, id := range ids {
		if counts[id] < 1 || counts[id] > 100 {
			result[id] = false
		}
	}
	return result, nil
}
