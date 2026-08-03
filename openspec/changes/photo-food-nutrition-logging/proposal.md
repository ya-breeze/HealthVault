## Why

HealthVault users currently lack an automated way to log meals and track nutrition. Manually calculating calories and macros from photos or packaged foods is tedious and error-prone. 

By integrating OpenAI Vision (Luna) for automated food recognition, storing a local USDA FoodData Central SQLite database with FTS5 search, supporting custom user food entries, and establishing a photo-first resilient storage pipeline, HealthVault will provide instant, high-accuracy food and nutrition logging.

## What Changes

- **Photo-First Resilient Storage**: Save uploaded meal photos to authenticated local server storage (`/api/media/{photo_id}`) and persist `FoodMeal` records before calling the LLM vision service, preventing photo loss on LLM errors.
- **LLM Vision Food Analysis**: Integrate OpenAI Vision API to segment food items, estimate gram weights, provide confidence scores, and generate clarification questions when dish details or portion sizes are ambiguous.
- **Local USDA SQLite Database**: Embed and maintain a local SQLite dataset of USDA FoodData Central core (Foundation & SR Legacy) foods indexed with SQLite FTS5 for zero-latency ingredient matching and per-100g nutrient scaling.
- **Periodic USDA Sync**: Automatic monthly background check/download for updated USDA FDC release datasets.
- **Custom User Foods**: Enable users to manually log nutrition from food packages or custom recipes, storing them in a user custom foods index that takes precedence during food matching.
- **Nutrient Calculation & Synchronization**: Calculate the 7 primary macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) for each meal and auto-sync aggregate totals into HealthVault's main `Nutrition` table.

## Capabilities

### New Capabilities
- `food-photo-recognition`: Secure photo upload, resilient pre-LLM persistence, OpenAI Vision analysis, and multi-turn clarification UI.
- `usda-nutrition-database`: Local SQLite USDA FTS5 search index, periodic update downloader, and user custom food entry store.
- `food-nutrition-logging`: Meal itemization, nutrient scaling, manual macro entry, and automatic sync to HealthVault `Nutrition` storage.

### Modified Capabilities
- `data-model`: Extend GORM models with `FoodMeal`, `FoodItem`, `CustomFood`, and media storage references.
- `data-api`: Add routes for meal upload, media serving, clarification responses, food search, custom food CRUD, and meal confirmation.

## Impact

- **Backend**: New Go packages/services for OpenAI Vision client, USDA FTS5 SQLite manager, media storage handler, and HTTP endpoints in `pkg/server`.
- **Database**: GORM migrations for `FoodMeal`, `FoodItem`, `CustomFood`, and an isolated or attached SQLite FTS5 database for USDA reference data.
- **Frontend**: Next.js photo upload component, camera preview, interactive clarification dialog, portion weight adjuster slider, and custom food entry modal.
- **Config**: New environment variables (`OPENAI_API_KEY`, `OPENAI_MODEL`, `HEALTHVAULT_UPLOADS_DIR`).
