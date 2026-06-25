## 1. Feature branch

- [x] 1.1 Create feature branch `feature/libra-import` from `main` in the HealthVault repo

## 2. Libra import package

- [x] 2.1 Create `backend/pkg/libraImport/reader.go` with `Read(r io.Reader) (*ingest.PayloadJSON, *Counts, error)` — parse semicolon-delimited CSV, skip `#`-prefixed lines, deduplicate to first occurrence per UTC calendar date, import `weight` (kg) and `body fat` (%) when non-empty
- [x] 2.2 Validate `#Version:` header (must be `6`) and `#Units:` header (must be `kg`); return a typed error on mismatch so the handler can respond 422
- [x] 2.3 Define `Counts` struct in `reader.go` with `Weight int` and `BodyFat int` fields
- [x] 2.4 Write `backend/pkg/libraImport/reader_test.go` covering: happy-path parse, deduplication, body fat import, version mismatch 422 error, units mismatch 422 error

## 3. Import HTTP handler

- [x] 3.1 Create `backend/pkg/server/handler_import_libra.go` with `importLibraHandler(storage)`: parse multipart form, read `file` field (400 if missing), call `libraImport.Read()` (422 on parse error), look up user (404 if not found), wrap `ingest.Process()` in `storage.DB().Transaction()`, respond 200 with JSON counts
- [x] 3.2 Register route in `backend/pkg/server/server.go`: `api.HandleFunc("/import/libra", importLibraHandler(storage)).Methods("POST")`

## 4. Build and static checks

- [x] 4.1 Run `make build` (or `go build ./...`) in `backend/`; fix any compilation errors
- [x] 4.2 Run `go vet ./...` in `backend/`; fix any warnings

## 5. Frontend — Libra card on Import page

- [x] 5.1 Add `importLibra(file: File): Promise<Record<string, number>>` to `frontend/lib/api.ts` — multipart POST to `/api/import/libra`
- [x] 5.2 Add a Libra import card to `frontend/app/import/page.tsx` below the Health Connect card: file input (`.csv` only), Import button with loading/disabled state, result table, error display — each card has its own independent state

## 6. Deploy and verify

- [x] 6.1 Commit all changes on the feature branch and push to GitHub
- [x] 6.2 Redeploy `hcw-wip` stack from the feature branch via Portainer
- [x] 6.3 Wait for the stack to come up (`portainer.py wait hcw-wip`)
- [x] 6.4 Log in to the WIP UI, navigate to Import, and upload the Libra CSV; verify weight and body_fat counts are returned
- [x] 6.5 Re-upload the same file and confirm record counts in DB are unchanged (idempotency)

## 7. E2E tests

- [x] 7.1 Add Libra card visibility test to `e2e/tests/import.spec.ts`
- [x] 7.2 Add API-level test: POST to `/api/import/libra` with missing file returns 400
- [x] 7.3 Run full E2E suite against WIP stack; fix any failures
