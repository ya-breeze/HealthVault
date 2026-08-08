package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	photostorage "github.com/ya-breeze/healthvault/pkg/storage"
	"github.com/ya-breeze/healthvault/pkg/usda"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

// multipartOverheadBytes is added to the request-body cap on top of
// MaxUploadBytes, so a photo legitimately right at the limit isn't rejected
// for the few extra bytes of multipart boundaries and headers around it.
const multipartOverheadBytes = 64 * 1024

// CreateMeal handles POST /api/food/meals: a multipart upload with the photo
// in the "photo" field. Photo-first: the file is saved and the FoodMeal row
// committed as processing before the vision call runs, so no outcome of that
// call — success, failure, or timeout — can lose the photo.
func (h *foodHandlers) CreateMeal(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+multipartOverheadBytes)
	file, _, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "photo file is required", http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	familyID := FamilyIDFromCtx(r)
	mealID := uuid.New()

	// The client-supplied filename (available on the discarded second return
	// value above) plays no part in the stored path — see photostorage's
	// package doc — so a traversal-shaped filename has nothing to reach.
	relPath, err := h.photos.Save(file, h.maxUploadBytes, claims.UserID, photostorage.OwnerMeal, mealID)
	if err != nil {
		writeUploadError(w, err)
		return
	}

	meal := database.FoodMeal{
		UserID:    claims.UserID,
		PhotoPath: relPath,
		Status:    database.MealStatusProcessing,
		LoggedAt:  time.Now().UTC(),
	}
	meal.ID = mealID
	meal.FamilyID = familyID
	if err := h.storage.DB().Create(&meal).Error; err != nil {
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}

	h.analyzeMeal(r.Context(), &meal)
	writeJSONStatus(w, http.StatusCreated, meal)
}

func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, photostorage.ErrTooLarge):
		http.Error(w, "upload exceeds maximum size", http.StatusRequestEntityTooLarge)
	case errors.Is(err, photostorage.ErrHEIC):
		http.Error(w, "HEIC images are not supported; please use JPEG, PNG, or WebP", http.StatusUnsupportedMediaType)
	case errors.Is(err, photostorage.ErrUnsupportedFormat):
		http.Error(w, "unsupported image format", http.StatusUnsupportedMediaType)
	default:
		http.Error(w, "upload error", http.StatusInternalServerError)
	}
}

// analyzeMeal runs the two-call vision pipeline and persists its outcome:
//   - failed, on any error including a timeout — the photo is retained.
//   - pending_clarification, when the model could not confidently recognize
//     the photo — items are stored unresolved, awaiting a future clarify
//     round (POST /api/food/meals/{id}/clarify).
//   - pending_review, once every item has been offered its candidate
//     shortlist and either bound or left unresolved.
//
// The meal's own aggregate is left at zero; it is computed only on confirm.
func (h *foodHandlers) analyzeMeal(ctx context.Context, meal *database.FoodMeal) {
	ctx, cancel := context.WithTimeout(ctx, h.visionTimeout)
	defer cancel()

	photoBytes, err := h.photos.Read(meal.PhotoPath)
	if err != nil {
		h.failMeal(meal)
		return
	}

	recognized, err := h.vision.Recognize(ctx, photoBytes, mimeTypeForExt(extOf(meal.PhotoPath)))
	if err != nil {
		h.failMeal(meal)
		return
	}

	if len(recognized.ClarificationQuestions) > 0 {
		items := unresolvedItemsFrom(recognized.Items, meal.ID, meal.UserID, meal.FamilyID)
		h.persistAnalysis(meal, database.MealStatusPendingClarification, recognized.Raw, items)
		return
	}

	items := h.resolveItems(ctx, meal, recognized.Items)
	h.persistAnalysis(meal, database.MealStatusPendingReview, recognized.Raw, items)
}

