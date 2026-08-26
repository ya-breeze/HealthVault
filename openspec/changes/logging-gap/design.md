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
decision 3 (below) exists to avoid needing. Because the window is anchored to yesterday rather than
to the last weigh-in, a stale last weigh-in means fewer EMA points fall inside the trailing 28-day
window as it keeps sliding forward while the point set does not — which reduces both `n` and
`Σ(x-x̄)²` in decision 3's `SE(slope)` formula. This tends to widen the interval, but it is not a
guarantee: `SE(slope)` also depends on `Σresiduals²`, which shrinks independently of how much of the
window the points span. A densely, near-linearly logged series that stops several days before the
window's end can have residuals close to zero regardless of staleness, so a shrinking point set does
not reliably widen the interval enough on its own to trigger the silence rule — the reduced-spread
effect is a real but insufficient safeguard by itself.

**A separate, explicit freshness gate is therefore added** to decision 5's hard floor: if the most
recent kept weigh-in inside the window is more than 7 days before the window's last day, the system
reports "not enough data yet" without computing a slope, regardless of how narrow the regression's
own `SE(slope)` would otherwise be. 7 days reuses the cadence already established elsewhere in this
exact requirement — `food-day-completeness`'s Downstream Coverage Contract's own window — rather than
inventing a new constant; it is roughly twice the ~3.33-day mean weigh-in gap idea #10's grilling
found in real data, so it does not fire on ordinary cadence, only on a genuine multi-day gap between
the last weigh-in and today. This is the concrete "days since last weigh-in" check the paragraph
above deliberately avoided computing directly for the *window's own placement* — it is scoped
narrowly to gating output, not to moving the window, so it doesn't reopen the special-casing this
decision otherwise avoids.

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

**Same-day records:** the day-gap denominator is undefined (zero) when a candidate shares its
calendar day with `lastKept` (a re-weigh, or two synced sources landing on the same day). The rule
only rate-checks a candidate against `lastKept` when they fall on different calendar days; a
same-day candidate is never evaluated against `lastKept` and is always provisionally kept, flowing
into decision 4's day-bucketing step, which collapses same-day raw values (e.g. averaging them)
before the EMA and regression ever see them. A same-day duplicate therefore can't itself be
rejected by this rule — only diluted into that day's bucket — which is an accepted trade-off for
keeping the same-day case well-defined instead of dividing by zero.

**`lastKept` is unaffected by same-day-exempted candidates.** Because a same-day candidate is never
rate-checked, it also never updates `lastKept` — the reference used for the next different-day
candidate remains whichever record last passed a rate check (or the bootstrap-selected initial
anchor), never whichever same-day record happened to be walked most recently. Without this rule,
a same-day pair's second record — arbitrary in ordering, and, per the trade-off above, possibly the
wrong one of the pair — could silently become the anchor every later record is rate-checked
against, reintroducing exactly the order-dependent failure the initial-anchor bootstrap below
exists to close for the leading case. So `lastKept`'s value depends only on records that have
actually passed a rate check (or the bootstrap), never on one that was merely exempted from one.

**The first record is not assumed trustworthy.** Seeding `lastKept` from the window's first raw
record unconditionally means a single garbage leading reading (a fat-fingered entry, e.g. 75 kg
next to a true ~91 kg series) becomes the reference every subsequent good reading is rate-checked
against — and a rejected record never becomes the new `lastKept`, so good readings can be wrongly
rejected for as many days as it takes the accumulating calendar gap to bring the rate back under
2.0 kg/day on its own. Before starting the walk, the first two raw candidate records that fall on
different calendar days (chronologically, before day-bucketing — bucketing runs once, only after
this entire rejection walk finishes, never partway through; a same-day pair is skipped here the same
way the main walk skips it) are rate-checked against each other the same way; if they disagree, the
earlier one is dropped (rejected, as above) and the check repeats against the next different-day
record until two consecutive different-day records agree or only one remains, which becomes the
initial `lastKept`. This is the existing rate rule applied to bootstrap the anchor, not a new
statistical method — it costs one extra comparison and closes the one case where this algorithm's
order-dependence has an unbounded (rather than self-correcting) failure mode.

