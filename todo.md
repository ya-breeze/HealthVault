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

## Weight chart: goal weight, BMI bands, and trend projection

Deferred 2026-08-19 while scoping `weight-chart-scale-and-trend` (fixing the weight
chart's 0-100 Y-axis bug and adding a smoothed trend line). The reference
screenshot that prompted that change — the "Libra" weight-tracking app, which
HealthVault already imports CSV exports from via `libra-import` — also shows a
goal-weight line, horizontal BMI-category bands, and a dashed line projecting
the trend forward to the goal. None of that made it into the scaling/trend
change; each piece below is a separate, sizeable candidate.

**Findings:** HealthVault has no persisted place for a goal today — there's no
User/Family settings table, only per-metric timestamped records (see
`backend/pkg/database/models.go`). The cleanest fit is a new metric type (e.g.
`weight_goal`) reusing the existing per-type time-series pattern (`TYPE_META`,
the data-API's `?bucket=`) rather than a new subsystem — "latest record wins"
is exactly a goal's semantics. BMI bands need the user's height; `height` is
already a logged point-in-time metric, so bands could read the latest
`Height` record and simply not render if none exists.

Candidates, roughly sized:

1. **Goal weight (small–medium).** New `weight_goal` metric type, a way to
   set/edit it in the UI, and a horizontal reference line on the weight chart.
2. **BMI category bands (small; needs height, not goal).** WHO threshold
   bands converted to kg via the user's latest `Height` record; hidden if no
   height is on file.
3. **Projected trend line to goal (small; depends on 1).** Extrapolate the
   weight trend line's recent slope forward until it crosses the goal weight.
4. **Trend line for other point-in-time metrics (small).** The EMA trend
   computation added for `weight` is metric-agnostic; extending it to heart
   rate, blood pressure, etc. is a "should we" question, not a "can we" one.

**Recommendation if picked up:** start with (1), since (2) and (3) both build
on it (BMI bands need height, not goal, but pair naturally with the same UI
work as the goal-weight input).
