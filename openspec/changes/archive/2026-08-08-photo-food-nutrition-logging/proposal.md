## Why

HealthVault users currently lack an automated way to log meals and track nutrition. Manually calculating calories and macros from photos or packaged foods is tedious and error-prone.

By integrating OpenAI Vision for automated food recognition, storing a local USDA FoodData Central SQLite database with FTS5 search, supporting custom user food entries, and establishing a photo-first resilient storage pipeline, HealthVault will provide fast, reviewable food and nutrition logging. Because vision model quality and pricing change over time, HealthVault will also support an operator-run calibration workflow that compares current candidate models against locally captured, weighed-food ground truth.

## What Changes

- **Photo-First Resilient Storage**: Save uploaded meal photos to authenticated local server storage and persist a `FoodMeal` record before calling the LLM vision service, preventing photo loss on LLM errors.
- **LLM Vision Food Analysis**: Integrate OpenAI Vision to segment food items, estimate gram weights, provide confidence scores, and generate clarification questions when dish details or portion sizes are ambiguous. The call is made synchronously within the upload request, so the feature adds no background worker.
- **Local USDA SQLite Database**: Embed a local SQLite dataset of USDA FoodData Central core (Foundation & SR Legacy) foods indexed with SQLite FTS5, used to retrieve ranked *candidates* that the model or user selects from, rather than silently auto-matching.
- **One-Shot USDA Import**: Load and refresh the dataset with an operator-run `hcw import-usda` command. SR Legacy is a frozen dataset and Foundation Foods publishes roughly twice a year, so no scheduled background updater is introduced.
- **Custom User Foods**: Enable users to manually log nutrition from food packages or custom recipes, storing them in a user custom foods index that takes precedence during food matching.
- **Nutrient Calculation**: Calculate the 7 primary macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) for each meal item and aggregate them per meal.
- **Meals as a First-Class Data Type**: Expose confirmed meals through the existing `GET /api/data/{type}` registry as `food_meal`, so meals appear in the frontend data views and the MCP server alongside other types.
- **Model Calibration**: Let authenticated users save photos of foods with measured gram weights as calibration samples, then provide a manually invoked CLI that benchmarks explicitly selected vision models and reports accuracy, latency, and estimated cost using current operator-supplied prices.

## Non-Goals

- **No writes to the `Nutrition` table.** `Nutrition` is telemetry ingested from Health Connect webhooks and file imports, where every row traces to a source payload. Food meals are user-authored records with no payload lineage, and many Health Connect setups already sync nutrition entries from other apps. Writing meals into the same table would produce two independent producers of the same nutrient facts with no discriminator to tell them apart — the `(user_id, start_time)` unique constraint only dedupes exact timestamp collisions, which will not occur across sources. `FoodMeal` is therefore the source of truth for photo-logged food, and `Nutrition` is left untouched. Unifying the two into a single daily total is deferred to a future change that would add an explicit source discriminator.
- Full 150+ micronutrient tracking UI (stored in details JSON, but not exposed in primary UI for now).
- Barcode scanning (reserved for a future phase).
- Scheduled calibration runs or automatic changes to the production model setting.

## Capabilities

### New Capabilities
- `food-photo-recognition`: Secure photo upload, resilient pre-LLM persistence, OpenAI Vision analysis, retry after failure, and bounded multi-turn clarification.
- `usda-nutrition-database`: Local SQLite USDA FTS5 candidate search, operator-run import command, and user custom food entry store.
- `food-nutrition-logging`: Meal itemization, nutrient scaling, manual (photo-free) entry, and meal lifecycle including deletion.
- `food-model-calibration`: Ground-truth sample capture and a repeatable, manual model quality/cost benchmark with reproducible reports.

### Modified Capabilities
- `data-model`: Scope the existing `source_payload_id` lineage rule to ingested health metric types, and add the food logging tables (`FoodMeal`, `FoodItem`, `CustomFood`, `FoodCalibrationSample`) as a distinct family-scoped, user-authored category.
- `data-api`: Register `food_meal` in the generic data type registry (26 types) and add the `/api/food/*` route group.
- `record-deletion`: Require type-specific cleanup when deleting a `food_meal` — cascade to its items and remove the stored photo.

## Impact

- **Backend**: New Go packages for the OpenAI Vision client, USDA FTS5 SQLite manager, media storage handler, HTTP endpoints in `pkg/server`, and two CLI commands (`hcw import-usda`, `hcw calibrate-food-models`).
- **Database**: GORM migrations for `FoodMeal`, `FoodItem`, `CustomFood`, `FoodCalibrationSample`, and a separate SQLite database file for USDA reference data.
- **Frontend**: Next.js photo upload component, camera preview, interactive clarification dialog, portion weight adjuster, custom food entry modal, and calibration sample capture UI.
- **Config**: New environment variables under the existing `HCW` viper prefix — `HCW_OPENAI_API_KEY`, `HCW_OPENAI_MODEL`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH`, `HCW_MAX_UPLOAD_BYTES`, `HCW_VISION_TIMEOUT`.
