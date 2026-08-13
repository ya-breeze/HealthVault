<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# food-search-translation Specification

## Purpose
TBD - created by archiving change multilingual-food-search. Update Purpose after archive.
## Requirements
### Requirement: Per-User Cached Query Translation
The system SHALL translate a free-text food search query into USDA-vocabulary
English before searching the USDA reference index, using a per-user cache keyed by
the normalized (trimmed, lowercased) query text, so that a repeated query does not
re-invoke the translation model.

#### Scenario: Cache hit skips translation
- **WHEN** a user searches a query for which a `FoodSearchTranslation` row already exists for that user
- **THEN** the system SHALL use the cached translated term to search USDA and SHALL NOT call the translation model

#### Scenario: Cache miss triggers translation and caches the result
- **WHEN** a user searches a query with no existing `FoodSearchTranslation` row for that user
- **THEN** the system SHALL call the translation model, store the result as a `FoodSearchTranslation` row for that user and query, and use the translated term to search USDA

#### Scenario: Cache entries are private to the user who created them
- **WHEN** two different users search the same query text for the first time
- **THEN** each SHALL get their own `FoodSearchTranslation` row, and a translation cached for one user SHALL NOT be reused for another user's search

### Requirement: Translation Only Runs When The Reference Database Is Available
The system SHALL perform the cache lookup and, on a miss, the translation call only
after confirming the USDA reference index is available, so that a deployment without
a loaded USDA database never pays for a translation it cannot use.

#### Scenario: USDA unavailable skips translation entirely
- **WHEN** the USDA reference index is not loaded for the deployment
- **THEN** `GET /api/food/search` SHALL return the existing degraded response (custom
  foods only) without checking the `FoodSearchTranslation` cache or calling the
  translation model

### Requirement: Custom Food Matching Is Unaffected By Translation
The system SHALL check the caller's custom foods for an exact case-insensitive name
match against the literal, untranslated query before applying translation, unchanged
from existing behavior.

#### Scenario: Exact custom food match short-circuits translation
- **WHEN** a user's query exactly matches (case-insensitively) one of their own custom foods
- **THEN** the system SHALL return that custom food and SHALL NOT invoke translation or search USDA

### Requirement: User-Triggered Refresh Regenerates a Cached Translation
The system SHALL accept a `refresh` parameter on `GET /api/food/search` that forces a
fresh translation for the given query, overwriting any existing cached mapping for
that user and query in place, and uses the new translation to search.

#### Scenario: Refresh overwrites an existing mapping
- **WHEN** a user re-issues a search with `refresh=true` for a query that already has a cached `FoodSearchTranslation` row
- **THEN** the system SHALL call the translation model again, overwrite the existing row's translated term with the new result, and search USDA using the new term

#### Scenario: Refresh on a query with no existing mapping behaves like a normal cache miss
- **WHEN** a user issues a search with `refresh=true` for a query with no existing `FoodSearchTranslation` row
- **THEN** the system SHALL call the translation model and store the result, the same as when `refresh` is unset

### Requirement: Search Degrades Gracefully When Translation Is Unavailable Or Fails
The system SHALL fall back to searching with the literal, untranslated query when the
translation model is unconfigured, the translation call fails, the translation call
exceeds a bounded deadline, or the cache write for a successful translation fails,
without failing the search request, and SHALL NOT cache a failed, missing, or
timed-out translation. On a failed refresh specifically, any previously cached
mapping for that query SHALL be left unchanged (the refresh upsert only happens on a
successful translation, and a translation whose upsert itself fails is treated as a
failed translation, not a successful-but-unpersisted one).

#### Scenario: Translation client unconfigured
- **WHEN** no translation model is configured for the deployment
- **THEN** `GET /api/food/search` SHALL search USDA using the literal query, SHALL NOT create a `FoodSearchTranslation` row, and the response SHALL omit `translated_query`

#### Scenario: Translation call fails
- **WHEN** the translation model call errors
- **THEN** `GET /api/food/search` SHALL still return USDA search results for the literal query, SHALL NOT create or update a `FoodSearchTranslation` row, and SHALL NOT return an error to the caller solely because translation failed

#### Scenario: Translation call exceeds its deadline
- **WHEN** the translation model call does not complete within the configured vision
  timeout (the same timeout already applied to photo-recognition calls)
