## ADDED Requirements

### Requirement: Local Open Food Facts SQLite Storage and FTS5 Candidate Retrieval

The system SHALL maintain a local SQLite database, separate from the application database and from the USDA reference database, containing Open Food Facts products filtered to those tagged with a Czech Republic and/or Slovakia country and having complete calories/protein/carbs/fat nutriments, with an FTS5 full-text index over product name and brand. A search SHALL require a non-empty brand and SHALL return only products whose brand matches it; product name SHALL be used to rank matches, not to admit brand-mismatched ones. A search SHALL return a ranked list of candidate products with their per-100g macro profiles and matched brand text; it SHALL NOT bind an item to a product on its own.

#### Scenario: Full-text search returns ranked, brand-matched candidates

- **WHEN** searching with name "bílý jogurt" and brand "Olma"
- **THEN** the system queries the FTS5 index requiring a brand match on "Olma", ranks the brand-matched results by how well "bílý jogurt" matches their product name, and returns up to the configured number of candidates, each with its Open Food Facts barcode (`code`), product name, brand text, and per-100g macro profile

#### Scenario: A product name term alone never substitutes for a brand match

- **WHEN** searching with name "jogurt" and brand "Olma", and the index contains yogurt products from other brands but none from Olma
- **THEN** the system returns an empty candidate list rather than returning the other brands' yogurt products, even though their product names match

#### Scenario: Search with no results

- **WHEN** a search's brand and name terms together match no indexed product
- **THEN** the system returns an empty candidate list rather than an unranked or arbitrary fallback

#### Scenario: Country and completeness filtering applied at import, not at query time

- **WHEN** the Open Food Facts database is queried
- **THEN** every candidate it can return already satisfies the country and nutriment-completeness filter, because filtering happens once during import rather than being re-applied on every search

### Requirement: Lookup by Open Food Facts Barcode

The system SHALL support looking up a single product by its Open Food Facts `code` (barcode) within an open index, used when binding a chosen candidate to a `FoodItem` or resolving a persisted `off_code` back to its macro profile. This is distinct from the index being unavailable at all (no import has run — see "Operator-Run Open Food Facts Import"): a lookup within an open index that simply doesn't contain that code, and an absent index that has no lookup to perform, SHALL be reported differently, mirroring how the existing USDA-unavailable case (`h.usda == nil`) is already distinguished from an unknown `fdc_id` within an available index.

#### Scenario: Lookup an indexed barcode

- **WHEN** looking up a `code` that exists in the local index
- **THEN** the system returns that product's description, brand, and per-100g macro profile

#### Scenario: Lookup a code absent from an available index

- **WHEN** looking up a `code` that does not exist in the local index (e.g. the product fell outside the import's country/completeness filter), while an Open Food Facts database is present and open
- **THEN** the system returns no result rather than an error, so a caller can degrade gracefully

#### Scenario: Lookup attempted with no index available

- **WHEN** an `off_code` is supplied for binding but no Open Food Facts database has ever been imported, so there is no open index to query
- **THEN** the system reports the reference source as unavailable rather than attempting a lookup and rather than reporting the code itself as unknown, the same distinction `resolveReferenceProfile` already makes for USDA

### Requirement: Operator-Run Open Food Facts Import

The system SHALL provide an `hcw import-off` command that downloads (or reads a local copy of) the Open Food Facts full data export, filters it to products tagged with a Czech Republic and/or Slovakia country and complete calories/protein/carbs/fat nutriments, and builds the local SQLite FTS5 index. The import SHALL write to a temporary file, validate a minimum expected row count, and only then atomically replace the existing database, so a failed or partial import leaves the previous database in service. The system SHALL NOT run any scheduled or background dataset updater for this data, consistent with the project having no background-job infrastructure.

#### Scenario: Successful import replaces the database

- **WHEN** an operator runs the import command against a valid Open Food Facts export
- **THEN** the system builds a new database to a temporary path, validates it meets the minimum row count, and atomically replaces the previously served database

#### Scenario: Failed or undersized import does not replace the database

- **WHEN** an import produces fewer rows than the configured minimum, or fails partway through
- **THEN** the system discards the partial build and leaves any previously imported database serving queries unchanged

#### Scenario: No database imported yet

- **WHEN** candidate resolution queries the Open Food Facts index before any import has run
- **THEN** the system reports that no database is present and the caller treats this as an empty candidate list rather than an error, degrading to the USDA source
