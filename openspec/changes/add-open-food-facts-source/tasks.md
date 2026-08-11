## 1. Data model

- [ ] 1.1 Add `OffCode *string` and `Brand string` to `FoodItem` in `backend/pkg/database/models_food.go` (`OffCode` indexed, same shape as `FdcID`/`CustomFoodID`; `Brand` follows `Preparation`/`State`'s pattern — empty means unknown, not nullable)
- [ ] 1.2 Add `OffCode *string` to `GroundTruthItem` in the same file
- [ ] 1.3 Add `Brand string` to `vision.Item` and `OffCode *string` to `vision.Candidate` in `backend/pkg/vision/vision.go`
- [ ] 1.4 Confirm GORM auto-migration picks up the new columns on next startup (no manual migration needed — additive nullable/default-empty columns)

## 2. `off` package (mirrors `backend/pkg/usda`)

- [ ] 2.1 Create `backend/pkg/off/off.go`: `Food` struct (`Code string`, `ProductName string`, `Profile database.NutrientProfile`), `Index` type, `Open`/`Close`
- [ ] 2.2 Implement `Index.Search(term string, limit int) ([]Food, error)` over an FTS5 index on `product_name` + `brands`, reusing the OR-joined sanitization approach from `usda/query.go`
- [ ] 2.3 Implement `Index.ByCode(code string) (*Food, error)` for binding a chosen candidate
- [ ] 2.4 Implement `Index.Count()`
- [ ] 2.5 Define schema: `off_foods` table (`code TEXT PRIMARY KEY`, `product_name`, `brands`, `calories`, `protein`, `carbs`, `fat`, `sugar`, `sodium`, `fiber`) + FTS5 virtual table over `product_name`, `brands`
- [ ] 2.6 Implement `Builder`/`Add`/`Promote`/`Discard` with the same build-to-temp-then-atomic-rename pattern and a `MinExpectedRows` guard as `usda.Builder`
- [ ] 2.7 Create `backend/pkg/off/query.go` with `sanitizeFTSQuery` (adapted from `usda/query.go`) and a `QueryFor(name, brand string) string` analogous to `usda.QueryFor`, OR-joining name and brand tokens

## 3. Open Food Facts import

- [ ] 3.1 Create `backend/pkg/off/import.go`: fetch the OFF export (URL or local path, mirroring `usda.Fetch`'s flexibility) and stream-parse it as gzip-compressed JSONL (avoids loading the full export into memory; handle gzip decoding explicitly)
- [ ] 3.2 Filter during import: keep only products whose `countries_tags` includes Czech Republic and/or Slovakia
- [ ] 3.3 Map nutriments explicitly per design.md's "Explicit per-field mapping" decision: calories from `nutriments.energy-kcal_100g` only (never `energy_100g`/`energy-kj_100g`); sodium from `nutriments.sodium_100g`, falling back to `nutriments.salt_100g / 2.5` when only salt is present, defaulting to 0 when neither is present; protein/carbs/fat/sugar/fiber from their direct `_100g` fields, defaulting sugar/fiber to 0 when absent. A product missing calories or missing protein/carbs/fat is excluded as incomplete — sodium/sugar/fiber absence does NOT exclude a product, matching the completeness bar already stated in the off-nutrition-database spec (calories/protein/carbs/fat only)
- [ ] 3.4 Handle malformed/unparseable rows by skipping and counting them, not aborting the import
- [ ] 3.5 Call `Builder.Add` for each kept, mapped product
- [ ] 3.6 Create `backend/cmd/commands/cmdimportoff.go`: cobra command `import-off`, following `cmdimportusda.go`'s structure (fetch → build → promote → report count, including skipped/malformed row count)
- [ ] 3.7 Register the new command alongside `import-usda` in the CLI root
- [ ] 3.8 Add `OFF_DB_PATH` to `backend/pkg/config/config.go` (default e.g. `./data/off.db`), following the existing `USDA_DB_PATH` pattern exactly

## 4. Candidate resolution changes

- [ ] 4.1 Wire an `off.Index` into the server alongside the existing `usda.Index` (same optional/nilable-on-missing-file handling as USDA)
- [ ] 4.2 Update the vision recognition prompt/schema so `Recognize` extracts `Brand` per item (empty when no legible brand), alongside the existing `Preparation`/`State` extraction
- [ ] 4.3 In `food_upload.go`'s candidate-resolution path: for an item with non-empty `Brand`, build the retrieval term via `off.QueryFor(name, brand)` and call `off.Search`; if zero candidates, fall back to `usda.Search`. For an item with empty `Brand`, call `usda.Search` directly and do not query `off` at all
- [ ] 4.4 Add a new "at most one of `fdc_id`/`off_code`/`custom_food_id`" validation to `food_manual.go`'s `CreateManualMeal` — **this is a new check, not an extension**: today this endpoint has no such validation at all (`resolveReferenceProfile` picks `FdcID` if both are set, but both fields get persisted regardless). Fix this as part of adding the third field, don't just add `OffCode` to a pattern that doesn't exist yet at this call site
- [ ] 4.5 Extend the existing pairwise checks in `food_item.go` (PATCH and item-addition paths) from two-way to three-way, including `OffCode`
- [ ] 4.6 Extend `resolveReferenceProfile` to accept `offCode *string` and resolve via `off.Index.ByCode` when set
- [ ] 4.7 Update the candidate-building code in `food_upload.go` (`vision.Candidate` construction) to populate `OffCode` for Open Food Facts candidates, matching how `FdcID` is populated for USDA ones
- [ ] 4.8 Update binding-on-selection logic so a chosen OFF candidate sets `item.OffCode` and scales macros via `ApplyProfile`, matching the existing USDA/custom-food binding path

## 5. Frontend

- [ ] 5.1 Show which reference field a matched item is bound to (USDA vs Open Food Facts) as a small badge/label in the meal review UI, alongside the existing candidate display
- [ ] 5.2 Confirm the food review API response already includes `off_code` and `brand` (it will, once added to the `FoodItem`/`Item` JSON tags) and the frontend type definitions are updated to match

## 6. Tests

- [ ] 6.1 Unit tests for `off` package: `Search` ranking/sanitization, `ByCode` hit/miss, `Builder`/`Promote` including the undersized-import-is-discarded case (mirror `usda_test.go`'s coverage)
- [ ] 6.2 Unit tests for `backend/pkg/off/import.go`'s parser/filter/mapping — the highest-risk external-data boundary — using small fixture JSONL files covering: gzip decoding, CZ/SK inclusion vs. other-country exclusion, complete vs. incomplete-nutriment exclusion, malformed/unparseable rows being skipped rather than aborting the import, `energy-kcal_100g` vs. `energy-kj_100g` field selection, and `sodium_100g` vs. `salt_100g` fallback (including the /2.5 conversion)
- [ ] 6.3 Unit tests for the three-way "at most one reference set" validation in `food_item.go` and the new validation in `food_manual.go` — cover every pairing (fdc_id+off_code, fdc_id+custom_food_id, off_code+custom_food_id) plus all three set together
- [ ] 6.4 Unit/integration test for the brand-gated resolution logic in the candidate-resolution flow: brand present + OFF hit, brand present + OFF miss (falls back to USDA), brand absent (goes straight to USDA, OFF never queried)
- [ ] 6.5 E2E: extend `e2e/tests/food.spec.ts` (or add a new spec) to cover a meal item whose match comes from the Open Food Facts source, if the E2E environment can seed/stub an OFF database — otherwise document why not and cover the fallback logic at the unit level instead

## 7. Docs

- [ ] 7.1 Add an Open Food Facts attribution note (ODbL 1.0, required for OFF-derived data shown to users) wherever meal/food data sourced from OFF is displayed or documented
- [ ] 7.2 Document the `import-off` command and `OFF_DB_PATH` config in the project README/ops notes, alongside the existing USDA import documentation — explicitly note that both `import-usda` and `import-off` require a backend restart to take effect, since the reference index is opened once at server startup and does not reload
