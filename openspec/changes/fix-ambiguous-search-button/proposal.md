## Why

On the meal review page's "Add item" panel, `ItemResolver.tsx` renders two controls both labeled exactly "Search": a mode tab (already the active/default mode) and the actual submit button that calls the search API. Clicking the tab calls `setMode('search')` with the value it already holds, so React bails out with zero re-render — no loading state, no request, no error, nothing visibly happens. A user hit this directly: they clicked the (visually prominent) tab expecting it to search and got no feedback at all, while the separate gray submit button below it worked fine. The two same-labeled controls are also why every e2e test that clicks "Search" has to disambiguate with `.last()`.

## What Changes

- Rename the mode-tab label in `ItemResolver.tsx` (e.g. to "Search food") so it is no longer identical to the submit button's "Search" label, matching the existing tab-naming pattern already used in `ManualItemEditor.tsx`.
- No change to the tab's behavior (it still just switches `mode`) or to the submit button's behavior — this is a labeling/disambiguation fix only.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `food-nutrition-logging`: adds a requirement that the item-resolution mode selector and search-submit control must carry visibly distinct labels, so this exact defect (identical "Search" labels causing a no-op click) cannot silently regress.

## Impact

- `frontend/components/food/ItemResolver.tsx` — rename the mode-tab button label.
- `e2e/tests/food.spec.ts` — tests currently disambiguate the two "Search" buttons with `.last()`; after the rename this remains correct but is no longer strictly necessary. No test behavior changes are required, though selectors may be tightened for clarity.
