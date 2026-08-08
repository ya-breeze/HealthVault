package usda

import (
	"archive/zip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// Default FoodData Central CSV bundles. SR Legacy is frozen — its final release
// was April 2018 and USDA does not update it — while Foundation Foods publishes
// roughly twice a year. That cadence is why import is an operator-run command
// rather than a scheduled poller.
var DefaultSources = []string{
	"https://fdc.nal.usda.gov/fdc-datasets/FoodData_Central_sr_legacy_food_csv_2018-04.zip",
}

// FDC nutrient IDs for the 7 macros HealthVault tracks.
const (
	nutrientEnergyKcal = 1008
	nutrientProtein    = 1003
	nutrientCarbs      = 1005
	nutrientFat        = 1004
	nutrientSugars     = 2000 // "Sugars, total including NLEA"
	nutrientSugarsAlt  = 1063
	nutrientSodium     = 1093
	nutrientFiber      = 1079
)

// Fetch returns a readable local copy of src, downloading it if it is a URL.
// The returned cleanup removes any temporary file.
func Fetch(src string) (string, func(), error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return src, func() {}, nil
	}
	tmp, err := os.CreateTemp("", "fdc-*.zip")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	cleanup := func() { os.Remove(tmp.Name()) } //nolint:errcheck

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(src) //nolint:noctx
	if err != nil {
		tmp.Close() //nolint:errcheck
		cleanup()
		return "", nil, fmt.Errorf("download %s: %w", src, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		tmp.Close() //nolint:errcheck
		cleanup()
		return "", nil, fmt.Errorf("download %s: HTTP %d", src, resp.StatusCode)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close() //nolint:errcheck
		cleanup()
		return "", nil, fmt.Errorf("save download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close download: %w", err)
	}
	return tmp.Name(), cleanup, nil
}

// ImportZip reads an FDC CSV bundle and adds every food it describes to b.
// It returns the number of foods added.
func ImportZip(zipPath string, b *Builder) (int, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", zipPath, err)
	}
	defer zr.Close() //nolint:errcheck

	foods, err := readFoods(zr)
	if err != nil {
		return 0, err
	}
	if len(foods) == 0 {
		return 0, errors.New("no rows in food.csv; is this a FoodData Central CSV bundle?")
	}
	if err := applyNutrients(zr, foods); err != nil {
		return 0, err
	}

	n := 0
	for _, f := range foods {
		if err := b.Add(*f); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// openCSV finds a file by base name anywhere in the bundle; FDC nests everything
// under a release-dated directory whose name changes between releases.
func openCSV(zr *zip.ReadCloser, base string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if path.Base(f.Name) == base {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s: %w", base, err)
			}
			return rc, nil
		}
	}
	return nil, fmt.Errorf("%s not found in bundle", base)
}

// columnIndex maps required header names to their positions, so a column order
// change between releases does not silently shift values into the wrong fields.
func columnIndex(header []string, want ...string) (map[string]int, error) {
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimSpace(strings.ToLower(h))] = i
	}
	for _, w := range want {
		if _, ok := idx[w]; !ok {
			return nil, fmt.Errorf("missing column %q (have %v)", w, header)
		}
	}
	return idx, nil
}

func readFoods(zr *zip.ReadCloser) (map[int64]*Food, error) {
	rc, err := openCSV(zr, "food.csv")
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck

	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read food.csv header: %w", err)
	}
	idx, err := columnIndex(header, "fdc_id", "data_type", "description")
	if err != nil {
		return nil, fmt.Errorf("food.csv: %w", err)
	}

	foods := map[int64]*Food{}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read food.csv: %w", err)
		}
		id, err := strconv.ParseInt(strings.TrimSpace(rec[idx["fdc_id"]]), 10, 64)
		if err != nil {
			continue // skip malformed row rather than abort the whole import
		}
		foods[id] = &Food{
			FdcID:       id,
			DataType:    rec[idx["data_type"]],
			Description: rec[idx["description"]],
		}
	}
	return foods, nil
}

func applyNutrients(zr *zip.ReadCloser, foods map[int64]*Food) error {
	rc, err := openCSV(zr, "food_nutrient.csv")
	if err != nil {
		return err
	}
	defer rc.Close() //nolint:errcheck

	r := csv.NewReader(rc)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("read food_nutrient.csv header: %w", err)
	}
	idx, err := columnIndex(header, "fdc_id", "nutrient_id", "amount")
	if err != nil {
		return fmt.Errorf("food_nutrient.csv: %w", err)
	}

	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read food_nutrient.csv: %w", err)
		}
		fdcID, err1 := strconv.ParseInt(strings.TrimSpace(rec[idx["fdc_id"]]), 10, 64)
		nutID, err2 := strconv.Atoi(strings.TrimSpace(rec[idx["nutrient_id"]]))
		amount, err3 := strconv.ParseFloat(strings.TrimSpace(rec[idx["amount"]]), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		f, ok := foods[fdcID]
		if !ok {
			continue
		}
		assignNutrient(&f.Profile, nutID, amount)
	}
	return nil
}

// assignNutrient maps an FDC nutrient amount onto the per-100g profile.
// FDC reports sodium in mg per 100g; HealthVault's Nutrition columns are grams,
// so it is converted here rather than at every read site.
func assignNutrient(p *database.NutrientProfile, nutrientID int, amount float64) {
	switch nutrientID {
	case nutrientEnergyKcal:
		p.CaloriesPer100g = amount
	case nutrientProtein:
		p.ProteinPer100g = amount
	case nutrientCarbs:
		p.CarbsPer100g = amount
	case nutrientFat:
		p.FatPer100g = amount
	case nutrientSugars, nutrientSugarsAlt:
		if p.SugarPer100g == 0 {
			p.SugarPer100g = amount
		}
	case nutrientSodium:
		p.SodiumPer100g = amount / 1000.0
	case nutrientFiber:
		p.DietaryFiberPer100g = amount
	}
}
