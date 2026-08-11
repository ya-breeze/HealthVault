## Why

USDA FoodData Central is a US reference set, and its per-100g values are frequently wrong for the European branded products the user actually buys (e.g. a Czech yogurt's real protein content differs from whatever generic USDA entry matches by name). Open Food Facts is a crowd-sourced, actively-maintained product database with strong European/Czech coverage and real per-product label data, keyed by barcode. Adding it as a second reference source lets photo-recognition matches reflect the actual product on the shelf instead of a generic US estimate.

## What Changes

- Add a new local SQLite+FTS5 index for Open Food Facts data (`backend/pkg/off`), built the same way the existing USDA index is: an operator-run import command that downloads/filters the official OFF export and atomically promotes a validated build.
- Import filtering keeps only products tagged with a Czech Republic and/or Slovakia country and complete nutriments, since the full OFF export is 3.5M+ globally noisy branded-product rows.
- Add `FoodItem.OffCode` and `GroundTruthItem.OffCode` (both nullable strings) as a third reference column alongside the existing `FdcID` and `CustomFoodID`, following the same pattern already in use for those two (parallel nullable columns, disambiguated by which is set, "at most one set" validation) — and fix the existing manual-meal-entry endpoint, which today has no such exclusivity check at all, so the three-way rule is actually enforced everywhere rather than assumed.
- Extend food recognition to extract a `brand` alongside each item's name, when a label/brand is visibly legible in the photo (empty otherwise) — the same way `preparation`/`state` are already extracted. This is what makes OFF matching safe: candidate selection is a text-only model call with no photo access, so without a brand signal there is no way to tell *which* of several differently-branded products a generic name like "yogurt" refers to, and auto-binding to an arbitrary one could be less accurate than USDA's generic estimate, not more.
- Change candidate resolution in the photo-recognition flow: query OFF only when a brand was extracted, using name+brand as the retrieval term; fall back to USDA when OFF returns zero candidates for that query, or immediately when no brand was extracted at all. Each candidate is inherently source-tagged by which reference field it would bind (no new source enum needed).

## Capabilities

### New Capabilities
- `off-nutrition-database`: local Open Food Facts SQLite+FTS5 index — schema, search, by-code lookup, operator-run import with country/completeness filtering, atomic build-and-promote.

### Modified Capabilities
- `data-model`: `FoodItem` and `GroundTruthItem` gain a third nullable reference column (`off_code`), `FoodItem` gains a `brand` field, and the "at most one reference set" rule extends from two fields to three.
- `food-photo-recognition`: recognized items gain a `brand` field, extracted the same way `preparation`/`state` already are.
- `usda-nutrition-database`: the existing candidate-resolution requirement ("Match Selection and Explicit Non-Match") is amended so OFF is queried, brand-gated, before USDA, falling back to USDA when OFF returns no candidates or no brand was extracted.
- `food-nutrition-logging`: the item PATCH ("Item Resolution"), item POST ("Item Addition"), and manual-meal-creation ("Manual Food Logging") endpoints all accept `off_code` as a third bindable reference alongside `fdc_id`/`custom_food_id` — including the manual-meal endpoint's exclusivity rule, which is new there, not an extension (see Impact).
- `food-model-calibration`: detection-scoring's reference-ID matching includes Open Food Facts codes alongside USDA/custom-food IDs.

## Impact

- **Backend**: new `backend/pkg/off` package (mirrors `backend/pkg/usda`); new `backend/cmd/commands/cmdimportoff.go` CLI command; `backend/pkg/database/models_food.go` (new `OffCode`/`Brand` fields); `backend/pkg/server/food_item.go`, `food_manual.go` (three-way reference checks — `food_manual.go` needs this added, not extended, since it has none today), `food_upload.go` (brand extraction, brand-gated OFF query, fallback); `backend/pkg/vision/vision.go` (`Item.Brand`, `Candidate.OffCode`/`Brands`, `ItemCandidates.ItemName`/`ItemBrand`); `backend/pkg/vision/openai.go` (`Select`'s prompt and payload now carry the recognized item's own name/brand, fixing a pre-existing gap that becomes safety-critical for OFF's same-brand multi-SKU disambiguation — see design.md); `backend/pkg/server` item PATCH/POST handlers (accept `off_code`).
- **Data**: new local SQLite file (analogous to `HCW_USDA_DB_PATH`), e.g. `HCW_OFF_DB_PATH`, populated by the new import command — not committed to the repo, not auto-downloaded at runtime. Activating it (first import or any reimport) requires restarting the backend process, matching the existing (previously undocumented) USDA behavior.
- **Frontend**: review UI shows which reference field a match is bound to (OFF vs USDA) so the source is visible when confirming a meal.
- **No migration**: the new columns are purely additive; existing `FoodItem`/`GroundTruthItem` rows are unaffected and remain bound via `FdcID`/`CustomFoodID` exactly as before.
- **Out of scope**: NutriDatabaze.cz (Czech generic-food database) stays a `todo.md` backlog note — it needs account registration and a license read only a human can do. A barcode-scan-to-CustomFood entry flow is not built here either, though the new `ByCode` lookup could support one later. Sending the photo into the candidate-selection call (an alternative fix for brand ambiguity, touching USDA/custom-food selection too) was considered and rejected in favor of extracting brand once during recognition — see design.md.
