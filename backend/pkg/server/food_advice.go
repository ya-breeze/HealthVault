package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

var adviceReasonCode = regexp.MustCompile(`^[a-z][a-z_]{0,31}$`)

type foodAdviceWindow struct {
	MeanCalories     float64 `json:"mean_calories"`
	MeanProteinGrams float64 `json:"mean_protein_grams"`
	MeanCarbsGrams   float64 `json:"mean_carbs_grams"`
	MeanFatGrams     float64 `json:"mean_fat_grams"`
	MeanSugarGrams   float64 `json:"mean_sugar_grams"`
	MeanSodiumGrams  float64 `json:"mean_sodium_grams"`
}

type foodAdviceRequest struct {
	Label   string           `json:"label"`
	Reasons []string         `json:"reasons"`
	Window  foodAdviceWindow `json:"window"`
	Refresh bool             `json:"refresh"`
}

type foodAdviceContext struct {
	TargetCalories     int    `json:"target_calories"`
	TargetProteinGrams int    `json:"target_protein_grams"`
	TargetCarbsGrams   int    `json:"target_carbs_grams"`
	TargetFatGrams     int    `json:"target_fat_grams"`
	DisplayLanguage    string `json:"display_language"`
}

type foodAdviceResponse struct {
	Available   bool               `json:"available"`
	Reason      string             `json:"reason,omitempty"`
	Lines       []string           `json:"lines,omitempty"`
	GeneratedAt *time.Time         `json:"generated_at,omitempty"`
	Context     *foodAdviceContext `json:"context,omitempty"`
}

func writeFoodAdviceUnavailable(w http.ResponseWriter, reason string) {
	writeJSON(w, foodAdviceResponse{Available: false, Reason: reason})
}

