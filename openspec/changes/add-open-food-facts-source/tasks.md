## 1. Data model

- [ ] 1.1 Add `OffCode *string` to `FoodItem` in `backend/pkg/database/models_food.go` (indexed, same shape as `FdcID`/`CustomFoodID`)
- [ ] 1.2 Add `OffCode *string` to `GroundTruthItem` in the same file
- [ ] 1.3 Add `OffCode *string` to `vision.Candidate` in `backend/pkg/vision/vision.go`
- [ ] 1.4 Confirm GORM auto-migration picks up the new column on next startup (no manual migration needed — additive nullable column)

## 2. `off` package (mirrors `backend/pkg/usda`)

- [ ] 2.1 Create `backend/pkg/off/off.go`: `Food` struct (`Code string`, `ProductName string`, `Profile database.NutrientProfile`), `Index` type, `Open`/`Close`
- [ ] 2.2 Implement `Index.Search(term string, limit int) ([]Food, error)` over an FTS5 index on `product_name` + `brands`, reusing the OR-joined sanitization approach from `usda/query.go`
- [ ] 2.3 Implement `Index.ByCode(code string) (*Food, error)` for binding a chosen candidate
- [ ] 2.4 Implement `Index.Count()`
- [ ] 2.5 Define schema: `off_foods` table (`code TEXT PRIMARY KEY`, `product_name`, `brands`, `calories`, `protein`, `carbs`, `fat`, `sugar`, `sodium`, `fiber`) + FTS5 virtual table over `product_name`, `brands`
- [ ] 2.6 Implement `Builder`/`Add`/`Promote`/`Discard` with the same build-to-temp-then-atomic-rename pattern and a `MinExpectedRows` guard as `usda.Builder`
- [ ] 2.7 Create `backend/pkg/off/query.go` with `sanitizeFTSQuery`, copied/adapted from `usda/query.go`

## 3. Open Food Facts import

- [ ] 3.1 Create `backend/pkg/off/import.go`: fetch the OFF export (URL or local path, mirroring `usda.Fetch`'s flexibility) and stream-parse it (JSONL recommended — avoids loading the full export into memory)
- [ ] 3.2 Filter during import: keep only products whose `countries_tags` includes Czech Republic and/or Slovakia, and whose `nutriments` include usable calories/protein/carbs/fat values
- [ ] 3.3 Map each kept product's per-100g nutriments into `database.NutrientProfile` and call `Builder.Add`
- [ ] 3.4 Create `backend/cmd/commands/cmdimportoff.go`: cobra command `import-off`, following `cmdimportusda.go`'s structure (fetch → build → promote → report count)
- [ ] 3.5 Register the new command alongside `import-usda` in the CLI root
- [ ] 3.6 Add `OFF_DB_PATH` to `backend/pkg/config/config.go` (default e.g. `./data/off.db`), following the existing `USDA_DB_PATH` pattern exactly

## 4. Candidate resolution changes

- [ ] 4.1 Wire an `off.Index` into the server alongside the existing `usda.Index` (same optional/nilable-on-missing-file handling as USDA)
- [ ] 4.2 In `food_upload.go`'s candidate-resolution path: query `off.Search` first; if it returns zero candidates, query `usda.Search`; if it returns any, use those and skip USDA for that item
- [ ] 4.3 Extend the three "at most one reference set" checks in `food_item.go` and `food_manual.go` (currently pairwise on `FdcID`/`CustomFoodID`) to a three-way check including `OffCode`
- [ ] 4.4 Extend `resolveReferenceProfile` (or equivalent) to accept `OffCode` and resolve via `off.Index.ByCode` when set
- [ ] 4.5 Update the candidate-building code in `food_upload.go` (`vision.Candidate` construction) to populate `OffCode` for Open Food Facts candidates, matching how `FdcID` is populated for USDA ones
- [ ] 4.6 Update binding-on-selection logic so a chosen OFF candidate sets `item.OffCode` and scales macros via `ApplyProfile`, matching the existing USDA/custom-food binding path

## 5. Frontend

- [ ] 5.1 Show which reference field a matched item is bound to (USDA vs Open Food Facts) as a small badge/label in the meal review UI, alongside the existing candidate display
- [ ] 5.2 Confirm the food review API response already includes `off_code` (it will, once added to the `FoodItem` JSON tags) and the frontend type definitions are updated to match

## 6. Tests

- [ ] 6.1 Unit tests for `off` package: `Search` ranking/sanitization, `ByCode` hit/miss, `Builder`/`Promote` including the undersized-import-is-discarded case (mirror `usda_test.go`'s coverage)
- [ ] 6.2 Unit tests for the three-way "at most one reference set" validation in `food_item.go`/`food_manual.go`
- [ ] 6.3 Unit/integration test for the OFF-first, USDA-fallback-on-zero-results resolution logic in the candidate-resolution flow
- [ ] 6.4 E2E: extend `e2e/tests/food.spec.ts` (or add a new spec) to cover a meal item whose match comes from the Open Food Facts source, if the E2E environment can seed/stub an OFF database — otherwise document why not and cover the fallback logic at the unit level instead

## 7. Docs

- [ ] 7.1 Add an Open Food Facts attribution note (ODbL 1.0, required for OFF-derived data shown to users) wherever meal/food data sourced from OFF is displayed or documented
- [ ] 7.2 Document the `import-off` command and `OFF_DB_PATH` config in the project README/ops notes, alongside the existing USDA import documentation
