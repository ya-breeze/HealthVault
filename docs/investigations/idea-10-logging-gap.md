# Investigation: Adaptive TDEE from energy balance (idea #10)

Research notes for idea-forge plan
`idea-10-healthvault-adaptive-tdee-from-energy-ba-investigate.md`. Filed as "adaptive TDEE
replacing the activity multiplier," then reshaped by two `/grilling` passes against the production
database (2026-08-23 and 2026-08-25, both recorded as comments on the idea-forge issue and quoted
in full inside the plan file).

## Outcome

This investigation concluded in a proposed OpenSpec change, `openspec/changes/logging-gap/`
(`proposal.md`, `design.md`, spec deltas for a new `logging-gap` capability plus modifications to
`food-meal-history` and `dashboard-ui`, and an implementer `tasks.md` with every box left unticked).
`openspec validate logging-gap --strict` passes. No production code was written or wired up — per
this repo's spec-first workflow, that is deliberately out of scope for this Attempt and is left for
a later, separate implementation pass against the approved spec.

The shipped framing is **not** the idea as originally filed. The second grilling pass killed
"adaptive TDEE replacing the activity multiplier" outright: the database has no third source of
expenditure to validate a computed metabolism against (`total_calories` turned out to be exercise
calories, not TDEE, despite the name), and even after `food-day-completeness` (#9) — which shipped
and merged between the two grilling passes — its own gate only removes the *worst* noise; a 922
kcal day for a 91 kg man still passes as "Complete." The chosen design instead reports the **gap
between what was logged and what the weight trend implies was eaten** — a diagnostic for the food
log, not an assertion of measured metabolism. ADR-006's steps-inferred activity multiplier is
untouched; this is a new, independent Food Card that reads the existing Nutrition Target's output
and never overrides it.

The design's four judgment calls (window anchor, outlier rule, interval formula, and where the new
daily-totals endpoint lives) are settled in `design.md`'s eight numbered decisions, each pinned to
which of the grilling comment's four explicitly-open questions it resolves.

## Relevant code

- `backend/pkg/server/nutrition_target.go` — `GET /api/users/me/nutrition-target` is already merged
  (Phase 3) and is the Implied Intake formula's input. It 422s with ordered reasons
  (`missing_profile`, `missing_measurements`, `missing_goal_weight`,
  `insufficient_activity_data`) — a precondition failure the design gives its own card state
  (decision 6), distinct from "not enough data yet."
- `backend/pkg/server/activity_level.go` — the existing trailing-28-day-window pattern (ADR-006's
  steps inference) this feature's window (decision 1) and, if ported server-side later, its
  computation shape are modeled on.
- `backend/pkg/{server,database}/food_completeness.go` — `food-day-completeness` (#9) is fully
  implemented and merged: `GET /api/food/completeness`, confirm/retract endpoints, Complete /
  Confirmed Complete / Unconfirmed / Incomplete states. This feature reads those states to decide
  which days' logged intake counts (Complete/Confirmed Complete only — others excluded, not
  averaged in as zero) but adds no new logic to that capability.
- `frontend/lib/dataTypeMeta.ts` — `emaSeries` (alpha 0.25) and `linearRegression`, Phase 2's
  trend-weight machinery, already wired into the weight chart page. This feature reuses both
  functions directly (design.md decision 4) rather than reimplementing trend smoothing.
- `frontend/app/food/history/page.tsx` — sums per-day totals **client-side**, over `confirmed`
  status meals only. No backend aggregation endpoint exists yet; the proposed
  `GET /api/food/daily-totals` (design.md decision 7) mirrors this exact rule server-side but does
  not migrate the history page onto it — it already has the rows loaded for display.
- `backend/pkg/server/api.go` — the metric-type registry, where `total_calories`, `active_calories`,
  and `basal_metabolic_rate` are confirmed as real, registered types backed by Health Connect
  import data (exercise/BMR figures), unrelated to expenditure estimation. Naming anything in this
  change "TDEE" or "total calories" would collide with live, meaningfully-different data — the
  proposal and design both call this out explicitly as a naming trap to avoid.
