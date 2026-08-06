## Context

HealthVault uses Go with GORM (SQLite) on the backend and Next.js (TypeScript) on the frontend. See proposal.md for overall motivation and capabilities.

Two properties of the existing codebase shaped this design:

- **There is no background job infrastructure.** `backend/pkg/server/server.go` starts exactly one goroutine (`ListenAndServe`). There is no scheduler, worker pool, or job table to build on.
- **`Nutrition` is telemetry, not user-authored data.** `backend/pkg/database/models.go` gives every health metric row a non-null `source_payload_id`, and `openspec/specs/data-model/spec.md` states that rule normatively. Food meals have no payload lineage, so they are not written into that table (see proposal Non-Goals).

## Goals / Non-Goals

**Goals:**
- Implement a photo-first upload pipeline where raw images are stored locally and `FoodMeal` database records are created and committed prior to calling OpenAI Vision.
- Embed USDA FoodData Central (Foundation + SR Legacy) in a local SQLite database with an FTS5 index used for *candidate retrieval*.
- Support custom user foods with high-priority matching over standard USDA entries.
- Calculate 7 key macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) scaled from per-100g profiles.
- Expose confirmed meals through the existing `/api/data/{type}` registry as `food_meal`.
- Capture locally stored, weighed-food ground truth and compare explicitly selected vision models for accuracy, latency, and cost through a manual CLI.

**Non-Goals:**
- Writing to or modifying the `Nutrition` table.
- Introducing a background scheduler, worker, or job queue.
- Full 150+ micronutrient tracking UI (stored in details JSON, but not exposed in primary UI for now).
- Barcode scanning.

## Key Design Decisions

### 1. Synchronous Vision Call (no background worker)

The upload handler saves the photo, creates the `FoodMeal` row with status `processing`, commits, and *then* calls OpenAI Vision **within the same HTTP request**, updating the row to `pending_clarification` or `pending_review` before responding.

Rationale: vision latency is typically 3–10s, which is acceptable for a single-user application, and this avoids introducing the first background goroutine in the codebase. An async design would need a worker, a polling endpoint, *and* a startup reconciliation pass — because a process restart mid-analysis would otherwise strand a meal in `processing` forever.

The `processing` status is still persisted, because the photo-first commit happens before the call. A meal found in `processing` after a restart is a crash remnant and is recoverable through the same retry endpoint that serves `failed` meals — no separate reconciliation job is needed.

**Trade-off:** the client holds an open request for the duration of the vision call. The upload endpoint documents a server-side timeout (`HCW_VISION_TIMEOUT`, default 60s); on timeout the meal is marked `failed` with the photo retained.

### 2. Photo Storage & Media Access

- **Storage Location**: Local directory specified by `HCW_UPLOADS_DIR` (defaults to `./data/uploads`).
- **File Naming**: `{user_id}/{owner_kind}/{owner_id}.{ext}`, where `owner_kind` is `meal` or `calibration` and `{ext}` is derived from the **sniffed** content type (`jpg`, `png`, `heic`, `webp`) — not from the client-supplied filename or `Content-Type` header. The path is **entirely server-generated**, so path traversal is not a concern.
- **Media Endpoints**: Photos are addressed through their owning resource rather than a standalone photo ID, because no model carries a photo identifier — only a `PhotoPath`:
  - `GET /api/food/meals/{id}/photo`
  - `GET /api/food/calibration-samples/{id}/photo`

  Both are behind the existing JWT middleware and resolve the owning row scoped to the authenticated user before touching the filesystem.

### 3. GORM Models & Schemas

All four tables embed `models.TenantModel` (from `kin-core`), matching every other table in `backend/pkg/database/models.go`. Note that `TenantModel` has **no `BeforeCreate` hook** — `ID` and `FamilyID` must be assigned explicitly on every `Create()`, exactly as `backend/pkg/ingest/ingest.go` does.

