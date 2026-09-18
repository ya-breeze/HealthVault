package usda_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/usda"
)

// buildPreSaturatedFatIndex builds a promoted database using the schema this
// package shipped before saturated_fat existed, to prove Open/Search/ByFdcID
// still work against a file an operator hasn't reimported yet — see
// docs/specs/saturated-fat-signal.md's blocking finding on this exact gap.
func buildPreSaturatedFatIndex(t *testing.T, id int64, desc string, kcal float64) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "usda.db")
	db, err := sql.Open("sqlite3", target)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close() //nolint:errcheck
	const legacySchema = `
CREATE TABLE usda_foods (
  rowid       INTEGER PRIMARY KEY,
  fdc_id      INTEGER NOT NULL UNIQUE,
  description TEXT    NOT NULL,
  data_type   TEXT    NOT NULL,
  calories    REAL    NOT NULL DEFAULT 0,
  protein     REAL    NOT NULL DEFAULT 0,
  carbs       REAL    NOT NULL DEFAULT 0,
  fat         REAL    NOT NULL DEFAULT 0,
  sugar       REAL    NOT NULL DEFAULT 0,
  sodium      REAL    NOT NULL DEFAULT 0,
  fiber       REAL    NOT NULL DEFAULT 0
);
CREATE VIRTUAL TABLE usda_foods_fts USING fts5(description, content='usda_foods', content_rowid='rowid');
`
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema (sqlite_fts5 build tag set?): %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO usda_foods (fdc_id, description, data_type, calories, protein) VALUES (?,?,?,?,?)`,
		id, desc, "sr_legacy_food", kcal, 31.0,
	); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO usda_foods_fts(usda_foods_fts) VALUES('rebuild')`); err != nil {
		t.Fatalf("rebuild fts: %v", err)
	}
	return target
}

func TestOpen_PreSaturatedFatDatabaseDegradesGracefully(t *testing.T) {
	path := buildPreSaturatedFatIndex(t, 42, "Oats, raw", 389)
	idx, err := usda.Open(path)
	if err != nil {
		t.Fatalf("Open on a pre-upgrade database must not fail: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	byID, err := idx.ByFdcID(42)
	if err != nil {
		t.Fatalf("ByFdcID on a pre-upgrade database must not fail: %v", err)
	}
	if byID == nil || byID.Profile.CaloriesPer100g != 389 || byID.Profile.SaturatedFatPer100g != 0 {
		t.Fatalf("got %+v, want the oats profile with SaturatedFatPer100g 0", byID)
	}

	results, err := idx.Search("oats", usda.DefaultCandidates)
	if err != nil {
		t.Fatalf("Search on a pre-upgrade database must not fail: %v", err)
	}
	if len(results) != 1 || results[0].Profile.SaturatedFatPer100g != 0 {
		t.Fatalf("got %+v, want one result with SaturatedFatPer100g 0", results)
	}
}

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

// A non-positive limit falls back to the default shortlist size. The default is
// large because the correct food ranked 12th and 17th for two ordinary queries
// on the real dataset; a small shortlist simply would not contain it.
func TestSearch_DefaultCandidateLimit(t *testing.T) {
	if usda.DefaultCandidates < 30 {
		t.Fatalf("DefaultCandidates = %d, want at least 30", usda.DefaultCandidates)
	}
	// The filler rows all share a description, so an unbounded search matches
	// far more rows than the default allows through.
	idx, err := usda.Open(buildIndex(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("filler food", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != usda.DefaultCandidates {
		t.Errorf("got %d candidates for limit 0, want DefaultCandidates (%d)", len(got), usda.DefaultCandidates)
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
