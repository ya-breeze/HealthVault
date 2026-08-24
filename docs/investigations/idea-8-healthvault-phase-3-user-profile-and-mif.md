# Investigation: Phase 3 — user profile and Mifflin-St Jeor nutrition targets

Research notes for idea-forge plan
`idea-8-healthvault-phase-3-user-profile-and-mif-investigate.md`. Phase 3 of
the four-phase dashboard/food-tracking initiative; hard-depends on Phase 2
(goal weight), which shipped in `goal-weight-bmi-bands-trend-projection`
(commit `a311830`). Goal here: give the user a computed daily nutrition
target (calories + protein/carb/fat) for Phase 4's food cards to compare
intake against, per `docs/adr/ADR-003-nutrition-targets-from-goal-weight.md`.

## Outcome

This investigation concluded in a proposed OpenSpec change,
`openspec/changes/user-profile-and-nutrition-target/` (proposal, design, spec
deltas for two new capabilities — `user-profile`, `nutrition-target` — and
three modified ones — `user-settings`, `display-language`,
`mobile-touch-targets` — plus an implementer `tasks.md` with every box left
unticked). `openspec validate user-profile-and-nutrition-target --strict`
passes. No production code was written or wired up — per this repo's
spec-first workflow, that is deliberately out of scope for this Attempt and
is left for a later, separate implementation pass against the approved spec.

Unlike idea-6, this change arrived at the investigation stage already mostly
settled: it followed a `/grilling` + `/domain-modeling` session (2026-08-23)
that resolved the open questions by querying the production database
directly rather than by argument (see the plan's "What the production data
decided" section). That left this Attempt's job closer to formalizing an
already-decided design into OpenSpec artifacts than discovering the design
from scratch — the three numbers ADR-003 explicitly deferred (activity-tier
count/multipliers, the no-goal protein fallback, and the g/kg/split figures)
were pinned down in `design.md` rather than left as open questions.

## What was found

- **The production-data ruling was reproducible from the plan alone.** The
  plan's own read-only queries (three genuine `body_fat` readings from
  mid-2024, `total_calories` averaging 51 kcal/record, `steps` at 119/120
  days and 8,552/day average) fully justify Mifflin-St Jeor over the two
  alternatives ADR-003 was choosing between, and independently justify
  inferring activity from steps rather than asking for it. No new querying
  was needed to write the proposal; the plan's "Settled decisions" section
  translated directly into `design.md`'s decisions.
- **The design's three genuinely new numbers** (the 5-tier
  1.2/1.375/1.55/1.725/1.9 multiplier table mapped to steps-per-day bands,
  the 28-day trailing window with tail-only trimming at a 500-step/day
  floor, and the 7-valid-day minimum before falling back to
  `insufficient_activity_data`) were not handed down by the grilling session
  — it settled the *mechanism* (infer from steps, trim incomplete edge days,
  allow override) but not these specific thresholds. They are flagged as
  this proposal's own judgment call in `design.md`'s "Risks / Trade-offs",
  the same way Phase 2's proposal flagged its own regression-window
  constants.
- **`UserSettings`'s single-blob, no-merge storage is the design's central
  hazard, not an incidental detail.** Phase 3 adds a third writer
  (`birthdate`/`sex`/`activity_override`) to a document already written by
  two features (dashboard order, display language) with no server-side
  merge (`UpsertUserSettings` does a full-row `DoUpdates`). `LanguageContext`
  already centralizes round-trip safety for the first two writers via
  `useSerialQueue().claim()`-backed `updateSettings`; the proposal requires
  the new profile form to call that same function rather than
  `api.updateSettings` directly, and calls out that this is a one-line
  difference with a silent-data-loss failure mode if missed — deliberately
  spelled out in `design.md` rather than left implicit in "reuse the
  existing form idiom."
