## Context

`GET /api/food/search` (backend/pkg/server/food.go) checks the caller's custom foods
for an exact case-insensitive name match, then falls through to `usda.Search`, which
runs literal FTS5 term matching against the local USDA FoodData Central (Foundation +
SR Legacy) index (backend/pkg/usda/). That corpus is American-English generic-food
naming only — confirmed empirically that 0 of its 7,793 rows contain the word
"porridge", only "oatmeal"/"oats" — and has no synonym or translation layer at all.
Any query in a different regional spelling or a different language returns zero
results, with no signal to the user about why.

The backend already has a working OpenAI integration (`backend/pkg/vision`) used for
photo-based food recognition, with a `Client` interface (`OpenAIClient` in production,
`Unconfigured` when no API key is set) and an established pattern for text-only
structured-output calls (`Clarify`, `Select` — see openai.go).

Open Food Facts (`backend/pkg/off`) was evaluated during this change's brainstorming
as an alternative *primary* database for generic-ingredient search (todo.md had
flagged this as an open question). Empirical comparison confirmed OFF is a
branded/packaged-SKU catalog, not a generic-ingredient one — e.g. every OFF result for
"oatmeal", "rice", "apple", "banana" was a specific branded product (Carrefour, Rice
Krispies, Nestlé...), never a generic "cooked rice" or "raw banana" entry. USDA remains
the right source for generic/home-cooked food search; OFF's existing role
(barcode lookup, brand-gated candidate search during photo recognition) is unchanged
by this design.

## Goals / Non-Goals

**Goals:**
- Let a user search in any language or regional spelling and reach the right USDA
  reference food.
- Make repeat searches for the same phrase (by the same user) fast and free after the
  first translation — most of a given user's logged foods repeat.
- Let a user correct a wrong or stale translation without operator involvement.
- Degrade to today's literal-only behavior when the LLM is unavailable or fails,
  rather than blocking search.

**Non-Goals:**
- Not switching the primary reference database to Open Food Facts (evaluated and
  rejected above).
- Not translating photo-recognition output — `vision.Recognize`/`Clarify` already
  produce English item names as part of their own prompt contract; this change only
  touches the free-text `/api/food/search` path used by `ItemResolver` and
  `ManualItemEditor`.
- Not building a shared/global translation dictionary across users. Mappings are
  strictly per-user (see Decisions) — simpler, and consistent with `CustomFood`'s
  existing per-user scoping.
- Not adding a standalone "manage my translations" UI. The only mutation surface is
  the inline refresh control next to search results.

## Decisions

### Per-user cache table, not a shared/global dictionary
`FoodSearchTranslation` is scoped by `UserID`, mirroring `CustomFood`'s
`models.TenantModel` + `UserID` + unique-indexed-name pattern exactly. A global
dictionary would need moderation (one user's bad translation would corrupt search for
everyone) and cross-tenant visibility rules; per-user avoids both at the cost of every
user separately "teaching" the system food terms someone else may have already taught
it. Given HealthVault's family-of-users scale, that cost is negligible next to the
complexity a shared table would add.

### Translate on every search (cache-checked), not fallback-only-on-empty-results
Two options were considered: (a) only invoke translation when the literal query
returns zero USDA results, or (b) always resolve through the cache/LLM and use the
translated term as the actual search input. (b) was chosen: it means a query that
happens to share a token with an unrelated USDA row (a false-positive from the literal
path) doesn't silently skip translation, and it keeps one code path instead of two
(literal-search-then-maybe-retry vs. always-translate). The cost — an LLM call the
very first time a user ever types a given phrase — is a one-time cost per phrase per
user; every subsequent identical search is a cache hit with no LLM call at all.

### Cache key is normalized (trim + lowercase) original query text
Keeps "Porridge", "porridge", and "  porridge " as one cache entry. No language
tagging in the key — the LLM sees whatever text the user typed and its output alone
determines the USDA-facing search term, so two different input languages that happen
to normalize to the same literal string would collide (extremely unlikely in
practice, e.g. an English and a transliterated non-English word being byte-identical
after lowercasing) and is an acceptable edge case, not worth a language-detection step.