// PostFoodAdvice lazily generates and caches advice for the caller's current
// Logged Day and complete normalized model input.
func (h *foodHandlers) PostFoodAdvice(w http.ResponseWriter, r *http.Request) {
	if !isSameOriginRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	claims := ClaimsFromCtx(r)
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req foodAdviceRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	reasons, ok := normalizeAdviceRequest(&req)
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	settingsJSON, err := h.callerSettingsJSON(claims.UserID)
	if err != nil {
		slog.Warn("nutrition advice settings lookup failed", "err", err, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}
	now := time.Now().UTC()
	loc := database.ResolveTimezone(settingsJSON)
	language := primaryAdviceLanguage(displayLanguageFromSettings(settingsJSON))
	target, unavailableReason, err := computeNutritionTargetForProfile(
		h.storage, claims.UserID, now, loc, parseUserProfile(settingsJSON))
	if err != nil {
		slog.Warn("nutrition advice target computation failed", "err", err, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}
	if unavailableReason != "" {
		slog.Warn("nutrition advice target unavailable", "reason", unavailableReason, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}

	in := vision.AdviceInput{
		Label: req.Label, Reasons: reasons,
		MeanCalories: req.Window.MeanCalories, MeanProteinGrams: req.Window.MeanProteinGrams,
		MeanCarbsGrams: req.Window.MeanCarbsGrams, MeanFatGrams: req.Window.MeanFatGrams,
		MeanSugarGrams: req.Window.MeanSugarGrams, MeanSodiumGrams: req.Window.MeanSodiumGrams,
		TargetCalories: target.Calories, TargetProteinGrams: target.ProteinGrams,
		TargetCarbsGrams: target.CarbsGrams, TargetFatGrams: target.FatGrams,
		DisplayLanguage: language,
	}
	encodedInput, err := json.Marshal(in)
	if err != nil {
		slog.Warn("nutrition advice input encoding failed", "err", err, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}
	sum := sha256.Sum256(encodedInput)
	inputHash := hex.EncodeToString(sum[:])
	loggedDay := database.LocalDate(now, loc)
	responseContext := foodAdviceContext{
		TargetCalories: target.Calories, TargetProteinGrams: target.ProteinGrams,
		TargetCarbsGrams: target.CarbsGrams, TargetFatGrams: target.FatGrams,
		DisplayLanguage: language,
	}

	if !req.Refresh {
		if served, err := h.serveCachedAdvice(w, claims.UserID, loggedDay, inputHash, responseContext); err != nil {
			slog.Warn("nutrition advice cache lookup failed", "err", err, "user_id", claims.UserID)
			writeFoodAdviceUnavailable(w, "unavailable")
			return
		} else if served {
			return
		}
	}

	h.adviceMu.Lock()
	defer h.adviceMu.Unlock()
	if !req.Refresh {
		if served, err := h.serveCachedAdvice(w, claims.UserID, loggedDay, inputHash, responseContext); err != nil {
			slog.Warn("nutrition advice repeated cache lookup failed", "err", err, "user_id", claims.UserID)
			writeFoodAdviceUnavailable(w, "unavailable")
			return
		} else if served {
			return
		}
	}

	tctx, cancel := context.WithTimeout(r.Context(), h.visionTimeout)
	defer cancel()
	lines, err := h.vision.Advise(tctx, in)
	if err != nil {
		reason := "unavailable"
		if errors.Is(err, vision.ErrNotConfigured) {
			reason = "unconfigured"
		}
		slog.Warn("nutrition advice generation failed", "err", err, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, reason)
		return
	}
	if len(lines) == 0 {
		slog.Warn("nutrition advice generation returned no lines", "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}
	linesJSON, err := json.Marshal(lines)
	if err != nil {
		slog.Warn("nutrition advice line encoding failed", "err", err, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}
	generatedAt := time.Now().UTC()
	row := database.FoodAdvice{
		UserID: claims.UserID, LoggedDay: loggedDay, Label: in.Label,
		ReasonCodes: strings.Join(in.Reasons, ","), Language: language,
		InputHash: inputHash, Lines: string(linesJSON), GeneratedAt: generatedAt,
	}
	row.ID = uuid.New()
	row.FamilyID = FamilyIDFromCtx(r)
	if err := h.storage.DB().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"logged_day", "label", "reason_codes", "language", "input_hash", "lines", "generated_at", "updated_at",
		}),
	}).Create(&row).Error; err != nil {
		slog.Warn("nutrition advice cache write failed", "err", err, "user_id", claims.UserID)
		writeFoodAdviceUnavailable(w, "unavailable")
		return
	}
	writeJSON(w, foodAdviceResponse{
		Available: true, Lines: lines, GeneratedAt: &generatedAt, Context: &responseContext,
	})
}

func normalizeAdviceRequest(req *foodAdviceRequest) ([]string, bool) {
	if req.Label != "good" && req.Label != "fair" && req.Label != "needs_attention" {
		return nil, false
	}
	if len(req.Reasons) > 6 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(req.Reasons))
	reasons := make([]string, 0, len(req.Reasons))
	for _, reason := range req.Reasons {
		if !adviceReasonCode.MatchString(reason) {
			return nil, false
		}
		if _, exists := seen[reason]; exists {
			continue
		}
		seen[reason] = struct{}{}
		reasons = append(reasons, reason)
	}
	figures := [...]float64{
		req.Window.MeanCalories, req.Window.MeanProteinGrams, req.Window.MeanCarbsGrams,
		req.Window.MeanFatGrams, req.Window.MeanSugarGrams, req.Window.MeanSodiumGrams,
	}
	for _, figure := range figures {
		if math.IsNaN(figure) || math.IsInf(figure, 0) || figure < 0 || figure > 100000 {
			return nil, false
		}
	}
	return reasons, true
}

func primaryAdviceLanguage(language string) string {
	primary, _, _ := strings.Cut(language, "-")
	primary, _, _ = strings.Cut(primary, "_")
	primary = strings.ToLower(primary)
	if primary != "ru" {
		return "en"
	}
	return primary
}

func (h *foodHandlers) serveCachedAdvice(
	w http.ResponseWriter, userID uuid.UUID, loggedDay, inputHash string, responseContext foodAdviceContext,
) (bool, error) {
	var row database.FoodAdvice
	err := h.storage.DB().Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.LoggedDay != loggedDay || row.InputHash != inputHash {
		return false, nil
	}
	var lines []string
	if err := json.Unmarshal([]byte(row.Lines), &lines); err != nil || len(lines) == 0 {
		return false, nil
	}
	writeJSON(w, foodAdviceResponse{
		Available: true, Lines: lines, GeneratedAt: &row.GeneratedAt, Context: &responseContext,
	})
	return true, nil
}
