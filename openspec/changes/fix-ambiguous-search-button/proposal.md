## Why

Two independent components in the food-entry UI each pair a mode-selector tab with a search-submit button in a way that lets the tab pass for the "go" control:

- **`ItemResolver.tsx`** (meal review page, "Add item" panel): the mode tab and the submit button are both labeled exactly "Search". Since search mode is selected by default, clicking the tab calls `setMode('search')` with the value it already holds — a primitive-state `useState` setter, so React bails out with zero re-render: no loading state, no request, no error, nothing visibly happens. Confirmed independently by e2e tests, which all have to disambiguate the two "Search"-labeled buttons with `.last()`.
- **`ManualItemEditor.tsx`** (manual meal-entry page, `/food/manual`): the mode tab is labeled "Search food" and already carries a solid blue fill (`bg-blue-600`, active by default), while the actual submit button is labeled "Search" but styled as a plain gray control (`bg-gray-100`) — visually secondary despite being the only control that fires a request. A user reported exactly this: typing a food name (Cyrillic: "Перепелиное яйцо") then clicking the prominent blue "Search"-labeled control produced no visible result, while the small gray "Search" button beside it worked. Unlike `ItemResolver`, this isn't a React same-value bail-out (the tab click does update state via `update()`, which always creates a new object) — it's a visual-affordance defect: the wrong control looks like the primary action.

Both share the same underlying shape: a colored, prominent mode-tab that reads as the actionable "search" control, positioned next to the actual (less prominent or identically-labeled) submit button. There is currently zero e2e coverage of the search path on `/food/manual` (the one existing manual-entry test only exercises "Enter macros" mode), which is likely why this went unnoticed.

## What Changes

- `ItemResolver.tsx`: rename the mode-tab label from "Search" to "Search food", removing the literal duplicate label.
- Both `ItemResolver.tsx` and `ManualItemEditor.tsx`: restyle the mode tabs to use a non-filled "selected" indicator (e.g. an underline/border accent, not a solid colored pill) instead of a solid button-like fill, and ensure the actual search-submit button in each component is the one solid, colored, primary-looking control in its panel. This makes "solid colored button = triggers an action" a consistent, unambiguous signal in both places, rather than relying on labels alone.
- `ManualItemEditor.tsx`: match its search-submit button's padding/rounding/font-weight to `ItemResolver.tsx`'s, move it to the right of the weight field (matching `ItemResolver.tsx`'s input-then-button order), and let the weight field fill the row's available width instead of a fixed width — so the two panels' search rows read as the same control pattern, not just the same color rule.
- No change to either control's underlying behavior — the tab still only switches `mode`/`source`, the submit button still only calls `search()`.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `food-nutrition-logging`: adds a requirement that, in the item-resolution UI (both the meal-review "Add item" panel and the manual meal-entry form), the mode-selector control(s) and the search-submit control are visually and textually distinguishable, so the submit control cannot be mistaken for the mode selector or vice versa.

## Impact

- `frontend/components/food/ItemResolver.tsx` — rename the mode-tab label; restyle mode-tab "active" state.
- `frontend/components/food/ManualItemEditor.tsx` — restyle mode-tab "active" state; restyle the search-submit button to be the visually primary control, match its styling/order to `ItemResolver.tsx`'s, and make the weight field fill available width.
- `e2e/tests/food.spec.ts` — tighten the existing `.last()`-disambiguated "Search" selectors on the review page now that labels are unique; add new coverage for the manual-entry page's search path (currently untested), including a non-ASCII (Cyrillic) query to directly cover the reported case.
