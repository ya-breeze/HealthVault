## 1. Data model

- [x] 1.1 Add `FoodSearchTranslation` model in `backend/pkg/database` (`models.TenantModel` + `UserID`, `OriginalQuery`, `TranslatedQuery`, unique index on `(user_id, original_query)`), following `CustomFood`'s pattern
- [x] 1.2 Register the model for GORM auto-migration alongside the other food tables

## 2. Translation client

- [x] 2.1 Add `Translate(ctx context.Context, query string) (string, error)` to the `vision.Client` interface
- [x] 2.2 Implement `Translate` on `OpenAIClient` (openai.go): text-only structured-output call, system prompt instructing normalization to USDA FoodData Central American-English generic-food naming, reusing the existing `call`/schema plumbing used by `Clarify`/`Select` (inherits `store: false` from that shared helper)
- [x] 2.3 Implement `Translate` on `Unconfigured` returning a sentinel "not configured" error the handler treats as a fallback, not a failure
- [x] 2.4 Unit tests for `OpenAIClient.Translate` (success, API error, malformed response) mirroring existing `Clarify`/`Select` test patterns
- [x] 2.5 Wrap the `Translate` call site in `food.go` with `context.WithTimeout(r.Context(), h.visionTimeout)`, matching the existing `Recognize`/`Clarify`/`EstimateWeights` call sites — required because `OpenAIClient.HTTPClient` has no timeout of its own and this call sits synchronously on the search request
- [x] 2.6 Add `Translate` to every other `vision.Client` implementation so the interface change compiles: `vision.Fake` (with `TranslateResult`/`TranslateErr`/`TranslateCalls` fields following its existing per-method pattern), and the server-package test doubles `slowRecognizeClient` (food_upload_test.go), `slowClarifyClient` (food_clarify_test.go), `gatedRecognizeClient` (food_reanalyze_test.go)

## 3. Search handler

