## Context

HealthVault uses Go with GORM (SQLite) on the backend and Next.js (TypeScript) on the frontend. See proposal.md for overall motivation and capabilities.

## Goals / Non-Goals

**Goals:**
- Implement a photo-first upload pipeline where raw images are stored locally and `FoodMeal` database records are created prior to calling OpenAI Vision.
- Embed USDA FoodData Central (Foundation + SR Legacy) in a local SQLite database table with an FTS5 full-text index for fast ingredient matching.
- Support custom user foods with high-priority matching over standard USDA entries.
- Calculate 7 key macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) scaled per 100g.
- Synchronize confirmed meal totals directly into HealthVault's existing main [`Nutrition`](file:///Users/ek/work/HealthVault/backend/pkg/database/models.go#L258-L273) table.
- Capture locally stored, weighed-food ground truth and compare explicitly selected vision models for accuracy, latency, and cost through a manual CLI.

**Non-Goals:**
- Full 150+ micronutrient tracking UI (stored in details JSON, but not exposed in primary UI for now).
- Barcode scanning (reserved for a future phase).
- Scheduled calibration runs or automatic changes to the production `OPENAI_MODEL` setting.

## Key Design Decisions

### 1. Photo Storage & Media Handler
- **Storage Location**: Local directory specified by `HEALTHVAULT_UPLOADS_DIR` (defaults to `./data/uploads`).
- **File Naming & Path**: `{user_id}/{meal_id}_{timestamp}.jpg`.
- **Media Endpoint**: `GET /api/media/{photo_id}` protected by HealthVault JWT authentication middleware. Ensures User A cannot access User B's uploaded food photos.

### 2. GORM Models & Schemas

```go
type FoodMeal struct {
    models.TenantModel
    UserID      uuid.UUID  `gorm:"type:uuid;not null;index"`
    PhotoPath   string     `gorm:"type:text"`
    Status      string     `gorm:"type:varchar(32);not null;default:'processing'"` // processing, pending_clarification, confirmed, failed
    LoggedAt    time.Time  `gorm:"not null"`
    RawResponse string     `gorm:"type:text"` // raw JSON from OpenAI Vision
    Items       []FoodItem `gorm:"foreignKey:MealID"`
}

type FoodItem struct {
    ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
    MealID                uuid.UUID `gorm:"type:uuid;not null;index"`
    Name                  string    `gorm:"not null"`
    FdcID                 *int64    `gorm:"index"`
    CustomFoodID          *uuid.UUID `gorm:"type:uuid;index"`
    WeightGrams           float64   `gorm:"not null"`
    Confidence            float64   `gorm:"not null"`
    Calories              float64   `gorm:"not null"`
    ProteinGrams          float64   `gorm:"not null"`
    CarbsGrams            float64   `gorm:"not null"`
    FatGrams              float64   `gorm:"not null"`
    SugarGrams            float64   `gorm:"not null"`
    SodiumGrams           float64   `gorm:"not null"`
    DietaryFiberGrams     float64   `gorm:"not null"`
}

type CustomFood struct {
    models.TenantModel
    UserID            uuid.UUID `gorm:"type:uuid;not null;index"`
    Name              string    `gorm:"not null;index"`
    CaloriesPer100g   float64   `gorm:"not null"`
    ProteinPer100g    float64   `gorm:"not null"`
    CarbsPer100g      float64   `gorm:"not null"`
    FatPer100g        float64   `gorm:"not null"`
    SugarPer100g      float64   `gorm:"not null"`
    SodiumPer100g     float64   `gorm:"not null"`
    FiberPer100g      float64   `gorm:"not null"`
}

type FoodCalibrationSample struct {
    models.TenantModel
    UserID       uuid.UUID `gorm:"type:uuid;not null;index"`
    PhotoPath    string    `gorm:"type:text;not null"`
    GroundTruth  string    `gorm:"type:text;not null"` // JSON: food names/IDs and measured grams
    CapturedAt   time.Time `gorm:"not null"`
}
```

### 3. Local USDA SQLite & FTS5 Search Engine
- **Tables**: `usda_foods` and `usda_foods_fts` (Virtual FTS5 table).
- **Search Precedence**:
  1. Query `custom_foods` matching `user_id` and search term.
  2. Query `usda_foods_fts` for top matching Foundation / SR Legacy candidates.

