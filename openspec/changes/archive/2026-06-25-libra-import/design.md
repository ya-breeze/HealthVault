## Context

HealthVault already has a `POST /api/import/health-connect` endpoint and a `pkg/hcimport` translator package that converts a Health Connect SQLite archive into `ingest.PayloadJSON`. The Libra import follows the same pattern: a source-specific translator package feeds the existing `ingest.Process()` function, which handles upserts and deduplication at the DB level.

The `Weight` and `BodyFat` database models already exist. No schema migrations are needed.

## Goals / Non-Goals

**Goals:**
- Parse Libra's semicolon-delimited CSV export format
- Import `weight` (kg) and `body fat` (%) columns into existing models
- Deduplicate to one record per calendar date (first occurrence wins)
- Return per-type counts in the HTTP response
- Add a Libra card to the existing Import UI page

**Non-Goals:**
- Storing `weight trend` or `body fat trend` (app-computed, not raw measurements)
- Importing `muscle mass` (no dedicated model; data is absent in practice)
- Importing the `log` field
- Unit conversion (file header asserts `kg`; other units not handled)
- Merging or replacing existing weight records that already exist in HealthVault

## Decisions

### 1. New `pkg/libraImport` package (not inline in handler)

Same pattern as `pkg/hcimport`. Keeps the CSV parsing logic independently testable.

Alternatives considered: inline parsing in the handler — rejected because it makes unit testing harder without an HTTP server.

### 2. Deduplication at parse time (first occurrence per calendar date)

Libra exports two rows per day (same weight, timestamps ~1-2h apart — likely local vs UTC). We drop the second occurrence before calling `ingest.Process()` rather than relying on the DB unique index to reject it. This gives an accurate count in the response and avoids unnecessary DB round-trips.

Key: "calendar date" is determined by truncating the timestamp's UTC date (i.e. `time.UTC().Truncate(24*time.Hour)` equivalent — compare `YYYY-MM-DD` string prefix).

### 3. Accept plain `.csv` upload (not zip)

Libra exports a single CSV file directly, unlike Health Connect which exports a zip. The handler reads the multipart file directly with no extraction step.

### 4. Wrap ingest in a single transaction (same as HC import)

`ingest.Process()` is called inside `storage.DB().Transaction()` so the entire import is atomic.

### 5. Body fat imported when non-empty

If `body_fat` column is non-empty for a row, a `BodyFat` record is written at the same timestamp as the weight record. The response counts both independently (`{"weight": N, "body_fat": M}`).

## Risks / Trade-offs

- **CSV format version changes** → Mitigation: check the `#Version:` header and return 422 if version ≠ 6.
- **Non-kg units** → Mitigation: check the `#Units:` header and return 422 if units ≠ `kg`. Future work can add conversion.
- **Large files** → existing nginx 64 MB limit from the `/api/import/` location block is sufficient for any realistic Libra export.
