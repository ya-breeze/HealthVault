## 1. Backend: activity-level inference

- [x] 1.1 Add a pure function computing the trailing 28-day step average applying two exclusion
      rules per design.md: (a) zero-record days excluded anywhere in the window, not just the tail,
      and (b) <500-step (but nonzero-record) days trimmed only from the trailing edge, stopping at
      the first day that clears 500 — given a caller-supplied "today" and a source of daily step
      sums/record-counts (reuse the existing `GET /api/data/steps?bucket=day` aggregation logic —
      see `backend/pkg/server/api.go`'s bucketed-query path)
- [x] 1.2 Add the 5-tier table (Sedentary/Lightly active/Moderately active/Very active/Extra active
      → 1.2/1.375/1.55/1.725/1.9) mapping the resulting average to a multiplier
- [x] 1.3 Return "unavailable" (not a default tier) when fewer than 7 valid days remain after
      exclusion

## 2. Backend: `GET /api/users/me/nutrition-target`

- [x] 2.1 Add `NutritionTargetHandler` in `backend/pkg/server/api.go`, alongside `summaryHandler`:
      auth via `ClaimsFromCtx`, no request body. Unlike `summaryHandler`, do NOT call `resolveUser`
      / support `?user=` — this endpoint is self-only (see design.md's "Self-only" decision)
- [x] 2.2 Read `birthdate`/`sex`/`activity_override` from the user's `UserSettings` JSON
      (`storage.GetUserSettings`), applying the "malformed/absent → not set" interpretation from
      `user-profile`'s spec (not a schema-validated read — the settings blob stays opaque at the
      storage layer)
- [x] 2.3 Read latest `weight`, `height`, `weight_goal` records via existing storage methods
- [x] 2.4 Resolve the activity tier: `activity_override` if valid, else the task-1 inference over
      the user's `steps` history. Use the explicit override-value→tier→multiplier table in
      design.md/`user-profile`'s spec (do NOT derive the mapping from string similarity between the
      enum values and tier names — `"active"`/`"very_active"` do not positionally match "Very
      active"/"Extra active")
- [x] 2.5 Implement the 4-reason 422 precondition check in the exact order from design.md
      (`missing_profile` → `missing_measurements` → `missing_goal_weight` →
      `insufficient_activity_data`), returning as soon as the first unmet reason is found
- [x] 2.6 Implement the Mifflin-St Jeor + activity multiplier + protein/carb/fat-with-floor
      computation from design.md, converting the `height` metric's stored metres to centimetres
      and computing calendar age (UTC, matching the rest of the API) from `birthdate` at request
      time. Clamp `carbs_grams` to 0 (not negative) when protein alone meets or exceeds `calories`;
      round all four output values to the nearest whole unit before returning
- [x] 2.7 Return HTTP 200 with `calories`, `protein_grams`, `carbs_grams`, `fat_grams`, and the
      echoed inputs (`measured_weight_kg`, `goal_weight_kg`, `height_m`, `age_years`, `sex`,
      `activity_multiplier`, `activity_tier`) on success
- [x] 2.8 Register the route in `backend/pkg/server/server.go` behind the existing auth middleware

## 3. Backend: unit tests

- [x] 3.1 Unit-test the trailing-window exclusion function (task 1.1) against: a day with 0 records
      at the trailing edge, a day with 0 records in the interior of the window (must be excluded,
      not averaged in as 0), a day with fewer than 500 steps, a day with exactly 500 steps (kept,
      not trimmed), a day with more than 500 steps, a trimmed-then-clean-day sequence (trailing-edge
      trimming stops at the first clean day scanning backward), and a window with exactly 7 valid
      days vs. 6 valid days (`insufficient_activity_data` boundary)
- [x] 3.2 Unit-test the tier-boundary mapping (task 1.2) at each exact boundary value (4,999 /
      5,000 / 7,499 / 7,500 / 9,999 / 10,000 / 12,499 / 12,500 steps/day)
- [x] 3.3 Unit-test the `activity_override` → tier/multiplier resolution (task 2.4) for all 5 enum
      values, specifically asserting `"active"` → 1.725 and `"very_active"` → 1.9 (the two
      non-obvious cases)
- [x] 3.4 Unit-test `NutritionTargetHandler`'s 4-reason precondition check ordering (task 2.5),
      including a case with multiple unmet reasons to confirm only the first-checked one is
      reported
- [x] 3.5 Unit-test the computation itself (task 2.6): the standard case, the fat-floor-engaged
      case, the protein-exceeds-calories case (`carbs_grams` clamped to 0), and malformed
      `birthdate`/`sex` handling (future date, unparsable, age outside 5–120 inclusive, unrecognized
      `sex` value)

## 4. Frontend: profile fields + activity override in the settings blob

- [x] 4.1 Extend `UserSettings` in `frontend/lib/api.ts` with `birthdate?: string`,
      `sex?: 'male' | 'female'`, `activity_override?: 'sedentary' | 'light' | 'moderate' |
      'active' | 'very_active'`
- [x] 4.2 Add `api.getNutritionTarget()` calling `GET /api/users/me/nutrition-target`, typed to
      return either the success shape or throw/return a typed 422 reason (matching the existing
      `ReanalyzeFailedError`-style typed-error convention in `api.ts` for the 4 reason codes)

## 5. Frontend: `/settings` route + Profile form

- [x] 5.1 Create `frontend/app/settings/page.tsx` (or `SettingsClient.tsx` following the existing
      `DataTypeClient.tsx` split), rendered behind the existing `Header` chrome
- [x] 5.2 Build the Profile form: birthdate input (required), sex select (required, 2 options),
      activity override select (optional, "Automatic (based on steps)" + 5 tiers, labeled per the
      tier names in design.md/`user-profile`'s mapping table — not the raw enum values), following
      `CustomFoodModal.tsx`'s hand-rolled `<input>`/`<select>` idiom, every control wrapped in
      `TapTarget`
- [x] 5.3 Wire the form's save to `useLanguage().updateSettings(patch)` — NOT `api.updateSettings`
      or `api.putSettings` directly — per design.md's "one-line difference with a silent-data-loss
      failure mode if missed"
- [x] 5.4 Client-side required-field validation for birthdate and sex before submit (mirrors
      `CustomFoodModal`'s name-required check)

## 6. Frontend: relocate Display Language

- [x] 6.1 Move the `<select id="display-language">` control and its `handleLanguageChange` wiring
      from `frontend/components/Header.tsx` into the new `/settings` Profile section, keeping the
      `id` and `setLanguage` call unchanged
- [x] 6.2 Replace the removed control in `Header.tsx` with a link/icon to `/settings`

## 7. E2E: move and extend the settings race test

- [x] 7.1 Move the `Settings lost-update race` describe block out of `e2e/tests/dashboard.spec.ts`
      into a new `e2e/tests/settings.spec.ts`, updating navigation to `/settings` where the control
      now lives. Delete the `Dashboard card reorder` describe block's `switching language
      mid-reorder keeps the unsaved order` test rather than moving it: it exercises a same-page,
      no-navigation language switch reachable "without leaving edit mode," a premise that no
      longer holds once Display Language moves off the dashboard page — the effect-dependency
      regression it guards against can no longer be triggered this way
- [x] 7.2 Extend the race test to a third writer: save the profile form in the same session as a
      language switch (no navigation between them, both live on `/settings`), assert both persist.
      Separately, verify a dashboard reorder made just before navigating to `/settings` survives
      the trip — client-side navigation only, since `LanguageProvider`'s write queue is mounted at
      the root layout and persists across routes
- [x] 7.3 Add coverage for the profile form's own required-field validation and successful save
- [x] 7.4 Run full suite against `hcw-wip`

## 8. Docs

- [x] 8.1 Correct `docs/adr/ADR-003-nutrition-targets-from-goal-weight.md`'s Consequences section:
      remove the "softer dependency on Phase 2" claim (now a hard dependency, since a goal is
      required), remove the internally-inconsistent "cards must show calories/carbs/fat even when
      protein is unavailable" claim, fold in the three settled numbers (activity tiers, no-goal
      behavior, g/kg/split figures), and flip its `Status` from `Proposed` to `Accepted` (per this
      project's ADR lifecycle rule — its content is now finalized by this change)
- [x] 8.2 New `docs/adr/ADR-006-<slug>.md` recording the steps-inference-instead-of-storage
      decision (`Status: Proposed` until this change merges, then flip to `Accepted` at merge
      alongside 8.1's ADR-003 flip)
- [x] 8.3 `CONTEXT.md`: add an **Activity Level** glossary entry (Nutrition targets section);
      update the existing **Nutrition Target** entry to mention the two new profile fields instead
      of leaving "activity level" unexplained
- [x] 8.4 Mark the Phase 3 backlog item in `todo.md` as claimed by this change, and correct its
      now-stale claims: the activity-tier count is decided (5, not "still undecided"), and the
      calorie/carb/fat targets now have a hard dependency on Phase 2 too, not "only the protein
      target does" (mirrors the ADR-003 corrections in 8.1)

## 9. Verification

- [x] 9.1 `make lint`
- [x] 9.2 `make test`
- [x] 9.3 `npx tsc --noEmit` and `npm run build` in `frontend/`
- [x] 9.4 `openspec validate --strict` for this change
