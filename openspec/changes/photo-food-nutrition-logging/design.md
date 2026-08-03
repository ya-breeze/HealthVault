## Context

HealthVault uses Go with GORM (SQLite) on the backend and Next.js (TypeScript) on the frontend. See proposal.md for overall motivation and capabilities.

## Goals / Non-Goals

**Goals:**
- Implement a photo-first upload pipeline where raw images are stored locally and `FoodMeal` database records are created prior to calling OpenAI Vision.
- Embed USDA FoodData Central (Foundation + SR Legacy) in a local SQLite database table with an FTS5 full-text index for fast ingredient matching.
- Support custom user foods with high-priority matching over standard USDA entries.
- Calculate 7 key macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) scaled per 100g.
- Synchronize confirmed meal totals directly into HealthVault's existing main [`Nutrition`](file:///Users/ek/work/HealthVault/backend/pkg/database/models.go#L258-L273) table.

**Non-Goals:**
- Full 150+ micronutrient tracking UI (stored in details JSON, but not exposed in primary UI for now).
- Barcode scanning (reserved for a future phase).

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

## Risks / Trade-offs

- **[Risk] OpenAI Vision Latency** → **Mitigation**: Immediate response to upload endpoint returning `meal_id` with `status: "processing"`. Frontend can poll or receive async update.
- **[Risk] SQLite FTS5 database size** → **Mitigation**: Limit dataset to core Foundation & SR Legacy datasets (~8k items, ~50MB disk footprint).
