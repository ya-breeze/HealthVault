## 1. Database & Models

- [x] 1.1 Add `FoodMeal`, `FoodItem`, `CustomFood` GORM models to `backend/pkg/database/models.go`, all embedding `models.TenantModel`; `FoodItem` carries `user_id`, `macro_source`, `preparation` and `state`; `FoodMeal` carries `clarify_log` and the 7 aggregate columns
- [x] 1.2 Add AutoMigrate entries in `backend/pkg/database/db.go`, including the `(user_id, name)` unique index on `CustomFood`
- [x] 1.3 Add USDA SQLite schema (separate DB file) and FTS5 candidate-retrieval queries in `backend/pkg/usda`
- [x] 1.4 Add config fields and env vars under the existing `HCW` viper prefix: `HCW_OPENAI_API_KEY`, `HCW_OPENAI_MODEL`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH`, `HCW_MAX_UPLOAD_BYTES`, `HCW_VISION_TIMEOUT`
- [x] 1.5 Unit tests: tenant field assignment (`ID`/`FamilyID` set explicitly), `FoodItem.user_id` matches its meal, two meals sharing one `logged_at` both persist, duplicate custom food name rejected

## 2. Media Storage & Upload Pipeline

- [x] 2.1 Implement server-generated path storage helper in `backend/pkg/storage` (client filename never used)
- [x] 2.2 Implement upload validation: max body size, content sniffing accepting only JPEG/PNG/WebP, plus an explicit ISO-BMFF brand check so HEIC is rejected with a 415 naming the format
- [x] 2.3 Implement owner-scoped photo handlers for `GET /api/food/meals/{id}/photo` and `GET /api/food/calibration-samples/{id}/photo`
- [x] 2.4 Implement photo-first persistence in `POST /api/food/meals`: save file, commit `FoodMeal` as `processing`, then analyze
- [x] 2.5 Tests: oversized upload rejected, non-image rejected, HEIC rejected with 415, traversal-shaped filename ignored, 401 unauthenticated, 404 cross-user photo access, 404 photo access by a family member

## 3. USDA Import & Candidate Search

- [x] 3.1 Implement `hcw import-usda`: download Foundation/SR Legacy, build FTS5 index into a temp file, validate minimum row count, atomic rename
- [x] 3.2 Implement candidate search: custom foods first (exact case-insensitive), then top-N FTS5 candidates, with the query built from name + preparation + state as ranking hints (never filters)
- [x] 3.3 Tests: failed import leaves prior DB serving, search before first import returns empty + error state, custom food precedence, cross-user custom food isolation

## 4. OpenAI Vision Integration

- [x] 4.1 Create vision client in `backend/pkg/vision` with structured output parsing, per-call model override, `store: false`, and returned model ID / token usage / latency. The item schema includes `preparation` and `state` (both may be unknown)
      — `OpenAIClient` (`pkg/vision/openai.go`) implements `Client` against OpenAI's Chat Completions API with `response_format: json_schema` (strict), sending `store: false` on every request. Recognize sends the photo as a base64 data URL; Clarify and Select are text-only (no image content, verified by `TestOpenAIClient_Clarify_SendsNoImageContent`). Returns the response's own `model` id, prompt/completion token counts, and latency. `preparation`/`state` "unknown" is mapped to `""`, the backend's own unknown convention. Wired into `server.go`: used when `HCW_OPENAI_API_KEY` is set, `Unconfigured` otherwise. Tested against a mock HTTP server (no live API calls in the test suite).
- [x] 4.2 Integrate synchronous analysis into `POST /api/food/meals` bounded by `HCW_VISION_TIMEOUT`; mark `failed` and retain the photo on error or timeout
- [x] 4.3 Implement candidate-shortlist selection: pass retrieved candidates to the model, record explicit non-match as `macro_source = none`
- [x] 4.4 Make every analysis run replace the meal's existing `FoodItem` rows in the same transaction as the status write
- [x] 4.5 Tests: analysis failure retains photo and sets `failed`, timeout path, unresolved item stores zeroed macros and is excluded from the aggregate, re-analysis leaves no leftover items, a wrong preparation guess still leaves the correct food in the shortlist
      — failure retains photo (`TestCreateMeal_RecognizeErrorMarksFailedAndRetainsPhoto`), timeout (`TestCreateMeal_TimeoutMarksFailed`), unresolved item zeroed (`TestCreateMeal_NoMatchLeavesItemUnresolved`), re-analysis replaces items (`TestRetryMeal_ReplacesItemsNotAppends`), wrong-preparation-keeps-correct-food (usda layer: `TestQueryFor_WrongPreparationDoesNotExcludeCorrectFood`).

## 5. Meal Lifecycle Endpoints

- [x] 5.1 Implement `GET /api/food/meals/{id}` (detail with items) and `POST /api/food/meals/{id}/retry`, accepting only `failed` or `processing` whose `updated_at` is older than `HCW_VISION_TIMEOUT`, 409 otherwise
- [x] 5.2 Implement `POST /api/food/meals/{id}/clarify`: text-only rounds that do not re-send the image, persisting each Q/A pair to `clarify_log` and replaying the full history each round, re-running food lookup when an answer resolves a previously unknown preparation or state, capped at 3 then `pending_review`
- [x] 5.3 Implement `PUT /api/food/meals/{id}/confirm`: recalculate scaled macros, aggregate items whose `macro_source` is `reference` or `manual`, allow correcting `logged_at`, set `confirmed` — no `Nutrition` write
- [x] 5.4 Implement `PATCH /api/food/meals/{id}/items/{item_id}` to bind a reference food, supply macros directly, or change weight; 409 once the meal is confirmed
- [x] 5.5 Implement `POST /api/food/meals/manual` for photo-free entry from food references or direct macro values, accepting an explicit `logged_at`
- [x] 5.6 Implement custom food CRUD (`POST|GET /api/food/custom`, `PUT|DELETE /api/food/custom/{id}`) and `GET /api/food/search`
- [x] 5.7 Tests: retry rejected while a call is in flight and accepted once stale, clarify request carries no image content and replays earlier answers, round cap, confirm writes zero `Nutrition` rows, manual-only meal aggregates non-zero, item patch rescales macros, 404 on cross-user custom food mutation, manual meal never enters `processing`
      — `TestRetryMeal_LiveProcessingRejectedWith409`, `TestRetryMeal_StaleProcessingIsAccepted`, `TestClarifyMeal_AnswerResolvesToReviewCarriesNoImage` (no-image is a structural guarantee: `vision.Client.Clarify` takes no image parameter), `TestClarifyMeal_MultipleRoundsUpToCapThenPendingReview` (round cap + full-history replay), `TestConfirmMeal_NoNutritionRowWritten`, `TestCreateManualMeal_DirectMacros`, `TestPatchMealItem_WeightOnlyChangeRescalesFromExistingBinding`, `TestCreateManualMeal_CrossUserCustomFoodReturns404`, `TestCreateManualMeal_NeverEntersProcessing`.
- [x] 5.8 Extend `PATCH .../items/{item_id}` to accept an optional `name`, applied independently of the manual/reference/weight branch (including a `name`-only patch with no other field, which must not hit the "nothing to update" 400) — corrects the displayed description without implying a macro change
      — found via real dogfood use: binding an item to a corrected food left the original vision-model guess on screen forever, with no way to fix it. A blank/whitespace-only name must not alone satisfy the update guard, caught in tier-1 review. `TestPatchMealItem_BindUpdatesNameWhenProvided`, `TestPatchMealItem_RebindAlreadyMatchedItemChangesNameAndMacros`, `TestPatchMealItem_NameAloneIsAcceptedAndDoesNotTouchMacros`, `TestPatchMealItem_BlankNameAloneReturns400` in `food_meal_lifecycle_test.go`.

## 6. Registry Integration & Deletion

- [x] 6.1 Register `food_meal` in `typeRegistry` (`backend/pkg/server/api.go`) anchored on `logged_at`; add to the frontend type list and MCP tool type list
- [x] 6.2 Add a per-type column allowlist to the registry and honour it in `QueryRecords`, which currently does an unprojected `Find` into `[]map[string]any`; `food_meal` exposes only `id`, `logged_at`, `name`, `status` and the 7 aggregate macros
- [x] 6.3 Add `logged_at` to the frontend time-column detection in `DataTypeClient.tsx`, which recognizes only `time`/`start_time`/`timestamp`
- [x] 6.4 Extend the delete path so `food_meal` cascades to `FoodItem` rows and removes the photo file
- [x] 6.5 Tests: `GET /api/data/food_meal` returns meals without nested items, response omits `photo_path` and `raw_response`, family member can read meal macros via `?user=` but gets 404 on the photo, delete cascades to items and photo, delete succeeds when the photo file is already missing, cross-user delete returns 404 and removes nothing
      — covered by `TestQueryRecords_FoodMealColumnAllowlist`, `TestDeleteRecordHandler_FoodMealCascadesItemsAndPhoto`, `TestDeleteRecordHandler_FoodMealMissingPhotoFileStillSucceeds`, `TestDeleteRecordHandler_FoodMealCrossUserReturns404AndRemovesNothing`, `TestMealPhoto_FamilyMemberReturns404`.

## 7. Frontend UI Components

- [x] 7.1 Photo upload and camera capture component with loading state and the external-model disclosure; file input declares `accept="image/jpeg,image/png,image/webp"` so iOS transcodes HEIC on selection, and camera capture encodes via `canvas.toBlob('image/jpeg')`
- [x] 7.2 Add the meal review page as a statically-exportable route reading the meal ID from a query parameter (`/food/review/?meal=<uuid>`) — `output: 'export'` means a `[id]` segment cannot be generated for runtime UUIDs
- [x] 7.3 Clarification questions modal
- [x] 7.4 Portion weight adjuster and itemized macro summary card, including the resolution UI for `macro_source = none` items backed by the item PATCH endpoint
- [x] 7.5 Custom food entry modal with edit and delete
- [x] 7.6 Manual (photo-free) meal entry form
- [x] 7.8 Make the item resolution UI reachable for any item, not just `macro_source = none` ones, so the owner can correct an already-matched item; item names wrap instead of single-line-truncating so the full vision-model guess is always readable; binding to a search result syncs the item's displayed name to the match; manual mode gains an editable name field
      — found via real dogfood use (screenshot showed truncated names and no way to fix a wrong match). `frontend/components/food/MealItemRow.tsx`, `frontend/components/food/ItemResolver.tsx`.
- [ ] 7.7 Calibration sample capture and management UI for photos, food identities, and measured gram weights
      — blocked on task 8.2 (calibration sample CRUD endpoints don't exist yet); not started.

## 8. Model Calibration

- [ ] 8.1 Add `FoodCalibrationSample` model, migration, tenant-scoped queries, and photo cleanup on delete
- [ ] 8.2 Add authenticated create/list/delete calibration sample endpoints that create no `FoodMeal` records
- [ ] 8.3 Implement `hcw calibrate-food-models` dataset selection, dry-run and explicitly confirmed execution modes, runtime model IDs, repeated trials, operator-supplied pricing
- [ ] 8.4 Implement deterministic ground-truth matching, detection/weight accuracy metrics, latency and cost aggregation, thresholds, Pareto-frontier selection, and reproducible JSON/Markdown reports
- [ ] 8.5 Tests: sample isolation, dry-run makes zero API calls, partial model failures do not abort the run, metric and price calculations, threshold selection, deterministic report metadata

## 9. Validation

- [x] 9.1 Run `make lint` and `make test`; fix all findings
      — clean throughout; also `cd frontend && npx tsc --noEmit` and a full `next build` static export, both clean.
- [x] 9.2 Add Playwright coverage in `e2e/tests` for the upload → review → confirm flow and the manual entry flow
      — `e2e/tests/food.spec.ts`: manual meal entry (confirmed status, macros shown), custom food CRUD via the UI, and photo upload → review (asserts a real terminal/actionable status is reached, not a specific recognition outcome, since the fixture photo is synthetic).
- [x] 9.3 Run the E2E suite against the deployed WIP stack with `BASE_URL` set, and record the result
      — hcw-wip repointed to this branch; ran `hcw import-usda` against the shared volume (7,793 foods) and restarted so the running server picked up the index. Full suite (27 tests, all specs) green against `http://192.168.1.54:8888`. The photo-upload path was exercised against the **real** OpenAI API (`gpt-5.6-luna`) end-to-end via direct curl too: Recognize correctly asked for clarification on the synthetic test image rather than hallucinating food ("abstract colored shapes"); a follow-up Clarify call correctly interpreted a free-text answer into 3 items; Select correctly bound all 3 to real USDA FDC IDs with correct scaled macros; Confirm correctly aggregated (501.5 kcal). hcw-wip is `class: dogfood` (single real account, no separate prod), corrected in `data.json` after this was flagged — every meal/custom-food created during testing was deleted afterward and verified against the live `hcw.db` (0 residual rows). Found and fixed a real bug along the way: `DeleteCustomFood` soft-deleted, permanently blocking name reuse (see the `food_custom.go` fix commit).
- [x] 9.4 Run `openspec validate --changes --strict`
      — passes.
