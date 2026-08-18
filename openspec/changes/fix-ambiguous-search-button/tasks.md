## 1. ItemResolver.tsx (meal review page)

- [x] 1.1 Rename the mode tab's label from "Search" to "Search food", leaving its `onClick={() => setMode('search')}` behavior unchanged
- [x] 1.2 Restyle the active mode tab from a solid `bg-amber-600` fill to a non-filled selected-state indicator (e.g. underline/border accent)
- [x] 1.3 Confirm the submit button (calls `search()`) remains the panel's one solid-filled control, unchanged behavior

## 2. ManualItemEditor.tsx (manual meal-entry page)

- [x] 2.1 Restyle the active mode tab from a solid `bg-blue-600` fill to a non-filled selected-state indicator, matching the pattern used in ItemResolver.tsx
- [x] 2.2 Restyle the search-submit button from `bg-gray-100 text-gray-700` to a solid colored fill (e.g. `bg-blue-600 hover:bg-blue-700 text-white`) so it is the row's visually primary control; behavior (`onClick={search}`) unchanged
- [x] 2.3 Verify tap targets stay at the existing 48×48 minimum after the restyle (via `TapTarget`)

## 3. E2E test updates

- [x] 3.1 Tighten the `.last()`-disambiguated "Search" selectors in `e2e/tests/food.spec.ts` (review-page tests) to target the submit button by its now-unique role/name
- [x] 3.2 Add a new e2e test exercising the reference-search path on `/food/manual` (currently uncovered — the only existing manual-entry test uses "Enter macros" mode), including a non-ASCII/Cyrillic query, asserting results render after clicking the actual submit button
- [ ] 3.3 Run the full food e2e suite against the WIP stack and confirm all tests pass

## 4. Manual verification

- [ ] 4.1 Deploy to the WIP stack and manually reproduce the original repro on `/food/manual`: type a Cyrillic food name, confirm the mode tab and submit button are visually distinct, click the actual submit button, and confirm USDA results appear
- [ ] 4.2 Repeat on the meal-review page's "Add item" panel to confirm the ItemResolver.tsx fix as well
