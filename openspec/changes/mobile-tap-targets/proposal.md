## Why

A prior mobile-friendliness pass (PR #15) fixed input font sizes, header nav target sizes, and a handful of review-page controls (the delete-item button and its confirm/cancel pair got a 44px minimum). But most of the interactive controls used to search, select, and edit food items in this flow — search-result picker rows, the search submit button, the manual-entry mode toggles and save button, the translation-refresh icon button, and controls in other components (history, custom foods, modals, header) — were never sized at all, and are still too small to reliably tap on a real Android phone. That pass was an ad-hoc commit with no written standard and no shared sizing primitive, so the fix didn't propagate past the few elements it directly touched, and there is nothing today that stops a new small control from being added.

## What Changes

- Adopt 48×48dp as the minimum tap-target size for interactive controls in the food flow (Android Material Design guideline, matching the user's actual device — not the 44px iOS/WCAG figure).
- Add a shared `TapTarget` component (button/icon-button wrapper enforcing the 48px minimum) and migrate every in-scope interactive control to use it, rather than patching sizes on individual elements. `TapTarget` spreads all native button props through (`title`, `aria-label`, `data-testid`, `disabled`, `onClick`, etc.) so migrating a control doesn't drop attributes the existing e2e suite locates it by — e.g. `MealItemRow`'s delete button, which `e2e/tests/food.spec.ts` finds via `button[title="Delete item"]`.
- Audit and fix undersized controls across: food upload, manual entry, review (item rows, search-result pickers, meal meta edit, delete-meal, add-item, reanalyze), the Clarify and Custom Food modal buttons, the food history list row actions, the custom foods list row actions, and the app header/toast dismiss control (bumping the existing 44px header targets from PR #15 up to the new 48px standard for consistency).
- Add a Playwright test at a mobile viewport that asserts the audited controls meet the 48px minimum, so the standard is enforced going forward instead of relying on manual checks alone.

**Explicitly out of scope** (deferred to a separate follow-up change): horizontal layout overflow — the custom foods list row missing `min-w-0`/`truncate`/`flex-shrink-0`, and the Clarify/Custom Food modal cards missing `overflow-hidden`, which can push controls physically off-screen inside the fixed modal backdrop. This change only makes controls big enough to tap; it does not fix layout that can push them out of view.

## Capabilities

### New Capabilities
- `mobile-touch-targets`: minimum tap-target size standard for interactive controls in the food flow, enforced via a shared component and verified by an automated mobile-viewport test.

### Modified Capabilities
(none — this introduces a new cross-cutting UI standard rather than changing the requirements of an existing capability)

## Impact

- New shared component: `frontend/components/ui/TapTarget.tsx` (or equivalent), used by the components below.
- Frontend components touched: `MealItemRow.tsx`, `ItemResolver.tsx`, `ManualItemEditor.tsx`, `MealMetaEditor.tsx`, `DeleteMealControl.tsx`, `AddItemForm.tsx`, `ReanalyzeControl.tsx`, `ClarifyModal.tsx`, `CustomFoodModal.tsx`, food history list, custom foods list, `Header.tsx`, `Toast.tsx`.
- New Playwright spec (mobile viewport) asserting minimum control size across the audited components.
- No backend, API, or data-model changes.
