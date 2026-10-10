package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ya-breeze/healthvault/pkg/database"
	"gorm.io/gorm"
)

type phonePeriodInput struct {
	Period    string `json:"period,omitempty" jsonschema:"today, yesterday (default), last_7_days (seven completed local days), or range with start_date/end_date."`
	StartDate string `json:"start_date,omitempty" jsonschema:"Inclusive YYYY-MM-DD, only with period range."`
	EndDate   string `json:"end_date,omitempty" jsonschema:"Inclusive YYYY-MM-DD, only with period range. At most 92 calendar days; no future dates."`
}

type phoneHistoryInput struct {
	phonePeriodInput
	Limit  int    `json:"limit,omitempty" jsonschema:"Meals per page, 1 to 20; default 10."`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next_cursor from the preceding page; keep the period and dates identical."`
}

type phonePeriod struct {
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date"`
	Timezone         string `json:"timezone"`
	TimezoneFallback bool   `json:"timezone_fallback"`
	Today            string `json:"today"`
}

func resolvePhonePeriod(in phonePeriodInput, settings string, now time.Time) (phonePeriod, *time.Location, error) {
	loc := database.ResolveTimezone(settings)
	stored := database.SettingsRawString(settings, "timezone")
	if stored == "Local" {
		loc = time.UTC
	}
	today, _ := time.Parse("2006-01-02", database.LocalDate(now, loc))
	p := phonePeriod{Timezone: loc.String(), Today: today.Format("2006-01-02")}
	_, zoneErr := time.LoadLocation(stored)
	p.TimezoneFallback = stored == "" || stored == "Local" || zoneErr != nil
	if in.Period == "" {
		in.Period = "yesterday"
	}
	var from, to time.Time
	if in.Period != "range" && (in.StartDate != "" || in.EndDate != "") {
		return p, loc, errors.New("use period range with start_date and end_date")
	}
	switch in.Period {
	case "today":
		from, to = today, today
	case "yesterday":
		from, to = today.AddDate(0, 0, -1), today.AddDate(0, 0, -1)
	case "last_7_days":
		from, to = today.AddDate(0, 0, -7), today.AddDate(0, 0, -1)
	case "range":
		var err error
		from, err = time.Parse("2006-01-02", in.StartDate)
		if err != nil {
			return p, loc, errors.New("start_date must be YYYY-MM-DD")
		}
		to, err = time.Parse("2006-01-02", in.EndDate)
		if err != nil {
			return p, loc, errors.New("end_date must be YYYY-MM-DD")
		}
	default:
		return p, loc, errors.New("period must be today, yesterday, last_7_days or range")
	}
	if to.Before(from) || to.After(today) || to.After(from.AddDate(0, 0, 91)) {
		return p, loc, errors.New("use an ordered range of at most 92 calendar days ending no later than today")
	}
	p.StartDate, p.EndDate = from.Format("2006-01-02"), to.Format("2006-01-02")
	if _, _, err := database.LocalDayWindow(p.StartDate, p.EndDate, loc); err != nil {
		return p, loc, err
	}
	return p, loc, nil
}

type phoneHistoryCursor struct {
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	Timezone  string    `json:"timezone"`
	LoggedAt  time.Time `json:"logged_at"`
	ID        uuid.UUID `json:"id"`
}

type phoneHistoryMeal struct {
	MealID         string                       `json:"meal_id"`
	LoggedAt       time.Time                    `json:"logged_at"`
	LocalDate      string                       `json:"local_date"`
	Name           string                       `json:"name"`
	Description    string                       `json:"description"`
	TextTruncated  bool                         `json:"text_truncated"`
	Status         string                       `json:"status"`
	Confirmed      bool                         `json:"confirmed"`
	KnownTotals    map[string]float64           `json:"known_totals"`
	UnknownItems   int                          `json:"unknown_items"`
	EstimatedItems int                          `json:"estimated_items"`
	ItemCount      int                          `json:"item_count"`
	CookingContext *database.MealCookingContext `json:"cooking_context,omitempty"`
}

