package off

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestCountryTagMatches(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"en:czech-republic", true},
		{"en:slovakia", true},
		{"en:czechia", true},
		{"en:germany", false},
		{"en:france", false},
	}
	for _, tc := range cases {
		if got := countryTagMatches(tc.tag); got != tc.want {
			t.Errorf("countryTagMatches(%q) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestOffProduct_MatchesCountry(t *testing.T) {
	p := offProduct{CountriesTags: []string{"en:germany", "en:slovakia"}}
	if !p.matchesCountry() {
		t.Error("expected match on slovakia tag")
	}
	p2 := offProduct{CountriesTags: []string{"en:germany", "en:france"}}
	if p2.matchesCountry() {
		t.Error("expected no match")
	}
}

func f64(v float64) *float64 { return &v }

func TestOffProduct_ToFood_CompletenessFilter(t *testing.T) {
	complete := offProduct{
		Code: "1", ProductName: "x", Brands: "y",
		Nutriments: offNutriments{
			EnergyKcal100g: f64(100), Proteins100g: f64(5), Carbohydrates100g: f64(10), Fat100g: f64(2),
		},
	}
	if _, ok := complete.toFood(); !ok {
		t.Error("expected complete product to pass the filter")
	}

	missingCalories := complete
	missingCalories.Nutriments.EnergyKcal100g = nil
	if _, ok := missingCalories.toFood(); ok {
		t.Error("expected missing calories to fail the filter")
	}

	missingProtein := complete
	missingProtein.Nutriments.Proteins100g = nil
	if _, ok := missingProtein.toFood(); ok {
		t.Error("expected missing protein to fail the filter")
	}

	missingCarbs := complete
	missingCarbs.Nutriments.Carbohydrates100g = nil
	if _, ok := missingCarbs.toFood(); ok {
		t.Error("expected missing carbs to fail the filter")
	}

	missingFat := complete
	missingFat.Nutriments.Fat100g = nil
	if _, ok := missingFat.toFood(); ok {
		t.Error("expected missing fat to fail the filter")
	}

	// Sodium/sugar/fiber absence must NOT exclude a product — only
	// calories/protein/carbs/fat are the completeness bar.
	if _, ok := complete.toFood(); !ok {
		t.Error("expected missing sodium/sugar/fiber to still pass the filter")
	}
}

func TestOffProduct_ToFood_EnergyFieldSelection(t *testing.T) {
	p := offProduct{
		Code: "1", ProductName: "x",
		Nutriments: offNutriments{
			EnergyKcal100g: f64(250), Proteins100g: f64(5), Carbohydrates100g: f64(10), Fat100g: f64(2),
		},
	}
	food, ok := p.toFood()
	if !ok {
		t.Fatal("expected ok")
	}
	if food.Profile.CaloriesPer100g != 250 {
		t.Errorf("CaloriesPer100g = %v, want 250 (from energy-kcal_100g)", food.Profile.CaloriesPer100g)
	}
}

func TestOffProduct_ToFood_SodiumFallsBackToSaltOverTwoPointFive(t *testing.T) {
	withSodium := offProduct{
		Nutriments: offNutriments{
			EnergyKcal100g: f64(1), Proteins100g: f64(1), Carbohydrates100g: f64(1), Fat100g: f64(1),
			Sodium100g: f64(0.4), Salt100g: f64(999), // sodium present must win outright
		},
	}
	food, _ := withSodium.toFood()
	if food.Profile.SodiumPer100g != 0.4 {
		t.Errorf("SodiumPer100g = %v, want 0.4 (sodium_100g takes priority over salt_100g)", food.Profile.SodiumPer100g)
	}

	saltOnly := offProduct{
		Nutriments: offNutriments{
			EnergyKcal100g: f64(1), Proteins100g: f64(1), Carbohydrates100g: f64(1), Fat100g: f64(1),
			Salt100g: f64(2.5),
		},
	}
	food2, _ := saltOnly.toFood()
	if food2.Profile.SodiumPer100g != 1.0 {
		t.Errorf("SodiumPer100g = %v, want 1.0 (2.5 salt / 2.5)", food2.Profile.SodiumPer100g)
	}

	neither := offProduct{
		Nutriments: offNutriments{
			EnergyKcal100g: f64(1), Proteins100g: f64(1), Carbohydrates100g: f64(1), Fat100g: f64(1),
		},
	}
	food3, _ := neither.toFood()
	if food3.Profile.SodiumPer100g != 0 {
		t.Errorf("SodiumPer100g = %v, want 0 when neither sodium nor salt is present", food3.Profile.SodiumPer100g)
	}
}

// gzipLines gzip-compresses a JSONL payload built from the given lines and
// writes it to a temp file, returning its path.
func gzipLines(t *testing.T, lines ...string) string {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	for _, l := range lines {
		if _, err := gw.Write([]byte(l + "\n")); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	path := filepath.Join(t.TempDir(), "export.jsonl.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

const czProductLine = `{"code":"1","product_name":"Jogurt","brands":"Olma","countries_tags":["en:czech-republic"],"nutriments":{"energy-kcal_100g":65,"proteins_100g":5,"carbohydrates_100g":10,"fat_100g":2}}`
const deProductLine = `{"code":"2","product_name":"Joghurt","brands":"Danone","countries_tags":["en:germany"],"nutriments":{"energy-kcal_100g":65,"proteins_100g":5,"carbohydrates_100g":10,"fat_100g":2}}`
const incompleteProductLine = `{"code":"3","product_name":"Mystery","brands":"X","countries_tags":["en:czech-republic"],"nutriments":{"proteins_100g":5}}`

func TestImportJSONL_CountryAndCompletenessFiltering(t *testing.T) {
	path := gzipLines(t, czProductLine, deProductLine, incompleteProductLine)

	target := filepath.Join(t.TempDir(), "off.db")
	b, err := NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	defer b.Discard()

	stats, err := ImportJSONL(path, b)
	if err != nil {
		t.Fatalf("ImportJSONL: %v", err)
	}
	if stats.Kept != 1 {
		t.Errorf("Kept = %d, want 1 (only the CZ, complete product)", stats.Kept)
	}
	if stats.Filtered != 2 {
		t.Errorf("Filtered = %d, want 2 (wrong country + incomplete)", stats.Filtered)
	}
	if stats.Malformed != 0 {
		t.Errorf("Malformed = %d, want 0", stats.Malformed)
	}
}

func TestImportJSONL_MalformedLineSkippedAndCounted(t *testing.T) {
	path := gzipLines(t, czProductLine, `{not valid json`, `{"product_name":"no code","countries_tags":["en:czech-republic"]}`)

	target := filepath.Join(t.TempDir(), "off.db")
	b, err := NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	defer b.Discard()

	stats, err := ImportJSONL(path, b)
	if err != nil {
		t.Fatalf("ImportJSONL: %v", err)
	}
	if stats.Kept != 1 {
		t.Errorf("Kept = %d, want 1", stats.Kept)
	}
	if stats.Malformed != 2 {
		t.Errorf("Malformed = %d, want 2 (bad JSON + missing code)", stats.Malformed)
	}
}

// A truncated/corrupt gzip stream must abort the entire import outright,
// not be folded into Malformed — even though many valid rows can be read
// before the corruption is discovered.
func TestImportJSONL_TruncatedGzipAbortsImport(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	for range 50 {
		if _, err := gw.Write([]byte(czProductLine + "\n")); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	full := buf.Bytes()
	truncated := full[:len(full)/2] // valid header, corrupt/incomplete body

	path := filepath.Join(t.TempDir(), "export.jsonl.gz")
	if err := os.WriteFile(path, truncated, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	target := filepath.Join(t.TempDir(), "off.db")
	b, err := NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	defer b.Discard()

	_, err = ImportJSONL(path, b)
	if err == nil {
		t.Fatal("ImportJSONL succeeded on a truncated gzip stream, want an error")
	}
}
