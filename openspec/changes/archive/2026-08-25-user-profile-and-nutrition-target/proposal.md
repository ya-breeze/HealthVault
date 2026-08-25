## Why

Phase 4's food dashboard cards (ADR-004) need a daily calorie/macro target to compare intake
against. Nothing today computes one, and nothing today asks the two profile facts (birthdate, sex)
Mifflin-St Jeor needs. This is Phase 3 of the four-phase dashboard/food-tracking initiative — Phase
1 (card hide/show) shipped in #27, Phase 2 (goal weight, BMI bands, trend projection) shipped in
`goal-weight-bmi-bands-trend-projection`.

ADR-003 already chose the formula split (calories/BMR from measured weight, protein g/kg from Goal
Weight) but explicitly deferred three numbers to this phase: the activity-tier count and
multipliers, the protein behaviour when no goal is set, and the actual g/kg/split figures. A
read-only query against the production database, run before this proposal, ruled out both
formula alternatives ADR-003 was choosing between (Katch-McArdle needs `body_fat`, which has only
three genuine readings from mid-2024; measured-TDEE needs `total_calories`, whose records average
51 kcal — per-activity fragments, not whole-day totals) and confirmed the two inputs Mifflin-St
Jeor actually needs are solid: 1,225 weight records and a plausible height. It also surfaced that
`steps` is by far the best-covered metric (7,342 records, 119 of the last 120 days, avg 8,552/day)
— dense enough to infer an activity level from, instead of asking for one.

## What Changes

- **Two new profile fields** — birthdate and sex — stored as new keys (`birthdate`, `sex`) in the
  existing per-user `UserSettings` JSON blob, the same storage `display_language` already uses.
  No new table, no new endpoint. Age is derived from birthdate at calculation time, not stored,
  so it can never silently drift stale.
- **Activity level is inferred, not asked.** A trailing 28-day average of daily step counts (from
  the existing `steps` metric) maps to one of 5 standard Mifflin-St Jeor/Harris-Benedict-style
  multipliers. Two separate exclusion rules apply: a day with no step records at all is excluded
  wherever it falls in the window (trailing edge or interior — missing data is never averaged in
  as a real zero), while a day with an implausibly low but nonzero count (sync lag / a partial day
  still being captured) is trimmed only from the trailing edge, stopping at the first day that
  clears the floor — so a genuinely low-but-real day further back in the window is never
  discarded. A third new `UserSettings` key, `activity_override`, lets a user whose activity
  doesn't register as steps (cycling, lifting) pin one of the same 5 tiers directly; when set, it
  always wins over inference.
- **New capability `nutrition-target`**: `GET /api/users/me/nutrition-target`, following the
  existing `summaryHandler` precedent (auth via `ClaimsFromCtx`, no request body, plain JSON).
  Computed fresh on every read from the user's current profile fields, latest measured `weight`
  and `height`, latest `weight_goal`, and the step-based activity inference — never stored, so
  every input's own history (a metric's history, or a corrected birthdate) is what the target
  reconstructs from, with nothing to fall out of sync. A `weight_goal` is **required**: with no
  goal, protein and therefore carbs/fat are uncomputable (they fill whatever calories protein
  doesn't use), so the endpoint reports a specific unmet-precondition reason rather than computing
  a partial target.
- **New route `/settings`** — the app's first real settings screen, with a Profile section (the
  two required fields plus the activity override, hand-rolled `<input>`/`<select>` in the
  `CustomFoodModal.tsx` idiom, wrapped in `TapTarget`). **Display Language moves here from
  `Header.tsx`**, replaced in the header by a link to `/settings`.
- **A third writer joins the existing settings write queue.** `UserSettings` is stored and
  overwritten as one opaque blob with no server-side merge; round-trip safety is a frontend-only
  concern already centralized in `LanguageContext`'s `useSerialQueue().claim()`-backed
  `updateSettings`. After this change there are three independent features writing to that blob
  (dashboard order, Display Language, the new profile form) instead of two, and the profile form
  is required to write through that same queue rather than PUTing directly — the exact bug the
  `Settings lost-update race` E2E test already guards the first two writers against.
- **Corrections to ADR-003**, now that the three deferred numbers are settled: the "softer
  dependency on Phase 2" line is wrong once a goal is required (it's now a hard dependency); the
  claim that Phase 4 cards must show calories/carbs/fat with protein unavailable is internally
  inconsistent with ADR-003's own "carbs/fat fill what's left once calories and protein are fixed"
  and is removed now that a goal is required; the three deferred items are filled in.
- **New ADR** recording the steps-inference decision itself (a new architectural pattern — deriving
  a profile-shaped value from time-series data at read time instead of storing it — distinct from
  ADR-003's formula choice).

## What's explicitly out of scope

- Adaptive TDEE (would eventually replace the activity multiplier) and log completeness — both
  already filed as separate backlog items.
- Phase 4 itself: the food cards, healthiness label, and LLM recommendations that will consume this
  endpoint.
- Any encryption-at-rest or retention policy for the new birthdate/sex fields. They are the first
  classic identifying PII this app stores, added to the same unencrypted `UserSettings` blob
  `display_language` already lives in — consistent with the app's existing (undocumented) posture,
  but that posture itself is not decided by this change. Flagged for the operator, not silently
  expanded in scope here.

## Capabilities

### New Capabilities
- `user-profile`: birthdate/sex/activity-override storage, the derived-age and inferred-activity
  rules, and the `/settings` Profile form.
- `nutrition-target`: the Mifflin-St Jeor computation, the activity-tier table, the protein/carb/fat
  split, and the `GET /api/users/me/nutrition-target` contract (including its unmet-precondition
  responses).

### Modified Capabilities
- `user-settings`: documents the concurrent-write-safety rule that every `UserSettings`-writing
  frontend feature (now three) must follow, generalizing what `LanguageContext` already implements
  for the first two.
- `display-language`: the UI Language Switcher requirement is updated to say it lives on `/settings`
  now, not in the header.
- `mobile-touch-targets`: the 48×48px minimum's scope list is extended to include the new
  `/settings` screen.

## Impact

- Backend (Go): `backend/pkg/server/api.go` (new `NutritionTargetHandler` next to
  `summaryHandler`, reading `UserSettings` JSON plus `weight`/`height`/`weight_goal`/`steps` via
  existing storage methods), `backend/pkg/server/server.go` (new route registration).
- Frontend (TS/React): new `frontend/app/settings/` route + `ProfileForm` component, `Header.tsx`
  (remove inline language `<select>`, add a `/settings` link), `LanguageContext.tsx` (no change to
  its queue — the profile form is a new caller of its existing `updateSettings`), `frontend/lib/api.ts`
  (`UserSettings` interface gains `birthdate`/`sex`/`activity_override`, new
  `getNutritionTarget()`).
- `e2e/tests/dashboard.spec.ts`'s `Settings lost-update race` describe block moves to a new
  `e2e/tests/settings.spec.ts` and gains the profile form as a third writer. The `Dashboard card
  reorder` describe block's `switching language mid-reorder keeps the unsaved order` test is
  deleted rather than moved: it depends on the language switcher being reachable without leaving
  the dashboard's edit mode, a premise the `/settings` relocation removes.
- Docs: `docs/adr/ADR-003-nutrition-targets-from-goal-weight.md` (three corrections),
  `docs/adr/ADR-006-*.md` (new), `CONTEXT.md` (glossary: **Activity Level**, **Nutrition Target**
  updated), `todo.md` (Phase 3 marked claimed).