### Translate is a new `vision.Client` method, not a separate service/package
`Translate(ctx, query string) (string, error)` is added to the existing `Client`
interface (implemented by `OpenAIClient` and `Unconfigured`) alongside `Recognize`,
`Clarify`, `Select`, `EstimateWeights`. It reuses the same structured-output
(`response_format: json_schema`, `store: false`) call plumbing as `Clarify`/`Select` —
no new HTTP client, no new credential, no new vendor. A dedicated
`pkg/translate` package was considered and rejected: the only thing it would own is
one prompt and one schema, which fits naturally as one more method on the interface
that already exists for exactly this kind of text-only OpenAI call.

### Refresh reuses `GET /api/food/search?refresh=true`, not a separate endpoint
`refresh=true` skips the cache lookup, unconditionally calls `Translate`, and
**upserts** (`ON CONFLICT (user_id, original_query) DO UPDATE`) the mapping row in
place, then searches with the fresh term — one request, one write, no
delete-then-recreate window. A dedicated `POST /api/food/search/refresh` endpoint was
considered for stricter REST semantics (a `GET` here has a side effect: it writes the
cache) but rejected as unnecessary ceremony for an endpoint only ever invoked by an
explicit, non-prefetched user click, where repeating the call is harmless (each call
just re-translates and overwrites, no accumulation).

### Translate is time-bounded by the existing vision timeout, not the bare HTTP client default
`OpenAIClient.HTTPClient` is a plain `&http.Client{}` with no timeout, and unlike the
async photo-recognition flows (`Recognize`/`Clarify`/`EstimateWeights`, each already
wrapped in `context.WithTimeout(r.Context(), h.visionTimeout)` at their call sites),
this call sits synchronously on a `GET` the browser is waiting on. The handler SHALL
wrap the `Translate` call the same way, using the same `h.visionTimeout` config value.
A timeout is treated identically to any other `Translate` error: fail-open, log, fall
back to the literal query, do not write to the cache (see "Failure handling" below).
Reviewed and flagged independently by both a pre-implementation design review and a
PR review comment (Codex) — see tasks.md 2.5/4.7 for the corresponding implementation
and regression-test tasks.

### Translation lookup runs only when USDA is available
The cache lookup and `Translate` call happen after the existing `h.usda == nil` /
`ErrNoDatabase` short-circuit in `food.go`, not before it. Translating a query that
has nowhere to search against would spend an LLM call (and, on a cache miss, a DB
write) for zero benefit on a deployment without a loaded USDA index — the same
degraded-source handling already applies to the custom-food-only fallback path.

### `translated_query` is omitted when the translation doesn't change the query
Translation runs on every cache miss, not only when the literal query returns zero
USDA results (see "Translate on every search" above) — so a well-formed English query
like "chicken breast" still costs one LLM round-trip and would, without this
exception, show a "Searched as: chicken breast" banner and refresh control on
essentially every first-time English search. The response SHALL omit
`translated_query` when the translated term is equal to the normalized (trimmed,
lowercased) original query — the common case for existing English-speaking users —
so the banner appears only when translation actually changed what was searched for.
The row is still cached either way, so a repeat search of the same phrase remains a
cache hit with no LLM call.

### Search interface discloses external transmission before it happens
`food-photo-recognition` already requires the upload interface to state that a photo
will be sent to an external model provider before it happens (see
`openspec/specs/food-photo-recognition/spec.md`, "external model provider" scenario).
This change follows the same pattern: the search interface SHALL carry a standing
notice (not a per-search interstitial) that new search terms may be sent to an
external model provider for translation, visible before the first search is ever
submitted. `Translate` reuses the same `call()` helper as `Clarify`/`Select`, which
already sets `store: false` on every request — no new decision needed there, it's
inherited from the existing plumbing.

### Failure handling is fail-open, not fail-closed
If `Translate` errors or times out, the handler logs and falls back to searching with
the literal query for that request — it does not fail the whole `/api/food/search`
call, and it does not write anything to the cache (so the next search retries the LLM
rather than being stuck with a missing/bad mapping). This matches the existing
pattern for `h.usda == nil` and `usda.ErrNoDatabase`: a degraded reference source
degrades the response, it never 500s the request.