// resolveItems retrieves a candidate shortlist per recognized item, offers
// every non-empty shortlist to the model in a single Select call, and binds
// whatever it chooses. An item with no candidates, or not selected, or
// selected as "none of these" keeps MacroSource none — see
// design.md "Matching is candidate retrieval, not auto-assignment."
func (h *foodHandlers) resolveItems(
	ctx context.Context, meal *database.FoodMeal, recognizedItems []vision.Item,
) []database.FoodItem {
	items := make([]database.FoodItem, len(recognizedItems))
	candidateSets := make([][]vision.Candidate, len(recognizedItems))
	itemCandidates := make([]vision.ItemCandidates, 0, len(recognizedItems))

	for i, ri := range recognizedItems {
		item := database.FoodItem{
			UserID: meal.UserID, MealID: meal.ID, Name: ri.Name,
			Preparation: ri.Preparation, State: ri.State,
			WeightGrams: ri.WeightGrams, Confidence: ri.Confidence,
			MacroSource: database.MacroSourceNone,
		}
		item.ID = uuid.New()
		item.FamilyID = meal.FamilyID
		items[i] = item

		candidates := h.retrieveCandidates(meal.UserID, ri.Name, ri.Preparation, ri.State)
		candidateSets[i] = candidates
		if len(candidates) > 0 {
			itemCandidates = append(itemCandidates, vision.ItemCandidates{ItemIndex: i, Candidates: candidates})
		}
	}

	if len(itemCandidates) == 0 {
		return items
	}

	sel, err := h.vision.Select(ctx, itemCandidates)
	if err != nil {
		return items
	}

	for _, s := range sel.Selections {
		if s.ItemIndex < 0 || s.ItemIndex >= len(items) {
			continue
		}
		candidates := candidateSets[s.ItemIndex]
		if s.CandidateIndex < 0 || s.CandidateIndex >= len(candidates) {
			continue // "none of these"
		}
		chosen := candidates[s.CandidateIndex]
		profile, ok := h.profileForCandidate(meal.UserID, chosen)
		if !ok {
			continue
		}
		items[s.ItemIndex].FdcID = chosen.FdcID
		items[s.ItemIndex].CustomFoodID = chosen.CustomFoodID
		items[s.ItemIndex].ApplyProfile(profile)
	}
	return items
}

// retrieveCandidates mirrors Search's precedence rule: an exact (case-
// insensitive) custom food name match wins outright; otherwise the USDA
// shortlist.
func (h *foodHandlers) retrieveCandidates(userID uuid.UUID, name, preparation, state string) []vision.Candidate {
	var custom database.CustomFood
	err := h.storage.DB().
		Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, name).
		First(&custom).Error
	if err == nil {
		id := custom.ID
		return []vision.Candidate{{CustomFoodID: &id, Description: custom.Name}}
	}
	if h.usda == nil {
		return nil
	}
	term := usda.QueryFor(name, preparation, state)
	foods, err := h.usda.Search(term, usda.DefaultCandidates)
	if err != nil {
		return nil
	}
	out := make([]vision.Candidate, len(foods))
	for i, f := range foods {
		fdcID := f.FdcID
		out[i] = vision.Candidate{FdcID: &fdcID, Description: f.Description}
	}
	return out
}

func (h *foodHandlers) profileForCandidate(userID uuid.UUID, c vision.Candidate) (database.NutrientProfile, bool) {
	if c.CustomFoodID != nil {
		cf, err := h.findOwnedCustomFood(*c.CustomFoodID, userID)
		if err != nil {
			return database.NutrientProfile{}, false
		}
		return cf.Profile(), true
	}
	if c.FdcID != nil && h.usda != nil {
		food, err := h.usda.ByFdcID(*c.FdcID)
		if err != nil || food == nil {
			return database.NutrientProfile{}, false
		}
		return food.Profile, true
	}
	return database.NutrientProfile{}, false
}

func unresolvedItemsFrom(recognizedItems []vision.Item, mealID, userID, familyID uuid.UUID) []database.FoodItem {
	items := make([]database.FoodItem, len(recognizedItems))
	for i, ri := range recognizedItems {
		item := database.FoodItem{
			UserID: userID, MealID: mealID, Name: ri.Name,
			Preparation: ri.Preparation, State: ri.State,
			WeightGrams: ri.WeightGrams, Confidence: ri.Confidence,
			MacroSource: database.MacroSourceNone,
		}
		item.ID = uuid.New()
		item.FamilyID = familyID
		items[i] = item
	}
	return items
}

// persistAnalysis replaces the meal's FoodItem rows and writes its status in
// one transaction, so a re-analysis (a future retry endpoint) can never
// append a second set alongside the first. On a write failure the meal is
// left failed rather than silently stuck in processing.
func (h *foodHandlers) persistAnalysis(meal *database.FoodMeal, status, rawResponse string, items []database.FoodItem) {
	err := h.storage.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("meal_id = ?", meal.ID).Delete(&database.FoodItem{}).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return tx.Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
			Updates(map[string]any{"status": status, "raw_response": rawResponse}).Error
	})
	if err != nil {
		h.failMeal(meal)
		return
	}
	meal.Status = status
	meal.RawResponse = rawResponse
	meal.Items = items
}

func (h *foodHandlers) failMeal(meal *database.FoodMeal) {
	h.storage.DB().Model(&database.FoodMeal{}).Where("id = ?", meal.ID).
		Update("status", database.MealStatusFailed) //nolint:errcheck
	meal.Status = database.MealStatusFailed
}

// extOf returns the stored file extension from a photostorage relative path
// ("{user_id}/{owner_kind}/{owner_id}.{ext}"), or "" if there isn't one.
func extOf(relPath string) string {
	i := strings.LastIndexByte(relPath, '.')
	if i < 0 {
		return ""
	}
	return relPath[i+1:]
}

func mimeTypeForExt(ext string) string {
	return contentTypeForExt(ext)
}
