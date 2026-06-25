## Context

HealthVault currently ingests data only via a real-time webhook (`POST /webhook/{username}`). Android's Health Connect app can export its full local database as a zip archive containing a single SQLite file (`health_connect_export.db`). This file uses Android's internal Health Connect schema with epoch-millisecond timestamps, integer-coded enums, and energy in joules — all of which differ from HealthVault's internal representations.

The existing `ingest.Process()` function accepts a `*PayloadJSON` struct and handles all upsert logic. The goal is to translate the HC SQLite schema into that struct so all storage logic is reused without modification.

## Goals / Non-Goals

**Goals:**
- Accept a zip upload via `POST /api/import/health-connect` (JWT auth, multipart)
- Extract the HC SQLite database from the zip, read all supported tables, convert units/enums, and call `ingest.Process()` with the result
- Support all non-empty HC tables: heart rate (series), steps, sleep+stages, exercise, distance, total calories, oxygen saturation, speed (series)
- Add `Speed` model (point-in-time m/s samples) to HealthVault
- Return per-type import counts in the JSON response
- Add an Import UI page with a file picker
- Structure routes under `/api/import/` for future importers

**Non-Goals:**
- Streaming/resumable uploads — the archive is ~15 MB, single request is fine
- Importing HC tables with zero rows in the reference export (blood pressure, blood glucose, etc.) — they are mapped by the same code path but produce no output
- Real-time progress events (WebSocket/SSE)
- Validating the zip against an HC version schema

## Decisions

### 1. New `pkg/hcimport` package, not extending `pkg/ingest`

The HC reader is a translation layer: it opens a different SQLite file and maps a foreign schema. Mixing this into `pkg/ingest` would couple the HC-specific enum tables and unit conversions to the generic payload processor. A separate package keeps the boundary clean and makes adding future importers (Garmin, Apple Health) easy — each gets its own translator that outputs a `*PayloadJSON`.

### 2. Reuse `ingest.Process()` unchanged

The existing upsert/deduplication logic in `ingest.Process()` is correct and tested. The HC reader outputs the same `*PayloadJSON` the webhook handler uses, so zero storage logic is duplicated.

### 3. Extract zip to temp file, then open with GORM sqlite driver

The HC database is opened read-only via `database/sql` (stdlib) with the same `mattn/go-sqlite3` driver already in the module. No new dependency needed. The temp file is removed in a `defer` immediately after extraction.

### 4. Speed modelled as point-in-time (like HeartRate), not interval

Speed is recorded as a timestamped sample series (`speed_record_table` → child `speed_record_table` series), identical in shape to heart rate. Storing it as individual point-in-time rows (time + m/s) is consistent with the existing pattern and queryable with the generic `dataHandler`.

### 5. Energy conversion: joules → kcal

The HC `total_calories_burned_record_table.energy` column is in joules. HealthVault stores calories in kcal. Conversion: `kcal = joules / 4184.0`.

### 6. Sleep stage and exercise type enum maps

HC uses integer codes. HealthVault uses string names. The mapping is a static Go map defined in `pkg/hcimport`. Unknown codes are mapped to `"unknown"` so imports never fail on future HC versions adding new enum values.

Sleep stage codes (observed: 1, 4, 5, 6):
- 1 → `"awake_in_bed"`, 2 → `"sleeping"`, 3 → `"awake"`, 4 → `"out_of_bed"`, 5 → `"light"`, 6 → `"deep"`, 7 → `"rem"`

Exercise type codes (observed: 4, 33, 53, 58):
- 4 → `"biking"`, 33 → `"running"`, 53 → `"walking"`, 58 → `"yoga"` (and ~80 others mapped)

### 7. `/api/import/` route prefix

Grouping under `/api/import/` makes the router self-documenting and avoids future naming collisions (e.g., `/api/import/apple-health`, `/api/import/garmin`).

## Risks / Trade-offs

- **Temp disk usage during import**: zip is extracted to the scratchpad temp dir. For a 15 MB DB this is negligible; very large archives could be a concern. → Mitigation: no action now; add size limit header check if this becomes an issue.
- **HC schema changes**: Android may add/remove columns in future HC versions. → Mitigation: queries use explicit column names; missing columns produce zero rows, not panics.
- **Heart rate series volume**: 271K series rows imported in one transaction may be slow. → Mitigation: `ingest.Process()` already batch-inserts via GORM; acceptable for a one-time bulk import.
