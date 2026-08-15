## 1. Shared Component

- [ ] 1.1 Create `frontend/components/ui/TapTarget.tsx`: a button/icon-button wrapper enforcing a minimum 48×48px (`min-h-12 min-w-12`) clickable area, usable both as a plain button and wrapping an icon-only control. Spread all native button props through via `{...rest}` (not an allowlist) so `title`, `aria-label`, `data-testid`, `disabled`, `onClick`, and anything else pass through unchanged — several existing controls are located by these attributes in `e2e/tests/food.spec.ts` (e.g. `button[title="Delete item"]` for `MealItemRow`'s delete button, `data-testid="clarify-modal"` for the Clarify modal), and migrating them must not break those selectors.
- [ ] 1.2 Add a brief usage note (top-of-file comment or short README section) stating the 48px minimum standard, so future components in this flow use `TapTarget` by default rather than a bare `<button>`.

## 2. Migrate Review Flow Controls

- [ ] 2.1 `MealItemRow.tsx`: migrate the delete control (already at 44px from PR #15 — bump to the new 48px standard) and its confirm/cancel pair to `TapTarget`.
- [ ] 2.2 `ItemResolver.tsx`: migrate to `TapTarget` — the search/manual-entry mode toggle buttons, the search-result picker buttons (`<ul>` items), the Search submit button, the translation-refresh icon button (currently has no sizing classes at all), and the manual-mode Save button.
- [ ] 2.3 `ManualItemEditor.tsx`: migrate search-result picker buttons, mode toggle buttons, and any other unsized action buttons in this file (audit it the same way as 2.2 — check for a search-submit and save button here too) to `TapTarget`.
- [ ] 2.4 `MealMetaEditor.tsx`: migrate edit/cancel/save controls to `TapTarget`.
- [ ] 2.5 `DeleteMealControl.tsx`: migrate confirm/cancel controls to `TapTarget`.
- [ ] 2.6 `AddItemForm.tsx`: migrate its action button(s) to `TapTarget`.
- [ ] 2.7 `ReanalyzeControl.tsx`: migrate its action button(s) to `TapTarget`.

## 3. Migrate Modals

- [ ] 3.1 `ClarifyModal.tsx`: migrate the Submit button to `TapTarget`. (This component currently has no separate Cancel/Close button — only Submit, plus an optional `deleteControl` slot rendered by the caller, which is covered by `DeleteMealControl`'s own migration in 2.1/2.5.) Sizing only — do not add overflow/scroll-cap changes; that's out of scope for this change.
- [ ] 3.2 `CustomFoodModal.tsx`: audit its actual buttons first (do not assume a submit/cancel/close set matches `ClarifyModal`'s) and migrate whatever controls exist to `TapTarget`. Sizing only, same scope note as 3.1.

## 4. Migrate List Screens

- [ ] 4.1 Food history list: migrate row action controls to `TapTarget`.
- [ ] 4.2 Custom foods list (`app/food/custom/page.tsx`): migrate Edit/Delete/Confirm/Cancel controls to `TapTarget` (sizing only — do not fix the `min-w-0`/`truncate`/`flex-shrink-0` layout-overflow issue in this file; leave a short code comment marking it for the follow-up change). Note: this row's controls sit right next to an un-truncated food name with no `flex-shrink-0` protection, so enlarging them to 48px can make that pre-existing overflow bug surface more visibly than before — check the row at 375px width after migrating; if it crowds, that's the known deferred issue, not a new regression, but confirm the buttons themselves stay fully visible and tappable.

## 5. Migrate Header and Toast

- [ ] 5.1 `Header.tsx`: migrate nav/menu controls from the existing 44px sizing to `TapTarget` (48px).
- [ ] 5.2 `Toast.tsx`: migrate the dismiss control to `TapTarget`.

## 6. Verification

- [ ] 6.1 Add a Playwright test at a mobile viewport (e.g. 375×667) that renders the review page (with a meal item present) and asserts the delete control, a search-result picker item, and the meal-meta edit control each have a bounding box of at least 48×48px via `getBoundingClientRect()` / `boundingBox()`. Note in the test file that this is a representative spot-check of a few controls, not exhaustive coverage of every control the "every interactive control" requirement names — full coverage is manual (6.4).
- [ ] 6.2 Extend or add a test asserting the header nav controls and toast dismiss control meet the 48×48px minimum at a mobile viewport.
- [ ] 6.3 Run the full e2e suite against the deployed `hcw-wip` stack to confirm no regressions in existing flows (add-item, delete-item, confirm, clarify, custom food, history) — this is also the regression check that `TapTarget`'s prop passthrough (task 1.1) didn't break any existing `title`/`data-testid`/`aria-label`-based selector.
- [ ] 6.4 Manually verify the review, history, custom-foods, and modal flows on a real Android phone before merge; note the result in the PR.

## 7. Docs

- [ ] 7.1 Confirm `design.md`'s stated 48px standard reads correctly as the documented rule for future controls in this codebase (no separate doc needed beyond the OpenSpec artifacts and the `TapTarget` usage note from 1.2).
