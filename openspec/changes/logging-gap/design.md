## Context

Full background, evidence, and the grilled decision this change transcribes live on the idea-forge
issue for idea #10 (the body, then the "Grilled — the feature changed shape" comment, 2026-08-25,
which supersedes the body). Two things carry forward from that comment that this design does not
re-litigate: the feature reports a **Logging Gap**, not a TDEE, and it never touches ADR-006's
activity multiplier. What follows settles the four questions the comment explicitly left open for
`opsx:propose` ("Still open" section), plus the concrete shapes needed to build it.

## Goals / Non-Goals

**Goals:**
- Compute, from data the app already has, a kcal/day figure describing how much of the weight
  trend's implied intake isn't showing up in the food log — with an honest interval, not a false
  point estimate.
- Say nothing rather than something implausible: silence is the safety mechanism, not a clamp.
- Reuse Phase 2's EMA/regression machinery and `food-day-completeness`'s state machine rather than
  building parallel versions of either.
- Settle the four explicitly-open questions from the grilling comment (below).

**Non-Goals:**
- No TDEE, no replacement of ADR-006's activity multiplier.
- No new `food-day-completeness` behavior — its states are read, not extended.
- No backend port of the regression/interval math. It stays client-side, next to the EMA/regression
  functions it reuses, exactly where Trend Projection's equivalent computation already lives.
- No migration of the food history page's own client-side totals to the new endpoint.

## Decisions

### 1. Window: 28 days, ending yesterday — not the last weigh-in

The grilling comment's open question #4 asked whether the window ends at *today* or at the *last
weigh-in* (which, at grilling time, was 5 days stale). **Decision: yesterday**, in the caller's
Logged Day sense (today is always in progress and excluded, matching `food-day-completeness` and
ADR-006's own trailing-window convention) — not the most recent weigh-in.

Anchoring at "today" is the one convention every other trailing window in this app already uses
(steps, day completeness); anchoring at "last weigh-in" would make the window's own length and
placement a function of how stale the data happens to be, which is exactly the kind of special case
decision 3 (below) exists to avoid needing. A stale last weigh-in doesn't need its own gate: it
just means the regression's most recent EMA point is several days old, so the fitted line's
uncertainty over the remaining days is larger, which decision 4's interval already accounts for
without any additional logic. No separate "weight is N days stale" check is added.

### 2. Outlier rejection: rate-based, not a flat per-point delta

The grilling comment's decision 4 says "departs from the smoothed trend by more than roughly 2
kg/day." Weigh-ins are sparse (22 across 90 days, mean gap 3.33 days per the comment's correction
4), so a flat "differs from the trend by 2 kg" comparison would flag ordinary short-term water
weight on any multi-day gap. **Decision: rate-based**, evaluated against the previous *kept* raw
weigh-in, in chronological order:

```
reject w[i] if |w[i].value - lastKept.value| / (w[i].day - lastKept.day) > 2.0 kg/day
```