### Refresh failure preserves the prior display; it isn't treated as "never translated"
On a failed refresh, the server falls back to a literal-query search and omits
`translated_query` (same as any other `Translate` failure — see "Failure handling"
below) — but the *previous* cached mapping is untouched (the upsert never ran). The
frontend SHALL NOT read that omission as "no translation exists" and silently drop
the banner; a failed refresh SHALL show an inline error (e.g. "Couldn't refresh
translation, try again") while leaving the prior "Searched as" display in place,
since the old mapping is still what's cached and would be used again on the next
plain search. `ManualItemEditor.tsx` has no error-state plumbing today (its `search()`
has no `try/catch`) and needs it added for this.

### Test doubles must implement `Translate` too
`vision.Client` is implemented by more than `OpenAIClient` and `Unconfigured`:
`vision.Fake` (the general test double) and three server-package-local doubles
(`slowRecognizeClient`, `slowClarifyClient`, `gatedRecognizeClient`) all satisfy the
interface today. Adding `Translate` to the interface without adding it to all four
breaks the build, not just the new tests — see tasks.md 2.6.

## Risks / Trade-offs

- **[Risk] LLM translates confidently but wrong** (e.g. maps a query to an
  unrelated USDA term) → **Mitigation**: the response surfaces `translated_query` so
  the mismatch is visible to the user immediately, and the refresh control lets them
  regenerate it in one click. This is the same trust model already used for vision
  candidate matching (`Select` never auto-assigns; the user always confirms).
- **[Risk] Added latency on first-time searches** (one LLM round-trip, ~1s+) →
  **Mitigation**: only pays on a cache miss; the target user's own stated usage
  pattern (mostly repeat ingredients) means this is a one-time cost per phrase.
- **[Risk] Concurrent first-time searches for the same new phrase by the same user**
  (e.g. a double-click) could both miss the cache and both call the LLM →
  **Mitigation**: harmless — both calls succeed independently, the unique-index
  upsert leaves exactly one row regardless of write order; the extra LLM call is a
  minor cost, not a correctness bug.
- **[Trade-off] Per-user cache means N users searching "porridge" pay for translation
  N times**, not once → accepted per the per-user-dictionary decision above; revisit
  only if usage data shows this is a real cost driver.
- **[Minor risk] No query length cap before an LLM call** → accepted; search only
  fires on explicit submit (not per-keystroke), so this isn't a rate-amplification
  risk, just an uncapped-cost one. Not worth a hard requirement at this scale; revisit
  if abuse or cost data says otherwise.
- **[Minor risk] Cache key normalization (trim + lowercase) has no Unicode
  normalization (NFC)** → byte-different, visually-identical strings (e.g.
  differently-composed Cyrillic sequences) would miss the cache and each pay for a
  redundant LLM call, notably for this change's own motivating language. Accepted as
  a minor cost, not a correctness bug — the worst case is an extra cache miss, not a
  wrong result — and not worth the added normalization-library dependency at this
  stage.

## Migration Plan

- New table `FoodSearchTranslation`, added via GORM auto-migration (same mechanism as
  existing tables — no manual SQL migration file in this codebase). This is the fifth
  food-logging tenant table; `openspec/specs/data-model/spec.md`'s "Food logging
  tables" requirement is updated via a `data-model` delta in this change (see
  `specs/data-model/spec.md`) so the canonical inventory doesn't go stale on archive.
- No backfill: the cache starts empty and populates itself as users search. No
  existing behavior depends on this table's absence.
- Rollback: reverting the code change leaves an unused, harmless table behind (or it
  can be dropped manually) — no data migration to reverse.
- No new environment variable: reuses `HCW_OPENAI_API_KEY` / `HCW_OPENAI_MODEL`,
  already present in every deployment that has photo recognition configured. A
  deployment without those set already degrades vision to `Unconfigured`; this change
  makes `/api/food/search` degrade the same way (literal-only, no `translated_query`).

## Open Questions

None outstanding — all decisions above were confirmed during brainstorming with the
user, including the explicit choice of per-user caching over a shared dictionary and
the GET-with-side-effect refresh shape over a dedicated endpoint.
