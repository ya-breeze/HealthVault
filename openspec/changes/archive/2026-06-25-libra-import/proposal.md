## Why

Users who track weight with the Libra Android app have years of historical data that cannot currently be imported into HealthVault. Adding a Libra CSV import endpoint lets users bring that history in without manual entry.

## What Changes

- New `POST /api/import/libra` endpoint accepting a Libra `.csv` export file
- New `pkg/libraImport` package that parses the semicolon-delimited CSV format, deduplicates to one record per calendar date (keeping the first occurrence), and returns a `PayloadJSON` with weight and body fat records
- Import UI extended with a second card on the existing `/import` page for Libra CSV upload
- Body fat percentage imported when the column is non-empty (maps to existing `BodyFat` model)

## Capabilities

### New Capabilities

- `libra-import`: Parse and ingest Libra CSV exports — weight (kg) and body fat (%) per day, deduplicated to first record per calendar date

### Modified Capabilities

- `import-ui`: Existing import page gains a second source card for Libra alongside the Health Connect card

## Impact

- New file: `backend/pkg/libraImport/reader.go` (+ `reader_test.go`)
- New file: `backend/pkg/server/handler_import_libra.go`
- Modified: `backend/pkg/server/server.go` (new route)
- Modified: `frontend/app/import/page.tsx` (second card)
- Modified: `frontend/lib/api.ts` (new `importLibra` function)
- No new database models or migrations required
- No new nginx config changes required (existing `/api/import/` location block covers it)
