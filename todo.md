# HealthVault — future ideas / backlog

Informal notes, not OpenSpec changes yet. Promote an item to an OpenSpec change
(`opsx:propose`) when someone decides to actually pick it up.

## Dashboard customization + food-tracking initiative (phases 2-4)

Grilled 2026-08-21 (`/mattpocock-skills:grill-with-docs`) starting from "let users
show/hide dashboard elements, and add food-related elements." That decomposed into
4 sequential, independently-shippable OpenSpec changes; full context, the domain
vocabulary, and 4 ADRs recording the architectural decisions below are in
[PR #27](https://github.com/ya-breeze/HealthVault/pull/27) (`CONTEXT.md`'s new
Dashboard/Nutrition-targets sections, `docs/adr/ADR-001..004`, all `Status:
Proposed`).

**Phase 1 — Dashboard card hide/show** is *not* backlog: it's fully specified and
ready for its own `opsx:propose` now (see ADR-001). It extends the existing
`PRIMARY_METRICS`/`dashboard_order` card-registry pattern with a per-Card
show/hide toggle in the existing Edit/Done mode, scoped to the 8 vitals-grid
Cards only (not the needs-attention banner / Log Food row / More Data row).
Phases 2-4 below depend on it existing (they add new Card types to the same
registry) but are otherwise only scoped at the boundary level — each still
needs its own detailed grilling/proposal when picked up.

### Phase 2 — Goal weight + BMI bands + trend projection

Realizes the "Weight chart: goal weight, BMI bands, and trend projection"
backlog item further down this file — see that section for the full sizing
research. The one thing decided since that research (ADR-002): **Goal Weight
is a new metric type (`weight_goal`, latest-record-wins)**, not a
`UserSettings` field, specifically so goal history isn't lost the way a
JSON-blob overwrite would lose it, and so Phase 3's targets can read "the
goal at the time" consistently. BMI bands (WHO thresholds, from the latest
`Height` record) and the trend projection (extrapolating the existing weight
EMA trend line to the goal) are otherwise unchanged from that section's
candidates 1-3.

### Phase 3 — User profile (age/sex/activity level) + Nutrition Target

New: a **Nutrition Target** (daily calories + protein/carb/fat grams), split
between two weight inputs (ADR-003, revised after review 2026-08-21): calories
come from the full Mifflin-St Jeor BMR formula × an activity-level multiplier,
using the user's **latest measured weight** (BMR is a function of actual body
mass — computing it from Goal Weight instead would understate real energy
needs and risk an unsafely aggressive deficit). **Protein** specifically is a
g/kg rate applied to **Goal Weight from Phase 2** — sized to the body being
worked toward, per standard practice. Carb/fat targets fill whatever's left of
the calorie budget. Requires three new static profile fields that don't exist
anywhere yet: age (or birthdate), sex, and activity level. Still undecided,
deferred to this phase's own `opsx:propose`: activity-level tier count and
multipliers (simple 3-tier vs. standard 5-tier Harris-Benedict-style), and
what the protein target (and any recommendation text built on it) should show
when no `weight_goal` is set yet. The calorie/carb/fat targets no longer have
a hard dependency on Phase 2 — only the protein target does — but Phase 3 is
still sequenced after Phase 2 so a goal is normally in place before Phase 4
needs it.

### Phase 4 — Food dashboard Cards + LLM recommendations/chat

Two new Food Cards (per ADR-001's registry, defaulting to visible):

- **Card A** — today's calories/macros vs. the Phase-3 Nutrition Target.
- **Card B** (single merged Card, not two) — a **Healthiness Label** (Good /
  Fair / Needs attention, rolling 7-day window) plus 1-2 short recommendation
  lines under it. The label itself is a **deterministic heuristic** over
  already-logged `FoodMeal` macro/sugar/sodium fields — explicitly *not*
  LLM-judged (ADR-004), so it stays free, fast, and reproducible on every
  dashboard load regardless of entry source (photo/manual/barcode).

LLM involvement is downstream of the label, not the label itself: (1) an
automatic once-daily cached call that generates/refines the recommendation
text, (2) a user-triggered "get advice" button for an on-demand refresh, and
(3) a small chat affordance for follow-up/clarifying questions about the
user's nutrition. Still undecided: exact heuristic thresholds (macro-share
ranges, sugar/sodium cutoffs), and the chat's persistence model (ongoing
thread vs. ephemeral per session) — both deferred to this phase's own
`opsx:propose`. Depends on Phase 3's Nutrition Target existing.

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

> **Update (2026-08-21):** candidates 1-3 below are now scoped as **Phase 2** of
> the dashboard/food-tracking initiative at the top of this file, with the
> goal-weight storage question below resolved by ADR-002 (new metric type, not
> a `UserSettings` field). Candidate 4 (trend lines for other metrics) is not
> part of that phase and remains an open, separate idea.

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
