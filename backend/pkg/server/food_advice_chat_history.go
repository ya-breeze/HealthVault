package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

const (
	nutritionChatHistoryWindowDays = 7
	nutritionChatHistoryMaxDays    = 90
	nutritionChatMaxContributors   = 20
	nutritionChatMaxDayMeals       = 30
	nutritionChatMaxDayItems       = 50
)

type nutritionChatHistoryTools struct {
	storage   database.Storage
	userID    uuid.UUID
	loc       *time.Location
	threshold int
	now       time.Time
}

func newNutritionChatHistoryTools(
	storage database.Storage, userID uuid.UUID, loc *time.Location, settingsJSON string, now time.Time,
) vision.NutritionChatToolExecutor {
	return &nutritionChatHistoryTools{
		storage: storage, userID: userID, loc: loc,
		threshold: database.ResolveUsualMealsPerDay(settingsJSON), now: now,
	}
}

func (t *nutritionChatHistoryTools) Execute(
	ctx context.Context, name string, arguments json.RawMessage,
) (json.RawMessage, error) {
	var result any
	var err error
	switch name {
	case "explain_nutrition_signal":
		var args struct {
			Signal string `json:"signal"`
		}
		if !decodeNutritionChatToolArguments(arguments, &args) || !nutritionHistorySignals[args.Signal] {
			return nil, vision.ErrInvalidNutritionChatToolCall
		}
		result, err = t.explainNutritionSignal(ctx, args.Signal)
	case "get_health_trend":
		var args struct {
			Metric string `json:"metric"`
			Days   int    `json:"days"`
		}
		if !decodeNutritionChatToolArguments(arguments, &args) ||
			!nutritionHistoryMetrics[args.Metric] || !nutritionHistoryTrendDays[args.Days] {
			return nil, vision.ErrInvalidNutritionChatToolCall
		}
		result, err = t.getHealthTrend(ctx, args.Metric, args.Days)
	case "get_day_details":
		var args struct {
			Date string `json:"date"`
		}
		if !decodeNutritionChatToolArguments(arguments, &args) {
			return nil, vision.ErrInvalidNutritionChatToolCall
		}
		result, err = t.getDayDetails(ctx, args.Date)
	default:
		return nil, vision.ErrInvalidNutritionChatToolCall
	}
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode nutrition chat history result: %w", err)
	}
	return encoded, nil
}

var nutritionHistorySignals = map[string]bool{
	"protein": true, "carbs": true, "fat": true, "sugar": true, "sodium": true,
}

var nutritionHistoryMetrics = map[string]bool{"steps": true, "sleep": true, "weight": true}

var nutritionHistoryTrendDays = map[int]bool{7: true, 28: true, 90: true}

func decodeNutritionChatToolArguments(raw json.RawMessage, dst any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

type nutritionHistoryDay struct {
	Date             string
	State            string
	Eligible         bool
	UnconfirmedMeals int
	Meals            []database.FoodMeal
}

func (t *nutritionChatHistoryTools) foodDays(
	ctx context.Context, from, to time.Time,
) ([]nutritionHistoryDay, error) {
	windowEnd := to.AddDate(0, 0, 1)
	var meals []database.FoodMeal
	if err := t.storage.DB().WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("created_at, id") }).
		Where("user_id = ? AND logged_at >= ? AND logged_at < ?", t.userID, from.UTC(), windowEnd.UTC()).
		Order("logged_at, id").Find(&meals).Error; err != nil {
		return nil, fmt.Errorf("read nutrition chat food history: %w", err)
	}

	fromKey, toKey := from.Format("2006-01-02"), to.Format("2006-01-02")
	var confirmations []database.FoodDayCompletion
	if err := t.storage.DB().WithContext(ctx).
		Where("user_id = ? AND local_date >= ? AND local_date <= ?", t.userID, fromKey, toKey).
		Find(&confirmations).Error; err != nil {
		return nil, fmt.Errorf("read nutrition chat day confirmations: %w", err)
	}
	confirmed := make(map[string]bool, len(confirmations))
	for _, row := range confirmations {
		confirmed[row.LocalDate] = true
	}

	mealsByDay := make(map[string][]database.FoodMeal, len(meals))
	for _, meal := range meals {
		key := database.LocalDate(meal.LoggedAt, t.loc)
		mealsByDay[key] = append(mealsByDay[key], meal)
	}

	days := make([]nutritionHistoryDay, 0, int(to.Sub(from).Hours()/24)+1)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		dayMeals := mealsByDay[key]
		times := make([]time.Time, len(dayMeals))
		unconfirmed := 0
		for i, meal := range dayMeals {
			times[i] = meal.LoggedAt
			if meal.Status != database.MealStatusConfirmed {
				unconfirmed++
			}
		}
		state := database.ComputeDayState(
			database.CollapseOccasions(times), t.threshold, confirmed[key],
		)
		eligible := (state == database.DayStateComplete || state == database.DayStateConfirmedComplete) &&
			unconfirmed == 0
		days = append(days, nutritionHistoryDay{
			Date: key, State: state, Eligible: eligible,
			UnconfirmedMeals: unconfirmed, Meals: dayMeals,
		})
	}
	return days, nil
}

