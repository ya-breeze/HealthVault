## 1. Database & Models

- [ ] 1.1 Add `FoodMeal`, `FoodItem`, `CustomFood` GORM models to `backend/pkg/database/models.go`, all embedding `models.TenantModel`; `FoodItem` carries `user_id`, `macro_source`, `preparation` and `state`; `FoodMeal` carries `clarify_log` and the 7 aggregate columns
- [ ] 1.2 Add AutoMigrate entries in `backend/pkg/database/db.go`, including the `(user_id, name)` unique index on `CustomFood`
- [ ] 1.3 Add USDA SQLite schema (separate DB file) and FTS5 candidate-retrieval queries in `backend/pkg/usda`
- [ ] 1.4 Add config fields and env vars under the existing `HCW` viper prefix: `HCW_OPENAI_API_KEY`, `HCW_OPENAI_MODEL`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH`, `HCW_MAX_UPLOAD_BYTES`, `HCW_VISION_TIMEOUT`
- [ ] 1.5 Unit tests: tenant field assignment (`ID`/`FamilyID` set explicitly), `FoodItem.user_id` matches its meal, two meals sharing one `logged_at` both persist, duplicate custom food name rejected

## 2. Media Storage & Upload Pipeline

- [ ] 2.1 Implement server-generated path storage helper in `backend/pkg/storage` (client filename never used)
- [ ] 2.2 Implement upload validation: max body size, content sniffing accepting only JPEG/PNG/WebP, plus an explicit ISO-BMFF brand check so HEIC is rejected with a 415 naming the format
- [ ] 2.3 Implement owner-scoped photo handlers for `GET /api/food/meals/{id}/photo` and `GET /api/food/calibration-samples/{id}/photo`
- [ ] 2.4 Implement photo-first persistence in `POST /api/food/meals`: save file, commit `FoodMeal` as `processing`, then analyze
- [ ] 2.5 Tests: oversized upload rejected, non-image rejected, HEIC rejected with 415, traversal-shaped filename ignored, 401 unauthenticated, 404 cross-user photo access, 404 photo access by a family member

## 3. USDA Import & Candidate Search

- [ ] 3.1 Implement `hcw import-usda`: download Foundation/SR Legacy, build FTS5 index into a temp file, validate minimum row count, atomic rename
- [ ] 3.2 Implement candidate search: custom foods first (exact case-insensitive), then top-N FTS5 candidates, with the query built from name + preparation + state as ranking hints (never filters)
- [ ] 3.3 Tests: failed import leaves prior DB serving, search before first import returns empty + error state, custom food precedence, cross-user custom food isolation

## 4. OpenAI Vision Integration

- [ ] 4.1 Create vision client in `backend/pkg/vision` with structured output parsing, per-call model override, `store: false`, and returned model ID / token usage / latency. The item schema includes `preparation` and `state` (both may be unknown)
- [ ] 4.2 Integrate synchronous analysis into `POST /api/food/meals` bounded by `HCW_VISION_TIMEOUT`; mark `failed` and retain the photo on error or timeout
- [ ] 4.3 Implement candidate-shortlist selection: pass retrieved candidates to the model, record explicit non-match as `macro_source = none`
- [ ] 4.4 Make every analysis run replace the meal's existing `FoodItem` rows in the same transaction as the status write
- [ ] 4.5 Tests: analysis failure retains photo and sets `failed`, timeout path, unresolved item stores zeroed macros and is excluded from the aggregate, re-analysis leaves no leftover items, a wrong preparation guess still leaves the correct food in the shortlist

## 5. Meal Lifecycle Endpoints

- [ ] 5.1 Implement `GET /api/food/meals/{id}` (detail with items) and `POST /api/food/meals/{id}/retry`, accepting only `failed` or `processing` whose `updated_at` is older than `HCW_VISION_TIMEOUT`, 409 otherwise
- [ ] 5.2 Implement `POST /api/food/meals/{id}/clarify`: text-only rounds that do not re-send the image, persisting each Q/A pair to `clarify_log` and replaying the full history each round, re-running food lookup when an answer resolves a previously unknown preparation or state, capped at 3 then `pending_review`
- [ ] 5.3 Implement `PUT /api/food/meals/{id}/confirm`: recalculate scaled macros, aggregate items whose `macro_source` is `reference` or `manual`, allow correcting `logged_at`, set `confirmed` — no `Nutrition` write
- [ ] 5.4 Implement `PATCH /api/food/meals/{id}/items/{item_id}` to bind a reference food, supply macros directly, or change weight; 409 once the meal is confirmed
- [ ] 5.5 Implement `POST /api/food/meals/manual` for photo-free entry from food references or direct macro values, accepting an explicit `logged_at`
- [ ] 5.6 Implement custom food CRUD (`POST|GET /api/food/custom`, `PUT|DELETE /api/food/custom/{id}`) and `GET /api/food/search`
- [ ] 5.7 Tests: retry rejected while a call is in flight and accepted once stale, clarify request carries no image content and replays earlier answers, round cap, confirm writes zero `Nutrition` rows, manual-only meal aggregates non-zero, item patch rescales macros, 404 on cross-user custom food mutation, manual meal never enters `processing`

## 6. Registry Integration & Deletion

- [ ] 6.1 Register `food_meal` in `typeRegistry` (`backend/pkg/server/api.go`) anchored on `logged_at`; add to the frontend type list and MCP tool type list
- [ ] 6.2 Add a per-type column allowlist to the registry and honour it in `QueryRecords`, which currently does an unprojected `Find` into `[]map[string]any`; `food_meal` exposes only `id`, `logged_at`, `name`, `status` and the 7 aggregate macros
- [ ] 6.3 Add `logged_at` to the frontend time-column detection in `DataTypeClient.tsx`, which recognizes only `time`/`start_time`/`timestamp`
- [ ] 6.4 Extend the delete path so `food_meal` cascades to `FoodItem` rows and removes the photo file
- [ ] 6.5 Tests: `GET /api/data/food_meal` returns meals without nested items, response omits `photo_path` and `raw_response`, family member can read meal macros via `?user=` but gets 404 on the photo, delete cascades to items and photo, delete succeeds when the photo file is already missing, cross-user delete returns 404 and removes nothing

## 7. Frontend UI Components

- [ ] 7.1 Photo upload and camera capture component with loading state and the external-model disclosure; file input declares `accept="image/jpeg,image/png,image/webp"` so iOS transcodes HEIC on selection, and camera capture encodes via `canvas.toBlob('image/jpeg')`
- [ ] 7.2 Add the meal review page as a statically-exportable route reading the meal ID from a query parameter (`/food/review/?meal=<uuid>`) — `output: 'export'` means a `[id]` segment cannot be generated for runtime UUIDs
- [ ] 7.3 Clarification questions modal
- [ ] 7.4 Portion weight adjuster and itemized macro summary card, including the resolution UI for `macro_source = none` items backed by the item PATCH endpoint
- [ ] 7.5 Custom food entry modal with edit and delete
- [ ] 7.6 Manual (photo-free) meal entry form
- [ ] 7.7 Calibration sample capture and management UI for photos, food identities, and measured gram weights

## 8. Model Calibration

- [ ] 8.1 Add `FoodCalibrationSample` model, migration, tenant-scoped queries, and photo cleanup on delete
- [ ] 8.2 Add authenticated create/list/delete calibration sample endpoints that create no `FoodMeal` records
- [ ] 8.3 Implement `hcw calibrate-food-models` dataset selection, dry-run and explicitly confirmed execution modes, runtime model IDs, repeated trials, operator-supplied pricing
- [ ] 8.4 Implement deterministic ground-truth matching, detection/weight accuracy metrics, latency and cost aggregation, thresholds, Pareto-frontier selection, and reproducible JSON/Markdown reports
- [ ] 8.5 Tests: sample isolation, dry-run makes zero API calls, partial model failures do not abort the run, metric and price calculations, threshold selection, deterministic report metadata

## 9. Validation

- [ ] 9.1 Run `make lint` and `make test`; fix all findings
- [ ] 9.2 Add Playwright coverage in `e2e/tests` for the upload → review → confirm flow and the manual entry flow
- [ ] 9.3 Run the E2E suite against the deployed WIP stack with `BASE_URL` set, and record the result
- [ ] 9.4 Run `openspec validate --changes --strict`
