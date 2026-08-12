// Package off maintains a local SQLite copy of Open Food Facts product data,
// filtered at import time to Czech/Slovak-market products with complete
// nutriments, with an FTS5 index over product name and brand.
//
// The index is used for *candidate retrieval*, gated by an extracted brand —
// never for auto-assignment. See
// openspec/changes/add-open-food-facts-source/design.md.
//
// Package structure deliberately mirrors pkg/usda rather than sharing a
// generic abstraction: the two sources have different schemas (this one has
// code/brands, no data_type) and different import/filter rules, which is
// exactly where a shared abstraction would buy the least and cost the most.
//
// Requires the sqlite_fts5 build tag; without it FTS5 virtual tables do not exist.
package off

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver

	"github.com/ya-breeze/healthvault/pkg/database"
)

// ErrNoDatabase means no import has run yet. Callers surface this as an empty
// candidate list plus a flag rather than failing the enclosing request.
var ErrNoDatabase = errors.New("Open Food Facts reference database not present")

// MinExpectedRows guards against a truncated or partial import being
// promoted over a good database. The CZ/SK-filtered corpus is far smaller
// than USDA's ~8k Foundation+SR Legacy set, but a healthy import is still
// expected to clear this floor comfortably; it exists to catch a badly
// truncated or misfiltered run, not to model the "correct" corpus size.
const MinExpectedRows = 500

// DefaultCandidates is the shortlist size handed to the caller for
// selection. Smaller than usda.DefaultCandidates is fine here: every
// candidate is already brand-filtered, so the shortlist is inherently
// narrower than an unfiltered USDA search over the same name.
const DefaultCandidates = 30

// Food is one Open Food Facts product with its per-100g profile.
type Food struct {
	Code        string                   `json:"code"`
	ProductName string                   `json:"product_name"`
	Brands      string                   `json:"brands"`
	Profile     database.NutrientProfile `json:"profile"`
}

// Index is a read handle on the local Open Food Facts database.
type Index struct {
	db   *sql.DB
	path string
}

// Open opens the Open Food Facts database at path. A missing file is
// reported as ErrNoDatabase so the caller can degrade rather than fail.
func Open(path string) (*Index, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNoDatabase
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=30000&mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open off db: %w", err)
	}
	return &Index{db: db, path: path}, nil
}

// Close releases the database handle.
func (i *Index) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	if err := i.db.Close(); err != nil {
		return fmt.Errorf("close off db: %w", err)
	}
	return nil
}

// searchFetchLimit is how many brand-matched rows Search pulls from SQLite
// before re-ranking by name overlap in Go and truncating to the caller's
// limit. Brand-matched sets are already narrow by construction (see
// buildBrandQuery), so this only bounds how much work a very common brand
// with many SKUs can force onto the in-process ranking step.
const searchFetchLimit = 200

