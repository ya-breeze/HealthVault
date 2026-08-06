package usda_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/usda"
)

func food(id int64, desc string, kcal float64) usda.Food {
	return usda.Food{
		FdcID: id, Description: desc, DataType: "sr_legacy_food",
		Profile: database.NutrientProfile{CaloriesPer100g: kcal, ProteinPer100g: 31},
	}
}

// buildIndex creates a promoted database with n filler rows plus the given ones.
func buildIndex(t *testing.T, extra ...usda.Food) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "usda.db")
	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	for _, f := range extra {
		if err := b.Add(f); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	// Pad past MinExpectedRows so Promote accepts the build.
	for i := range usda.MinExpectedRows {
		if err := b.Add(food(int64(900000+i), "Filler food item", 1)); err != nil {
			t.Fatalf("Add filler: %v", err)
		}
	}
	if _, err := b.Promote(); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	return target
}

// A search before any import must degrade, not fail the enclosing request.
func TestOpen_NoDatabase(t *testing.T) {
	_, err := usda.Open(filepath.Join(t.TempDir(), "absent.db"))
	if !errors.Is(err, usda.ErrNoDatabase) {
		t.Fatalf("err = %v, want ErrNoDatabase", err)
	}
}

func TestSearch_RanksLLMPhrasingAgainstSRLegacyDescription(t *testing.T) {
	path := buildIndex(t,
		food(1, "Chicken, broilers or fryers, breast, meat only, cooked, roasted", 165),
		food(2, "Beef, ground, 80% lean meat, cooked, pan-browned", 254),
	)
	idx, err := usda.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("grilled chicken breast", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no candidates for 'grilled chicken breast'")
	}
	if got[0].FdcID != 1 {
		t.Errorf("top candidate = %d (%q), want fdc 1", got[0].FdcID, got[0].Description)
	}
	if got[0].Profile.CaloriesPer100g != 165 {
		t.Errorf("profile not returned with candidate: %+v", got[0].Profile)
	}
}

// Punctuation and FTS5 operators must not raise a syntax error.
func TestSearch_HostileInputDoesNotError(t *testing.T) {
	idx, err := usda.Open(buildIndex(t, food(1, "Rice, white, cooked", 130)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	for _, q := range []string{`"`, `AND`, `rice AND (`, `*`, `--`, `NEAR/`, ``, `   `} {
		if _, err := idx.Search(q, 5); err != nil {
			t.Errorf("Search(%q) errored: %v", q, err)
		}
	}
}

func TestSearch_NoMatchReturnsEmptyNotError(t *testing.T) {
	idx, err := usda.Open(buildIndex(t, food(1, "Rice, white, cooked", 130)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("zzzzzznotafood", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d candidates, want 0", len(got))
	}
}

func TestByFdcID(t *testing.T) {
	idx, err := usda.Open(buildIndex(t, food(42, "Oats, raw", 389)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	f, err := idx.ByFdcID(42)
	if err != nil {
		t.Fatalf("ByFdcID: %v", err)
	}
	if f == nil || f.Profile.CaloriesPer100g != 389 {
		t.Fatalf("got %+v, want the oats profile", f)
	}

	missing, err := idx.ByFdcID(999999999)
	if err != nil {
		t.Fatalf("ByFdcID(missing): %v", err)
	}
	if missing != nil {
		t.Errorf("got %+v for a missing id, want nil", missing)
	}
}

// A short or truncated import must not be promoted over a working database.
func TestPromote_RefusesTruncatedImportAndKeepsPrevious(t *testing.T) {
	target := buildIndex(t, food(1, "Rice, white, cooked", 130))

	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read existing db: %v", err)
	}

	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.Add(food(2, "Only one food", 10)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	n, err := b.Promote()
	if err == nil {
		t.Fatal("Promote accepted a 1-row import, want refusal")
	}
	if n != 1 {
		t.Errorf("reported row count = %d, want 1", n)
	}

	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read db after refused promote: %v", err)
	}
	if len(before) != len(after) {
		t.Error("existing database was modified by a refused import")
	}

	// The previous database must still serve queries.
	idx, err := usda.Open(target)
	if err != nil {
		t.Fatalf("Open after refused import: %v", err)
	}
	defer idx.Close() //nolint:errcheck
	if got, err := idx.Search("rice", 5); err != nil || len(got) == 0 {
		t.Errorf("previous database no longer serves: %d results, err %v", len(got), err)
	}
}

func TestPromote_LeavesNoTempFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "usda.db")
	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.Add(food(1, "x", 1)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, _ = b.Promote() // refused: below minimum

	if _, err := os.Stat(target + ".building"); !os.IsNotExist(err) {
		t.Error("temporary build file left behind after a refused promote")
	}
}
