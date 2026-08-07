package database

import (
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/kin-core/models"
)

// Food logging tables are user-authored, unlike the health metric tables in
// models.go which are ingested telemetry. They deliberately carry no
// SourcePayloadID: there is no upstream webhook or import to trace them to.
// See openspec/specs/data-model "Food logging tables".

// FoodMeal statuses.
const (
	MealStatusProcessing           = "processing"
	MealStatusPendingClarification = "pending_clarification"
	MealStatusPendingReview        = "pending_review"
	MealStatusConfirmed            = "confirmed"
	MealStatusFailed               = "failed"
)

// FoodItem macro sources. A plain matched/unmatched boolean would conflate
// "bound to a reference food" with "has usable macros"; a manually entered
// item is exactly the case where those diverge, and treating it as unmatched
// would drop it from the meal aggregate.
const (
	MacroSourceReference = "reference" // bound to an FDC or custom food, scaled by weight
	MacroSourceManual    = "manual"    // values supplied directly by the user
	MacroSourceNone      = "none"      // unresolved; zeroed, excluded from aggregates
)

// MaxClarifyRounds bounds the clarification loop. Each round costs a model call.
const MaxClarifyRounds = 3

// FoodMeal is one logged meal. Photo-first: the row is committed with status
// processing before the vision call runs, so no analysis outcome can lose the photo.
type FoodMeal struct {
	models.TenantModel
	UserID       uuid.UUID `gorm:"type:uuid;not null;index"`
	PhotoPath    string    `gorm:"type:text"` // relative to the uploads dir; empty for manual entries
	Status       string    `gorm:"type:varchar(32);not null;default:'processing'"`
	LoggedAt     time.Time `gorm:"not null;index"`
	Name         string    `gorm:"type:text"`
	RawResponse  string    `gorm:"type:text"` // last structured response from the vision model
	ClarifyRound int       `gorm:"not null;default:0"`
	ClarifyLog   string    `gorm:"type:text"` // JSON []ClarifyEntry, accumulated across rounds

	// Aggregate over items whose MacroSource is reference or manual, written on confirm.
	Calories          float64 `gorm:"not null;default:0"`
	ProteinGrams      float64 `gorm:"not null;default:0"`
	CarbsGrams        float64 `gorm:"not null;default:0"`
	FatGrams          float64 `gorm:"not null;default:0"`
	SugarGrams        float64 `gorm:"not null;default:0"`
	SodiumGrams       float64 `gorm:"not null;default:0"`
	DietaryFiberGrams float64 `gorm:"not null;default:0"`

	Items []FoodItem `gorm:"foreignKey:MealID"`
}

// ClarifyEntry is one question/answer pair, persisted so later rounds can replay
// the full history instead of re-asking what the user already answered.
type ClarifyEntry struct {
	Round    int    `json:"round"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// FoodItem is one food within a meal. UserID is denormalized from the parent
// meal so item queries are user-scoped; TenantModel supplies only FamilyID.
type FoodItem struct {
	models.TenantModel
	UserID uuid.UUID `gorm:"type:uuid;not null;index"`
	MealID uuid.UUID `gorm:"type:uuid;not null;index"`
	Name   string    `gorm:"not null"`

	// Preparation and State feed the retrieval query alongside Name. SR Legacy
	// encodes both as trailing description qualifiers ("...cooked, roasted"), so
	// omitting them leaves indexed tokens unused and ranks the wrong food first.
	// Empty means unknown, which contributes no query token rather than blocking
	// retrieval. Persisted so a clarification answer can re-run retrieval.
	Preparation string `gorm:"type:varchar(24)"`
	State       string `gorm:"type:varchar(16)"`

	FdcID        *int64     `gorm:"index"`
	CustomFoodID *uuid.UUID `gorm:"type:uuid;index"`
	MacroSource  string     `gorm:"type:varchar(16);not null;default:'none'"`
	WeightGrams  float64    `gorm:"not null"`
	Confidence   float64    `gorm:"not null"`

	Calories          float64 `gorm:"not null"`
	ProteinGrams      float64 `gorm:"not null"`
	CarbsGrams        float64 `gorm:"not null"`
	FatGrams          float64 `gorm:"not null"`
	SugarGrams        float64 `gorm:"not null"`
	SodiumGrams       float64 `gorm:"not null"`
	DietaryFiberGrams float64 `gorm:"not null"`
}

// HasMacros reports whether the item contributes to its meal's aggregate.
func (i FoodItem) HasMacros() bool {
	return i.MacroSource == MacroSourceReference || i.MacroSource == MacroSourceManual
}

// CustomFood is a user's own per-100g profile, e.g. from a package label.
// Unique on (user_id, name) so name-based precedence over USDA has one winner.
type CustomFood struct {
	models.TenantModel
	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_custom_food_user_name"`
	Name   string    `gorm:"not null;uniqueIndex:idx_custom_food_user_name"`

	CaloriesPer100g     float64 `gorm:"not null"`
	ProteinPer100g      float64 `gorm:"not null"`
	CarbsPer100g        float64 `gorm:"not null"`
	FatPer100g          float64 `gorm:"not null"`
	SugarPer100g        float64 `gorm:"not null"`
	SodiumPer100g       float64 `gorm:"not null"`
	DietaryFiberPer100g float64 `gorm:"not null"`
}

