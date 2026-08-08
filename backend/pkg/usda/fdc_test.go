package usda_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ya-breeze/healthvault/pkg/usda"
)

// makeBundle writes a minimal FDC-shaped CSV zip, nested under a release
// directory the way the real bundles are.
func makeBundle(t *testing.T, foodCSV, nutrientCSV string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bundle.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close() //nolint:errcheck

	zw := zip.NewWriter(f)
	write := func(name, body string) {
		w, err := zw.Create("FoodData_Central_sr_legacy_food_csv_2018-04/" + name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	write("food.csv", foodCSV)
	write("food_nutrient.csv", nutrientCSV)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return p
}

const sampleFoodCSV = `"fdc_id","data_type","description","food_category_id","publication_date"
"171077","sr_legacy_food","Chicken, broilers or fryers, breast, meat only, cooked, roasted","5","2019-04-01"
"169704","sr_legacy_food","Rice, white, long-grain, regular, cooked","20","2019-04-01"
`

// Energy 1008, protein 1003, carbs 1005, fat 1004, sugars 2000, sodium 1093 (mg), fiber 1079.
const sampleNutrientCSV = `"id","fdc_id","nutrient_id","amount"
"1","171077","1008","165"
"2","171077","1003","31.02"
"3","171077","1093","74"
"4","169704","1008","130"
"5","169704","1005","28.17"
"6","169704","1079","0.4"
`

func TestImportZip_ParsesFoodsAndNutrients(t *testing.T) {
	bundle := makeBundle(t, sampleFoodCSV, sampleNutrientCSV)
	target := filepath.Join(t.TempDir(), "usda.db")

	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	n, err := usda.ImportZip(bundle, b)
	if err != nil {
		t.Fatalf("ImportZip: %v", err)
	}
	if n != 2 {
		t.Fatalf("imported %d foods, want 2", n)
	}
	b.Discard()
}

// FDC gives sodium in mg per 100g; the Nutrition columns this feeds are grams.
func TestImportZip_SodiumConvertedToGrams(t *testing.T) {
	bundle := makeBundle(t, sampleFoodCSV, sampleNutrientCSV)
	target := filepath.Join(t.TempDir(), "usda.db")

	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if _, err := usda.ImportZip(bundle, b); err != nil {
		t.Fatalf("ImportZip: %v", err)
	}
	// Pad past the promote minimum so we can query the result.
	for i := range usda.MinExpectedRows {
		if err := b.Add(food(int64(800000+i), "Filler", 1)); err != nil {
			t.Fatalf("Add filler: %v", err)
		}
	}
	if _, err := b.Promote(); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	idx, err := usda.Open(target)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer idx.Close() //nolint:errcheck

	f, err := idx.ByFdcID(171077)
	if err != nil || f == nil {
		t.Fatalf("ByFdcID: %v (food %v)", err, f)
	}
	if got := f.Profile.SodiumPer100g; got != 0.074 {
		t.Errorf("SodiumPer100g = %v, want 0.074 (74mg converted to grams)", got)
	}
	if f.Profile.CaloriesPer100g != 165 || f.Profile.ProteinPer100g != 31.02 {
		t.Errorf("profile = %+v, want 165 kcal / 31.02 g protein", f.Profile)
	}
}

// Column order changes between releases must be detected, not silently
// misinterpreted as values landing in the wrong fields.
func TestImportZip_ColumnOrderIndependent(t *testing.T) {
	reordered := `"description","fdc_id","data_type"
"Rice, white, cooked","169704","sr_legacy_food"
`
	nutrients := `"nutrient_id","fdc_id","amount","id"
"1008","169704","130","1"
`
	bundle := makeBundle(t, reordered, nutrients)
	target := filepath.Join(t.TempDir(), "usda.db")

	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	defer b.Discard()

	if _, err := usda.ImportZip(bundle, b); err != nil {
		t.Fatalf("ImportZip with reordered columns: %v", err)
	}
}

func TestImportZip_MissingColumnIsAnError(t *testing.T) {
	bad := `"fdc_id","description"
"1","No data_type column"
`
	bundle := makeBundle(t, bad, sampleNutrientCSV)
	target := filepath.Join(t.TempDir(), "usda.db")

	b, err := usda.NewBuilder(target)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	defer b.Discard()

	if _, err := usda.ImportZip(bundle, b); err == nil {
		t.Fatal("expected an error for a bundle missing data_type")
	}
}

func TestFetch_LocalPathPassesThrough(t *testing.T) {
	p := filepath.Join(t.TempDir(), "local.zip")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, cleanup, err := usda.Fetch(p)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer cleanup()
	if got != p {
		t.Errorf("Fetch(%q) = %q, want passthrough", p, got)
	}
}
