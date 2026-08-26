## 1. Settings model (`frontend/lib/api.ts`)

- [x] 1.1 Add `more_data_hidden?: boolean` to the `UserSettings` interface, documented alongside its neighboring keys (`dashboard_order`, `display_language`, etc.); no shape migration needed since an absent key means "not hidden."

## 2. Dashboard page state and persistence (`frontend/app/page.tsx`)

- [ ] 2.1 Load `more_data_hidden` from settings into local state alongside the existing `order` state, defaulting to `false`/absent. Normalize strictly (`s.more_data_hidden === true`), not `s.more_data_hidden ?? false` — the settings blob is opaque, unvalidated JSON (same contract as `dashboard_order`'s `entry.hidden === true` check in `reconcileMetricOrder`, see `frontend/lib/vitals.ts`), so a malformed stored value (e.g. `"false"`, `1`) must not be treated as hidden.
- [ ] 2.2 Add a `toggleMoreDataHidden()` function mirroring the existing `toggleHidden(index)` local-state-mutation style.
- [ ] 2.3 Update `handleDone()` to include `more_data_hidden` in the same `updateSettings()` payload as `dashboard_order` — one PUT, no new writer.
- [ ] 2.4 Gate the More Data render path on `dashboardReady` (the existing page-level flag, already `settingsLoaded && presenceReady`), not only `presenceReady`, so a hidden section can't flash unhidden before the saved preference is known. `dashboardReady` is already shared at the page level (see `design.md`'s Context), so this is a condition change on the existing silent gate (`presenceReady ? [...] : []` → `dashboardReady ? [...] : []`) — no new visible loading/error UI for the More Data section; it continues to render nothing while not ready, exactly as it does today.
- [ ] 2.5 Update the read-only (non-editing) render condition from `presentSecondaryTypes.length > 0` to `!moreDataHidden && presentSecondaryTypes.length > 0`.
- [ ] 2.6 In edit mode, render the More Data section (heading, toggle, link list) whenever `presentSecondaryTypes.length > 0`, regardless of `moreDataHidden`, visually distinguished (e.g. dimmed) when hidden — mirroring the vitals grid's dimmed-hidden-card treatment.
- [ ] 2.7 When `presentSecondaryTypes.length === 0`, do not render the More Data section or its toggle at all, in either mode.

## 3. UI control

- [ ] 3.1 Add a show/hide (eye/eye-off) control to the More Data section heading, visible only in edit mode, wired to `toggleMoreDataHidden()`. Reuse the eye/eye-off icon asset from `frontend/components/icons.tsx` (added in Phase 1) rather than adding a new one.
- [ ] 3.2 Disable the new control while `saving` is true, mirroring `VitalCard`'s existing `controlsDisabled={saving}` — `handleDone`'s write closes over state at click time, so a toggle clicked mid-save would otherwise be silently dropped (the same bug class Phase 1 had to fix for the per-card controls).

## 4. Localization

- [ ] 4.1 Add new i18n keys for the More Data toggle's accessible label (shown/hidden state) in both `frontend/lib/i18n/en.ts` and `ru.ts` (`Dictionary = typeof en` and `DICTIONARIES: Record<LanguageCode, Dictionary>` make a missing `ru` key a compile error).

## 5. Tests

- [ ] 5.1 Extend `e2e/tests/dashboard.spec.ts` with scenarios matching the spec delta: hide the More Data section and confirm it disappears from the read-only page but remains discoverable (dimmed) in edit mode; re-show it; hidden state persists across reload; the toggle is absent when zero secondary types have presence; a user-hidden section stays hidden even when the presence fetch fails (mock a failing presence response while `more_data_hidden` is `true`).
- [ ] 5.2 Cover the settings-load gating scenario added in 2.4: hold the settings GET open, assert the More Data section does not render prematurely; fail it and assert it still doesn't render unhidden.
- [ ] 5.3 Extend `restoreAllVisible()` (`e2e/tests/dashboard.spec.ts` and its `e2e/tests/settings.spec.ts` counterpart) to also un-hide a hidden More Data section, and wire it into every describe block's `beforeEach` that asserts on `more-data` testid visibility (e.g. `Data-type presence filtering`) — the sibling per-card `hidden` flag hit exactly this failure mode (state left hidden on the shared seeded account silently breaking unrelated tests) and needed this same fix; `more_data_hidden` is a second, independent flag on the same shared account and needs the same cleanup from the start.
- [ ] 5.4 Cover the accepted edge case from `design.md`'s Risks section: hide the More Data section, then drive every secondary type's presence to zero (e.g. via record deletion); confirm the section and its toggle disappear from edit mode without erroring; then restore presence for one secondary type and confirm the section is still hidden in the read-only view (i.e. the stored preference survived the toggle's temporary disappearance).
- [ ] 5.5 Cover composition with a simultaneous vitals-grid change in one `Done`: in a single edit session, toggle both a vitals card's visibility/order and the More Data section's hidden state, click Done once, reload, and confirm both changes persisted — `handleDone()`'s GET-merge-PUT write path previously had a lost-update race (see `settings.spec.ts`'s "Settings lost-update race" coverage) that a second key merged into the same payload could reintroduce.
- [ ] 5.6 Cover the strict-normalization requirement from 2.1: seed the account's settings directly via `PUT /users/me/settings` with `more_data_hidden` set to a truthy-but-not-`true` value (e.g. the string `"false"`, or `1`), load the dashboard, and confirm the More Data section renders as visible (not hidden) — a naive `?? false` read would incorrectly hide it.

## 6. Manual verification against WIP

- [ ] 6.1 Deploy the branch to the HealthVault WIP stack per CLAUDE.md's dry-run/E2E rules, then run the full `e2e/tests/dashboard.spec.ts` suite (including the new cases) against it before requesting review. Record the pass count here.

## 7. Docs

- [ ] 7.1 `CONTEXT.md`: update the "Dashboard Card" glossary entry — the More Data row gains a section-level, persisted, user-configurable hide/show preference, so it is no longer accurate to describe it (alongside the needs-attention banner and Log Food row) as a fixed, "not user-configurable" section.
