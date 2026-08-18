## Context

`ItemResolver.tsx` (used from `AddItemForm.tsx` on the meal review page) offers two ways to resolve an "Add item" row: searching a food database, or entering macros manually. It implements this as a two-tab switcher (`mode: 'search' | 'manual'`) plus, within the search tab, a text input and a submit button that calls `api.searchFood`.

Both the search-mode tab and the submit button currently render the literal label "Search". Since `mode` defaults to `'search'`, the tab is already selected when the panel opens. Clicking it calls `setMode('search')` with its current value; React's `useState` setter bails out on a same-value update, producing no re-render and, critically, no call into `search()`. The user sees no loading indicator, no error, and no results — indistinguishable from the app being broken, when in fact the actual submit button one row below works correctly.

## Goals / Non-Goals

**Goals:**
- Make the mode tab and the submit button visually and textually distinguishable, so a user cannot mistake one for the other.
- Preserve all existing behavior of both controls (tab still only switches `mode`; button still only calls `search()`).

**Non-Goals:**
- Not changing the search/translation backend logic (`food.go` `Search` handler, OpenAI translation) — it already works correctly once the real submit button is invoked.
- Not restructuring `ItemResolver`'s tab/panel layout beyond the label change.

## Decisions

- Rename the mode tab's label from `"Search"` to `"Search food"`, matching the existing sibling tab pattern in `ManualItemEditor.tsx` (which already uses a descriptive, non-generic label for its own mode tab rather than a bare verb). The submit button keeps its label as `"Search"` since it is the actual action being taken once the tab is selected.
- No component restructuring: this is a one-line label change plus corresponding e2e selector updates, not a rewrite of the mode-switch UI.

## Risks / Trade-offs

- [Risk] E2e tests currently rely on `.last()` to disambiguate the two "Search" buttons; after the rename this is no longer necessary but still functionally correct, so tests are not required to change to keep passing. → We will still tighten the affected selectors to name the tab explicitly, since leaving `.last()` behind a fix for the exact bug it exists to work around should not be self-verifying.
- [Risk] A future contributor could reintroduce a duplicate-label control elsewhere in the resolver. → Out of scope for this fix; not adding an automated lint for this narrow case given the project's scale.
