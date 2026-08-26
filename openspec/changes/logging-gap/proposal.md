## Why

Idea #10 set out to build "adaptive TDEE" — measuring metabolism from energy balance instead of
predicting it from Mifflin-St Jeor. Two `/grilling` passes against the production database (2026-08-23
and 2026-08-25, both recorded on the idea-forge issue) killed that framing: the database has no
third source of expenditure to validate against (`total_calories` is exercise calories, not TDEE,
despite the name), and even after `food-day-completeness` (#9) ships, its own gate only removes the
*worst* noise — a 922 kcal day for a 91 kg man still passes as "Complete." Reporting a number
labeled TDEE from that input, then prescribing a deficit under it, is a starvation recommendation
delivered with false confidence.

The second grilling pass reshaped the feature into something the data can actually support: instead
of asserting a metabolism, report **the gap between what was logged and what the weight trend
implies was eaten** — a diagnostic for the food log, not a replacement for the formula. ADR-006's
steps-inferred activity level is untouched; this is a new, independent Food Card.

This proposal transcribes that grilled design into OpenSpec deltas. Nothing here is a new decision
except where the idea-forge issue explicitly left a question open for `opsx:propose` to settle (the
outlier threshold, the interval formula, the daily-totals endpoint's home, and the trend window's
anchor point) — those four are resolved in `design.md` and are the only judgment calls this proposal
makes.

## What Changes

- **A new `logging-gap` capability** computing, over a trailing 28-day window (ending yesterday, matching
  ADR-006's steps window — never today, which is still in progress):
  - **Implied Intake** = the caller's Nutrition Target `calories` figure, adjusted by the weight
    trend's implied deficit/surplus (`trend slope (kg/day) × 7700`).
  - **Logging Gap** = Implied Intake − mean logged intake over the window's Complete /
    Confirmed Complete days only (per `food-day-completeness`'s states) — days that didn't pass that
    gate are excluded, not averaged in as zero.
  - An **uncertainty interval**, combined in quadrature from the Nutrition Target's fixed ±10%
    Mifflin-St Jeor error and the weight-trend regression's own slope standard error (converted to
    kcal/day via ×7700). Logged intake's own systematic bias (photo portion estimation) is not
    quantifiable and is surfaced as caveat text, not folded into the number.
  - **Silence when the interval covers zero**: no Logging Gap is shown, only "not enough data yet" —
    this is the safety mechanism that replaces the sanity clamp the original framing proposed, and it
    means no separate minimum-days threshold beyond the hard floor is needed (thin data produces a
    wide interval, which covers zero on its own). A hard floor still applies below 2 weigh-ins or
    fewer than 3 valid days in the window (matching `food-day-completeness`'s existing coverage-
    contract precedent for this category of feature): nothing is even attempted.
  - **Outlier rejection**: a raw weigh-in is excluded from the trend regression when its implied
    rate of change from the previous *kept* weigh-in exceeds 2 kg/day — physiologically impossible,
    not a subtler judgment call (those are left to the interval, which widens on its own around a
    wild point). The card notes when a reading was excluded; the reading itself is never altered or
    hidden elsewhere (chart, database).
  - This capability computes entirely client-side (reusing the existing `emaSeries`/
    `linearRegression` pair from Phase 2's weight-trend work) — no new persistence, no new backend
    computation beyond the one endpoint below.
- **A new backend endpoint**, `GET /api/food/daily-totals` (`food-meal-history` capability): per-day
  summed calories over the caller's `confirmed` meals, for a date range (calories only — no
  consumer of this change reads protein/carbs/fat, so those are left out rather than pre-built) —
  the server-side aggregation `food/history/page.tsx` currently does client-side, generalized so the
  Logging Gap Card (and, per the idea's own note, Phase 4 later) don't each re-fetch and re-sum raw
  meal rows themselves. The existing history page is not migrated to it in this change — it already
  has the rows loaded for its own display and has no reason to make a second round trip.
- **A new dashboard Food Card** — the Logging Gap Card — registered in the existing card
  order/visibility mechanism (ADR-001) alongside the 8 Vital Cards, hideable and defaulting to
  visible like every other card. This is the mechanism's first Food Card, so the registry (today
  `{ type: DataType }[]`, strictly one entry per presence-gated metric) needs a small generalization
  to admit a card with no backing `DataType` and no presence gate — this card's "nothing to show yet"
  states are internal content, not something the presence system hides for it.

## Not Changing

- **ADR-006's steps-inferred activity multiplier** — untouched. This feature reads the Nutrition
  Target's output; it never overrides or replaces it. Idea #10's original "replaces the activity
  multiplier" framing does not survive the grilling and is explicitly abandoned.
- **No TDEE is computed or stored anywhere**, and neither "TDEE" nor "total calories" is used as a
  name in code, API, or UI — `total_calories` already exists as an unrelated table (exercise
  calories) and colliding with it would be a durable trap (see the grilling comment, correction 3).
- **`food-day-completeness`'s own gate is not changed.** This feature consumes its states as-is; it
  does not add a calorie-plausibility check to that capability, which would be new scope for #9, not
  this change.
- **The food history page's client-side daily-total summation** — left as is; see "What Changes"
  above.

## Capabilities

### New Capabilities
- `logging-gap`: the Implied Intake / Logging Gap computation, its uncertainty interval and silence
  rule, outlier rejection, and the Logging Gap Card's content states.

### Modified Capabilities
- `food-meal-history`: adds the `GET /api/food/daily-totals` endpoint.
- `dashboard-ui`: generalizes the card order/visibility registry to admit a Food Card with no
  backing `DataType`, and registers the Logging Gap Card into it.

## Impact

- Backend (Go): a new handler file alongside `backend/pkg/server/food_completeness.go` (e.g.
  `food_daily_totals.go`), a new route in `backend/pkg/server/server.go`. No schema change — reads
  existing `FoodMeal` rows.
- Frontend (TS/React): `frontend/lib/api.ts` (new `getFoodDailyTotals` client call, `DailyTotal`
  type), a new `frontend/lib/loggingGap.ts` (outlier rejection, interval, silence rule — pure
  functions, reusing `emaSeries`/`linearRegression` from `frontend/lib/dataTypeMeta.ts`),
  `frontend/lib/vitals.ts` (registry generalization), a new `LoggingGapCard` component, `app/page.tsx`
  (renders the new card, wires it into Edit mode).
- Docs: `CONTEXT.md` (new glossary entries — Implied Intake, Logging Gap, Trend Weight, Food Card
  generalization note), new `docs/adr/ADR-008-<slug>.md`, `todo.md` (record Phase 4's prerequisite
  work this reuses, and that idea #10 shipped as this instead of adaptive TDEE).
- No implementation code ships in this Attempt — see the repo's `openspec/` workflow; this is the
  proposal an implementer's own pass will build from.
