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

## 6. Manual verification against WIP

- [ ] 6.1 Deploy branch to the HealthVault WIP stack per CLAUDE.md's dry-run/E2E rules, then run the full `e2e/tests/dashboard.spec.ts` suite (including the new cases) against it before requesting review.
