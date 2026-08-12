package off

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ya-breeze/healthvault/pkg/database"
)

// DefaultSource is the official Open Food Facts full JSONL export, updated
// nightly (unlike USDA's frozen SR Legacy bundle). Import stays an
// operator-run command regardless — see off/import: CmdImportOFF's doc
// comment and design.md's "Country/completeness filter" decision — the
// project has no background-job infrastructure to poll it automatically.
const DefaultSource = "https://static.openfoodfacts.org/data/openfoodfacts-products.jsonl.gz"

// Fetch returns a readable local copy of src, downloading it if it is a URL.
// The returned cleanup removes any temporary file. Mirrors usda.Fetch's
// "accept a URL or a local path" flexibility on purpose: OFF's export
// URL/format is far less stable than USDA's frozen zip, so a broken
// auto-download can be worked around by fetching the dump manually and
// importing from a local path (see design.md's OFF-download-stability risk).
func Fetch(src string) (string, func(), error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return src, func() {}, nil
	}
	tmp, err := os.CreateTemp("", "off-*.jsonl.gz")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	cleanup := func() { os.Remove(tmp.Name()) } //nolint:errcheck

	client := &http.Client{Timeout: 60 * time.Minute}
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

// countryTagMatches reports whether tag names a Czech or Slovak country, by
// substring match against Open Food Facts' normalized "en:"-prefixed
// countries_tags values (e.g. "en:czech-republic", "en:slovakia"). Substring
// rather than an exact tag list on purpose: it also matches variant/legacy
// spellings ("en:czechia") without needing to enumerate every one, and
// narrowing the corpus is strictly the safer failure mode than widening it
// (design.md "Scope for v1").
func countryTagMatches(tag string) bool {
	t := strings.ToLower(tag)
	return strings.Contains(t, "czech") || strings.Contains(t, "slovak")
}

// offProduct is the subset of one Open Food Facts JSONL record this import
// cares about. Nutriment fields are *float64 so a field's absence (nil) is
// distinguishable from an explicit 0 — required for the completeness filter
// and the salt->sodium fallback (design.md "Explicit per-field mapping").
type offProduct struct {
	Code          string        `json:"code"`
	ProductName   string        `json:"product_name"`
	Brands        string        `json:"brands"`
	CountriesTags []string      `json:"countries_tags"`
	Nutriments    offNutriments `json:"nutriments"`
}

type offNutriments struct {
	EnergyKcal100g    *float64 `json:"energy-kcal_100g"`
	Proteins100g      *float64 `json:"proteins_100g"`
	Carbohydrates100g *float64 `json:"carbohydrates_100g"`
	Fat100g           *float64 `json:"fat_100g"`
	Sugars100g        *float64 `json:"sugars_100g"`
	Fiber100g         *float64 `json:"fiber_100g"`
	Sodium100g        *float64 `json:"sodium_100g"`
	Salt100g          *float64 `json:"salt_100g"`
}

// matchesCountry reports whether p is tagged Czech and/or Slovak.
func (p *offProduct) matchesCountry() bool {
	for _, t := range p.CountriesTags {
		if countryTagMatches(t) {
			return true
		}
	}
	return false
}

// toFood maps p onto a Food, applying the explicit per-field nutrient
// mapping from design.md's "Explicit per-field mapping and unit handling"
// decision: calories from energy-kcal_100g only (never energy_100g or
// energy-kj_100g, which are off by roughly 4x if misread as kcal); sodium
// from sodium_100g, falling back to salt_100g/2.5 (the EU 2013 salt->sodium
// labeling conversion) when only salt is present, defaulting to 0 when
// neither is; sugar/fiber default to 0 when absent. ok is false when the
// product is missing calories or any of protein/carbs/fat — the
// completeness bar this import enforces. Sodium/sugar/fiber absence does
// NOT exclude a product.
func (p *offProduct) toFood() (Food, bool) {
	n := p.Nutriments
	if n.EnergyKcal100g == nil || n.Proteins100g == nil || n.Carbohydrates100g == nil || n.Fat100g == nil {
		return Food{}, false
	}
	profile := database.NutrientProfile{
		CaloriesPer100g: *n.EnergyKcal100g,
		ProteinPer100g:  *n.Proteins100g,
		CarbsPer100g:    *n.Carbohydrates100g,
		FatPer100g:      *n.Fat100g,
	}
	if n.Sugars100g != nil {
		profile.SugarPer100g = *n.Sugars100g
	}
	if n.Fiber100g != nil {
		profile.DietaryFiberPer100g = *n.Fiber100g
	}
	switch {
	case n.Sodium100g != nil:
		profile.SodiumPer100g = *n.Sodium100g
	case n.Salt100g != nil:
		profile.SodiumPer100g = *n.Salt100g / 2.5
	}
	return Food{
		Code:        p.Code,
		ProductName: p.ProductName,
		Brands:      p.Brands,
		Profile:     profile,
	}, true
}

// ImportStats reports what an import did. Kept is what got added to the
// index; Filtered is well-formed JSON excluded by the country or
// completeness filter (an expected, routine outcome for most of a 3.5M+ row
// global export); Malformed is JSON that failed to decode, or a record
// missing the barcode needed as its primary key.
type ImportStats struct {
	Kept      int
	Filtered  int
	Malformed int
}

// ImportJSONL streams path (a gzip-compressed Open Food Facts JSONL export)
// and adds every kept, mapped product to b.
//
// Two error classes are handled oppositely (design.md/tasks.md 3.4). A
// single line that fails to decode as JSON — or decodes but has no barcode —
// is counted in Malformed and the stream continues: one bad product among
// millions is routine, not exceptional. An error from the underlying
// reader/gzip decoder itself (corrupt or truncated gzip, unexpected EOF
// mid-stream) aborts the entire import immediately with a non-nil error
// instead of being folded into Malformed: a truncated gzip stream can yield
// thousands of valid rows before erroring at EOF — enough to clear
// MinExpectedRows on its own — and must never be silently promoted as if the
// import had completed normally. Callers must discard the build (see
// Builder.Discard) when this returns a non-nil error.
func ImportJSONL(path string, b *Builder) (ImportStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return ImportStats{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	gz, err := gzip.NewReader(f)
	if err != nil {
		return ImportStats{}, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gz.Close() //nolint:errcheck

	var stats ImportStats
	// bufio.Reader.ReadBytes, not bufio.Scanner: OFF product records can
	// exceed a Scanner's fixed max-token size and error out on a line that is
	// otherwise perfectly valid. ReadBytes has no such ceiling.
	r := bufio.NewReader(gz)
	for {
		line, readErr := r.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			if addErr := importLine(trimmed, b, &stats); addErr != nil {
				return stats, addErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return stats, fmt.Errorf("read Open Food Facts export: %w", readErr)
		}
	}
	return stats, nil
}

// importLine decodes and maps one JSONL line, updating stats and calling
// b.Add for a kept product. Only a genuine database-write failure from
// b.Add is returned as an error; decode/filter outcomes are recorded in
// stats instead, per ImportJSONL's doc comment.
func importLine(line []byte, b *Builder, stats *ImportStats) error {
	var p offProduct
	if err := json.Unmarshal(line, &p); err != nil {
		stats.Malformed++
		return nil
	}
	if p.Code == "" {
		stats.Malformed++
		return nil
	}
	if !p.matchesCountry() {
		stats.Filtered++
		return nil
	}
	food, ok := p.toFood()
	if !ok {
		stats.Filtered++
		return nil
	}
	if err := b.Add(food); err != nil {
		return err
	}
	stats.Kept++
	return nil
}
