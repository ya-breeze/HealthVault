## 1. Shared Component

- [ ] 1.1 Create `frontend/components/ui/TapTarget.tsx`: a button/icon-button wrapper enforcing a minimum 48×48px (`min-h-12 min-w-12`) clickable area, usable both as a plain button and wrapping an icon-only control, preserving `onClick`, `disabled`, `aria-label`, and `className` passthrough.
- [ ] 1.2 Add a brief usage note (top-of-file comment or short README section) stating the 48px minimum standard, so future components in this flow use `TapTarget` by default rather than a bare `<button>`.

## 2. Migrate Review Flow Controls

- [ ] 2.1 `MealItemRow.tsx`: migrate the delete control to `TapTarget`.
- [ ] 2.2 `ItemResolver.tsx`: migrate search-result picker buttons and the search/manual-entry mode toggle buttons to `TapTarget`.
- [ ] 2.3 `ManualItemEditor.tsx`: migrate search-result picker buttons and mode toggle buttons to `TapTarget`.
- [ ] 2.4 `MealMetaEditor.tsx`: migrate edit/cancel/save controls to `TapTarget`.
- [ ] 2.5 `DeleteMealControl.tsx`: migrate confirm/cancel controls to `TapTarget`.
- [ ] 2.6 `AddItemForm.tsx`: migrate its action button(s) to `TapTarget`.
- [ ] 2.7 `ReanalyzeControl.tsx`: migrate its action button(s) to `TapTarget`.

## 3. Migrate Modals

- [ ] 3.1 `ClarifyModal.tsx`: migrate submit/cancel/close buttons to `TapTarget` (sizing only — do not add overflow/scroll-cap changes; that's out of scope for this change).
- [ ] 3.2 `CustomFoodModal.tsx`: migrate submit/cancel/close buttons to `TapTarget` (sizing only, same scope note as 3.1).

## 4. Migrate List Screens

- [ ] 4.1 Food history list: migrate row action controls to `TapTarget`.
- [ ] 4.2 Custom foods list (`app/food/custom`): migrate Edit/Delete/Confirm/Cancel controls to `TapTarget` (sizing only — do not fix the `min-w-0`/`truncate`/`flex-shrink-0` layout-overflow issue in this file; leave a short code comment marking it for the follow-up change).

## 5. Migrate Header and Toast

- [ ] 5.1 `Header.tsx`: migrate nav/menu controls from the existing 44px sizing to `TapTarget` (48px).
- [ ] 5.2 `Toast.tsx`: migrate the dismiss control to `TapTarget`.

## 6. Verification

- [ ] 6.1 Add a Playwright test at a mobile viewport (e.g. 375×667) that renders the review page (with a meal item present) and asserts the delete control, a search-result picker item, and the meal-meta edit control each have a bounding box of at least 48×48px via `getBoundingClientRect()` / `boundingBox()`.
- [ ] 6.2 Extend or add a test asserting the header nav controls and toast dismiss control meet the 48×48px minimum at a mobile viewport.
- [ ] 6.3 Run the full e2e suite against the deployed `hcw-wip` stack to confirm no regressions in existing flows (add-item, delete-item, confirm, clarify, custom food, history).
- [ ] 6.4 Manually verify the review, history, custom-foods, and modal flows on a real Android phone before merge; note the result in the PR.

## 7. Docs

- [ ] 7.1 Confirm `design.md`'s stated 48px standard reads correctly as the documented rule for future controls in this codebase (no separate doc needed beyond the OpenSpec artifacts and the `TapTarget` usage note from 1.2).
