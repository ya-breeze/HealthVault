## Why

HealthVault's food-logging flow currently only produces and displays English food/ingredient
names, and the UI has no language switch. A Russian-speaking user cannot read what the AI
recognized in their own language, and expert users have no way to see the original English name
once it might be added. This change lets a user set a Display Language that drives both the UI
and AI-recognized food names, while preserving an English "ground truth" name for the cases where
it matters.

## What Changes

- Add a per-user **Display Language** setting, stored as a new key in the existing
  `UserSettings` JSON blob (`GET`/`PUT /api/users/me/settings` — already implemented, no schema
  change needed).
- Add a UI language switcher driven by that setting.
- Food photo recognition SHALL request both a **Display Name** (in the user's Display Language)
  and a **Canonical Name** (English) from the AI in the same recognition call, and persist both on
  the `FoodItem`.
- **BREAKING** (behavior, not schema): when Display Language is non-English, matching SHALL skip
  querying Open Food Facts and USDA entirely for that item — no attempt to translate the Display
  Name back to English for matching. The user's own custom-food matching (see next bullet) still
  runs regardless of language, since it's matching against the user's own catalog, not an
  English-vocabulary reference database. Matched-item macro precedence and the estimated-macro
  fallback are otherwise unaffected.
- The existing case-insensitive **exact**-name custom-food match (offered as the sole candidate,
  bypassing Open Food Facts/USDA) becomes a case-insensitive **fuzzy** name match, for every
  Display Language — this loosening is not conditional on language, it's a general improvement
  that also makes non-English matching viable at all. The existing separate frequency/recency-
  ranked custom-food candidate tier is unchanged.
- Food item detail views (confirmation, meal detail, Custom Food catalog) display the stored
  Display Name by default.
- Add a per-screen, non-persisted **Expert Mode** toggle (recognition/confirmation, food history,
  Custom Food catalog) that reveals the Canonical Name alongside the Display Name.
- Historical Food Items keep the Display Name generated at recognition time — changing a user's
  Display Language later does not retroactively re-translate existing items.

Out of scope: the manual food-search bar's existing query-translation behavior
(`food-search-translation`) is unaffected — it already handles arbitrary-language search queries
and is a separate flow from photo recognition. The `preparation`/`state` enum fields are out of
scope — confirmed via grep they are not rendered anywhere in the frontend UI.

## Capabilities

### New Capabilities
- `display-language`: the per-user Display Language setting (stored in `UserSettings`), the UI
  language switcher, and passing the selected language into recognition calls.
- `expert-mode`: the per-screen, non-persisted toggle that reveals a Food Item's or Custom Food's
  Canonical Name alongside its Display Name, on the recognition/confirmation screen, food history,
  and the Custom Food catalog.

### Modified Capabilities
- `food-photo-recognition`: recognition now produces a Display Name + Canonical Name pair per
  item instead of a single English name.
- `usda-nutrition-database`: the custom-food exact-name match becomes a fuzzy name match;
  Open Food Facts/USDA querying is skipped entirely when Display Language is non-English.
- `food-nutrition-logging`: `FoodItem` and `CustomFood` persist both a Display Name and a
  Canonical Name, exposed on item-detail and Custom Food catalog responses.

## Impact

- **Backend**: `FoodItem`/`CustomFood` models gain `display_name` and `canonical_name` fields
  (in addition to or replacing the current single `name` field — see design.md); vision/recognition
  prompt and response parsing change to request/parse both names; item-resolution logic gains a
  language-conditional branch that skips reference-DB search; custom-food matching logic changes
  from exact to fuzzy (needs a fuzzy-matching approach — see design.md).
- **Frontend**: language switcher UI (writes `display_language` via the existing settings API);
  i18n scaffolding for translating static UI strings; Expert Mode toggle component reused across
  three screens; meal history, confirmation, and catalog views read `display_name`/`canonical_name`
  instead of a single name field.
- **Dependency**: relies on the already-implemented per-user `UserSettings` store
  (`openspec/specs/user-settings/spec.md`, confirmed present and implemented on `main`) — the
  external "other stream" this change was originally waiting on has landed, so no cross-stream
  sequencing is needed.
- **No new external dependency**: the AI vision/recognition provider is already called for
  photo recognition; this only changes the prompt and expected response shape, not the provider.
