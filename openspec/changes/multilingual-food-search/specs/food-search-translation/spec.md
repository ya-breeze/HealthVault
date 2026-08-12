## ADDED Requirements

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
translation model is unconfigured or the translation call fails, without failing the
search request, and SHALL NOT cache a failed or missing translation.

#### Scenario: Translation client unconfigured
- **WHEN** no translation model is configured for the deployment
- **THEN** `GET /api/food/search` SHALL search USDA using the literal query, SHALL NOT create a `FoodSearchTranslation` row, and the response SHALL omit `translated_query`

#### Scenario: Translation call fails
- **WHEN** the translation model call errors or times out
- **THEN** `GET /api/food/search` SHALL still return USDA search results for the literal query, SHALL NOT create or update a `FoodSearchTranslation` row, and SHALL NOT return an error to the caller solely because translation failed

### Requirement: Response Surfaces the Translated Query
The system SHALL include the translated term used for the USDA search in
`FoodSearchResponse` whenever a translation was applied, so the caller can display
what was actually searched and offer to refresh it.

#### Scenario: Translated query included when translation applied
- **WHEN** a search uses a translated term, whether from cache or freshly translated
- **THEN** the response SHALL include a `translated_query` field equal to that term

#### Scenario: Translated query omitted when no translation was applied
- **WHEN** a search resolves via an exact custom food match, or falls back to the literal query because translation is unavailable or failed
- **THEN** the response SHALL omit the `translated_query` field
