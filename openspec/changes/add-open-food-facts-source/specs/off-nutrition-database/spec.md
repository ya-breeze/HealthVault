## ADDED Requirements

### Requirement: Local Open Food Facts SQLite Storage and FTS5 Candidate Retrieval

The system SHALL maintain a local SQLite database, separate from the application database and from the USDA reference database, containing Open Food Facts products filtered to those tagged with a Czech Republic and/or Slovakia country and having complete calories/protein/carbs/fat nutriments, with an FTS5 full-text index over product name and brand. A search SHALL return a ranked list of candidate products with their per-100g macro profiles; it SHALL NOT bind an item to a product on its own.

#### Scenario: Full-text search returns ranked candidates

- **WHEN** searching for a product term such as "bílý jogurt"
- **THEN** the system queries the FTS5 index and returns up to the configured number of ranked candidates, each with its Open Food Facts barcode (`code`), product name, and per-100g macro profile

#### Scenario: Search with no results

- **WHEN** a search term matches no indexed product
- **THEN** the system returns an empty candidate list rather than an unranked or arbitrary fallback

#### Scenario: Country and completeness filtering applied at import, not at query time

- **WHEN** the Open Food Facts database is queried
- **THEN** every candidate it can return already satisfies the country and nutriment-completeness filter, because filtering happens once during import rather than being re-applied on every search

### Requirement: Lookup by Open Food Facts Barcode

The system SHALL support looking up a single product by its Open Food Facts `code` (barcode), used when binding a chosen candidate to a `FoodItem` or resolving a persisted `off_code` back to its macro profile.

#### Scenario: Lookup an indexed barcode

- **WHEN** looking up a `code` that exists in the local index
- **THEN** the system returns that product's description and per-100g macro profile

#### Scenario: Lookup an unindexed barcode

- **WHEN** looking up a `code` that does not exist in the local index (e.g. the product fell outside the import's country/completeness filter, or the database has not been imported)
- **THEN** the system returns no result rather than an error, so a caller can degrade gracefully

### Requirement: Operator-Run Open Food Facts Import

The system SHALL provide an operator-run CLI command that downloads (or reads a local copy of) the Open Food Facts full data export, filters it to products tagged with a Czech Republic and/or Slovakia country and complete calories/protein/carbs/fat nutriments, and builds the local SQLite FTS5 index. The command SHALL build to a temporary location and only replace the existing database after the build passes a minimum row-count check, so a failed or partial import leaves the previous database in service. This mirrors the existing USDA import command rather than running as a background job, consistent with the project having no scheduled-job infrastructure.

#### Scenario: Successful import replaces the database

- **WHEN** an operator runs the import command against a valid Open Food Facts export
- **THEN** the system builds a new database to a temporary path, validates it meets the minimum row count, and atomically replaces the previously served database

#### Scenario: Failed or undersized import does not replace the database

- **WHEN** an import produces fewer rows than the configured minimum, or fails partway through
- **THEN** the system discards the partial build and leaves any previously imported database serving queries unchanged

#### Scenario: No database imported yet

- **WHEN** candidate resolution queries the Open Food Facts index before any import has run
- **THEN** the system reports that no database is present and the caller treats this as an empty candidate list rather than an error, degrading to the USDA source
