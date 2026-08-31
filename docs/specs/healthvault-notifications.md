# Toasts clear a page's bottom action bar
Idea: ya-breeze/idea-forge#172

## Why

Editing an item's weight on `/food/review/` raises a toast that covers the Confirm Meal button.

The review page renders its confirm bar as a fixed element anchored at `bottom: var(--nav-block)` (`frontend/app/food/review/ReviewClient.tsx:302`). The app-wide toast stack is anchored at `bottom: calc(1rem + var(--nav-block) + var(--edge-inset-b))` (`frontend/components/Toast.tsx:54`), which is 1rem above the mobile navigation bar — and therefore inside the confirm bar, which is roughly 4.5rem tall. The toast is `z-50`, the confirm bar is `z-30`, and the toast card sets `pointer-events-auto` so its dismiss button works. The toast therefore both hides the confirm button and swallows taps aimed at it for the full 3s auto-dismiss window.

Every weight edit triggers this. `MealItemRow`'s `commitWeight` calls `onUpdated(..., t('item.updated'))` on blur (`frontend/components/food/MealItemRow.tsx:86`), which reaches `applyMealUpdate` in `ReviewClient` and shows a success toast (`frontend/app/food/review/ReviewClient.tsx:78`). Adjusting the weights of several items — the normal way to correct an analysed meal — puts a toast over the confirm button after each one.

The same collision exists on `/food/manual/`, whose Save Meal bar is the same markup (`frontend/app/food/manual/page.tsx:95`), and at every viewport width: above the `sm` breakpoint `--nav-block` is `0px`, so the toast sits at `bottom-4` and the submit bar sits at `bottom: 0`, still overlapping.

ADR-008 established that the bottom navigation bar's clearance is a CSS token every bottom-anchored element reads, and named the gap this idea falls into: the tokens describe the space the *navigation bar* occupies, and nothing describes the space a *page's own* bottom bar occupies. The toast clears the navigation bar correctly — `e2e/tests/mobile-nav.spec.ts:436` asserts exactly that — and lands on the submit bar sitting on top of it. Nothing catches it, because no existing assertion compares the toast with a page's submit bar.

## How

Give the toast stack the same treatment ADR-008 gave the navigation bar: make the space a page's bottom bar occupies a value the toast reads, rather than a height the toast has to guess.

A CSS token cannot carry it. The bar exists on two pages, is conditional on `showConfirmBar` on one of them, and its height depends on the rendered button and the safe-area inset — so the value is only known at runtime, and the toast stack is in a different React subtree from the page that renders the bar. The mechanism is therefore a small registration context: a bar reports its measured border-box height while it is mounted, and the toast stack offsets by the tallest reported height.

A new client component, `frontend/components/ui/BottomActionBar.tsx`, owns both halves. It exports `BottomActionBarProvider` (the registry), `useBottomActionBarHeight()` (what the toast stack reads), and a default `BottomActionBar` component carrying the fixed-bar markup that `/food/manual/` and `/food/review/` duplicate today. Both pages switch to it, so the registration is structural rather than something each page must remember — the same reasoning that put tap-target sizing inside `TapTarget` and navigation-bar clearance inside `AuthenticatedShell`. A third bottom bar added later gets the clearance by using the component, which is the closest this codebase can get to closing ADR-008's stated hole ("cannot catch a *fourth* site added later").

The offset arithmetic has one subtlety worth stating, because ADR-008 turns on absorbing the safe-area inset exactly once. The bar already carries `--edge-inset-b` inside its own bottom padding, so that inset is inside the height the toast reads. The toast's offset therefore branches: with a bar registered it is `calc(1rem + var(--nav-block) + <height>px)`, and with none it stays exactly what it is today, `calc(1rem + var(--nav-block) + var(--edge-inset-b))`. Adding `--edge-inset-b` in the first branch too would double-count it on a notched device in landscape.

Measurement uses a `ResizeObserver` on the bar element, so the value tracks the bar growing (a wrapped button label, a longer translation) and viewport changes across the `sm` breakpoint without a resize listener of its own. Registration is keyed by `useId`, and the registry reports the maximum of the registered heights, so two bars mounted at once cannot leave the toast reading a stale one. Unmounting a bar — which is what confirming a meal does, since `showConfirmBar` goes false — unregisters it and drops the toast back to its current position.