**A residual limitation of the same-day exemption — closed by suppression, not re-validation:** a
same-day sibling of a rejected candidate is never rate-checked against whatever record actually
displaced it (per the same-day rule above), so it could otherwise reach the EMA without ever having
been validated against the point next to it in the bucketed series. E.g. 75.0 and 75.2 both on day 1
(75.0 rejected as the bootstrap candidate, 75.2 kept as its same-day-exempt sibling), 77.3 on day 2,
77.825 on day 3: day 1's surviving bucket value (75.2) is 2.1 kg/day from day 2's (77.3) — over this
rule's own 2.0 kg/day threshold — yet that pair was never rate-checked, because the bootstrap
comparison that mattered ran between 75.0 and 77.3 (and then 77.3 against 77.825), never between
75.2 and 77.3. With exactly the 3 EMA days this example has — the minimum decision 3 needs for
`SE(slope)` to be defined at all (`n ≥ 3`) — 3 points can land close to collinear by chance
regardless of whether one slipped through this way, so `SE(slope)` is not guaranteed to be large
enough to widen decision 3's interval past decision 5's silence threshold. The
interval-widening/statistical-silence backstop is therefore not a reliable bound for this specific
mechanism, unlike the framing this design otherwise applies to same-day dilution. This is a
different gap from the cluster-ambiguity one below (that one needs a *persistent*, multi-reading
second cluster; this one needs only a single same-day pair adjacent to a single rejection, plus a
short run of later readings that happen to line up with the surviving sibling).

Re-validating the sibling against the record that displaced its rejected candidate would reopen the
order-dependence problem the same-day rule exists to avoid, so that is not how this is closed.
Instead, the spec's outlier-rejection requirement adds a **same-day-sibling suppression rule**:
whenever the bootstrap rejects a candidate that has one or more same-day-exempted siblings, the
window is treated as ambiguous and the hard floor (decision 5) applies, the same way the rejection
cap does. The trigger is purely structural — "did the bootstrap reject a candidate with a surviving
same-day sibling" — never a comparison between the sibling and any other record, so no new
record-to-record check, and no order-dependence, is introduced. The cost is a small amount of
over-suppression: a same-day pair adjacent to a bootstrap rejection always goes quiet, even on the
(presumably common) occasion the surviving sibling would have been perfectly fine. That trade is
consistent with this design's general preference for silence over an unvalidated number.

**This is detected by an explicit flag from the rejection walk, not inferred from the `kept`/
`rejected` arrays.** Two things follow from that: the flag is set (or not) once, during the
bootstrap step of the walk over the whole lead-in-extended range, so it is unaffected by the
visible-window filtering the rejection cap applies to `rejected` — a bootstrap rejection and its
same-day sibling are typically both in the lead-in (the bootstrap runs on the chronologically
first records), and the flag still needs to fire even though neither record's day is in the
`windowStartDayOffset`-filtered arrays the rejection cap counts from. And the flag is scoped to
*this specific* mechanism (a rejection produced by the bootstrap step, with a same-day sibling),
not a general "some calendar day has both a kept and a rejected record" test: the ordinary main
walk can independently produce a kept record and a rejected record on the same calendar day with
no bootstrap or same-day-sibling relationship between them at all (e.g. `lastKept` on day 1 at
91.0; day 2 has two different-day-from-`lastKept` records, 94.0 and 91.2 — 94.0 is rate-checked
against 91.0 and fails, 91.2 is rate-checked against 91.0 and passes, independently of each other
and of the bootstrap). Treating "day appears in both arrays" as a proxy for the same-day-sibling
suppression condition would misfire on that ordinary case, so the walk reports the specific
condition directly instead of leaving callers to reconstruct it from the two arrays.

**A residual limitation of this bootstrap:** a purely causal, local pairwise walk cannot always
identify which of two mutually-disagreeing but internally-consistent leading readings is the
genuine one — e.g. 91, 75, 75.2, 91.2 rejects the true 91 as "the earlier of the disagreeing pair,"
anchors on 75/75.2, then rejects the true 91.2 too, since no lookahead beyond the immediate next
record is used. This is not fixable by a smarter tie-break in general: with only this data, the
choice between the two clusters is genuinely ambiguous to any online algorithm, real or not.

