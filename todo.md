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

---

## Short-name fuzzy matching for non-Latin catalogs

`fuzzyMinNearMatchLen = 10` (backend/pkg/server/fuzzy.go) refuses any
below-perfect custom-food name match when the shorter normalized name is
under ten runes. That gate is deliberate and is written into the
usda-nutrition-database spec ("A short name matches only itself"), including
its consequence for "languages that name staples in one word".

The consequence is heaviest exactly where the fuzzy match matters most.
Russian food names are usually short — борщ (4), блины (5), молоко (6),
гречка (6), сырники (7), вареники (8) — so near-matching is effectively
inert for them, leaving only normalization-identical matches. And for a
non-English Display Language neither Open Food Facts nor USDA is queried, so
the fallback is the user's top-ranked custom foods (capped at five) or the
macro estimate.

Lowering the flat gate is not the fix: at six runes a single differing
letter still clears the similarity threshold, which is precisely the
"Butter"/"Batter" false positive the gate was added to stop, and a false
positive binds unconditionally with no alternative offered.

Worth exploring instead, in its own change with its own spec delta:

1. **Suffix-tolerant matching for inflected languages.** Russian names vary
   by grammatical case — молоко/молока/молоком — so a difference confined to
   a short trailing segment of an otherwise identical name is usually the
   same food, while Butter/Batter differs in the stem. This targets the
   actual failure mode without loosening the stem comparison.
2. **Edit-distance cap rather than a length gate.** Allow one edit only once
   the name is long enough for one edit to read as a typo, which is what the
   current gate approximates crudely.
3. **Validate against real data first.** There is no Russian custom-food
   corpus here to tune against; retuning blind risks trading a documented
   false-negative for an undocumented false-positive, which is the more
   damaging direction.

Raised in code review on the russian-localization branch and deliberately
left as specced there, since it revisits an accepted trade-off rather than
fixing a defect.
