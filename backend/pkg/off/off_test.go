package off_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/database"
	"github.com/ya-breeze/healthvault/pkg/off"
)

func product(code, name, brands string, kcal float64) off.Food {
	return off.Food{
		Code: code, ProductName: name, Brands: brands,
		Profile: database.NutrientProfile{CaloriesPer100g: kcal, ProteinPer100g: 5},
	}
}

// buildIndex creates a promoted database with n filler rows plus the given ones.
func buildIndex(t *testing.T, extra ...off.Food) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "off.db")
	b, err := off.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	for _, f := range extra {
		if err := b.Add(f); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	// Pad past MinExpectedRows so Promote accepts the build.
	for i := range off.MinExpectedRows {
		if err := b.Add(product(
			fmt.Sprintf("filler-%d", i), "Filler product", "Filler Brand", 1)); err != nil {
			t.Fatalf("Add filler: %v", err)
		}
	}
	if _, err := b.Promote(); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	return target
}

func TestOpen_NoDatabase(t *testing.T) {
	_, err := off.Open(filepath.Join(t.TempDir(), "absent.db"))
	if !errors.Is(err, off.ErrNoDatabase) {
		t.Fatalf("err = %v, want ErrNoDatabase", err)
	}
}

// The core regression test for the group-level OR-vs-AND fix: a name match
// alone must never substitute for a brand match.
func TestSearch_RequiresBrandMatch(t *testing.T) {
	path := buildIndex(t,
		product("1", "Bílý jogurt", "Olma", 65),
		product("2", "Bílý jogurt", "Danone", 60),
	)
	idx, err := off.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("bílý jogurt", "Olma", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Code != "1" {
		t.Fatalf("got %+v, want only the Olma product", got)
	}

	// Searching for a brand that has no yogurt in the index must not return
	// the other brand's yogurt just because the name matches.
	got, err = idx.Search("bílý jogurt", "Nestle", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d candidates for an absent brand, want 0: %+v", len(got), got)
	}
}

// The token-level OR-vs-AND fix: a multi-word brand's own tokens must all be
// required together, not OR'd against each other.
func TestSearch_MultiWordBrandRequiresAllTokens(t *testing.T) {
	path := buildIndex(t,
		product("1", "Vanilla pudding", "Dr. Oetker", 150),
		product("2", "Cola", "Dr Pepper", 40),
	)
	idx, err := off.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("pudding", "Dr. Oetker", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Code != "1" {
		t.Fatalf("got %+v, want only the Dr. Oetker product", got)
	}

	// A query whose name term happens to match the unrelated "Dr Pepper" row
	// must not let it through just because it shares the brand token "dr" —
	// brand is the only filter, so this returns the Dr. Oetker product
	// (ranked by whatever little name overlap it has), never Dr Pepper.
	got, err = idx.Search("cola", "Dr. Oetker", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Code != "1" {
		t.Fatalf("got %+v, want only the Dr. Oetker product (Dr Pepper must never match a Dr. Oetker brand search)", got)
	}
}

func TestSearch_RanksByNameWithinBrandMatchedSet(t *testing.T) {
	path := buildIndex(t,
		product("1", "Bílý jogurt light", "Olma", 55),
		product("2", "Bílý jogurt", "Olma", 65),
	)
	idx, err := off.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("bílý jogurt", "Olma", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want both Olma products: %+v", len(got), got)
	}
}

// The core regression test for the group-level AND-vs-filter fix: a
// brand-matched product whose name shares no token with the query must still
// be returned (just ranked lower), never excluded.
func TestSearch_NameMismatchWithinBrandStillReturnsResult(t *testing.T) {
	path := buildIndex(t, product("1", "Bílý jogurt", "Olma", 65))
	idx, err := off.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	// "yogurt" shares no token with the indexed "Bílý jogurt" (Czech, no
	// shared substring), but the brand matches — the product must still come
	// back, just possibly ranked behind a better name match.
	got, err := idx.Search("yogurt", "Olma", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Code != "1" {
		t.Fatalf("got %+v, want the brand-matched product despite zero name-token overlap", got)
	}
}

func TestSearch_RanksNameMatchAboveNameMismatchWithinSameBrand(t *testing.T) {
	path := buildIndex(t,
		product("1", "Cottage cheese", "Olma", 98),
		product("2", "Bílý jogurt", "Olma", 65),
	)
	idx, err := off.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("jogurt", "Olma", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want both Olma products: %+v", len(got), got)
	}
	if got[0].Code != "2" {
		t.Fatalf("got[0] = %+v, want the name-matching yogurt ranked first", got[0])
	}
}

func TestSearch_EmptyBrandReturnsNoResults(t *testing.T) {
	idx, err := off.Open(buildIndex(t, product("1", "Bílý jogurt", "Olma", 65)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	got, err := idx.Search("bílý jogurt", "", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d candidates for an empty brand, want 0: %+v", len(got), got)
	}
}

// Punctuation and FTS5 operators in either name or brand must not raise a
// syntax error.
func TestSearch_HostileInputDoesNotError(t *testing.T) {
	idx, err := off.Open(buildIndex(t, product("1", "Bílý jogurt", "Olma", 65)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	for _, q := range []string{`"`, `AND`, `olma AND (`, `*`, `--`, `NEAR/`, ``, `   `} {
		if _, err := idx.Search(q, q, 5); err != nil {
			t.Errorf("Search(%q, %q) errored: %v", q, q, err)
		}
	}
}

func TestByCode(t *testing.T) {
	idx, err := off.Open(buildIndex(t, product("8594001222227", "Bílý jogurt", "Olma", 65)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	f, err := idx.ByCode("8594001222227")
	if err != nil {
		t.Fatalf("ByCode: %v", err)
	}
	if f == nil || f.Profile.CaloriesPer100g != 65 {
		t.Fatalf("got %+v, want the yogurt profile", f)
	}

	missing, err := idx.ByCode("0000000000000")
	if err != nil {
		t.Fatalf("ByCode(missing): %v", err)
	}
	if missing != nil {
		t.Errorf("got %+v for a missing code, want nil", missing)
	}
}

// A short or truncated import must not be promoted over a working database.
func TestPromote_RefusesTruncatedImportAndKeepsPrevious(t *testing.T) {
	target := buildIndex(t, product("1", "Bílý jogurt", "Olma", 65))

	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read existing db: %v", err)
	}

	b, err := off.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.Add(product("2", "Only one product", "Brand", 10)); err != nil {
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

	idx, err := off.Open(target)
	if err != nil {
		t.Fatalf("Open after refused import: %v", err)
	}
	defer idx.Close() //nolint:errcheck
	if got, err := idx.Search("bílý jogurt", "Olma", 5); err != nil || len(got) == 0 {
		t.Errorf("previous database no longer serves: %d results, err %v", len(got), err)
	}
}

func TestPromote_LeavesNoTempFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "off.db")
	b, err := off.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.Add(product("1", "x", "y", 1)); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, _ = b.Promote() // refused: below minimum

	if _, err := os.Stat(target + ".building"); !os.IsNotExist(err) {
		t.Error("temporary build file left behind after a refused promote")
	}
}

func TestCount(t *testing.T) {
	idx, err := off.Open(buildIndex(t, product("1", "x", "y", 1)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	n, err := idx.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != off.MinExpectedRows+1 {
		t.Errorf("Count = %d, want %d", n, off.MinExpectedRows+1)
	}
}
