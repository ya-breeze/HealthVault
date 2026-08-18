## 1. Frontend fix

- [ ] 1.1 Rename the search-mode tab label in `frontend/components/food/ItemResolver.tsx` from "Search" to "Search food" (or equivalent distinct label), leaving its `onClick={() => setMode('search')}` behavior unchanged
- [ ] 1.2 Verify the submit button (the one calling `search()`) keeps its "Search" label and behavior unchanged

## 2. E2E test updates

- [ ] 2.1 Update `e2e/tests/food.spec.ts` selectors that use `.last()` to disambiguate the two "Search" buttons to target the submit button by its new distinct role/name where practical, so the test asserts the fix rather than merely tolerating the old ambiguity
- [ ] 2.2 Run the food e2e suite against the WIP stack and confirm all search-related tests pass

## 3. Manual verification

- [ ] 3.1 Deploy to the dogfood/WIP stack and manually reproduce the original repro: open "Add item", confirm clicking the mode tab does nothing harmful (it's a no-op tab click, not a broken search), then click the actual "Search" submit button with a Cyrillic query and confirm results appear