The provider is mounted in `frontend/app/layout.tsx` wrapping `ToastProvider`, which is where `ToastProvider` already sits. `useToast` throws outside its provider; the new hook does the same, for the same reason.

The e2e guard goes in `e2e/tests/mobile-nav.spec.ts`, next to the existing bounding-box cases and reusing their `login`, `boxOf` and `intersects` helpers. It asserts what a screenshot review misses and what a `z-index`-only fix would not satisfy: the toast's box and the bar's box do not intersect, and the confirm button is still clickable while the toast is on screen. It runs at both viewports, because the collision exists at both.

Deliberately excluded:

- **Moving toasts to the top of the screen.** It would fix the collision in one line, but the header is not fixed (`frontend/components/Header.tsx:129`), so a top-anchored toast covers the app title and, on desktop, the settings and logout controls at scroll position 0. That trades one occluded control set for another, and moves the feedback away from the control that produced it.
- **Making the toast card `pointer-events-none`.** Taps would reach the button again, but the button would still be invisible under the toast, which is what the idea reports.
- **Reducing how often the review page raises a toast.** A toast per weight edit is noisy, but this change is about where they land. Toast frequency and content are untouched.
- **The `pb-24` in-flow padding both pages apply to `AuthenticatedShell` for their submit bar** (`frontend/app/food/review/ReviewClient.tsx:189`, `frontend/app/food/manual/page.tsx:47`). It is a hardcoded height of the same family, but it is scrolled content clearance rather than the reported defect, and folding it into the shared component means changing what the page scrolls to on two pages.
- **The full-screen overlays ADR-008 already exempts** — `ClarifyModal`, `CustomFoodModal`, `CameraCapture` and the More sheet. They cover the whole viewport on purpose.

The decision is recorded as `docs/adr/ADR-011`, because it extends ADR-008's cross-cutting rule with a second obligation, and a future bottom-anchored element has to know about it. ADR-008 gets an `> **Update (ADR-011):** …` note rather than a rewrite, per the ADR rules.

One coverage limit, stated rather than papered over: the safe-area half is not testable here. Headless Chromium reports `env(safe-area-inset-bottom)` as 0 and offers no way to set it, so the exactly-once reasoning above is verified by inspection and by the zero-inset case, exactly as ADR-008 recorded for the navigation bar.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Bottom action bar registry
- [x] Add `frontend/components/ui/BottomActionBar.tsx` as a `'use client'` module exporting `BottomActionBarProvider`, `useBottomActionBarHeight()` and a default `BottomActionBar` component.
- [x] Hold the registry as a map from `useId` key to measured height, and have `useBottomActionBarHeight()` return the maximum, or `0` when nothing is registered.
- [x] Have `BottomActionBar` render the fixed bar markup both pages use today — `fixed bottom-[var(--nav-block)] left-0 right-0 z-30 bg-gray-50/95 dark:bg-gray-900/95 backdrop-blur border-t border-gray-200 dark:border-gray-700 px-6 py-3 pb-[calc(0.75rem+var(--edge-inset-b))]` — plus `data-testid="bottom-action-bar"`, keeping the inner `max-w-md mx-auto` wrapper and accepting `children`.
- [x] Keep the `fixed` and `z-30` classes on that element: `e2e/tests/mobile-nav.spec.ts` locates both submit bars as `.fixed.z-30`, and those cases must keep passing unchanged.
- [x] Measure the element's border-box height with a `ResizeObserver`, register it while mounted, and unregister it on unmount so the height returns to `0`.
- [x] Throw from `useBottomActionBarHeight()` when it is called outside the provider, matching `useToast` in `frontend/components/Toast.tsx`.
- [x] Move ADR-008's offset explanation — currently duplicated as comments at `frontend/app/food/manual/page.tsx:87` and `frontend/app/food/review/ReviewClient.tsx:298` — into this component, and state in the same comment what the registration is for.
- [x] Mount `BottomActionBarProvider` around `ToastProvider` in `frontend/app/layout.tsx`.
- [x] Mark completed