- **THEN** the system SHALL abandon the call, treat it identically to a translation
  failure (literal-query fallback, no cache write), and SHALL NOT block the search
  request beyond that deadline

#### Scenario: Failed refresh does not erase the existing cached mapping or its display
- **WHEN** a user triggers `refresh=true` for a query with an existing
  `FoodSearchTranslation` row, and the refresh's translation call fails
- **THEN** the existing cached row SHALL remain unchanged, the response SHALL omit
  `translated_query` for that request, and the client SHALL show an error state for
  the refresh attempt rather than presenting the omission as "no translation exists"

#### Scenario: A successful translation whose cache write fails is treated as a failed translation
- **WHEN** the translation model call succeeds but persisting the resulting
  `FoodSearchTranslation` row fails (e.g. a transient database error)
- **THEN** the system SHALL search with the literal query, SHALL NOT report the
  freshly translated term in `translated_query`, and (on a refresh) SHALL leave any
  previously cached mapping for that query unchanged — an unpersisted translation is
  never presented as if it had been applied and cached

#### Scenario: A non-same-origin request never triggers translation or a cache write
- **WHEN** `GET /api/food/search` is a cache miss, or is called with `refresh=true`,
  and the request carries a `Sec-Fetch-Site` header whose value is not `same-origin`
  (including `same-site`, `cross-site`, and `none`)
- **THEN** the system SHALL skip the translation call and any cache write for that
  request, and SHALL search with the literal query instead, the same as when
  translation is unavailable

#### Scenario: A request with no Sec-Fetch-Site header is treated as trusted
- **WHEN** `GET /api/food/search` is a cache miss, or is called with `refresh=true`,
  and the request carries no `Sec-Fetch-Site` header at all
- **THEN** the system SHALL treat the request as same-origin and proceed with
  translation and caching as normal, since all modern browsers send this header on
  every request and its absence indicates a non-browser caller, not an attacker

### Requirement: Response Surfaces the Translated Query
The system SHALL include the translated term used for the USDA search in
`FoodSearchResponse` whenever a translation was applied and it changed what was
searched for, so the caller can display what was actually searched and offer to
refresh it.

#### Scenario: Translated query included when translation applied and differs from the input
- **WHEN** a search uses a translated term, whether from cache or freshly translated, and that term is not equal to the normalized (trimmed, lowercased) original query
- **THEN** the response SHALL include a `translated_query` field equal to that term

#### Scenario: Translated query omitted when no translation was applied
- **WHEN** a search resolves via an exact custom food match, or falls back to the literal query because translation is unavailable or failed
- **THEN** the response SHALL omit the `translated_query` field

#### Scenario: Translated query omitted when the translation equals the input
- **WHEN** a plain (non-refresh) search runs and the translated term (whether from cache or freshly translated) equals the normalized original query
- **THEN** the response SHALL omit `translated_query`, even though the mapping is still cached, so the interface shows no banner for a query that was already valid USDA vocabulary

#### Scenario: A successful refresh always reports its translated term, even when unchanged
- **WHEN** a user triggers `refresh=true` and the fresh translation succeeds, including the case where the resulting term equals the normalized original query
- **THEN** the response SHALL include `translated_query` equal to that term, so an omitted `translated_query` on a refresh response unambiguously means the refresh failed, never that the term was already correct

### Requirement: Search And Refresh Results Reflect Only The Latest Request
The search interface SHALL apply a search or refresh response to the displayed
results and translated-query banner only if no newer search or refresh request has
been issued since, so that a slower, older response can never overwrite a newer
one's results.

#### Scenario: A slow refresh response arrives after a newer search
- **WHEN** a user triggers a refresh, then issues a new plain search before the
  refresh's response arrives, and the refresh's response arrives after the search's
- **THEN** the interface SHALL discard the refresh's response and continue showing
  the newer search's results and translated-query state

### Requirement: Search Interface Discloses External Transmission Before It Happens
The search interface SHALL state, before any query is submitted, that new search
text may be sent to an external model provider for translation — the same
disclosure pattern already required for photo uploads.

#### Scenario: Disclosure visible before first search
- **WHEN** a user opens the food search interface
- **THEN** the interface SHALL display a standing notice that new search terms may be sent to an external model provider for translation, visible before the user submits a query