- [x] 3.1 In `foodHandlers.Search` (food.go), after both the existing custom-food exact-match check and the existing `h.usda == nil` / `ErrNoDatabase` short-circuit: look up `FoodSearchTranslation` for `(user_id, normalize(q))` — translation is never attempted when USDA itself is unavailable
- [x] 3.2 On cache hit (and `refresh` not set): use the cached `TranslatedQuery` as the USDA search term, skip calling `Translate`
- [x] 3.3 On cache miss, or `refresh=true`: call `Translate` (within the timeout from 2.5); on success, upsert (`ON CONFLICT (user_id, original_query) DO UPDATE`) the mapping and use the translated term. On a failed refresh, leave any existing row untouched (the upsert only runs on success)
- [x] 3.4 On `Translate` failure/error/timeout (including `Unconfigured`'s sentinel): log, fall back to searching with the literal query, do not write to the cache, do not fail the request
- [x] 3.5 Add `translated_query` to `FoodSearchResponse`, populated only when a translated term was actually used for the search AND differs from the normalized original query (case/whitespace-only matches are suppressed, even though the mapping is still cached)
- [x] 3.6 Parse the `refresh` query parameter (boolean, default false)
- [x] 3.7 Suppress `translated_query` on an equal-to-input result only on a plain search; a successful `refresh` always reports its term, even when unchanged, so an omitted `translated_query` on a refresh response unambiguously means the refresh failed
- [x] 3.8 Treat a failed cache write (the upsert in `translateAndCache`) as a failed translation: return `ok=false`, log, fall back to the literal query — never report a term as translated unless it was actually persisted
- [x] 3.9 Add an `isCrossSiteRequest` guard (`Sec-Fetch-Site: cross-site`) around both `translateAndCache` call sites (cache miss and `refresh=true`), so a cross-site request never triggers a translation call or cache write
- [x] 3.10 Tighten the guard to require `Sec-Fetch-Site: same-origin` specifically (rename to reflect this): treat `same-site`, `cross-site`, and `none` as untrusted, since this host runs sibling apps on other ports with the same host-scoped cookie; a missing header stays trusted (non-browser caller, not an attacker)
- [x] 3.11 Make the custom-food exact-match short-circuit Unicode-aware: fetch the user's custom foods and compare with `strings.EqualFold` instead of relying on SQLite's ASCII-only `LOWER()`, so a non-ASCII name (e.g. Cyrillic) matches regardless of case

## 4. Backend tests

- [x] 4.1 `food_search_test.go`: cache-hit path — fake `vision.Client` errors if `Translate` is called, assert it's never invoked and cached term is used
- [x] 4.2 `food_search_test.go`: cache-miss path — fake client returns a term, assert a `FoodSearchTranslation` row is persisted and the term is used in the USDA query
- [x] 4.3 `food_search_test.go`: `refresh=true` path — existing row overwritten, `Translate` called despite an existing cache entry
- [x] 4.4 `food_search_test.go`: `Translate` failure — search still returns literal-query USDA results, no row written, no error surfaced
- [x] 4.5 `food_search_test.go`: `Unconfigured` vision client — literal-only search, no `translated_query` in response, behavior matches pre-change baseline
- [x] 4.6 `food_search_test.go`: two users searching the same query text each get their own cache row
- [x] 4.7 `food_search_test.go`: timeout regression test — a `Translate` implementation that blocks past `h.visionTimeout` (mirroring `slowRecognizeClient`'s pattern) results in a literal-query fallback within the deadline, not a hung request
- [x] 4.8 `food_search_test.go`: USDA-unavailable path — `Translate` is never invoked when `h.usda == nil`
- [x] 4.9 `food_search_test.go`: translation-equals-input path — fake client returns the same (normalized) term as the query, assert the row is still cached but `translated_query` is omitted from the response
- [x] 4.10 `food_search_test.go`: failed refresh leaves the prior cached row's `TranslatedQuery` unchanged
- [x] 4.11 `food_search_test.go`: a cache-write failure (Translate succeeds, upsert fails) falls back to the literal query and omits `translated_query`
- [x] 4.12 `food_search_test.go`: a cross-site request (`Sec-Fetch-Site: cross-site`) skips translation and writes no cache row, on both a cache miss and `refresh=true`
- [x] 4.13 `food_search_test.go`: a successful refresh whose fresh translation equals the input still reports `translated_query`
- [x] 4.14 `food_search_test.go`: `Sec-Fetch-Site: same-site` and `none` also skip translation and cache writes, on both a cache miss and `refresh=true`; only `same-origin` (or a missing header) proceeds
- [x] 4.15 `food_search_test.go`: a custom food with a non-ASCII name matches case-insensitively (e.g. stored `Овсянка`, searched `овсянка`) and short-circuits translation, proving the match isn't ASCII-only

## 5. Frontend

- [x] 5.1 `frontend/lib/api.ts`: add `refresh` param to `searchFood`, add `translated_query` to the search response type
- [x] 5.2 `ItemResolver.tsx`: show "Searched as: `<translated_query>`" plus a refresh control when `translated_query` is present; refresh re-calls `searchFood` with `refresh: true`; while a refresh is in flight, show a distinct loading state from a normal search; on refresh failure, show an inline error and keep the previous "Searched as" display rather than clearing it
- [x] 5.3 `ManualItemEditor.tsx`: same display + refresh treatment in its search results section; add the `try/catch` + error-state plumbing it currently lacks so a failed refresh can be surfaced instead of silently swallowed
- [x] 5.4 Add a standing disclosure notice (near the search input in both components, or a shared location both render) stating that new search terms may be sent to an external model provider for translation, visible before a search is ever submitted
- [x] 5.5 Add a per-component request-sequence counter so `search`/`refresh` each capture the value current at call time and only apply their response to shared `results`/`translatedQuery`/error state if it's still current when the response arrives, dropping stale responses; `searching`/`refreshing` still clear on their own request's completion regardless of staleness
- [x] 5.6 `ManualItemEditor.tsx`: when `search`/`refresh` apply a fresh response while a candidate is still selected, clear `selected` (re-showing the refreshed result list instead of hiding it behind "Bound to ...") and clear the parent item's `fdc_id`/`custom_food_id` binding, so a stale binding never survives underneath a corrected search/refresh unnoticed — the user must re-pick from the fresh list

## 6. E2E

- [x] 6.1 Extend the `**/api/food/search**` mocks in `e2e/tests/food.spec.ts` (around lines 601 and 706) to include `translated_query`, and add a case asserting the "Searched as" text and refresh control render when present
- [x] 6.2 Add a case mocking a failed refresh response (no `translated_query`, e.g. a 200 with literal-query results) and assert the prior "Searched as" text and an error indication both remain visible
- [x] 6.3 Add a case mocking a successful refresh (response includes `translated_query` with a corrected term) and assert the banner updates to the new term with no error, distinguishing it from the failed-refresh case in 6.2; the equals-the-input edge of "successful" specifically is covered at the backend unit level (4.13)
