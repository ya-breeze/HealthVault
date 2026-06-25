## Why

Users export their health data from Android's Health Connect as a zip archive containing a SQLite database, but HealthVault has no way to ingest this format — only the real-time webhook is available. A bulk import endpoint lets users seed their full history in one step.

## What Changes

- Add `POST /api/import/health-connect` endpoint (JWT-authenticated, multipart file upload)
- Add `/api/import/` route prefix to group future import sources
- Add new `Speed` model and `speed` data type (point-in-time m/s samples extracted from Health Connect speed series)
- Add `pkg/hcimport` package to read and map the Health Connect SQLite schema to HealthVault's internal models
- Add an Import UI page with a file picker for the Health Connect zip

## Capabilities

### New Capabilities

- `health-connect-import`: Upload a Health Connect zip archive; parse the embedded SQLite database and ingest all supported health record types into HealthVault with deduplication
- `speed-data`: Store and query point-in-time speed samples (m/s), modelled identically to heart rate

### Modified Capabilities

_(none — no existing spec requirements change)_

## Impact

- **New route**: `POST /api/import/health-connect` under `RequireAuth` middleware
- **New model**: `Speed` in `pkg/database/models.go`; adds `speeds` table
- **New package**: `pkg/hcimport/` — reads HC SQLite, maps to `ingest.PayloadJSON`
- **Existing `ingest` package**: `payload.go` and `ingest.go` extended to handle `Speed`
- **Frontend**: new Import page with upload widget
- **Dependencies**: no new Go dependencies (stdlib `archive/zip`, `database/sql`, existing `gorm.io/driver/sqlite`)