The two mechanisms this design otherwise relies on elsewhere — a stale anchor's growing calendar
gap eventually widening the rate check's tolerance, and a mixed-cluster fit producing large OLS
residuals — only bound this failure when the wrongly-anchored cluster eventually stops producing
fresh readings, or when at least one genuine reading survives long enough to reach the regression.
Neither is guaranteed: the motivating shared-scale scenario (this decision's own justification for
having an outlier rule at all) is exactly a case where a second, wrong cluster can keep producing
fresh, internally-consistent daily readings indefinitely. In that case `lastKept` advances with the
wrong cluster, every genuine reading is rejected on arrival for as long as the wrong cluster keeps
reporting, and no mixed window ever reaches the OLS fit to widen the interval. The 4-point example
above happens to fall below decision 5's hard floor (only 2 raw records survive), so it resolves
safely, but that outcome does not generalize to a longer-running wrong cluster.

**The actual bound is an explicit rejection cap, not the two mechanisms above.** If outlier
rejection discards more than 3 raw records within the 28-day window itself (excluding the lead-in
extension), the system treats the window's weigh-in data as ambiguous and applies decision 5's hard
floor ("not enough data yet") rather than compute a trend from whichever cluster survived. A single
bad leading or mid-series reading — the case this rule exists for — produces at most 1-2 rejections
(the bad reading itself, plus at most one bootstrap rejection); a sustained two-cluster disagreement,
by contrast, rejects one entire side of the disagreement for as long as it persists. This converts
the otherwise-unbounded failure mode into the same silence this design already relies on everywhere
else, rather than resting on two mechanisms that only bound it when the wrong cluster happens to be
transient.

**A residual limitation of the cap itself:** in the persistent two-cluster case, only the
non-`lastKept` cluster's readings are ever rejected — the cluster currently anchoring `lastKept` is
internally consistent (near-zero rate against itself) and is never rejected, no matter how dense it
is. The cap therefore only fires when the *other* cluster contributes more than 3 different-day
readings within the 28-day window. This holds comfortably for the cadence this design otherwise
assumes (the ~3.33-day mean weigh-in gap cited above yields roughly 8 readings per 28 days, well
past the cap), but a genuine reader logging sparsely enough to fall at or under roughly one reading
per 7 days — e.g. every 10 days, ~2-3 times in a 28-day window — never crosses the cap even while a
denser wrong cluster persists throughout. In that case the wrong cluster reaches the OLS fit
undisturbed, dense and near-linear, and can produce a confidently narrow, wrong Logging Gap. As with
the bootstrap's own residual limitation above, this is not fixable by a smarter purely-causal,
single-window rule in general: distinguishing which of two internally-consistent clusters is genuine
needs information this rejection rule doesn't have (e.g. the caller's weight history before the
window). Accepted for the same reason as the bootstrap case and the "Risks" section below: it is
a known, disclosed gap rather than a solved one.

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

The 10% figure is cited for Mifflin-St Jeor's BMR estimate specifically, but it is applied here to
the Nutrition Target's *final* `calories` figure, which is BMR times ADR-006's steps-inferred
activity multiplier — the multiplier's own error is not separately quantified anywhere and is not
added as a third term. See "Risks" below.

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
   rejection inside the 28-day window, or more than 3 raw weigh-ins inside the 28-day window were
   rejected by outlier rejection (decision 2's rejection cap), or the bootstrap rejected a
   candidate that had one or more same-day-exempted siblings (decision 2's same-day-sibling
   suppression rule — this is a structural fact about the rejection walk over the whole
   lead-in-extended range, not scoped to the visible window, since the bootstrap runs on the
   chronologically first records and those are typically in the lead-in), or fewer than 3 days in
   the window are Complete or Confirmed Complete (`food-day-completeness`), or the most recent kept
   weigh-in inside the window is more than 7 days before the window's last day (decision 1's
   freshness gate). The card renders "not enough data yet" without computing a slope, Implied
   Intake, or interval at all.