A rejected point does not become the new `lastKept` — a single bad reading can't cascade-reject
everything after it, and a genuine fast change spread over several real weigh-ins is never
misread as one outlier. This runs once, before EMA smoothing, over the raw `weight` records inside
the (lead-in-extended, see decision 4) window. Rejected points stay in the database and on the
weight chart unchanged — this rejection is local to this capability's own regression input, exactly
as the grilling comment specifies ("keep it in the database and on the chart, and say in the card
that a reading was excluded").

### 3. Interval: quadrature of Nutrition Target's fixed error and the trend slope's standard error

The grilling comment's open question #2 recommends but doesn't fix deriving the interval from
regression residuals. **Decision:**

```
formulaError = 0.10 * nutritionTarget.calories        // fixed assumption, see below
trendErrorKcal = 7700 * SE(slope)                      // slope in kg/day, from the 28-day OLS fit
interval = sqrt(formulaError^2 + trendErrorKcal^2)
```

`SE(slope)` is the standard OLS slope standard error over the same `(day_offset, ema_value)` points
`linearRegression` already fits: `sqrt(Σresiduals² / (n-2)) / sqrt(Σ(x-x̄)²)`, computed from the EMA
points falling inside the last 28 calendar days (decision 4 below). This needs `n ≥ 3` distinct EMA
days to be defined (`n-2 ≥ 1`); with exactly `n = 2`, the fit is a perfect line through both points
(zero residual, zero degrees of freedom) and the standard error is genuinely undefined, not zero.
**When `n < 3`, `trendErrorKcal` is treated as unbounded** — the interval always covers zero and the
card falls through to the silent "not enough data yet" state (decision 5). This is what turns the
grilling comment's literal floor ("two weigh-ins ... below which there is nothing to compute") into
a real number: 2 weigh-ins is the point below which the regression itself has no slope to report at
all (see decision 6's hard floor); with exactly 2 or few distinct EMA days, a slope exists but its
uncertainty does not, and the interval mechanism suppresses output the same way it would for any
other under-determined case.

`formulaError` is a fixed 10% of the Nutrition Target's `calories` figure — the Mifflin-St Jeor
accuracy claim already cited in idea #10's body ("within ±10% for roughly three-quarters of
people"). This is a constant, not derived from anything the Nutrition Target endpoint returns (it
returns no error bars) or from this feature's own data. Logged intake's own bias (photo portion
estimation) has no quantified error and is deliberately left out of this formula — see "Risks"
below for how it's surfaced instead.

### 4. Trend Weight: reuse Phase 2's EMA, lead-in window matches Trend Projection's own pattern

`Trend Weight` is `emaSeries` (alpha 0.25, `frontend/lib/dataTypeMeta.ts`) over the
outlier-filtered, day-bucketed `weight` series — the same function and the same alpha Trend
Projection already uses, so "Trend Weight" means the same thing everywhere the app shows it. Like
Trend Projection, the raw fetch widens the window with lead-in days before the visible 28-day range
so the EMA has converged by the first day that matters, then `linearRegression` fits
`(day_offset, ema_value)` over exactly the last 28 calendar days of that series (28, not Trend
Projection's 30 — matching decision 1's ADR-006-aligned window, per the grilling comment's own
rejection of a second window).

### 5. Silence rule and hard floor

Two distinct gates, evaluated in order:

1. **Hard floor** (nothing is even attempted): fewer than 2 raw weigh-ins survive outlier
   rejection inside the 28-day window, or fewer than 3 days in the window are Complete or Confirmed
   Complete (`food-day-completeness`). The card renders "not enough data yet" without computing a
   slope, Implied Intake, or interval at all.
2. **Statistical silence** (a Logging Gap is computed, but suppressed): `|Logging Gap| < interval`,
   i.e. the range `[Logging Gap - interval, Logging Gap + interval]` includes zero. Same visible
   text as the hard floor — the grilling comment specifies one surface string for both, since the
   distinction (too little data vs. enough data but inconclusive) isn't something a user needs to
   act on differently.

**The valid-day floor is 3, not "at least one."** `food-day-completeness`'s own "Downstream
coverage contract" requirement already sets a 3-of-7-Logged-Day minimum for exactly this category of
feature — it names "an adaptive-TDEE computation" as its example. That contract's 7-day window
doesn't literally scope to this feature's 28-day one, so it isn't binding as written, but its
valid-day floor is adopted here rather than re-derived: decision 3's interval has no term for Mean
Logged Intake's own sampling variance across the valid days it averages, only for the Nutrition
Target's fixed error and the trend slope's standard error. A single Complete day that happens to be
plausible-but-atypical would not necessarily widen the interval at all, so gate 2 cannot be relied on
alone the way it can for thin weigh-in data. 3 matches the established precedent rather than being
independently derived, and does not fully close the gap — see "Risks."

### 6. Nutrition Target unavailable (422)

`GET /api/users/me/nutrition-target` can 422 (`missing_profile`, `missing_measurements`,
`missing_goal_weight`, `insufficient_activity_data`) — a precondition failure, not a data-volume
problem. The card SHALL show a distinct state from "not enough data yet" for this case — pointing at
completing the profile/goal weight (Phase 3), since silently reusing the same string would tell a
user with a perfectly good food log to "wait for more data" when the actual blocker is a form they
haven't filled in.

### 7. Backend: `GET /api/food/daily-totals`

New endpoint, `food-meal-history` capability, modeled directly on
`GET /api/food/completeness`'s existing shape (same 92-day cap, same `to`-clamped-to-yesterday-first
validation order, same caller-only scope, no `?user=` override):

```
GET /api/food/daily-totals?from=YYYY-MM-DD&to=YYYY-MM-DD
[{"date": "2026-08-19", "calories": 1753}, ...]
```

