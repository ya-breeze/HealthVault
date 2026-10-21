package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/vision"
)

func TestRecentHistory_LocalDateBoundaryIsolationAndBounds(t *testing.T) {
	st, user, family := newDashboardInternalStorage(t)
	h := NewFoodHandlers(st, nil, t.TempDir())
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUserSettings(user, family, `{"timezone":"Europe/Prague"}`); err != nil {
		t.Fatal(err)
	}
	// This seven-date window includes a DST change; it is not a fixed 168h duration.
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, loc).UTC()
	start := historyStart(now, loc)
	if want := time.Date(2026, 10, 21, 0, 0, 0, 0, loc).UTC(); !start.Equal(want) {
		t.Fatalf("start=%v want=%v", start, want)
	}
	seed := func(at time.Time, status string, uid uuid.UUID, n int) database.FoodMeal {
		m := database.FoodMeal{UserID: uid, LoggedAt: at, Status: status, Name: fmt.Sprint(n)}
		m.ID = uuid.New()
		m.FamilyID = family
		if err := st.DB().Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		for i := 0; i < n; i++ {
			it := database.FoodItem{UserID: uid, MealID: m.ID, Name: "item", WeightGrams: 100, MacroSource: database.MacroSourceManual, Calories: 100}
			it.ID = uuid.New()
			it.FamilyID = family
			if err := st.DB().Create(&it).Error; err != nil {
				t.Fatal(err)
			}
		}
		return m
	}
	boundary := seed(start, database.MealStatusConfirmed, user, 1)
	seed(start.Add(-time.Nanosecond), database.MealStatusConfirmed, user, 1)
	seed(now.Add(time.Second), database.MealStatusConfirmed, user, 1)
	seed(now, database.MealStatusPendingReview, user, 1)
	seed(now, database.MealStatusConfirmed, uuid.New(), 1)
	got, err := h.recentHistory(user, uuid.Nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != boundary.ID {
		t.Fatalf("wrong candidates %+v", got)
	}
	for i := 0; i < 22; i++ {
		seed(now.Add(-time.Duration(i)*time.Second), database.MealStatusConfirmed, user, 6)
	}
	got, err = h.recentHistory(user, uuid.Nil, now)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range got {
		count += len(m.Items)
	}
	if len(got) > 20 || count != 100 {
		t.Fatalf("meal count=%d items=%d", len(got), count)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].LoggedAt.Before(got[i].LoggedAt) {
			t.Fatal("candidate order changed")
		}
	}
}
func TestBuildHistorySuggestionRejectsInvalidIndicesAndDuplicates(t *testing.T) {
	source := database.FoodMeal{Name: "source", Status: database.MealStatusConfirmed}
	source.ID = uuid.New()
	item := database.FoodItem{Name: "food", WeightGrams: 100, Calories: 50, MacroSource: database.MacroSourceEstimated}
	item.ID = uuid.New()
	source.Items = []database.FoodItem{item}
	source.SetHistoryVersion()
	target := item
	target.ID = uuid.New()
	cases := []*vision.HistoryMatch{nil, {SourceMealIndex: -1}, {SourceMealIndex: 1}, {SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 2, SourceItemIndex: 0}}}, {SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 1}}}, {SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}, {ItemIndex: 0, SourceItemIndex: 0}}}}
	for _, match := range cases {
		if got := buildHistorySuggestion(match, []database.FoodMeal{source}, []database.FoodItem{target}); got != nil {
			t.Fatalf("accepted invalid match %+v", match)
		}
	}
	valid := &vision.HistoryMatch{SourceMealIndex: 0, Mappings: []vision.HistoryItemMapping{{ItemIndex: 0, SourceItemIndex: 0}}}
	got := buildHistorySuggestion(valid, []database.FoodMeal{source}, []database.FoodItem{target})
	if got == nil || got.Mappings[0].OriginalSource != database.MacroSourceEstimated || got.Mappings[0].ItemID != target.ID {
		t.Fatal("valid proposal lost source uncertainty")
	}
}

func TestRepeatCapabilities_RequiresEveryItemToBeUsable(t *testing.T) {
	st, user, family := newDashboardInternalStorage(t)
	h := NewFoodHandlers(st, nil, t.TempDir())
	now := time.Now().UTC()
	var meals []database.FoodMeal
	for _, mode := range []string{"valid", "none", "zero", "empty", "old", "future", "pending"} {
		m := database.FoodMeal{UserID: user, Name: mode, LoggedAt: now.Add(-time.Hour), Status: database.MealStatusConfirmed}
		m.ID = uuid.New()
		m.FamilyID = family
		switch mode {
		case "old":
			m.LoggedAt = now.AddDate(0, 0, -8)
		case "future":
			m.LoggedAt = now.Add(time.Hour)
		case "pending":
			m.Status = database.MealStatusPendingReview
		}
		if err := st.DB().Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		if mode != "empty" {
			for n := 0; n < 2; n++ {
				it := database.FoodItem{UserID: user, MealID: m.ID, Name: "food", WeightGrams: 100, MacroSource: database.MacroSourceManual, Calories: 50}
				it.ID = uuid.New()
				it.FamilyID = family
				if n == 1 && mode == "none" {
					it.MacroSource = database.MacroSourceNone
				}
				if n == 1 && mode == "zero" {
					it.WeightGrams = 0
				}
				if err := st.DB().Create(&it).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
		meals = append(meals, m)
	}
	caps, err := h.repeatCapabilities(user, meals, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range meals {
		if caps[m.ID] != (m.Name == "valid") {
			t.Fatalf("can_repeat(%s)=%v", m.Name, caps[m.ID])
		}
	}
}