```go
type FoodMeal struct {
    models.TenantModel
    UserID       uuid.UUID  `gorm:"type:uuid;not null;index"`
    PhotoPath    string     `gorm:"type:text"` // empty for manual, photo-free entries
    Status       string     `gorm:"type:varchar(32);not null;default:'processing'"`
    LoggedAt     time.Time  `gorm:"not null;index"`
    Name         string     `gorm:"type:text"`
    RawResponse  string     `gorm:"type:text"` // last structured response from OpenAI Vision
    ClarifyRound int        `gorm:"not null;default:0"`

    // Aggregate of the meal's matched items, written on confirmation.
    Calories          float64 `gorm:"not null;default:0"`
    ProteinGrams      float64 `gorm:"not null;default:0"`
    CarbsGrams        float64 `gorm:"not null;default:0"`
    FatGrams          float64 `gorm:"not null;default:0"`
    SugarGrams        float64 `gorm:"not null;default:0"`
    SodiumGrams       float64 `gorm:"not null;default:0"`
    DietaryFiberGrams float64 `gorm:"not null;default:0"`

    Items []FoodItem `gorm:"foreignKey:MealID"`
}
// Status: processing | pending_clarification | pending_review | confirmed | failed

type FoodItem struct {
    models.TenantModel
    MealID            uuid.UUID  `gorm:"type:uuid;not null;index"`
    Name              string     `gorm:"not null"`
    FdcID             *int64     `gorm:"index"`
    CustomFoodID      *uuid.UUID `gorm:"type:uuid;index"`
    Matched           bool       `gorm:"not null;default:false"`
    WeightGrams       float64    `gorm:"not null"`
    Confidence        float64    `gorm:"not null"`
    Calories          float64    `gorm:"not null"`
    ProteinGrams      float64    `gorm:"not null"`
    CarbsGrams        float64    `gorm:"not null"`
    FatGrams          float64    `gorm:"not null"`
    SugarGrams        float64    `gorm:"not null"`
    SodiumGrams       float64    `gorm:"not null"`
    DietaryFiberGrams float64    `gorm:"not null"`
}

type CustomFood struct {
    models.TenantModel
    UserID                uuid.UUID `gorm:"type:uuid;not null;index"`
    Name                  string    `gorm:"not null;index"`
    CaloriesPer100g       float64   `gorm:"not null"`
    ProteinPer100g        float64   `gorm:"not null"`
    CarbsPer100g          float64   `gorm:"not null"`
    FatPer100g            float64   `gorm:"not null"`
    SugarPer100g          float64   `gorm:"not null"`
    SodiumPer100g         float64   `gorm:"not null"`
    DietaryFiberPer100g   float64   `gorm:"not null"`
}

type FoodCalibrationSample struct {
    models.TenantModel
    UserID      uuid.UUID `gorm:"type:uuid;not null;index"`
    PhotoPath   string    `gorm:"type:text;not null"`
    GroundTruth string    `gorm:"type:text;not null"` // JSON: canonical names, aliases, optional reference IDs, measured grams
    CapturedAt  time.Time `gorm:"not null"`
}
```

Nutrient field names follow the existing `Nutrition` model (`DietaryFiberGrams`, `SodiumGrams`) so the two are directly comparable. `SodiumGrams` is grams, not milligrams, matching the existing column.

### 4. USDA Storage and Candidate Matching

- **Separate database file** at `HCW_USDA_DB_PATH` (default `./data/usda.db`), holding `usda_foods` and the `usda_foods_fts` FTS5 virtual table. Keeping reference data out of `hcw.db` means an import can rebuild it by atomic file rename without touching user data.
- **Import is a one-shot CLI command**, `hcw import-usda`, not a scheduled job. SR Legacy is frozen (final release April 2018) and Foundation Foods publishes roughly twice a year, so a monthly background poll would be machinery for an event that happens 0–2 times a year — and would be the first background job in the codebase. The command downloads to a temporary file, builds the index, validates a minimum row count, and only then renames into place, so a failed import leaves the previous database serving.

- **Matching is candidate retrieval, not auto-assignment.** This is the step most likely to produce silently wrong macros: SR Legacy descriptions read like `Chicken, broilers or fryers, breast, meat only, cooked, roasted`, while the vision model emits `grilled chicken breast`, and BM25 across that gap is unreliable. A confidently wrong match is worse than no match. So:
  1. Query `custom_foods` for the user's own entries first; an exact (case-insensitive) name hit wins outright.
  2. Otherwise query `usda_foods_fts` for the top N (default 5) ranked candidates.
  3. The vision model is given the shortlist and selects, or returns "none of these".
  4. If nothing is selected, the item is stored with `Matched = false` and zeroed macros, and the UI prompts the user to pick a food or enter macros manually. Items are never silently matched to a low-scoring candidate.

### 5. Clarification Rounds

Clarification is **text-only**. Round 1 sends the image; subsequent rounds send the stored structured result plus the question/answer pairs, and **do not re-send the image**. Re-uploading the photo each round would make image tokens the dominant cost of the feature for no additional information.

Rounds are bounded by `ClarifyRound` (max 3). On exceeding the bound the meal moves to `pending_review` and the user completes it manually.

### 6. API Endpoints

- `POST /api/food/meals` — multipart photo upload; creates the meal, runs analysis, returns the meal with items.
- `POST /api/food/meals/manual` — create a meal with no photo, from user-supplied items.
- `GET /api/food/meals/{id}` — meal with its items (used by the review UI).
- `GET /api/food/meals/{id}/photo` — stream the stored photo, owner-scoped.
- `POST /api/food/meals/{id}/retry` — re-run analysis on the stored photo for a meal in `failed` or `processing`.
- `POST /api/food/meals/{id}/clarify` — submit clarification answers.
- `PUT /api/food/meals/{id}/confirm` — finalize items and weights, set status `confirmed`.
- `POST /api/food/custom`, `GET /api/food/custom` — custom food create/list.
- `GET /api/food/search` — search custom + USDA foods.
- `POST|GET /api/food/calibration-samples`, `DELETE /api/food/calibration-samples/{id}`, `GET /api/food/calibration-samples/{id}/photo`.