2. **Statistical silence** (a Logging Gap is computed, but suppressed): `|Logging Gap| <= interval`,
   i.e. the closed range `[Logging Gap - interval, Logging Gap + interval]` includes zero — a gap
   exactly equal to the interval has zero sitting on the range's own boundary, which counts as
   covering, so it is suppressed too (see
   spec.md's boundary scenario). Same visible text as the hard floor — the grilling comment
   specifies one surface string for both, since the distinction (too little data vs. enough data
   but inconclusive) isn't something a user needs to act on differently.

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

**Loading and non-422 fetch failures.** The card issues four requests of its own (weight,
Nutrition Target, Day Completeness, Daily Totals) rather than reading from `app/page.tsx`'s shared
`dataMap`, so unlike a `VitalCard` it has its own fetch lifecycle. Two things follow:

- **While any of the four requests is in flight**, the card SHALL render a loading state distinct
  from all four content states (matching the dashboard's own `settingsStatus: 'loading'` pattern
  in `app/page.tsx`) — so it never flashes "not enough data yet" before its first fetch resolves.
  This part does match existing dashboard convention.
- **A non-422 failure from any of the four requests** (network error, 5xx, or a Nutrition Target
  error other than the 422 handled by decision 6) SHALL render a distinct "temporarily unavailable"
  state — never "not enough data yet". A 401 specifically never reaches this state: `apiFetch`
  (`frontend/lib/api.ts`) already intercepts every 401 transparently — refreshing the session and
  retrying, or routing to login on refresh failure — before any caller, including this card, sees
  the response.

  An earlier version of this decision folded non-422 failures into "not enough data yet", reasoning
  from every existing Vital Card's `.catch(() => [])` convention (`app/page.tsx`) and the claim that
  an outage would surface as "every card on the dashboard going quiet." That reasoning does not hold
  for this card specifically: unlike a `VitalCard`, the Logging Gap Card does not read from
  `app/page.tsx`'s shared `dataMap` — it issues its own four requests (decision 7's new endpoint plus
  three existing ones) — so a failure isolated to one of them (e.g. Daily Totals returning 500 while
  weight, Nutrition Target, and Day Completeness all succeed) has no reason to correlate with any
  other card's state, and could persist indefinitely while every other card on the dashboard keeps
  working normally. Telling the user to "gather more data" when the actual problem is a broken
  endpoint is misleading, not merely inconsistent with existing convention, so this card gets its own
  minimal error state instead: no retry logic and no distinction between which of the four requests
  failed, just one generic string ("temporarily unavailable, try again later") distinct from both
  "not enough data yet" and the 422 profile-completion state.

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
- **`formulaError` stands in for the whole Nutrition Target's error, not just the BMR component it
  was measured for.** The cited ±10% is Mifflin-St Jeor's own accuracy claim for the *BMR* estimate;
  the Nutrition Target this feature reads is BMR times ADR-006's steps-inferred activity multiplier,
  and that multiplier's own error has never been quantified (steps-inference is itself an estimate,
  not a measurement of activity). A systematic multiplier error — the target running persistently
  high or low for a given user — would inflate or shrink the Logging Gap without any corresponding
  change to `formulaError`, and could present as unlogged intake that is really activity
  mis-estimation. Accepted for now for the same reason as the bullet above: quantifying the
  multiplier's error needs real usage data this app doesn't have yet. This is disclosed to the user,
  not just here — spec.md's "Logging Gap Card content and placement" requirement makes the
  activity-multiplier caveat static card copy alongside the photo-estimation one, since a limitation
  documented only in a design doc a user never sees does not make the shown range honest to them.
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
- **The rejection cap (decision 2) trades a rare false silence for closing the unbounded
  false-confidence failure mode only when the genuine cluster logs often enough.** A genuinely
  volatile but real weigh-in history — more than 3 physiologically implausible day-to-day swings in
  one 28-day window, without a second wrong cluster involved — would trip the cap and report "not
  enough data yet" rather than compute a trend; this is accepted as the same trade every other
  silence rule in this design makes. But the cap does not close the failure mode in the other
  direction: a persistent wrong cluster paired with a genuine cadence sparse enough that fewer than 4
  of its readings fall in the 28-day window (see decision 2's own "residual limitation of the cap")
  never trips the cap at all, and can produce a confidently narrow, wrong Logging Gap indefinitely.
  This is accepted for the same reason as the bootstrap's residual limitation: closing it needs
  information (the caller's weight history before the window) this rule doesn't use, and the
  motivating scenario (a second person occasionally sharing a scale) is expected to be rare in this
  app's single/small-user context.
- **The same-day exemption (decision 2) could otherwise let a rejected candidate's sibling reach the
  EMA unvalidated against the record that displaced it** (see decision 2's own "residual limitation
  of the same-day exemption"). Unlike the other two residual limitations above, this one is closed:
  the same-day-sibling suppression rule fires the hard floor whenever a bootstrap rejection has a
  surviving same-day sibling, without re-validating the sibling against anything and without
  reopening the order-dependence problem the same-day rule exists to avoid. The trade is a small
  amount of over-suppression — a same-day pair adjacent to a bootstrap rejection always goes quiet,
  even when the sibling would have been fine — accepted as consistent with this design's general
  preference for silence over an unvalidated number.
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
