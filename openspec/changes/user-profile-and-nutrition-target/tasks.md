## 1. Backend: activity-level inference

- [ ] 1.1 Add a pure function computing the trailing 28-day step average with trailing-edge
      trimming (zero-record or <500-step days at the tail, stopping at the first day that clears
      500) per design.md, given a caller-supplied "today" and a source of daily step
      sums/record-counts (reuse the existing `GET /api/data/steps?bucket=day` aggregation logic —
      see `backend/pkg/server/api.go`'s bucketed-query path)
- [ ] 1.2 Add the 5-tier table (Sedentary/Lightly active/Moderately active/Very active/Extra active
      → 1.2/1.375/1.55/1.725/1.9) mapping the trimmed average to a multiplier
- [ ] 1.3 Return "unavailable" (not a default tier) when fewer than 7 valid days remain after
      trimming

## 2. Backend: `GET /api/users/me/nutrition-target`

- [ ] 2.1 Add `NutritionTargetHandler` in `backend/pkg/server/api.go`, alongside `summaryHandler`:
      auth via `ClaimsFromCtx`, no request body
- [ ] 2.2 Read `birthdate`/`sex`/`activity_override` from the user's `UserSettings` JSON
      (`storage.GetUserSettings`), applying the "malformed/absent → not set" interpretation from
      `user-profile`'s spec (not a schema-validated read — the settings blob stays opaque at the
      storage layer)
- [ ] 2.3 Read latest `weight`, `height`, `weight_goal` records via existing storage methods
- [ ] 2.4 Resolve the activity tier: `activity_override` if valid, else the task-1 inference over
      the user's `steps` history
- [ ] 2.5 Implement the 4-reason 422 precondition check in the exact order from design.md
      (`missing_profile` → `missing_measurements` → `missing_goal_weight` →
      `insufficient_activity_data`), returning as soon as the first unmet reason is found
- [ ] 2.6 Implement the Mifflin-St Jeor + activity multiplier + protein/carb/fat-with-floor
      computation from design.md, converting the `height` metric's stored metres to centimetres
      and computing calendar age from `birthdate` at request time
- [ ] 2.7 Return HTTP 200 with `calories`, `protein_grams`, `carbs_grams`, `fat_grams`, and the
      echoed inputs (`measured_weight_kg`, `goal_weight_kg`, `height_m`, `age_years`, `sex`,
      `activity_multiplier`, `activity_tier`) on success
- [ ] 2.8 Register the route in `backend/pkg/server/server.go` behind the existing auth middleware

## 3. Frontend: profile fields + activity override in the settings blob

- [ ] 3.1 Extend `UserSettings` in `frontend/lib/api.ts` with `birthdate?: string`,
      `sex?: 'male' | 'female'`, `activity_override?: 'sedentary' | 'light' | 'moderate' |
      'active' | 'very_active'`
- [ ] 3.2 Add `api.getNutritionTarget()` calling `GET /api/users/me/nutrition-target`, typed to
      return either the success shape or throw/return a typed 422 reason (matching the existing
      `ReanalyzeFailedError`-style typed-error convention in `api.ts` for the 4 reason codes)

## 4. Frontend: `/settings` route + Profile form

- [ ] 4.1 Create `frontend/app/settings/page.tsx` (or `SettingsClient.tsx` following the existing
      `DataTypeClient.tsx` split), rendered behind the existing `Header` chrome
- [ ] 4.2 Build the Profile form: birthdate input (required), sex select (required, 2 options),
      activity override select (optional, "Automatic (based on steps)" + 5 tiers), following
      `CustomFoodModal.tsx`'s hand-rolled `<input>`/`<select>` idiom, every control wrapped in
      `TapTarget`
- [ ] 4.3 Wire the form's save to `useLanguage().updateSettings(patch)` — NOT `api.updateSettings`
      or `api.putSettings` directly — per design.md's "one-line difference with a silent-data-loss
      failure mode if missed"
- [ ] 4.4 Client-side required-field validation for birthdate and sex before submit (mirrors
      `CustomFoodModal`'s name-required check)

## 5. Frontend: relocate Display Language

- [ ] 5.1 Move the `<select id="display-language">` control and its `handleLanguageChange` wiring
      from `frontend/components/Header.tsx` into the new `/settings` Profile section, keeping the
      `id` and `setLanguage` call unchanged
- [ ] 5.2 Replace the removed control in `Header.tsx` with a link/icon to `/settings`

## 6. E2E: move and extend the settings race test

- [ ] 6.1 Move the `Settings lost-update race` describe block and the `#display-language`-driving
      tests out of `e2e/tests/dashboard.spec.ts` into a new `e2e/tests/settings.spec.ts`, updating
      navigation to `/settings` where the control now lives
- [ ] 6.2 Extend the race test to a third writer: save the profile form in the same session as a
      dashboard reorder and a language switch (no navigation in between any of them), assert all
      three persist
- [ ] 6.3 Add coverage for the profile form's own required-field validation and successful save
- [ ] 6.4 Run full suite against `hcw-wip`

## 7. Docs

- [ ] 7.1 Correct `docs/adr/ADR-003-nutrition-targets-from-goal-weight.md`'s Consequences section:
      remove the "softer dependency on Phase 2" claim (now a hard dependency, since a goal is
      required), remove the internally-inconsistent "cards must show calories/carbs/fat even when
      protein is unavailable" claim, and fold in the three settled numbers (activity tiers, no-goal
      behavior, g/kg/split figures)
- [ ] 7.2 New `docs/adr/ADR-006-<slug>.md` recording the steps-inference-instead-of-storage
      decision (`Status: Proposed` until this change merges)
- [ ] 7.3 `CONTEXT.md`: add an **Activity Level** glossary entry (Nutrition targets section);
      update the existing **Nutrition Target** entry to mention the two new profile fields instead
      of leaving "activity level" unexplained
- [ ] 7.4 Mark the Phase 3 backlog item in `todo.md` as claimed by this change

## 8. Verification

- [ ] 8.1 `make lint`
- [ ] 8.2 `make test`
- [ ] 8.3 `npx tsc --noEmit` and `npm run build` in `frontend/`
- [ ] 8.4 `openspec validate --strict` for this change