Meals are additionally exposed read-only through the existing generic registry as `GET /api/data/food_meal` and deleted through `DELETE /api/data/food_meal/{id}`. The generic list handler returns meal rows without their items; the dedicated `GET /api/food/meals/{id}` is the detail view.

### 7. Upload Validation

The upload endpoint enforces a maximum body size (`HCW_MAX_UPLOAD_BYTES`, default 10 MiB) and validates the content by sniffing the decoded image header, accepting only JPEG, PNG, HEIC and WebP. The declared `Content-Type` and the client filename are both untrusted and neither determines the stored path or type.

### 8. Third-Party Disclosure and Retention

Meal photos are sent to OpenAI. The production vision client sets `store: false` on every request, matching the calibration client, and the upload UI states that the photo will be sent to an external model. Photos are retained until their owning meal or calibration sample is deleted; deleting either removes the row and its file.

### 9. Manual Model Calibration CLI

- **Invocation**: Add an operator-run `hcw calibrate-food-models` command. It reads calibration samples from the configured HealthVault database and uploads directory. The operator selects a dataset with required `--user <username>` scope and optional `--sample-ids`; there is no scheduler. Concurrent access alongside a running server is safe — `backend/pkg/database/db.go` already opens SQLite with `_journal_mode=WAL&_busy_timeout=30000`, and the command only reads.
- **Runtime Model Selection**: Candidate model IDs are passed with `--models` so newly available image-capable models can be evaluated without a code release. The production model is included only when explicitly named.
- **Comparable Requests**: Every candidate receives the same stored image, prompt version, structured output schema, image detail, and supported inference settings through the production vision client. The client sets `store: false` and records the model identifier returned by the API.
- **Repeated Trials**: `--runs-per-sample` defaults to 3 and can be reduced to 1 for a cheaper exploratory run. A failed or unsupported model call is recorded and does not abort other candidates.
- **Current Pricing Input**: `--pricing <json>` is required and maps each model to current input, cached-input, and output prices per million tokens. Prices are not compiled into HealthVault because they change independently of the application.
- **Cost Safety**: Without `--execute`, the command performs a dry run that lists samples, candidate models, trial count, and total planned API calls without sending images. An executing run requires explicit confirmation after that preview.
- **Metrics**: For each model, calculate structured-output success rate, food detection precision/recall/F1, matched-item weight mean absolute error and mean absolute percentage error, p50/p95 latency, token usage, and estimated cost per sample and for the full run.
- **Deterministic Matching**: Each ground-truth item stores a canonical food name, optional accepted aliases, and optional USDA/custom-food ID. A prediction matches when its normalized name matches an accepted name or both resolve to the same reference-food ID; one-to-one maximum-score matching prevents a prediction from satisfying multiple expected items. Weight errors are calculated only for matched items.
- **Selection**: The report shows the cost/quality Pareto frontier and, when operator-supplied minimum F1 and maximum weight-error thresholds are present, identifies the cheapest candidate that passes them. It does not change the configured production model.
- **Reproducibility**: Write JSON and Markdown reports containing the dataset hash and sample count, run timestamp, prompt/schema versions, requested and returned model IDs, inference settings, trial count, per-call results, usage, supplied prices, metrics, and threshold decision. A one-sample dataset is allowed but the report warns that it is not representative.
- **Privacy**: The execution confirmation states that every calibration photo will be sent once per planned trial to each selected external model. Calibration samples remain user/tenant scoped and are never added to meal history.

## Risks / Trade-offs

- **[Risk] Synchronous vision call holds an HTTP request for seconds** → **Mitigation**: bounded by `HCW_VISION_TIMEOUT`; the photo and meal row are already committed before the call, so a timeout costs the analysis, never the photo. Revisit only if latency becomes a real complaint.
- **[Risk] FTS5 fails to retrieve the right candidate for LLM-phrased food names** → **Mitigation**: retrieve top-N and let the model select rather than auto-assigning rank 1; surface unmatched items to the user instead of guessing. Match quality is measurable through the same calibration dataset.
- **[Risk] Meal totals and Health Connect nutrition data cannot be summed together** → **Accepted**: this is the deliberate consequence of not writing to `Nutrition`. Unifying them requires a source discriminator and is deferred to its own change.
- **[Risk] USDA import produces a broken or partial database** → **Mitigation**: build into a temporary file, validate a minimum row count, atomic rename; the previous database keeps serving until the new one is complete.
- **[Risk] Calibration overfits a small or unrepresentative sample set** → **Mitigation**: preserve per-sample results and dataset size in the report, warn for a single sample, and present thresholds plus a Pareto frontier rather than silently declaring an overall winner.
- **[Risk] Pricing becomes stale** → **Mitigation**: require an operator-supplied pricing file for every run and embed the exact supplied rates in the report.