// Search returns up to limit ranked candidates for a free-text product name,
// requiring brand to match. An empty brand, or a brand with no usable
// tokens, returns no results rather than matching on name alone — see
// buildBrandQuery. Only the brand predicate is used to filter in SQL; name
// terms rank the brand-matched set afterward in Go (rankByNameOverlap) and
// can never exclude a correctly-branded product, even one that shares no
// token with name — e.g. a Czech "Bílý jogurt" product still ranks (just
// lower) against a recognized English "yogurt". An empty result is a normal
// outcome, not an error.
func (i *Index) Search(name, brand string, limit int) ([]Food, error) {
	if i == nil || i.db == nil {
		return nil, ErrNoDatabase
	}
	if limit <= 0 {
		limit = DefaultCandidates
	}
	q := buildBrandQuery(brand)
	if q == "" {
		return nil, nil
	}
	rows, err := i.db.Query(`
		SELECT f.code, f.product_name, f.brands,
		       f.calories, f.protein, f.carbs, f.fat, f.sugar, f.sodium, f.fiber
		FROM off_foods_fts fts
		JOIN off_foods f ON f.rowid = fts.rowid
		WHERE off_foods_fts MATCH ?
		ORDER BY bm25(off_foods_fts)
		LIMIT ?`, q, searchFetchLimit)
	if err != nil {
		return nil, fmt.Errorf("off search: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Food
	for rows.Next() {
		var f Food
		var p database.NutrientProfile
		if err := rows.Scan(&f.Code, &f.ProductName, &f.Brands,
			&p.CaloriesPer100g, &p.ProteinPer100g, &p.CarbsPer100g,
			&p.FatPer100g, &p.SugarPer100g, &p.SodiumPer100g, &p.DietaryFiberPer100g); err != nil {
			return nil, fmt.Errorf("scan off row: %w", err)
		}
		f.Profile = p
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate off rows: %w", err)
	}

	rankByNameOverlap(out, name)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// rankByNameOverlap stable-sorts foods (already brand-matched by SQL) by how
// many of name's terms appear in each product's name, descending. Stability
// preserves the original bm25 (brand-match strength) order as the tie-break,
// including when name has no usable terms at all — every row keeps its
// brand-match rank rather than being reordered arbitrarily.
func rankByNameOverlap(foods []Food, name string) {
	terms := nameRankTerms(name)
	if len(terms) == 0 {
		return
	}
	type scored struct {
		food  Food
		score int
	}
	list := make([]scored, len(foods))
	for idx, f := range foods {
		lower := strings.ToLower(f.ProductName)
		score := 0
		for _, t := range terms {
			if strings.Contains(lower, t) {
				score++
			}
		}
		list[idx] = scored{food: f, score: score}
	}
	sort.SliceStable(list, func(a, b int) bool {
		return list[a].score > list[b].score
	})
	for idx, s := range list {
		foods[idx] = s.food
	}
}

// ByCode looks up a single product by its Open Food Facts barcode, used when
// binding an item to a chosen candidate. Only ever called on an open Index —
// callers distinguish "no index open" from "code not found in an open
// index" separately (mirroring h.usda == nil vs. an unknown fdc_id), rather
// than expecting ByCode to report "no database" itself.
func (i *Index) ByCode(code string) (*Food, error) {
	if i == nil || i.db == nil {
		return nil, ErrNoDatabase
	}
	var f Food
	var p database.NutrientProfile
	err := i.db.QueryRow(`
		SELECT code, product_name, brands, calories, protein, carbs, fat, sugar, sodium, fiber
		FROM off_foods WHERE code = ?`, code).
		Scan(&f.Code, &f.ProductName, &f.Brands,
			&p.CaloriesPer100g, &p.ProteinPer100g, &p.CarbsPer100g,
			&p.FatPer100g, &p.SugarPer100g, &p.SodiumPer100g, &p.DietaryFiberPer100g)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("off lookup: %w", err)
	}
	f.Profile = p
	return &f, nil
}

// Count returns the number of indexed products.
func (i *Index) Count() (int, error) {
	if i == nil || i.db == nil {
		return 0, ErrNoDatabase
	}
	var n int
	if err := i.db.QueryRow(`SELECT COUNT(*) FROM off_foods`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count off foods: %w", err)
	}
	return n, nil
}

const schema = `
CREATE TABLE off_foods (
  rowid        INTEGER PRIMARY KEY,
  code         TEXT    NOT NULL UNIQUE,
  product_name TEXT    NOT NULL,
  brands       TEXT    NOT NULL DEFAULT '',
  calories     REAL    NOT NULL DEFAULT 0,
  protein      REAL    NOT NULL DEFAULT 0,
  carbs        REAL    NOT NULL DEFAULT 0,
  fat          REAL    NOT NULL DEFAULT 0,
  sugar        REAL    NOT NULL DEFAULT 0,
  sodium       REAL    NOT NULL DEFAULT 0,
  fiber        REAL    NOT NULL DEFAULT 0
);
CREATE VIRTUAL TABLE off_foods_fts USING fts5(product_name, brands, content='off_foods', content_rowid='rowid');
`

// Builder writes a new Open Food Facts database to a temporary path. The
// caller promotes it with Promote once it validates, so a failed import can
// never leave the previous database broken or half-written.
type Builder struct {
	db      *sql.DB
	tx      *sql.Tx
	stmt    *sql.Stmt
	tmpPath string
	target  string
	n       int
}

// NewBuilder creates an empty Open Food Facts database next to target.
//
// All inserts run inside one transaction against a prepared statement, the
// same reasoning as usda.NewBuilder: autocommit per row would turn a routine
// import into extended disk churn over hundreds of thousands of rows.
func NewBuilder(target string) (*Builder, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, fmt.Errorf("create off dir: %w", err)
	}
	tmp := target + ".building"
	_ = os.Remove(tmp)
	db, err := sql.Open("sqlite3", tmp)
	if err != nil {
		return nil, fmt.Errorf("create off db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("create off schema (is the sqlite_fts5 build tag set?): %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("begin off import: %w", err)
	}
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO off_foods
		  (code, product_name, brands, calories, protein, carbs, fat, sugar, sodium, fiber)
		VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		db.Close()    //nolint:errcheck
		return nil, fmt.Errorf("prepare off insert: %w", err)
	}
	return &Builder{db: db, tx: tx, stmt: stmt, tmpPath: tmp, target: target}, nil
}

// Add inserts one product into the open import transaction.
func (b *Builder) Add(f Food) error {
	_, err := b.stmt.Exec(
		f.Code, f.ProductName, f.Brands,
		f.Profile.CaloriesPer100g, f.Profile.ProteinPer100g, f.Profile.CarbsPer100g,
		f.Profile.FatPer100g, f.Profile.SugarPer100g, f.Profile.SodiumPer100g,
		f.Profile.DietaryFiberPer100g)
	if err != nil {
		return fmt.Errorf("insert off food %s: %w", f.Code, err)
	}
	b.n++
	return nil
}

// Promote rebuilds the FTS index, validates the row count, and atomically
// replaces the target database. Below MinExpectedRows it refuses and
// discards the build, leaving any previously imported database in service.
func (b *Builder) Promote() (int, error) {
	defer os.Remove(b.tmpPath) //nolint:errcheck

	b.stmt.Close() //nolint:errcheck
	if err := b.tx.Commit(); err != nil {
		b.db.Close() //nolint:errcheck
		return 0, fmt.Errorf("commit off import: %w", err)
	}
	if _, err := b.db.Exec(`INSERT INTO off_foods_fts(off_foods_fts) VALUES('rebuild')`); err != nil {
		b.db.Close() //nolint:errcheck
		return 0, fmt.Errorf("build fts index: %w", err)
	}
	var n int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM off_foods`).Scan(&n); err != nil {
		b.db.Close() //nolint:errcheck
		return 0, fmt.Errorf("count imported rows: %w", err)
	}
	if err := b.db.Close(); err != nil {
		return 0, fmt.Errorf("close built db: %w", err)
	}
	if n < MinExpectedRows {
		return n, fmt.Errorf("imported %d foods, below the %d minimum; keeping the existing database", n, MinExpectedRows)
	}
	if err := os.Rename(b.tmpPath, b.target); err != nil {
		return n, fmt.Errorf("promote off db: %w", err)
	}
	return n, nil
}

// Discard abandons a build without promoting it.
func (b *Builder) Discard() {
	b.stmt.Close()       //nolint:errcheck
	b.tx.Rollback()      //nolint:errcheck
	b.db.Close()         //nolint:errcheck
	os.Remove(b.tmpPath) //nolint:errcheck
}