- `frontend/lib/vitals.ts` — `PRIMARY_METRICS: { type: DataType }[]` and `DashboardCardPref { type:
  DataType; hidden: boolean }` (ADR-001's card registry). Scoped today to the 8 vitals-grid metric
  types only; placing a new Food Card here needs the registry's card identifier widened to `DataType
  | 'logging_gap'` (design.md decision 8) — not a drop-in.

## Constraints and unknowns

- **The blocker named in the idea's body is already resolved in code, but not in the way the body
  assumed.** `food-day-completeness` shipped and merged between the idea being filed and this
  investigation. It fixes the *worst* noise (whole zero-intake days counted as real), but does not
  fix under-reporting on nominally-Complete days — the second grilling pass's correction 2 (a 922
  kcal Complete day) is why the feature could not simply "wait for #9 and then ship the original
  idea."
- **No third source of expenditure exists in the schema to validate a computed metabolism
  against**, confirmed by reading the data directly rather than inferring from naming: exercise
  calories are real and populated, but nothing else is. This is what makes "report a number and
  call it TDEE" structurally unsafe here, independent of how good the intake logging eventually
  gets — it is not a data-quality problem that more logging fixes.
- **The trend-weight math (EMA + OLS regression) exists only on the frontend today.** There is no
  backend port. This proposal's design keeps the computation client-side deliberately (design.md
  "Non-Goals"), reusing the existing functions rather than introducing a second implementation that
  could drift from the weight chart's own trend line.
- **The interval formula's two terms are independently unvalidated**, and design.md says so
  directly under "Risks": `formulaError` is a fixed 10% literature figure, not measured from this
  app's users, and the regression's standard error conflates genuine metabolic variability with EMA
  smoothing lag. The combined "~±310" figure lines up with the grilling comment's own worked
  example, which is the extent of validation available before an implementation exists to test
  against real data.
- **Logged intake's photo-estimation bias is deliberately excluded from the interval**, not because
  it's assumed to be zero but because it isn't quantifiable from anything currently measured. The
  design pushes this into caveat copy on the card rather than folding an unmeasured number into the
  interval math — an explicit, named gap rather than a silently optimistic one.
- **The dashboard card registry's `DataType`-only assumption is a real constraint, not a
  formality.** Every existing card has a `/api/data/{type}` presence signal the registry's
  visibility logic depends on; the Logging Gap Card has none (its "nothing to show" states are its
  own content states, not a presence gate). Design.md's decision 8 widens the registry to a
  `CardId` union rather than special-casing one card, on the stated reasoning that Phase 4's own
  upcoming Food Cards will need the same shape — this is a judgment call about scope, not a forced
  outcome, and is flagged in "Risks" as intentionally minimal rather than a general card-kind
  abstraction.
- **The window anchor (today vs. last weigh-in) was an explicitly open question** the grilling
  comment left for `opsx:propose`. Design.md resolves it as "yesterday," matching every other
  trailing window in the app (steps, day completeness) — a staleness problem in the input data (the
  most recent weigh-in was 5 days old at grilling time) is absorbed by the interval mechanism widening,
  not by a separate freshness check.

## Findings write-up

### What was found

- The idea as filed ("adaptive TDEE, replacing the activity multiplier") does not survive contact
  with the production data, confirmed twice: once with a factor-of-2.7 gap that turned out to be
  partly an artifact of counting unlogged days as zero-intake, and again — after correcting for
  that artifact — with a structural gap that no amount of logging alone closes, because nothing in
  the schema can serve as a second, independent measurement of expenditure to check a computed
  number against.
- The reshaped feature (Logging Gap) is a materially different deliverable than the one the idea
  was filed under: a diagnostic surfaced with an honest interval and a silence rule, not a
  metabolism estimate, and it explicitly does not touch ADR-006's activity multiplier. This is a
  case where grilling changed what ships, not just how it's built.
- Both `food-day-completeness` (the previously-blocking prerequisite) and the Nutrition Target
  endpoint (the formula-side input) are already implemented and merged, ahead of this
  investigation — this proposal did not have to design either, only to consume them as-is and read
  their documented preconditions/states correctly (the 422 reasons, the four completeness states).
- Four decisions the grilling comment explicitly left open (window anchor, outlier rule shape,
  interval formula, and the daily-totals endpoint's ownership) are now settled with concrete math
  and shapes in `design.md`, each traceable to which open question it resolves — nothing in the
  proposal is a new decision beyond those four.
- The dashboard card registry (ADR-001) needs a real, if small, generalization to host this
  feature — it was built for exactly 8 presence-gated vitals cards, and this is the first card that
  isn't one.

### Limitations of this investigation

- **No shipped/wired-up code was written.** Per this repo's spec-first workflow
  (`/data/CLAUDE.md`, "OpenSpec — MANDATORY"), implementing the new endpoint, the frontend
  computation library, the card, and the registry widening requires the OpenSpec propose → approve
  → implement cycle, out of scope for an investigate-only idea-forge plan.
- **The interval formula is reasoned about, not tested against real user data.** The "~±310"
  combined-error figure comes from the grilling comment's own worked example on one user's data;
  no sensitivity analysis was run against a different eating/weighing pattern (e.g. a user who
  weighs daily vs. one who weighs every 4 days, or one whose logging habit is much older than 17
  days). The silence rule is designed to make this safe by construction (a wide interval covers
  zero and suppresses output), but that safety property itself has not been exercised against a
  second real dataset.
- **The rate-based outlier rule's 2 kg/day threshold is carried over unchanged from the grilling
  comment**, which itself calls it "a starting figure, not a researched one" (idea-forge issue,
  "Still open" item 1). This investigation did not attempt to derive or validate a better threshold
  — it is transcribed into design.md decision 2 as-is, with the reasoning (rate-based, not flat
  per-point, to survive sparse multi-day gaps) added on top.
- **Whether the registry's `CardId` widening (decision 8) is the right shape for Phase 4's later
  Food Cards is not verified against Phase 4's own design**, since Phase 4 has not been proposed
  yet. Design.md is explicit that this is a minimal, single-member widening, not a general
  card-kind abstraction, and that it may need to grow again rather than being preemptively solved
  here.
- **Locale copy for the card's states (silence text, the 422-vs-no-data distinction, the
  outlier-excluded note, the photo-estimation-bias caveat) was not drafted.** The proposal and
  design name the states and the reasoning behind each string's existence, but leave exact
  English/Russian text to the implementation pass, consistent with prior idea-forge write-ups
  (idea-6, idea-9) doing the same.

### Suggested next steps

1. Implement against `openspec/changes/logging-gap/tasks.md` in order: the backend
   `GET /api/food/daily-totals` endpoint (task 1) and the frontend Logging Gap computation library
   (task 3) first, since the card (task 5) and the registry generalization (task 4) both depend on
   having real numbers to render and a card identifier to register.
2. When implementing the interval math (design.md decision 3), add a unit test using the two error
   terms from the idea-forge issue's evidence table (formulaError ≈275 from a ~2,750 Nutrition
   Target, trendErrorKcal ≈195 from the 28-day window) as a regression fixture — this is the only
   validated real-data example available before the endpoint exists to pull fresh numbers. Note the
   grilling comment's own arithmetic rounds the combined figure to "~±310," but the settled formula
   (`sqrt(275^2 + 195^2)`, mirrored in `specs/logging-gap/spec.md`'s own worked scenario and
   `tasks.md` 3.8) computes ≈337 from those same two terms — use 337 as the fixture's expected
   value, not 310.
3. Once shipped, watch how often the silence rule fires in practice (gate 1's hard floor vs. gate
   2's statistical silence) against the assumption that thin data self-resolves via a widening
   interval — if the card is silent for most users most of the time, that's a signal the interval
   formula is too conservative, not that the feature is broken.
4. Revisit the fixed 2 kg/day outlier threshold and the fixed 10% `formulaError` constant once
   real usage data exists across more than one household member — both are named in design.md's
   "Risks" as literature-derived or comment-derived figures, not measured ones, and are the two
   most likely values to need tuning after real-world exposure.
5. When Phase 4's Food Cards are proposed, check whether they fit the `CardId` union widened here
   (design.md decision 8) or need a third shape — the risk section explicitly leaves this
   unresolved rather than guessing ahead of Phase 4's own design.