// FoodCalibrationSample is a weighed-food ground-truth photo used to benchmark
// vision models. It never produces a FoodMeal.
type FoodCalibrationSample struct {
	models.TenantModel
	UserID      uuid.UUID `gorm:"type:uuid;not null;index"`
	PhotoPath   string    `gorm:"type:text;not null"`
	GroundTruth string    `gorm:"type:text;not null"` // JSON []GroundTruthItem
	CapturedAt  time.Time `gorm:"not null"`
}

// GroundTruthItem is one expected food in a calibration sample. Aliases and
// RefID make prediction matching deterministic rather than string-similarity based.
type GroundTruthItem struct {
	CanonicalName string   `json:"canonical_name"`
	Aliases       []string `json:"aliases,omitempty"`
	FdcID         *int64   `json:"fdc_id,omitempty"`
	CustomFoodID  *string  `json:"custom_food_id,omitempty"`
	WeightGrams   float64  `json:"weight_grams"`
}

// NutrientProfile is per-100g nutrition, the common shape returned by both the
// USDA index and custom foods so scaling has a single implementation.
type NutrientProfile struct {
	CaloriesPer100g     float64 `json:"calories_per_100g"`
	ProteinPer100g      float64 `json:"protein_per_100g"`
	CarbsPer100g        float64 `json:"carbs_per_100g"`
	FatPer100g          float64 `json:"fat_per_100g"`
	SugarPer100g        float64 `json:"sugar_per_100g"`
	SodiumPer100g       float64 `json:"sodium_per_100g"`
	DietaryFiberPer100g float64 `json:"dietary_fiber_per_100g"`
}

// Profile returns the custom food's per-100g values.
func (c CustomFood) Profile() NutrientProfile {
	return NutrientProfile{
		CaloriesPer100g:     c.CaloriesPer100g,
		ProteinPer100g:      c.ProteinPer100g,
		CarbsPer100g:        c.CarbsPer100g,
		FatPer100g:          c.FatPer100g,
		SugarPer100g:        c.SugarPer100g,
		SodiumPer100g:       c.SodiumPer100g,
		DietaryFiberPer100g: c.DietaryFiberPer100g,
	}
}

// ApplyProfile scales a per-100g profile by the item's weight and marks it
// as reference-sourced.
func (i *FoodItem) ApplyProfile(p NutrientProfile) {
	f := i.WeightGrams / 100.0
	i.Calories = p.CaloriesPer100g * f
	i.ProteinGrams = p.ProteinPer100g * f
	i.CarbsGrams = p.CarbsPer100g * f
	i.FatGrams = p.FatPer100g * f
	i.SugarGrams = p.SugarPer100g * f
	i.SodiumGrams = p.SodiumPer100g * f
	i.DietaryFiberGrams = p.DietaryFiberPer100g * f
	i.MacroSource = MacroSourceReference
}

// Aggregate sums the 7 macros over items that have usable macros. Items with
// MacroSource none are excluded rather than counted as zero-value foods.
func (m *FoodMeal) Aggregate(items []FoodItem) {
	m.Calories, m.ProteinGrams, m.CarbsGrams, m.FatGrams = 0, 0, 0, 0
	m.SugarGrams, m.SodiumGrams, m.DietaryFiberGrams = 0, 0, 0
	for _, it := range items {
		if !it.HasMacros() {
			continue
		}
		m.Calories += it.Calories
		m.ProteinGrams += it.ProteinGrams
		m.CarbsGrams += it.CarbsGrams
		m.FatGrams += it.FatGrams
		m.SugarGrams += it.SugarGrams
		m.SodiumGrams += it.SodiumGrams
		m.DietaryFiberGrams += it.DietaryFiberGrams
	}
}
