## Why

`GET /api/food/search` matches free text against USDA FoodData Central using literal
term matching, and USDA's corpus uses American-English generic-food naming only. A
user searching "Porridge" gets zero results — not a bug in the matching logic, but a
real vocabulary gap: none of USDA's 7,793 rows contain that word (it uses "oatmeal"
instead). The same gap blocks searching in any other language, e.g. Russian
("овсянка"). Users need to be able to search in their own words and have the system
find the right USDA reference food.

## What Changes

- Add a per-user query-translation cache (`FoodSearchTranslation`) that maps a
  normalized free-text query to the USDA-vocabulary English term it resolves to.
- Add `vision.Client.Translate`, a text-only LLM call (reusing the existing OpenAI
  integration already used for photo recognition) that produces that English term
  for a query in any language or regional spelling.
- `GET /api/food/search` now translates the query before searching USDA: cache hit
  skips the LLM entirely; cache miss calls `Translate`, stores the result, and uses
  it. Custom-food exact-match lookup is unaffected (still runs first, untranslated).
- Add `refresh=true` to `GET /api/food/search`: forces a fresh `Translate` call and
  overwrites the cached mapping in place, for when a cached translation turns out to
  be wrong.
- `FoodSearchResponse` gains an optional `translated_query` field so the frontend can
  show what was actually searched and offer a refresh action.
- `ItemResolver.tsx` and `ManualItemEditor.tsx` display the translated term next to
  search results and a refresh control, both only when translation was applied and
  differs from what the user typed — except on a successful `refresh=true`, which
  always shows its resulting term, even when it equals what the user typed, so an
  omitted term on a refresh response unambiguously means the refresh failed.
- The search interface states, before a query is ever sent, that new search text may
  be translated by an external model provider — the same disclosure pattern already
  used for photo uploads (`food-photo-recognition`'s "external model provider" notice).

Behavior is unchanged for an English query that already matches directly, and
unchanged entirely when the vision client is unconfigured (falls back to today's
literal-only search, as it does now when USDA itself is unavailable).

## Capabilities

### New Capabilities
- `food-search-translation`: per-user free-text-to-USDA-vocabulary query translation
  for `GET /api/food/search`, including caching, user-triggered refresh, an explicit
  translation deadline, and disclosure of external transmission.

### Modified Capabilities
- `data-model`: adds `FoodSearchTranslation` as a fifth food-logging tenant table.
  `openspec/specs/data-model/spec.md`'s "Food logging tables" requirement normatively
  inventories the food-logging tables by name and count; this change adds a table, so
  that inventory needs a delta rather than being silently left stale after archive.

## Impact

- **Backend**: `backend/pkg/server/food.go` (`Search` handler), `backend/pkg/vision/`
  (new `Translate` method on `Client`, `OpenAIClient`, `Unconfigured`, `Fake`, and the
  server-package test doubles `slowRecognizeClient`, `slowClarifyClient`,
  `gatedRecognizeClient` — all four non-`OpenAIClient` implementations must gain the
  method or the build fails), `backend/pkg/database/` (new `FoodSearchTranslation`
  model + migration).
- **Frontend**: `frontend/components/food/ItemResolver.tsx`,
  `frontend/components/food/ManualItemEditor.tsx`, `frontend/lib/api.ts`
  (`searchFood` gains `refresh` and the response type gains `translated_query`).
- **Tests**: `backend/pkg/server/food_search_test.go` (new cache/refresh/fallback/
  timeout cases), `e2e/tests/food.spec.ts` (extend existing search mocks).
- **Dependencies**: none new — reuses the existing OpenAI credential
  (`HCW_OPENAI_API_KEY`) already configured for photo recognition.