### Task 2: The toast stack clears the registered bar
- [x] In `frontend/components/Toast.tsx`, read `useBottomActionBarHeight()` and set the stack container's `bottom` through an inline style instead of the current `bottom-[calc(...)]` class.
- [x] Use `calc(1rem + var(--nav-block) + <height>px)` when a bar is registered, and `calc(1rem + var(--nav-block) + var(--edge-inset-b))` when none is.
- [x] Comment why the two branches differ: the bar already carries `--edge-inset-b` in its own bottom padding, so that inset is inside the measured height and must not be added a second time.
- [x] Leave the container's `pointer-events-none`, the card's `pointer-events-auto`, `z-50`, the variant styles and the 3s auto-dismiss untouched.
- [x] Mark completed

### Task 3: Both submit bars use the shared component
- [x] Replace the fixed submit bar in `frontend/app/food/manual/page.tsx` with `<BottomActionBar>`, keeping the Save Meal `TapTarget` and its disabled/saving behaviour exactly as they are.
- [x] Replace the fixed confirm bar in `frontend/app/food/review/ReviewClient.tsx` with `<BottomActionBar>`, still rendered only when `showConfirmBar` is true, and keeping `canConfirm`, `handleConfirm` and the button's labels unchanged.
- [x] Leave each page's `pb-24` shell padding as it is; it is out of this change's scope.
- [x] Confirm no other bottom-anchored page element exists that should adopt the component — `grep` for `fixed bottom` under `frontend/` finds only `BottomNav`, the toast stack and these two bars.
- [x] Mark completed

### Task 4: End-to-end guard
- [x] Add a `test.describe` to `e2e/tests/mobile-nav.spec.ts` for "a toast does not occlude a page's own bottom action bar", reusing the file's `login`, `boxOf` and `intersects` helpers.
- [x] Mock `**/api/food/meals/<id>` GET with a `pending_review` meal carrying one item (`id`, `meal_id`, `name`, `macro_source: 'reference'`, `weight_grams`, `confidence` and the macro fields of `FoodItem`), and mock `**/api/food/meals/<id>/items/*` PATCH to return the updated meal, so the weight edit resolves without touching the deployed data.
- [x] At the 390x844 mobile viewport: load `/food/review/?meal=<id>`, change the item's weight input and blur it, wait for the toast, and assert the toast's box does not intersect the bar's box.
- [x] Assert in the same case that the confirm button is still enabled and still hit-testable while the toast is visible — `click({ trial: true })` — since a control hidden under the toast is the reported defect and a stacking-order-only fix would not satisfy this.
- [x] Repeat both assertions at the 1280x800 desktop viewport, where `--nav-block` is `0px` and the bar sits at the screen edge.
- [x] Assert that with no bar on screen the toast returns to its base offset: raise a toast on a page that renders no `BottomActionBar` and check its bottom edge sits within about 1rem of the navigation bar's top edge.
- [x] Keep `the review page's submit bar clears it, and so does a toast` passing as written; it covers the toast against the navigation bar, which this change must not regress.
- [x] Mark completed

### Task 5: Record the decision and validate
- [x] Add `docs/adr/ADR-011-bottom-action-bars-register-their-height.md` with `Status: Proposed`, stating the obligation: a page's bottom-anchored action bar renders through `BottomActionBar`, and anything anchored above it reads the registered height rather than a literal.
- [x] Add a `> **Update (ADR-011):** …` note to `docs/adr/ADR-008-bottom-clearance-as-a-css-token.md` pointing at it, without rewriting ADR-008.
- [x] Run `make lint` and `make test`, and fix anything they report.
- [x] Build the frontend (`cd frontend && npm run build`) to catch type errors, since `make lint` covers the Go backend only and the project has no frontend linter.
- [ ] Deploy the branch to the `hcw-wip` stack and run `make test-e2e` against it, fixing failures before reporting the change done.
- [ ] State plainly that the non-zero safe-area-inset case is a manual check on a notched device, since headless Chromium reports the inset as 0.
- [ ] Flip ADR-011 from `Proposed` to `Accepted` as the last commit on the branch.
- [ ] Mark completed
