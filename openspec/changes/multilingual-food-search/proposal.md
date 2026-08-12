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
  search results and a refresh control, both only when translation was applied.

Behavior is unchanged for an English query that already matches directly, and
unchanged entirely when the vision client is unconfigured (falls back to today's
literal-only search, as it does now when USDA itself is unavailable).

## Capabilities

### New Capabilities
- `food-search-translation`: per-user free-text-to-USDA-vocabulary query translation
  for `GET /api/food/search`, including caching and user-triggered refresh.

### Modified Capabilities
(none — `/api/food/search`'s entry in the `data-api` endpoint table already reads
"Search custom + USDA foods" and needs no wording change; this change adds an
optional query param and response field, not a new endpoint or a behavior change to
an existing documented requirement.)

## Impact

- **Backend**: `backend/pkg/server/food.go` (`Search` handler), `backend/pkg/vision/`
  (new `Translate` method on `Client`, `OpenAIClient`, and `Unconfigured`),
  `backend/pkg/database/` (new `FoodSearchTranslation` model + migration).
- **Frontend**: `frontend/components/food/ItemResolver.tsx`,
  `frontend/components/food/ManualItemEditor.tsx`, `frontend/lib/api.ts`
  (`searchFood` gains `refresh` and the response type gains `translated_query`).
- **Tests**: `backend/pkg/server/food_search_test.go` (new cache/refresh/fallback
  cases), `e2e/tests/food.spec.ts` (extend existing search mocks).
- **Dependencies**: none new — reuses the existing OpenAI credential
  (`HCW_OPENAI_API_KEY`) already configured for photo recognition.