- **ADR-003 has two now-wrong statements**, both a direct consequence of
  this phase's decision to *require* a goal weight rather than fall back
  when one is absent: the "softer dependency on Phase 2" claim (it's now a
  hard dependency) and the internally-inconsistent claim that Phase 4 cards
  must show calories/carbs/fat with protein specifically unavailable (ADR-
  003's own math makes carbs/fat depend on protein being fixed first, so
  there's no partial-target state to support). Both corrections are recorded
  in the proposal and left as an implementer task (`tasks.md` 7.1) rather
  than applied now, since editing ADR-003 is implementation work, not
  proposal work.
- **The relocation of Display Language breaks an existing E2E test in a way
  that's easy to lose in the move.** `e2e/tests/dashboard.spec.ts`'s
  `Settings lost-update race` describe block drives `#display-language` from
  the `Header`; moving that control to `/settings` means the test must move
  too (to a new `settings.spec.ts`) and be extended to cover the profile
  form as a third writer, not just relocated verbatim. This is called out
  explicitly in the proposal's "Impact" section and as its own `tasks.md`
  section (6) so it isn't treated as a side effect of the route change.

## Limitations of this investigation

- **No shipped or wired-up code was written**, per this repo's spec-first
  workflow (`/data/CLAUDE.md`, "OpenSpec — MANDATORY"). The backend handler,
  the frontend `/settings` route, and the ADR-003 corrections all remain as
  unticked `tasks.md` items for a later implementation pass.
- **The 500-step/day trim floor and the 7-valid-day minimum are reasoned
  defaults, not measured against the production step data beyond the two
  specific days the plan already surfaced** (two days with zero rows, one
  day at 96 steps). Whether 500 and 7 are the right thresholds across the
  full 120-day/126-user sample the plan queried was not re-verified here;
  `design.md` documents the reasoning but the investigation did not re-run
  the production query to sanity-check the thresholds against a larger
  window.
- **The privacy question the plan raises (birthdate/sex as the app's first
  classic identifying PII, added to an unencrypted blob) is flagged, not
  resolved.** The proposal's "What's explicitly out of scope" section
  carries this forward as an operator decision rather than silently
  expanding this change's scope to include a `PRIVACY.md` or encryption-at-
  rest work.
- **The new ADR-006 (recording the steps-inference-at-read-time pattern) was
  not drafted**, only named as a required task (`tasks.md` 7.2) with its
  slug left for the implementer to fill in — the investigation identified
  that this decision is architecturally distinct from ADR-003's formula
  choice (a new pattern: deriving a profile-shaped value from time-series
  data at read time instead of storing it) but did not write the ADR itself.

## Suggested next steps

1. Get the spec-only PR for `openspec/changes/user-profile-and-nutrition-target/`
   reviewed and merged per this repo's normal OpenSpec lifecycle (this
   Attempt does not include an approval step, per the plan's own framing,
   but the standard project workflow's PR review still applies before
   `main`).
2. Implement in `tasks.md`'s order: activity inference (section 1) and the
   `GET /api/users/me/nutrition-target` handler (section 2) first, since the
   frontend profile form (sections 3-5) has nothing to call until the
   endpoint and the `UserSettings` field additions exist.
3. When wiring the profile form (section 4), get the
   `useLanguage().updateSettings(patch)` call right on the first pass —
   this is the single highest-risk line in the whole change per `design.md`,
   and the extended race test (section 6) should be written early enough to
   catch a `api.updateSettings`-direct mistake before it ships, not after.
4. Apply the ADR-003 corrections and write ADR-006 (section 7) in the same
   PR as the code, not deferred — per this repo's ADR rule, a new ADR
   introduced by an in-flight change starts `Proposed` and flips to
   `Accepted` only at merge time, together with archiving the OpenSpec
   change.
5. Decide the privacy question this investigation flagged but didn't
   resolve (whether birthdate/sex warrant a `PRIVACY.md` or encryption-at-
   rest change) before or alongside implementation, since it's a decision
   for the operator, not something the implementer should default silently
   either way.
