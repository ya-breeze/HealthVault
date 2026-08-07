// Package usda maintains a local SQLite copy of USDA FoodData Central core
// reference data (Foundation + SR Legacy) with an FTS5 index.
//
// The index is used for *candidate retrieval*, never for auto-assignment.
// SR Legacy descriptions read like "Chicken, broilers or fryers, breast, meat
// only, cooked, roasted" while a vision model emits "grilled chicken breast";
// BM25 across that gap is unreliable, and a confidently wrong match produces
// confidently wrong macros. Callers get a ranked shortlist and must choose.
//
// Requires the sqlite_fts5 build tag; without it FTS5 virtual tables do not exist.
package usda

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver

	"github.com/ya-breeze/healthvault/pkg/database"
)

// ErrNoDatabase means no import has run yet. Callers surface this as an empty
// candidate list plus a flag rather than failing the enclosing request.
var ErrNoDatabase = errors.New("USDA reference database not present")

// MinExpectedRows guards against a truncated or partial import being promoted
// over a good database. Foundation + SR Legacy is roughly 8k foods.
const MinExpectedRows = 1000

// DefaultCandidates is the shortlist size handed to the caller for selection.
//
// Deliberately large. BM25 penalizes long documents, and SR Legacy's canonical
// whole-food rows are long and heavily qualified ("Chicken, broilers or fryers,
// breast, meat only, cooked, roasted") while processed and deli rows are short
// ("Chicken breast tenders, breaded"). The short processed rows therefore
// outrank the whole foods for ordinary queries. Measured on the real 7,793-row
// dataset, the correct food ranked 12th for "chicken breast" and 17th for
// "white rice", so a five-item shortlist never contained the right answer.
const DefaultCandidates = 30

// Food is one USDA reference food with its per-100g profile.
type Food struct {
	FdcID       int64                    `json:"fdc_id"`
	Description string                   `json:"description"`
	DataType    string                   `json:"data_type"`
	Profile     database.NutrientProfile `json:"profile"`
}

// Index is a read handle on the local USDA database.
type Index struct {
	db   *sql.DB
	path string
}

// Open opens the USDA database at path. A missing file is reported as
// ErrNoDatabase so the caller can degrade rather than fail.
func Open(path string) (*Index, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNoDatabase
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=30000&mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open usda db: %w", err)
	}
	return &Index{db: db, path: path}, nil
}

// Close releases the database handle.
func (i *Index) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	if err := i.db.Close(); err != nil {
		return fmt.Errorf("close usda db: %w", err)
	}
	return nil
}

