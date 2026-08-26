## 1. Settings model (`frontend/lib/api.ts`)

- [ ] 1.1 Add `more_data_hidden?: boolean` to the `UserSettings` interface, documented alongside its neighboring keys (`dashboard_order`, `display_language`, etc.); no shape migration needed since an absent key means "not hidden."

## 2. Dashboard page state and persistence (`frontend/app/page.tsx`)

- [ ] 2.1 Load `more_data_hidden` from settings into local state alongside the existing `order` state, defaulting to `false`/absent.
- [ ] 2.2 Add a `toggleMoreDataHidden()` function mirroring the existing `toggleHidden(index)` local-state-mutation style.
- [ ] 2.3 Update `handleDone()` to include `more_data_hidden` in the same `updateSettings()` payload as `dashboard_order` — one PUT, no new writer.
- [ ] 2.4 Gate the More Data render path on the settings load (`dashboardReady`/equivalent), not only `presenceReady`, so a hidden section can't flash unhidden before the saved preference is known — add a loading/error placeholder state for this path if the existing settings-load gate isn't already shared with it.
- [ ] 2.5 Update the read-only (non-editing) render condition from `presentSecondaryTypes.length > 0` to `!moreDataHidden && presentSecondaryTypes.length > 0`.
- [ ] 2.6 In edit mode, render the More Data section (heading, toggle, link list) whenever `presentSecondaryTypes.length > 0`, regardless of `moreDataHidden`, visually distinguished (e.g. dimmed) when hidden — mirroring the vitals grid's dimmed-hidden-card treatment.
- [ ] 2.7 When `presentSecondaryTypes.length === 0`, do not render the More Data section or its toggle at all, in either mode.

## 3. UI control

- [ ] 3.1 Add a show/hide (eye/eye-off) control to the More Data section heading, visible only in edit mode, wired to `toggleMoreDataHidden()`. Reuse the eye/eye-off icon asset from `frontend/components/icons` (added in Phase 1) rather than adding a new one.

## 4. Localization

- [ ] 4.1 Add new i18n keys for the More Data toggle's accessible label (shown/hidden state) in both `frontend/lib/i18n/en.ts` and `ru.ts` (`Dictionary = typeof en` and `DICTIONARIES: Record<LanguageCode, Dictionary>` make a missing `ru` key a compile error).

## 5. Tests

- [ ] 5.1 Extend `e2e/tests/dashboard.spec.ts` with scenarios matching the spec delta: hide the More Data section and confirm it disappears from the read-only page but remains discoverable (dimmed) in edit mode; re-show it; hidden state persists across reload; the toggle is absent when zero secondary types have presence; a user-hidden section stays hidden even when the presence fetch fails (mock a failing presence response while `more_data_hidden` is `true`).
- [ ] 5.2 Cover the settings-load gating scenario added in 2.4: hold the settings GET open, assert the More Data section does not render prematurely; fail it and assert it still doesn't render unhidden.

## 6. Manual verification against WIP

- [ ] 6.1 Deploy the branch to the HealthVault WIP stack per CLAUDE.md's dry-run/E2E rules, then run the full `e2e/tests/dashboard.spec.ts` suite (including the new cases) against it before requesting review. Record the pass count here.