### 4. API Endpoints
- `POST /api/food/meals` (Multipart upload photo -> create `FoodMeal` -> trigger Vision analysis).
- `GET /api/media/{photo_id}` (Stream saved photo with auth check).
- `POST /api/food/meals/{id}/clarify` (Submit answers to clarification questions -> update items & macros).
- `PUT /api/food/meals/{id}/confirm` (Update weights/items -> sync aggregate macros to main `Nutrition` table).
- `POST /api/food/custom` (Create custom user food).
- `GET /api/food/search` (Search custom & USDA food database).
- `POST /api/food/calibration-samples` (Save a photo plus food identities and measured gram weights; does not create a meal or `Nutrition` record).
- `GET /api/food/calibration-samples` (List calibration sample metadata owned by the authenticated user).
- `DELETE /api/food/calibration-samples/{id}` (Delete an owned sample and its photo).

### 5. Manual Model Calibration CLI

- **Invocation**: Add an operator-run `hcw calibrate-food-models` command. It reads calibration samples from the configured HealthVault database and uploads directory. The operator selects a dataset with required `--user <username>` scope and optional `--sample-ids`; there is no scheduler.
- **Runtime Model Selection**: Candidate model IDs are passed with `--models` so newly available image-capable models can be evaluated without a code release. The production model is included only when explicitly named.
- **Comparable Requests**: Every candidate receives the same stored image, prompt version, structured output schema, image detail, and supported inference settings through the production vision client. The client sets `store: false` and records the model identifier returned by the API.
- **Repeated Trials**: `--runs-per-sample` defaults to 3 and can be reduced to 1 for a cheaper exploratory run. A failed or unsupported model call is recorded and does not abort other candidates.
- **Current Pricing Input**: `--pricing <json>` is required and maps each model to current input, cached-input, and output prices per million tokens. Prices are not compiled into HealthVault because they change independently of the application.
- **Cost Safety**: Without `--execute`, the command performs a dry run that lists samples, candidate models, trial count, and total planned API calls without sending images. An executing run requires explicit confirmation after that preview.
- **Metrics**: For each model, calculate structured-output success rate, food detection precision/recall/F1, matched-item weight mean absolute error and mean absolute percentage error, p50/p95 latency, token usage, and estimated cost per sample and for the full run.
- **Deterministic Matching**: Each ground-truth item stores a canonical food name, optional accepted aliases, and optional USDA/custom-food ID. A prediction matches when its normalized name matches an accepted name or both resolve to the same reference-food ID; one-to-one maximum-score matching prevents a prediction from satisfying multiple expected items. Weight errors are calculated only for matched items.
- **Selection**: The report shows the cost/quality Pareto frontier and, when operator-supplied minimum F1 and maximum weight-error thresholds are present, identifies the cheapest candidate that passes them. It does not change `OPENAI_MODEL`.
- **Reproducibility**: Write JSON and Markdown reports containing the dataset hash and sample count, run timestamp, prompt/schema versions, requested and returned model IDs, inference settings, trial count, per-call results, usage, supplied prices, metrics, and threshold decision. A one-sample dataset is allowed but the report warns that it is not representative.
- **Privacy**: The execution confirmation states that every calibration photo will be sent once per planned trial to each selected external model. Calibration samples remain user/tenant scoped and are never added to meal history or nutrition totals.

## Risks / Trade-offs

- **[Risk] OpenAI Vision Latency** → **Mitigation**: Immediate response to upload endpoint returning `meal_id` with `status: "processing"`. Frontend can poll or receive async update.
- **[Risk] SQLite FTS5 database size** → **Mitigation**: Limit dataset to core Foundation & SR Legacy datasets (~8k items, ~50MB disk footprint).
- **[Risk] Calibration overfits a small or unrepresentative sample set** → **Mitigation**: Preserve per-sample results and dataset size in the report, warn for a single sample, and present thresholds plus a Pareto frontier rather than silently declaring an overall winner.
- **[Risk] Annual pricing becomes stale** → **Mitigation**: Require a timestamped operator-supplied pricing file for every run and embed the exact supplied rates in the report.
