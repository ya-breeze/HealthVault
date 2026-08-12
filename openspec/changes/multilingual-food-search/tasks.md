## 1. Data model

- [ ] 1.1 Add `FoodSearchTranslation` model in `backend/pkg/database` (`models.TenantModel` + `UserID`, `OriginalQuery`, `TranslatedQuery`, unique index on `(user_id, original_query)`), following `CustomFood`'s pattern
- [ ] 1.2 Register the model for GORM auto-migration alongside the other food tables

## 2. Translation client

- [ ] 2.1 Add `Translate(ctx context.Context, query string) (string, error)` to the `vision.Client` interface
- [ ] 2.2 Implement `Translate` on `OpenAIClient` (openai.go): text-only structured-output call, system prompt instructing normalization to USDA FoodData Central American-English generic-food naming, reusing the existing `call`/schema plumbing used by `Clarify`/`Select`
- [ ] 2.3 Implement `Translate` on `Unconfigured` returning a sentinel "not configured" error the handler treats as a fallback, not a failure
- [ ] 2.4 Unit tests for `OpenAIClient.Translate` (success, API error, malformed response) mirroring existing `Clarify`/`Select` test patterns

## 3. Search handler

- [ ] 3.1 In `foodHandlers.Search` (food.go), after the existing custom-food exact-match check: look up `FoodSearchTranslation` for `(user_id, normalize(q))`
- [ ] 3.2 On cache hit (and `refresh` not set): use the cached `TranslatedQuery` as the USDA search term, skip calling `Translate`
- [ ] 3.3 On cache miss, or `refresh=true`: call `Translate`; on success, upsert (`ON CONFLICT (user_id, original_query) DO UPDATE`) the mapping and use the translated term
- [ ] 3.4 On `Translate` failure/error (including `Unconfigured`'s sentinel): log, fall back to searching with the literal query, do not write to the cache, do not fail the request
- [ ] 3.5 Add `translated_query` to `FoodSearchResponse`, populated only when a translated term was actually used for the search
- [ ] 3.6 Parse the `refresh` query parameter (boolean, default false)

## 4. Backend tests

- [ ] 4.1 `food_search_test.go`: cache-hit path — fake `vision.Client` errors if `Translate` is called, assert it's never invoked and cached term is used
- [ ] 4.2 `food_search_test.go`: cache-miss path — fake client returns a term, assert a `FoodSearchTranslation` row is persisted and the term is used in the USDA query
- [ ] 4.3 `food_search_test.go`: `refresh=true` path — existing row overwritten, `Translate` called despite an existing cache entry
- [ ] 4.4 `food_search_test.go`: `Translate` failure — search still returns literal-query USDA results, no row written, no error surfaced
- [ ] 4.5 `food_search_test.go`: `Unconfigured` vision client — literal-only search, no `translated_query` in response, behavior matches pre-change baseline
- [ ] 4.6 `food_search_test.go`: two users searching the same query text each get their own cache row

## 5. Frontend

- [ ] 5.1 `frontend/lib/api.ts`: add `refresh` param to `searchFood`, add `translated_query` to the search response type
- [ ] 5.2 `ItemResolver.tsx`: show "Searched as: `<translated_query>`" plus a refresh control when `translated_query` is present; refresh re-calls `searchFood` with `refresh: true`
- [ ] 5.3 `ManualItemEditor.tsx`: same display + refresh treatment in its search results section

## 6. E2E

- [ ] 6.1 Extend the `**/api/food/search**` mocks in `e2e/tests/food.spec.ts` (around lines 601 and 706) to include `translated_query`, and add a case asserting the "Searched as" text and refresh control render when present
