# HealthVault — future ideas / backlog

Informal notes, not OpenSpec changes yet. Promote an item to an OpenSpec change
(`opsx:propose`) when someone decides to actually pick it up.

## Open Food Facts (European product database)

Investigated 2026-08-08: is there a European equivalent of the USDA FoodData
Central database (currently used in `backend/pkg/usda/`) that could be
downloaded and used for personal food logging?

**Answer: [Open Food Facts](https://world.openfoodfacts.org)** — open,
crowd-sourced, global but strongest in Europe (founded in France). Barcode-keyed
branded products with per-100g nutrition, Nutri-Score, NOVA classification.
Downloadable in full (CSV/JSON/Parquet/MongoDB dump) or queryable via a free
live REST API (`world.openfoodfacts.org/api/v2/product/<barcode>.json`).
License: ODbL 1.0 for data (+ CC-BY-SA for photos) — attribution required;
share-alike only bites if the derived database is itself redistributed
publicly (relevant if a built index were committed to this public repo).

Two different ways it could plug into HealthVault, not equally sized:

1. **Barcode lookup for manual/custom food entry (small).** User enters/scans
   a barcode when logging a packaged food; call the OFF live API, pre-fill a
   `CustomFood`-shaped per-100g profile. No local index needed, no schema
   change — fits the existing `CustomFood` concept as-is. Reasonable size for
   a single OpenSpec change.

2. **Second reference-search source for photo recognition, alongside USDA
   (large).** Today `FoodItem.FdcID *int64` and `GroundTruthItem.FdcID *int64`
   hard-code the USDA integer ID; OFF uses string barcodes, so this needs the
   reference concept generalized (e.g. `Source + ExternalID` replacing
   `FdcID`), touching `models_food.go`, `usda/query.go`, every `food_*.go`
   handler, and calibration. OFF is also ~3.5M+ crowd-sourced branded
   products (uneven data quality, many incomplete) vs. USDA's curated ~8k
   generic-food SR Legacy set — a different shape of data (branded products,
   not generic "grilled chicken breast" style descriptions), so it may not
   even improve photo-recognition matching. Would need real design work
   (filtering to complete/EU entries, dedup, relevance tuning) before it's
   worth the schema churn.

**Recommendation if picked up:** start with (1) — it's additive, low-risk,
and directly useful for a European user logging packaged food. Only consider
(2) if (1) turns out insufficient in practice.
