## 1. Shared order/visibility model (`frontend/lib/vitals.ts`)

- [x] 1.1 Define the new saved-entry type `{ type: DataType; hidden: boolean }` alongside `PRIMARY_METRICS`.
- [x] 1.2 Extend `reconcileMetricOrder()` to accept `(string | { type: string; hidden: boolean })[] | undefined`, normalizing plain-string entries (old shape) to `{ type, hidden: false }`, and passing `{ type, hidden }` entries through as-is; keep existing drop-unknown / append-missing-as-visible behavior.
- [x] 1.3 Update `reconcileMetricOrder()`'s return type and all its callers/types accordingly.
- [x] 1.4 Widen `UserSettings.dashboard_order` in `frontend/lib/api.ts` (currently `string[]`) to accept both shapes — `(string | { type: string; hidden: boolean })[]` on read — and update its doc comment, which currently describes the key as a plain list of types.

## 2. Dashboard page state and persistence (`frontend/app/page.tsx`)

- [x] 2.1 Change `order` state to hold `{ type: DataType; hidden: boolean }[]` (via the updated `reconcileMetricOrder`).
- [x] 2.2 Add a `toggleHidden(index)` function mirroring `moveCard()`'s local-state-mutation style.
- [x] 2.3 Update `handleDone()` to write `updateSettings({ dashboard_order: order.map(m => ({ type: m.type, hidden: m.hidden })) })` — still through the existing `updateSettings` queued call, not a new writer.
- [x] 2.4 Update the read-only (non-editing) render path to filter `order` to `!hidden` before mapping to `VitalCard`s.
- [x] 2.5 Add the all-hidden placeholder: when every entry in `order` is hidden, render a placeholder message in the grid container instead of an empty grid.
- [x] 2.6 In edit mode, keep rendering every entry (including hidden ones) so they can be found and re-shown; visually distinguish hidden cards (e.g. dimmed row).
- [x] 2.7 Add the eye/eye-off toggle control next to each edit-mode card's existing move-up/move-down controls, wired to `toggleHidden`.

## 3. Vital card component

- [x] 3.1 Check `frontend/components/VitalCard.tsx` (or the edit-mode row markup in `page.tsx`, whichever renders the per-card edit-mode controls) for where to add the show/hide icon button; add a new icon to `frontend/components/icons` if an eye/eye-off icon doesn't already exist.

## 4. Localization

- [x] 4.1 Add new i18n keys for: the show/hide toggle's accessible label (shown/hidden state), and the all-hidden placeholder message — in both `frontend/lib/i18n/en.ts` and `ru.ts` (`Dictionary = typeof en` and `DICTIONARIES: Record<LanguageCode, Dictionary>` make a missing `ru` key a compile error, so this is enforced, not optional).
- [x] 4.2 Retitle the edit-mode entry control: `dashboard.editOrder` currently reads "Edit order" / "Изменить порядок", but the mode now also controls visibility. Rename the key (e.g. `dashboard.customize`) and reword both translations so the label matches what the mode does.

## 5. Tests

- [x] 5.1 ~~Add/extend unit tests for `reconcileMetricOrder()`~~ — **dropped.** The frontend has no test runner (no vitest/jest, no `*.test.ts`, no test script), so this task assumed infrastructure that doesn't exist. Rather than introduce a test framework for one function, `reconcileMetricOrder` stays covered end-to-end by 5.2, matching how the archived `dashboard-card-reorder` change shipped the same function. Decided with the user during apply.
- [x] 5.2 Extend `e2e/tests/dashboard.spec.ts` with scenarios matching the spec delta: hide a card and confirm it disappears from the read-only grid but reappears (dimmed) in edit mode; re-show a hidden card and confirm it returns to its prior position, not the end; hide every card and confirm the placeholder renders; reload/re-login and confirm hidden state persists.

- [x] 5.3 Cover the "Hidden cards are never revealed before the saved settings load" scenario: hold the settings GET open, assert no card and a loading placeholder, then fail it and assert no card and an error placeholder.

## 5a. Code-review fixes (round 1)

- [x] 5a.1 Spec delta: the MODIFIED block renamed its requirement header, which `openspec archive` (and the projected-specs CI job) rejects as "not found". Declare it as `## RENAMED Requirements` (FROM/TO) with MODIFIED on the new header, and keep the canonical **scenario** names — the CLI treats a renamed scenario as a dropped one.
- [x] 5a.2 Spec delta: also MODIFY the "Dashboard vitals grid" requirement, whose scenario asserted a flat "SHALL render 8 vital cards". Left alone, the archived spec tree would hold two contradicting requirements.
- [x] 5a.3 `frontend/app/page.tsx`: gate the grid on the settings load. `order` starts as all-visible defaults, so the grid flashed hidden cards in on every load and left them on screen for the whole session when the GET failed (with Customize disabled — no way to re-hide them). Added a loading/error placeholder and a matching spec scenario.
- [x] 5a.4 `e2e/tests/dashboard.spec.ts`: normalize visibility in `beforeEach`, not only in `finally`. Cleanup is best-effort on a shared account, so one failed run left a card hidden and every later run inverted the toggle and failed misleadingly.
- [x] 5a.5 `todo.md`: Phase 1 is shipped by this PR, so stop describing it as ready-to-propose backlog.
- [x] 5a.6 Regenerate and commit `openspec/specs.projected/` now that the delta applies cleanly (the CI drift check reads it).

## 5b. Code-review fixes (round 2)

- [x] 5b.1 `restoreAllVisible`: `isEnabled()` is one-shot and Customize renders *disabled* until the settings GET resolves, so in `beforeEach` the helper read the loading state, concluded there was nothing to restore, and returned — making 5a.4's pre-normalization a silent no-op. Wait (retrying) on Customize-or-Done being enabled, and detect edit mode from `Done` being present rather than from Customize being enabled. **Verified empirically**, not just by reading: a throwaway spec hid Sleep and left it hidden, after which the visibility tests pass with the fix and fail on the old one-shot logic with exactly the predicted `toHaveCount(0)` error.
- [x] 5b.2 `'hiding a card … persists across reload'`: the post-reload `toHaveCount(0)` ran before the grid existed (the new load gate), so it passed vacuously and would have passed even if the server had dropped the `hidden` flag. Anchor on a visible card first, then assert the absence.
- [x] 5b.3 `frontend/app/page.tsx`: the disabled Customize tooltip read "Loading your saved layout…" even after the load had failed. Gate it on `settingsStatus`, like the placeholder beside it.
- [x] 5b.4 `frontend/app/page.tsx`: the error state was terminal — one transient 500 replaced the vitals grid with a static paragraph for the whole session, with no in-page way back. Added a "Try again" control that re-runs the load, plus the `dashboard.retryLoad` key in both dictionaries, a spec scenario, and e2e coverage of the recovery path.

## 6. Manual verification against WIP

- [x] 6.1 Deploy branch to the HealthVault WIP stack per CLAUDE.md's dry-run/E2E rules, then run the full `e2e/tests/dashboard.spec.ts` suite (including the new cases) against it before requesting review. **Done:** deployed to `hcw-wip` (`class: wip`, http://192.168.1.54:8892); `dashboard.spec.ts` 15/15 pass and the full suite is 108 passed / 1 skipped / 0 failed. Also checked at a 360px viewport that the now-three-control edit row doesn't overflow a grid cell (`scrollWidth === clientWidth === 149`). **Re-verified after the round-1 review fixes:** redeployed and re-ran the full suite — `dashboard.spec.ts` 16/16, overall 108 passed / 1 skipped / 0 failed.