type phoneHistoryResult struct {
	phonePeriod
	Meals      []phoneHistoryMeal `json:"meals"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
	Notes      []string           `json:"notes"`
}

type phoneDailyEvidence struct {
	database.DailyTotal
	ConfirmedMeals          int     `json:"confirmed_meals"`
	RecordedMeals           int     `json:"recorded_meals"`
	UnknownConfirmedItems   int     `json:"unknown_confirmed_items"`
	EstimatedConfirmedItems int     `json:"estimated_confirmed_items"`
	Completeness            *string `json:"completeness"`
	CompletenessBasis       string  `json:"completeness_basis"`
	OccasionCount           int     `json:"occasion_count"`
	PartialToday            bool    `json:"partial_today"`
}

type phoneDailyResult struct {
	phonePeriod
	Days             []phoneDailyEvidence `json:"days"`
	UsualMealsPerDay int                  `json:"usual_meals_per_day"`
	Notes            []string             `json:"notes"`
}

var phoneHistoryNotes = []string{
	"Energy (calories) is kcal. Nutrient amounts ending in _grams are grams.",
	"Totals include confirmed recorded meals only. Drafts do not contribute.",
	"Unknown nutrients contribute no known amounts; numeric zero is not proof of zero intake. Nutrition values may be estimates.",
	"No records means no recorded food, not fasting. Completeness is a logging heuristic or owner assertion; today is partial.",
	"Owner assertions are stored date flags, not proof of full intake. Meal changes can make an older flag outdated. Read-only tools never clean up flags; retract and reconfirm in HealthVault when needed.",
	"Sodium is elemental grams, not added salt. Meal summaries may shorten text; use get_food_meal for details.",
}

func phoneReadError(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}
}

// Each evidence tool resolves identity/settings under the same backup barrier as
// meal writes. No caller-provided selector can widen its scope.
func (h *foodHandlers) phoneReadPeriod(id uuid.UUID, in phonePeriodInput) (phonePeriod, *time.Location, int, error) {
	if _, err := h.storage.FindUserByID(id); err != nil {
		return phonePeriod{}, nil, 0, errors.New("connected user unavailable")
	}
	settings, err := h.storage.GetUserSettings(id)
	if err != nil {
		return phonePeriod{}, nil, 0, errors.New("connected user settings unavailable")
	}
	p, loc, err := resolvePhonePeriod(in, settings, time.Now())
	return p, loc, database.ResolveUsualMealsPerDay(settings), err
}

func (h *foodHandlers) addPhoneHistoryTools(s *mcp.Server, id uuid.UUID, barrier *sync.RWMutex) {
	closed := false
	read := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}
	mcp.AddTool(s, &mcp.Tool{Name: "list_food_meals", Description: "List the connected user's recorded meals for local days. Default yesterday. Follow next_cursor for every meal; confirmed nutrients are summarized separately from drafts. Use get_food_meal for full portions/details and get_food_daily_totals for complete period totals independent of pagination.", Annotations: read}, func(ctx context.Context, _ *mcp.CallToolRequest, in phoneHistoryInput) (*mcp.CallToolResult, phoneHistoryResult, error) {
		barrier.RLock()
		defer barrier.RUnlock()
		p, loc, _, err := h.phoneReadPeriod(id, in.phonePeriodInput)
		if err != nil {
			return phoneReadError(err), phoneHistoryResult{}, nil
		}
		out, err := h.phoneMealHistory(ctx, id, p, loc, in)
		if err != nil {
			return phoneReadError(err), phoneHistoryResult{}, nil
		}
		return nil, out, nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "get_food_daily_totals", Description: "Read complete confirmed recorded nutrition totals for every local day in a period, independently of meal-list pagination. Default yesterday; last_7_days excludes today. Returns draft counts, unknown/estimated item counts, logging completeness and partial-today flags. These are logged amounts, not proof of all food consumed.", Annotations: read}, func(ctx context.Context, _ *mcp.CallToolRequest, in phonePeriodInput) (*mcp.CallToolResult, phoneDailyResult, error) {
		barrier.RLock()
		defer barrier.RUnlock()
		p, loc, threshold, err := h.phoneReadPeriod(id, in)
		if err != nil {
			return phoneReadError(err), phoneDailyResult{}, nil
		}
		out, err := h.phoneDailyTotals(ctx, id, p, loc, threshold)
		if err != nil {
			return phoneReadError(err), phoneDailyResult{}, nil
		}
		if encoded, err := json.Marshal(out); err != nil || len(encoded) > 64*1024 {
			return phoneReadError(errors.New("daily evidence too large; request a shorter period")), phoneDailyResult{}, nil
		}
		return nil, out, nil
	})
}

func phoneWindow(p phonePeriod, loc *time.Location) (time.Time, time.Time) {
	// The period resolver has already validated both calendar boundaries.
	from, to, _ := database.LocalDayWindow(p.StartDate, p.EndDate, loc)
	return from, to
}

func phoneShortText(s string) (string, bool) {
	runes := []rune(s)
	if len(runes) > 512 {
		return string(runes[:512]), true
	}
	return s, false
}

func (h *foodHandlers) phoneMealHistory(ctx context.Context, id uuid.UUID, p phonePeriod, loc *time.Location, in phoneHistoryInput) (phoneHistoryResult, error) {
	out := phoneHistoryResult{phonePeriod: p, Meals: []phoneHistoryMeal{}, Notes: phoneHistoryNotes}
	limit := in.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 20 {
		return out, errors.New("limit must be between 1 and 20")
	}
	start, end := phoneWindow(p, loc)
	q := h.storage.DB().WithContext(ctx).Model(&database.FoodMeal{}).Where("user_id = ? AND logged_at >= ? AND logged_at < ?", id, start, end)
	if in.Cursor != "" {
		var c phoneHistoryCursor
		decoded, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		if err != nil || len(in.Cursor) > 2048 || json.Unmarshal(decoded, &c) != nil || c.ID == uuid.Nil || c.LoggedAt.IsZero() || c.StartDate != p.StartDate || c.EndDate != p.EndDate || c.Timezone != p.Timezone {
			return out, errors.New("invalid cursor or changed period; restart the list without cursor")
		}
		q = q.Where("(logged_at < ? OR (logged_at = ? AND id < ?))", c.LoggedAt.UTC(), c.LoggedAt.UTC(), c.ID)
	}
	var meals []database.FoodMeal
	if err := q.Select("id", "logged_at", "name", "description", "status", "cooking_context", "calories", "protein_grams", "carbs_grams", "fat_grams", "sugar_grams", "sodium_grams", "dietary_fiber_grams", "saturated_fat_grams").
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Select("id", "meal_id", "macro_source").Where("user_id = ?", id) }).
		Order("logged_at DESC, id DESC").Limit(limit + 1).Find(&meals).Error; err != nil {
		return out, errors.New("meal history unavailable")
	}
	out.HasMore = len(meals) > limit
	if out.HasMore {
		meals = meals[:limit]
	}
	for _, m := range meals {
		name, nt := phoneShortText(m.Name)
		description, dt := phoneShortText(m.Description)
		row := phoneHistoryMeal{MealID: m.ID.String(), LoggedAt: m.LoggedAt.In(loc), LocalDate: database.LocalDate(m.LoggedAt, loc), Name: name, Description: description, TextTruncated: nt || dt, Status: m.Status, Confirmed: m.Status == database.MealStatusConfirmed, KnownTotals: map[string]float64{}, ItemCount: len(m.Items), CookingContext: m.CookingContext}
		for _, it := range m.Items {
			if !it.HasMacros() {
				row.UnknownItems++
			}
			if it.MacroSource == database.MacroSourceEstimated {
				row.EstimatedItems++
			}
		}
		if row.Confirmed {
			row.KnownTotals = map[string]float64{"calories": m.Calories, "protein_grams": m.ProteinGrams, "carbs_grams": m.CarbsGrams, "fat_grams": m.FatGrams, "sugar_grams": m.SugarGrams, "sodium_grams": m.SodiumGrams, "dietary_fiber_grams": m.DietaryFiberGrams, "saturated_fat_grams": m.SaturatedFatGrams}
		}
		out.Meals = append(out.Meals, row)
	}
	for {
		if out.HasMore {
			last := meals[len(out.Meals)-1]
			b, _ := json.Marshal(phoneHistoryCursor{StartDate: p.StartDate, EndDate: p.EndDate, Timezone: p.Timezone, LoggedAt: last.LoggedAt.UTC(), ID: last.ID})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			return out, errors.New("meal history cannot be encoded")
		}
		if len(encoded) <= 64*1024 {
			break
		}
		if len(out.Meals) <= 1 {
			return out, errors.New("meal summary too large; read its details in HealthVault")
		}
		out.Meals = out.Meals[:len(out.Meals)-1]
		out.HasMore = true
	}
	return out, nil
}

func (h *foodHandlers) phoneDailyTotals(ctx context.Context, id uuid.UUID, p phonePeriod, loc *time.Location, threshold int) (phoneDailyResult, error) {
	out := phoneDailyResult{phonePeriod: p, Days: []phoneDailyEvidence{}, UsualMealsPerDay: threshold, Notes: phoneHistoryNotes}
	// One read transaction keeps totals, completeness and source counts consistent.
	err := h.storage.DB().WithContext(ctx).Transaction(func(db *gorm.DB) error {
		totals, err := database.DailyTotalsRange(db, id, loc, p.StartDate, p.EndDate)
		if err != nil {
			return err
		}
		completedEnd := p.EndDate
		if completedEnd == p.Today {
			day, _ := time.Parse("2006-01-02", p.Today)
			completedEnd = day.AddDate(0, 0, -1).Format("2006-01-02")
		}
		states, err := database.DayRangeReadOnly(db, id, loc, threshold, p.StartDate, completedEnd)
		if err != nil {
			return err
		}
		start, end := phoneWindow(p, loc)
		var meals []database.FoodMeal
		if err := db.Select("id", "logged_at", "status").Preload("Items", func(q *gorm.DB) *gorm.DB { return q.Select("id", "meal_id", "macro_source").Where("user_id = ?", id) }).
			Where("user_id = ? AND logged_at >= ? AND logged_at < ?", id, start, end).Find(&meals).Error; err != nil {
			return err
		}
		counts := map[string]phoneDailyEvidence{}
		loggedTimes := map[string][]time.Time{}
		statesByDate := map[string]database.DayCompleteness{}
		for _, state := range states {
			statesByDate[state.Date] = state
		}
		for _, m := range meals {
			date := database.LocalDate(m.LoggedAt, loc)
			loggedTimes[date] = append(loggedTimes[date], m.LoggedAt)
			c := counts[date]
			c.RecordedMeals++
			if m.Status == database.MealStatusConfirmed {
				c.ConfirmedMeals++
				for _, it := range m.Items {
					if !it.HasMacros() {
						c.UnknownConfirmedItems++
					}
					if it.MacroSource == database.MacroSourceEstimated {
						c.EstimatedConfirmedItems++
					}
				}
			}
			counts[date] = c
		}
		for _, total := range totals {
			r := counts[total.Date]
			r.DailyTotal = total
			state := statesByDate[total.Date].State
			r.Completeness = &state
			r.OccasionCount = database.CollapseOccasions(loggedTimes[total.Date])
			r.PartialToday = total.Date == p.Today
			switch state {
			case database.DayStateComplete:
				r.CompletenessBasis = "occasion_threshold_heuristic"
			case database.DayStateConfirmedComplete:
				r.CompletenessBasis = "owner_assertion"
			default:
				r.CompletenessBasis = "insufficient_logging"
			}
			if r.PartialToday {
				r.Completeness = nil
				r.CompletenessBasis = "not_evaluated_today"
			}
			out.Days = append(out.Days, r)
		}
		return nil
	})
	if err != nil {
		return out, errors.New("daily nutrition evidence unavailable")
	}
	return out, nil
}