type nutritionSignalDayResult struct {
	Date             string  `json:"date"`
	State            string  `json:"state"`
	Eligible         bool    `json:"eligible"`
	UnconfirmedMeals int     `json:"unconfirmed_meals"`
	NutrientGrams    float64 `json:"nutrient_grams"`
}

type nutritionSignalContributor struct {
	Date                  string  `json:"date"`
	Food                  string  `json:"food"`
	NutrientGrams         float64 `json:"nutrient_grams"`
	ShareOfEligibleWindow float64 `json:"share_of_eligible_window"`
	MacroSource           string  `json:"macro_source"`
	Confidence            float64 `json:"confidence"`
}

type nutritionSignalHistoryResult struct {
	Signal                  string                       `json:"signal"`
	WindowStart             string                       `json:"window_start"`
	WindowEnd               string                       `json:"window_end"`
	EligibleDays            int                          `json:"eligible_days"`
	WindowTotalGrams        float64                      `json:"window_total_grams"`
	ContributorTotalGrams   float64                      `json:"contributor_total_grams"`
	MealItemDifferenceGrams float64                      `json:"meal_item_difference_grams"`
	MeanGramsPerEligibleDay float64                      `json:"mean_grams_per_eligible_day"`
	Days                    []nutritionSignalDayResult   `json:"days"`
	Contributors            []nutritionSignalContributor `json:"contributors"`
	ContributorsTruncated   bool                         `json:"contributors_truncated"`
}

func nutritionSignalMealValue(meal database.FoodMeal, signal string) float64 {
	switch signal {
	case "protein":
		return meal.ProteinGrams
	case "carbs":
		return meal.CarbsGrams
	case "fat":
		return meal.FatGrams
	case "sugar":
		return meal.SugarGrams
	case "sodium":
		return meal.SodiumGrams
	default:
		return 0
	}
}

func nutritionSignalItemValue(item database.FoodItem, signal string) float64 {
	switch signal {
	case "protein":
		return item.ProteinGrams
	case "carbs":
		return item.CarbsGrams
	case "fat":
		return item.FatGrams
	case "sugar":
		return item.SugarGrams
	case "sodium":
		return item.SodiumGrams
	default:
		return 0
	}
}

func (t *nutritionChatHistoryTools) explainNutritionSignal(
	ctx context.Context, signal string,
) (nutritionSignalHistoryResult, error) {
	todayKey := database.LocalDate(t.now, t.loc)
	today, err := time.ParseInLocation("2006-01-02", todayKey, t.loc)
	if err != nil {
		return nutritionSignalHistoryResult{}, fmt.Errorf("resolve nutrition chat day: %w", err)
	}
	to := today.AddDate(0, 0, -1)
	from := to.AddDate(0, 0, -(nutritionChatHistoryWindowDays - 1))
	days, err := t.foodDays(ctx, from, to)
	if err != nil {
		return nutritionSignalHistoryResult{}, err
	}

	result := nutritionSignalHistoryResult{
		Signal: signal, WindowStart: from.Format("2006-01-02"), WindowEnd: to.Format("2006-01-02"),
		Days:         make([]nutritionSignalDayResult, 0, len(days)),
		Contributors: make([]nutritionSignalContributor, 0),
	}
	for _, day := range days {
		dayValue := 0.0
		for _, meal := range day.Meals {
			if meal.Status == database.MealStatusConfirmed {
				dayValue += nutritionSignalMealValue(meal, signal)
			}
		}
		result.Days = append(result.Days, nutritionSignalDayResult{
			Date: day.Date, State: day.State, Eligible: day.Eligible,
			UnconfirmedMeals: day.UnconfirmedMeals, NutrientGrams: dayValue,
		})
		if !day.Eligible {
			continue
		}
		result.EligibleDays++
		result.WindowTotalGrams += dayValue
		for _, meal := range day.Meals {
			if meal.Status != database.MealStatusConfirmed {
				continue
			}
			for _, item := range meal.Items {
				value := nutritionSignalItemValue(item, signal)
				if !(value > 0) {
					continue
				}
				result.ContributorTotalGrams += value
				name := strings.TrimSpace(item.Name)
				if name == "" {
					name = strings.TrimSpace(item.CanonicalName)
				}
				result.Contributors = append(result.Contributors, nutritionSignalContributor{
					Date: day.Date, Food: name, NutrientGrams: value,
					MacroSource: item.MacroSource, Confidence: item.Confidence,
				})
			}
		}
	}
	if result.EligibleDays > 0 {
		result.MeanGramsPerEligibleDay = result.WindowTotalGrams / float64(result.EligibleDays)
	}
	result.MealItemDifferenceGrams = result.WindowTotalGrams - result.ContributorTotalGrams
	for i := range result.Contributors {
		if result.WindowTotalGrams > 0 {
			result.Contributors[i].ShareOfEligibleWindow =
				result.Contributors[i].NutrientGrams / result.WindowTotalGrams
		}
	}
	sort.SliceStable(result.Contributors, func(i, j int) bool {
		return result.Contributors[i].NutrientGrams > result.Contributors[j].NutrientGrams
	})
	if len(result.Contributors) > nutritionChatMaxContributors {
		result.Contributors = result.Contributors[:nutritionChatMaxContributors]
		result.ContributorsTruncated = true
	}
	return result, nil
}

