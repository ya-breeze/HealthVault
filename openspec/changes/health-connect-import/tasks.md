## 1. Feature branch

- [ ] 1.1 Create feature branch `feature/health-connect-import` from `main` in HealthVault repo

## 2. Speed model and data type

- [ ] 2.1 Add `Speed` struct to `pkg/database/models.go` with fields: `UserID`, `SourcePayloadID`, `Time time.Time`, `MetersPerSecond float64`; unique index on `(user_id, time)`
- [ ] 2.2 Add `Speed` to `AutoMigrate` call in `pkg/database/db.go`
- [ ] 2.3 Add `SpeedJSON` struct to `pkg/ingest/payload.go` and append `Speed []SpeedJSON` to `PayloadJSON`
- [ ] 2.4 Add Speed upsert loop to `ingest.Process()` in `pkg/ingest/ingest.go` (upsert on `user_id`+`time`, identical pattern to `HeartRate`)
- [ ] 2.5 Register `"speed"` → `{"speeds", "time"}` in `typeRegistry` in `pkg/server/api.go`

## 3. HC import package

- [ ] 3.1 Create `pkg/hcimport/reader.go` with `Read(dbPath string) (*ingest.PayloadJSON, *Counts, error)` — opens the HC SQLite file read-only via `database/sql`, reads each supported table, converts units/enums, returns a populated `*PayloadJSON` plus per-type row counts
- [ ] 3.2 Implement HC → HealthVault field mappings in `reader.go`:
  - Heart rate: join `heart_rate_record_table` ↔ `heart_rate_record_series_table` on `row_id`/`parent_key`; `beats_per_minute` → `BPM`, `epoch_millis` → RFC3339 `Time`
  - Steps: `start_time`/`end_time` (ms→RFC3339), `count` → `Count`
  - Sleep sessions: `start_time`/`end_time` ms→RFC3339, duration computed; join `sleep_stages_table` on `row_id`/`parent_key`, map `stage_type` int to string name
  - Exercise: `start_time`/`end_time` ms→RFC3339, `exercise_type` int → string name
  - Distance: `start_time`/`end_time` ms→RFC3339, `distance` (metres) → `Meters`
  - Total calories: `start_time`/`end_time` ms→RFC3339, `energy` joules → kcal (`÷ 4184.0`)
  - Oxygen saturation: `time` ms→RFC3339, `percentage` → `Percentage`
  - Speed: join `SpeedRecordTable` ↔ `speed_record_table` on `row_id`/`parent_key`; `speed` (m/s) → `MetersPerSecond`, `epoch_millis` → RFC3339 `Time`
- [ ] 3.3 Define static enum maps in `pkg/hcimport/enums.go`: sleep stage int→string (1→`"awake_in_bed"`, 4→`"out_of_bed"`, 5→`"light"`, 6→`"deep"`, 7→`"rem"`, etc.), exercise type int→string (4→`"biking"`, 33→`"running"`, 53→`"walking"`, 58→`"yoga"`, etc.); unknown values → `"unknown"`
- [ ] 3.4 Define `Counts` struct in `pkg/hcimport/reader.go` with one int field per imported type (used to build the JSON response)

## 4. Import HTTP handler

- [ ] 4.1 Create `pkg/server/handler_import_hc.go` with `importHealthConnectHandler(storage)`:
  - Parse multipart form (32 MB limit)
  - Read `file` field; return 400 if missing
  - Extract `health_connect_export.db` from the zip into a temp file (defer removal)
  - Return 422 if the entry is not found in the zip
  - Call `hcimport.Read(tempPath)` to get `*PayloadJSON` and `*Counts`
  - Call `ingest.Process(storage.DB(), userID, familyID, uuid.New(), payload)`
  - Respond 200 with JSON counts object
- [ ] 4.2 Register the route in `pkg/server/server.go` under the `api` (authenticated) subrouter: `api.HandleFunc("/import/health-connect", importHealthConnectHandler(storage)).Methods("POST")`

## 5. Build and static checks

- [ ] 5.1 Run `make build` (or `go build ./...`) in `backend/`; fix any compilation errors
- [ ] 5.2 Run `make lint` (or `go vet ./...`) in `backend/`; fix any warnings

## 6. Frontend — Import page

- [ ] 6.1 Read `node_modules/next/dist/docs/` (per AGENTS.md) for any version-specific conventions before writing frontend code
- [ ] 6.2 Add `importHealthConnect(file: File): Promise<Record<string, number>>` to `lib/api.ts` — sends multipart POST to `/import/health-connect`, returns parsed counts JSON
- [ ] 6.3 Create `app/import/page.tsx` — client component with: file input (`accept=".zip"`), import button (disabled while loading), result table showing per-type counts, error display
- [ ] 6.4 Add `"speed"` to `DATA_TYPES` array in `lib/api.ts`
- [ ] 6.5 Add an "Import" nav link to the main navigation in `app/page.tsx` (or layout, wherever nav lives)

## 7. Deploy and verify

- [ ] 7.1 Commit all changes on the feature branch and push to GitHub
- [ ] 7.2 Redeploy `healthvault-wip` stack from the feature branch via Portainer
- [ ] 7.3 Wait for the stack to come up (`portainer.py wait healthvault-wip`)
- [ ] 7.4 Log in to the WIP UI and navigate to the Import page
- [ ] 7.5 Upload the Health Connect zip and verify import counts match expected values (heart_rate ≈ 271942, steps ≈ 5392, sleep ≈ 318, etc.)
- [ ] 7.6 Confirm `GET /api/data/speed` returns speed records for the importing user
- [ ] 7.7 Re-upload the same zip and confirm record counts in DB are unchanged (idempotency)