// Search returns up to limit ranked candidates for a free-text food name.
// An empty result is a normal outcome, not an error: it means "nothing matched",
// and the caller records the item as unresolved rather than guessing.
func (i *Index) Search(term string, limit int) ([]Food, error) {
	if i == nil || i.db == nil {
		return nil, ErrNoDatabase
	}
	if limit <= 0 {
		limit = DefaultCandidates
	}
	q := sanitizeFTSQuery(term)
	if q == "" {
		return nil, nil
	}
	rows, err := i.db.Query(`
		SELECT f.fdc_id, f.description, f.data_type,
		       f.calories, f.protein, f.carbs, f.fat, f.sugar, f.sodium, f.fiber
		FROM usda_foods_fts fts
		JOIN usda_foods f ON f.rowid = fts.rowid
		WHERE usda_foods_fts MATCH ?
		ORDER BY bm25(usda_foods_fts)
		LIMIT ?`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("usda search: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Food
	for rows.Next() {
		var f Food
		var p database.NutrientProfile
		if err := rows.Scan(&f.FdcID, &f.Description, &f.DataType,
			&p.CaloriesPer100g, &p.ProteinPer100g, &p.CarbsPer100g,
			&p.FatPer100g, &p.SugarPer100g, &p.SodiumPer100g, &p.DietaryFiberPer100g); err != nil {
			return nil, fmt.Errorf("scan usda row: %w", err)
		}
		f.Profile = p
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usda rows: %w", err)
	}
	return out, nil
}

// ByFdcID looks up a single food, used when binding an item to a chosen food.
func (i *Index) ByFdcID(id int64) (*Food, error) {
	if i == nil || i.db == nil {
		return nil, ErrNoDatabase
	}
	var f Food
	var p database.NutrientProfile
	err := i.db.QueryRow(`
		SELECT fdc_id, description, data_type, calories, protein, carbs, fat, sugar, sodium, fiber
		FROM usda_foods WHERE fdc_id = ?`, id).
		Scan(&f.FdcID, &f.Description, &f.DataType,
			&p.CaloriesPer100g, &p.ProteinPer100g, &p.CarbsPer100g,
			&p.FatPer100g, &p.SugarPer100g, &p.SodiumPer100g, &p.DietaryFiberPer100g)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usda lookup: %w", err)
	}
	f.Profile = p
	return &f, nil
}

// Count returns the number of indexed foods.
func (i *Index) Count() (int, error) {
	if i == nil || i.db == nil {
		return 0, ErrNoDatabase
	}
	var n int
	if err := i.db.QueryRow(`SELECT COUNT(*) FROM usda_foods`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count usda foods: %w", err)
	}
	return n, nil
}

const schema = `
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

// Builder writes a new USDA database to a temporary path. The caller promotes
// it with Promote once it validates, so a failed import can never leave the
// previous database broken or half-written.
type Builder struct {
	db      *sql.DB
	tx      *sql.Tx
	stmt    *sql.Stmt
	tmpPath string
	target  string
	n       int
}

// NewBuilder creates an empty USDA database next to target.
//
// All inserts run inside one transaction against a prepared statement. Left as
// autocommit, each of the ~8k rows would be its own transaction with its own
// fsync, turning a routine import into minutes of disk churn.
func NewBuilder(target string) (*Builder, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, fmt.Errorf("create usda dir: %w", err)
	}
	tmp := target + ".building"
	_ = os.Remove(tmp)
	db, err := sql.Open("sqlite3", tmp)
	if err != nil {
		return nil, fmt.Errorf("create usda db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("create usda schema (is the sqlite_fts5 build tag set?): %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("begin usda import: %w", err)
	}
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO usda_foods
		  (fdc_id, description, data_type, calories, protein, carbs, fat, sugar, sodium, fiber)
		VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		db.Close()    //nolint:errcheck
		return nil, fmt.Errorf("prepare usda insert: %w", err)
	}
	return &Builder{db: db, tx: tx, stmt: stmt, tmpPath: tmp, target: target}, nil
}

// Add inserts one food into the open import transaction.
func (b *Builder) Add(f Food) error {
	_, err := b.stmt.Exec(
		f.FdcID, f.Description, f.DataType,
		f.Profile.CaloriesPer100g, f.Profile.ProteinPer100g, f.Profile.CarbsPer100g,
		f.Profile.FatPer100g, f.Profile.SugarPer100g, f.Profile.SodiumPer100g,
		f.Profile.DietaryFiberPer100g)
	if err != nil {
		return fmt.Errorf("insert usda food %d: %w", f.FdcID, err)
	}
	b.n++
	return nil
}

// Promote rebuilds the FTS index, validates the row count, and atomically
// replaces the target database. Below MinExpectedRows it refuses and discards
// the build, leaving any previously imported database in service.
func (b *Builder) Promote() (int, error) {
	defer os.Remove(b.tmpPath) //nolint:errcheck

	b.stmt.Close() //nolint:errcheck
	if err := b.tx.Commit(); err != nil {
		b.db.Close() //nolint:errcheck
		return 0, fmt.Errorf("commit usda import: %w", err)
	}
	if _, err := b.db.Exec(`INSERT INTO usda_foods_fts(usda_foods_fts) VALUES('rebuild')`); err != nil {
		b.db.Close() //nolint:errcheck
		return 0, fmt.Errorf("build fts index: %w", err)
	}
	var n int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM usda_foods`).Scan(&n); err != nil {
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
		return n, fmt.Errorf("promote usda db: %w", err)
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