type nutritionTrendPoint struct {
	Date  string   `json:"date"`
	Value float64  `json:"value"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	Count int64    `json:"count"`
}

type nutritionHealthTrendResult struct {
	Metric string                `json:"metric"`
	Unit   string                `json:"unit"`
	Days   int                   `json:"days"`
	From   string                `json:"from"`
	To     string                `json:"to"`
	Points []nutritionTrendPoint `json:"points"`
}

func (t *nutritionChatHistoryTools) getHealthTrend(
	ctx context.Context, metric string, days int,
) (nutritionHealthTrendResult, error) {
	todayKey := database.LocalDate(t.now, t.loc)
	today, err := time.ParseInLocation("2006-01-02", todayKey, t.loc)
	if err != nil {
		return nutritionHealthTrendResult{}, fmt.Errorf("resolve nutrition chat trend day: %w", err)
	}
	from := today.AddDate(0, 0, -(days - 1))
	tr := database.TimeRange{From: from.UTC(), To: t.now.UTC()}
	var rows []map[string]any
	unit := ""
	switch metric {
	case "steps":
		unit = "steps"
		rows, err = t.storage.QueryAggregateSteps(database.BucketDay, t.userID, tr)
	case "sleep":
		unit = "hours"
		rows, err = t.storage.QueryAggregate(
			"sleeps", "start_time", "duration_seconds", database.AggFamilyCumulative,
			database.BucketDay, t.loc, t.userID, tr,
		)
	case "weight":
		unit = "kilograms"
		rows, err = t.storage.QueryAggregate(
			"weights", "time", "kilograms", database.AggFamilyPoint,
			database.BucketDay, t.loc, t.userID, tr,
		)
	}
	if err != nil {
		return nutritionHealthTrendResult{}, fmt.Errorf("read nutrition chat %s trend: %w", metric, err)
	}
	result := nutritionHealthTrendResult{
		Metric: metric, Unit: unit, Days: days, From: from.Format("2006-01-02"), To: todayKey,
		Points: make([]nutritionTrendPoint, 0, len(rows)),
	}
	for _, row := range rows {
		bucket, _ := row["bucket_start"].(string)
		if len(bucket) >= 10 {
			bucket = bucket[:10]
		}
		valueKey := "sum"
		if metric == "weight" {
			valueKey = "avg"
		}
		value, _ := nutritionChatNumber(row[valueKey])
		if metric == "sleep" {
			value /= 3600
		}
		countFloat, _ := nutritionChatNumber(row["count"])
		point := nutritionTrendPoint{Date: bucket, Value: value, Count: int64(countFloat)}
		if metric == "weight" {
			if v, ok := nutritionChatNumber(row["min"]); ok {
				point.Min = &v
			}
			if v, ok := nutritionChatNumber(row["max"]); ok {
				point.Max = &v
			}
		}
		result.Points = append(result.Points, point)
	}
	return result, nil
}

func nutritionChatNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

type nutritionDayItemResult struct {
	Food              string  `json:"food"`
	WeightGrams       float64 `json:"weight_grams"`
	Calories          float64 `json:"calories"`
	ProteinGrams      float64 `json:"protein_grams"`
	CarbsGrams        float64 `json:"carbs_grams"`
	FatGrams          float64 `json:"fat_grams"`
	SugarGrams        float64 `json:"sugar_grams"`
	SodiumGrams       float64 `json:"sodium_grams"`
	DietaryFiberGrams float64 `json:"dietary_fiber_grams"`
	MacroSource       string  `json:"macro_source"`
	Confidence        float64 `json:"confidence"`
}

type nutritionDayMealResult struct {
	LoggedAt          string                   `json:"logged_at"`
	Name              string                   `json:"name"`
	Status            string                   `json:"status"`
	Calories          float64                  `json:"calories"`
	ProteinGrams      float64                  `json:"protein_grams"`
	CarbsGrams        float64                  `json:"carbs_grams"`
	FatGrams          float64                  `json:"fat_grams"`
	SugarGrams        float64                  `json:"sugar_grams"`
	SodiumGrams       float64                  `json:"sodium_grams"`
	DietaryFiberGrams float64                  `json:"dietary_fiber_grams"`
	Items             []nutritionDayItemResult `json:"items"`
}

type nutritionDayDetailsResult struct {
	Date           string                   `json:"date"`
	State          string                   `json:"state"`
	Eligible       bool                     `json:"eligible"`
	Meals          []nutritionDayMealResult `json:"meals"`
	MealsTruncated bool                     `json:"meals_truncated"`
	ItemsTruncated bool                     `json:"items_truncated"`
}

func (t *nutritionChatHistoryTools) getDayDetails(
	ctx context.Context, date string,
) (nutritionDayDetailsResult, error) {
	day, err := time.ParseInLocation("2006-01-02", date, t.loc)
	if err != nil || day.Format("2006-01-02") != date {
		return nutritionDayDetailsResult{}, vision.ErrInvalidNutritionChatToolCall
	}
	todayKey := database.LocalDate(t.now, t.loc)
	today, err := time.ParseInLocation("2006-01-02", todayKey, t.loc)
	if err != nil {
		return nutritionDayDetailsResult{}, fmt.Errorf("resolve nutrition chat detail day: %w", err)
	}
	oldest := today.AddDate(0, 0, -(nutritionChatHistoryMaxDays - 1))
	if day.Before(oldest) || day.After(today) {
		return nutritionDayDetailsResult{}, vision.ErrInvalidNutritionChatToolCall
	}
	days, err := t.foodDays(ctx, day, day)
	if err != nil {
		return nutritionDayDetailsResult{}, err
	}
	loaded := days[0]
	result := nutritionDayDetailsResult{
		Date: date, State: loaded.State, Eligible: loaded.Eligible,
		Meals: make([]nutritionDayMealResult, 0, len(loaded.Meals)),
	}
	itemCount := 0
	for _, meal := range loaded.Meals {
		if len(result.Meals) >= nutritionChatMaxDayMeals {
			result.MealsTruncated = true
			break
		}
		out := nutritionDayMealResult{
			LoggedAt: meal.LoggedAt.In(t.loc).Format(time.RFC3339), Name: meal.Name, Status: meal.Status,
			Calories: meal.Calories, ProteinGrams: meal.ProteinGrams, CarbsGrams: meal.CarbsGrams,
			FatGrams: meal.FatGrams, SugarGrams: meal.SugarGrams, SodiumGrams: meal.SodiumGrams,
			DietaryFiberGrams: meal.DietaryFiberGrams, Items: make([]nutritionDayItemResult, 0),
		}
		for _, item := range meal.Items {
			if itemCount >= nutritionChatMaxDayItems {
				result.ItemsTruncated = true
				break
			}
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimSpace(item.CanonicalName)
			}
			out.Items = append(out.Items, nutritionDayItemResult{
				Food: name, WeightGrams: item.WeightGrams, Calories: item.Calories,
				ProteinGrams: item.ProteinGrams, CarbsGrams: item.CarbsGrams,
				FatGrams: item.FatGrams, SugarGrams: item.SugarGrams,
				SodiumGrams: item.SodiumGrams, DietaryFiberGrams: item.DietaryFiberGrams,
				MacroSource: item.MacroSource, Confidence: item.Confidence,
			})
			itemCount++
		}
		result.Meals = append(result.Meals, out)
	}
	return result, nil
}

var _ vision.NutritionChatToolExecutor = (*nutritionChatHistoryTools)(nil)