One entry per day in the resolved range, summed over that day's `confirmed`-status `FoodMeal` rows
only (unconfirmed/failed/processing meals have no final nutrition numbers, same rule the history
page's own totals already follow) — a day with none gets a zero, not an omitted entry, so the
Logging Gap computation can index by date without a presence check first.

`calories` only, not protein/carbs/fat: the only consumer this change builds (the Logging Gap
computation) reads `calories` alone. Macro fields are left out rather than pre-built for a Phase 4
consumer that isn't designed yet — the same restraint decision 8 already applies to the card
registry ("this union grows again then — not preemptively solved here"). Widen the response shape
when a concrete consumer needs them.

**Why a new endpoint instead of reusing `GET /api/food/meals`:** that endpoint pages raw meal rows;
computing a 28-day (or, with lead-in, wider) window of daily sums from it means fetching and paging
through every meal row in range client-side purely to re-derive a sum the server already has all the
inputs for. The idea's own body flags this as needed regardless of this feature ("Phase 4 likely
needs this too") — built once here, deliberately, rather than as a one-off inline query.

### 8. Dashboard registry generalization

Today `PRIMARY_METRICS: { type: DataType }[]` (`frontend/lib/vitals.ts`) and
`DashboardCardPref { type: DataType; hidden: boolean }` assume every registered card is backed by a
`DataType` with a presence signal. The Logging Gap Card is neither — it has no `/api/data/{type}`
presence to gate on; its "nothing to show" cases are its own internal content states (decision 5,
decision 6), not something the presence system decides for it.

**Decision:** widen the registry's card identifier from `DataType` to `DataType | 'logging_gap'`
(a `CardId` union), and change `PRIMARY_METRICS`'s entries and `DashboardCardPref.type` to that
union. `reconcileMetricOrder` and the Edit-mode reorder/show-hide list treat `'logging_gap'` exactly
like any `DataType` entry for ordering/visibility purposes; the one place they diverge is the
existing "Presence-based visibility" gate (`hasPresence`, and the `SECONDARY_TYPES` /
zero-presence-exclusion logic in `app/page.tsx`), which SHALL NOT be evaluated for `'logging_gap'` —
it is always eligible to render (subject only to the user's own hidden/visible choice), the same way
it is always included in Edit mode's card list regardless of whether it currently has anything to
show. This is a small, additive widening — every existing `DataType`-backed card's behavior is
unchanged — and it's the shape Phase 4's own Food Cards (todo.md: "Card A", "Card B") will need too,
so it's worth generalizing correctly here rather than hardcoding a single-card special case that
gets redone at Phase 4.

## Risks / Trade-offs

- **The interval formula's two terms are not independently validated** — `formulaError` is a fixed
  literature figure, not measured from this app's own users, and `SE(slope)` assumes the EMA series'
  residuals are a reasonable proxy for weight-trend uncertainty (they conflate genuine metabolic
  variability with EMA smoothing lag). Accepted: the alternative is inventing a more elaborate model
  with no more real validation behind it. The combined "roughly ±310" figure from the idea's own
  grilled evidence table is in the same range this formula produces given that data, which is the
  extent of validation available before shipping.
- **The interval has no term for Mean Logged Intake's own sampling variance.** Both of the interval's
  terms come from the Nutrition Target formula and the weight-trend regression; neither scales with
  how many valid (Complete/Confirmed Complete) days fed Mean Logged Intake, nor with how much
  day-to-day variance exists among them. Decision 5's 3-valid-day floor (raised from "at least one"
  to match `food-day-completeness`'s existing Downstream Coverage Contract precedent) narrows this
  gap but does not close it: 3 atypical-but-plausible days can still average to a Mean Logged Intake
  the interval doesn't widen to compensate for. A more complete fix — adding a standard-error-of-the-
  mean term over the valid days' logged totals, combined in the same quadrature — is deliberately
  left as a follow-up rather than done here, since it needs real usage data to validate rather than
  another unmeasured constant.
- **Logged intake's photo-estimation bias is not in the interval at all**, by design (decision 3) —
  the card's copy SHALL disclose this as a caveat ("logged calories are estimated from photos and may
  run high or low") rather than implying the shown range accounts for it. A user reading only the
  number without the caveat could over-trust it; this is a known, accepted gap in what the number
  can promise.
- **Rate-based outlier rejection can still reject a real fast change** if a single weigh-in swings
  more than 2 kg/day from the last kept one (e.g. a genuine post-restriction refeed). Accepted per
  the grilling comment's own framing: this rule exists only for the "physiologically impossible"
  case, and anything subtler is intentionally left to the interval mechanism instead of a more
  elaborate rejection rule.
- **The registry generalization (decision 8) is minimal, not a general "card kind" abstraction.**
  It adds exactly one non-`DataType` member to the union rather than building a pluggable card-kind
  system for cards not yet designed (Phase 4's). If Phase 4 needs a third shape entirely (not just
  "no presence gate"), this union grows again then — not preemptively solved here.

## Migration Plan

Purely additive: one new backend endpoint (no schema change), one new frontend module, one small
registry-type widening that is a strict superset of today's `DataType`-only shape (existing
`dashboard_order` values already saved by users remain valid `DashboardCardPref` entries
unchanged). No existing endpoint's response shape changes. Reverting the frontend stops rendering
the card and the registry entry is simply never added to `PRIMARY_METRICS`; reverting the backend
leaves `GET /api/food/daily-totals` unused with no other endpoint depending on it.
