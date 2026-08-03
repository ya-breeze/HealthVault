## 1. Database & Models

- [ ] 1.1 Add `FoodMeal`, `FoodItem`, and `CustomFood` GORM models to `backend/pkg/database/models.go`
- [ ] 1.2 Add AutoMigrate entries in `backend/pkg/database/db.go`
- [ ] 1.3 Add local USDA FTS5 database schema creation and search queries in `backend/pkg/database/usda.go`

## 2. Media Storage & Resilient Upload Pipeline

- [ ] 2.1 Implement secure local file storage helper in `backend/pkg/storage`
- [ ] 2.2 Implement authenticated `/api/media/{photo_id}` handler in `backend/pkg/server/handler_media.go`
- [ ] 2.3 Implement pre-LLM photo saving logic in `POST /api/food/meals` handler

## 3. USDA Importer & Auto-Downloader

- [ ] 3.1 Implement USDA Foundation/SR Legacy CSV downloader and SQLite FTS5 indexer in `backend/pkg/usda`
- [ ] 3.2 Add periodic monthly background update timer/cron for USDA dataset updates

## 4. OpenAI Vision Integration

- [ ] 4.1 Create OpenAI Vision client package in `backend/pkg/vision` with structured output parsing
- [ ] 4.2 Integrate Vision client with `POST /api/food/meals` to extract foods, weights, confidence scores, and clarification questions

## 5. API Endpoints & Clarification Flow

- [ ] 5.1 Implement `POST /api/food/meals/{id}/clarify` endpoint to handle clarification responses
- [ ] 5.2 Implement `PUT /api/food/meals/{id}/confirm` endpoint to update portion weights and sync aggregates to the main `Nutrition` table
- [ ] 5.3 Implement `POST /api/food/custom` and `GET /api/food/search` endpoints for custom foods and USDA search

## 6. Frontend UI Components

- [ ] 6.1 Create photo upload and camera capture component with loading state
- [ ] 6.2 Create interactive clarification questions modal
- [ ] 6.3 Create portion weight adjuster slider and itemized macro summary card
- [ ] 6.4 Create custom food entry modal
- [ ] 6.5 Create calibration sample capture and management UI for photos, food identities, and measured gram weights

## 7. Model Calibration

- [ ] 7.1 Add `FoodCalibrationSample` GORM model, migration, tenant-scoped storage queries, and photo cleanup
- [ ] 7.2 Add authenticated create/list/delete calibration sample endpoints without creating meal or `Nutrition` records
- [ ] 7.3 Refactor the production Vision client to accept a per-call model override and return requested/actual model IDs, token usage, latency, and structured-output errors
- [ ] 7.4 Implement `hcw calibrate-food-models` user/sample dataset selection, dry-run and explicitly confirmed execution modes with runtime model IDs, repeated trials, and operator-supplied pricing
- [ ] 7.5 Implement deterministic ground-truth matching, detection/weight accuracy metrics, latency and cost aggregation, quality thresholds, Pareto-frontier selection, and reproducible JSON/Markdown reports
- [ ] 7.6 Add unit and integration tests for sample isolation, dry-run zero-call behavior, partial model failures, metric calculations, price calculations, threshold selection, and deterministic report metadata
