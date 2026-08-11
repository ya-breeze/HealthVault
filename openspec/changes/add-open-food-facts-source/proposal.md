## Why

USDA FoodData Central is a US reference set, and its per-100g values are frequently wrong for the European branded products the user actually buys (e.g. a Czech yogurt's real protein content differs from whatever generic USDA entry matches by name). Open Food Facts is a crowd-sourced, actively-maintained product database with strong European/Czech coverage and real per-product label data, keyed by barcode. Adding it as a second reference source lets photo-recognition matches reflect the actual product on the shelf instead of a generic US estimate.

## What Changes

- Add a new local SQLite+FTS5 index for Open Food Facts data (`backend/pkg/off`), built the same way the existing USDA index is: an operator-run import command that downloads/filters the official OFF export and atomically promotes a validated build.
- Import filtering keeps only products tagged with a Czech Republic and/or Slovakia (or broader EU) country and complete nutriments, since the full OFF export is 3.5M+ globally noisy branded-product rows.
- Add `FoodItem.OffCode` and `GroundTruthItem.OffCode` (both nullable strings) as a third reference column alongside the existing `FdcID` and `CustomFoodID`, following the same pattern already in use for those two (parallel nullable columns, disambiguated by which is set, "at most one set" validation).
- Change candidate resolution in the photo-recognition flow: query the OFF index first; only query USDA when OFF returns zero candidates. Each candidate is inherently source-tagged by which reference field it would bind (no new source enum needed).
- Document the accepted limitation that OFF is weak on generic/staple foods, so a low-quality OFF match can block the USDA fallback under the strict zero-result trigger.

## Capabilities

### New Capabilities
- `off-nutrition-database`: local Open Food Facts SQLite+FTS5 index — schema, search, by-code lookup, operator-run import with country/completeness filtering, atomic build-and-promote.

### Modified Capabilities
- `data-model`: `FoodItem` and `GroundTruthItem` gain a third nullable reference column (`off_code`), and the "at most one reference set" rule extends from two fields to three.
- `usda-nutrition-database`: the existing candidate-resolution requirement ("Match Selection and Explicit Non-Match") is amended to query the OFF index before USDA, falling back to USDA only when OFF returns no candidates.

## Impact

- **Backend**: new `backend/pkg/off` package (mirrors `backend/pkg/usda`); new `backend/cmd/commands/cmdimportoff.go` CLI command; `backend/pkg/database/models_food.go` (new column); `backend/pkg/server/food_item.go`, `food_manual.go`, `food_upload.go` (three-way reference checks, OFF-first fallback query); `backend/pkg/vision/vision.go` (`Candidate.OffCode`).
- **Data**: new local SQLite file (analogous to `HCW_USDA_DB_PATH`), e.g. `HCW_OFF_DB_PATH`, populated by the new import command — not committed to the repo, not auto-downloaded at runtime.
- **Frontend**: review UI shows which reference field a match is bound to (OFF vs USDA) so the source is visible when confirming a meal.
- **No migration**: the new column is purely additive; existing `FoodItem`/`GroundTruthItem` rows are unaffected and remain bound via `FdcID`/`CustomFoodID` exactly as before.
- **Out of scope**: NutriDatabaze.cz (Czech generic-food database) stays a `todo.md` backlog note — it needs account registration and a license read only a human can do. A barcode-scan-to-CustomFood entry flow is not built here either, though the new `ByCode` lookup could support one later.
