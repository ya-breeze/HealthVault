## Purpose

Manages local SQLite storage of USDA FoodData Central core reference data with FTS5 candidate retrieval, an operator-run import command, and custom user food entries.

## ADDED Requirements

### Requirement: Local USDA SQLite Storage and FTS5 Candidate Retrieval
The system SHALL maintain a local SQLite database, separate from the application database, containing USDA FoodData Central Foundation and SR Legacy foods with an FTS5 full-text index. A search SHALL return a ranked list of candidate foods with their per-100g macro profiles; it SHALL NOT bind an item to a food on its own.

#### Scenario: Full-text search returns ranked candidates
- **WHEN** searching for a food term such as "grilled chicken breast"
- **THEN** the system queries the FTS5 index and returns up to the configured number of ranked candidates, each with its FDC ID, description, and per-100g macro profile

#### Scenario: Search with no results
- **WHEN** a search term matches no indexed food
- **THEN** the system returns an empty candidate list rather than an unranked or arbitrary fallback

### Requirement: Match Selection and Explicit Non-Match
The system SHALL resolve a recognized food name to a reference food by offering the retrieved candidate shortlist for selection, and SHALL record an explicit non-match rather than binding to a low-ranked candidate. Custom foods owned by the user SHALL take precedence: a case-insensitive exact name match against the user's custom foods SHALL be selected without consulting the USDA index.

#### Scenario: Custom food takes precedence
- **WHEN** a user has a custom food whose name exactly matches a recognized item name
- **THEN** the system selects that custom food and does not substitute a USDA entry

#### Scenario: Candidate selected from shortlist
- **WHEN** the vision model is given a candidate shortlist for a recognized item and selects one
- **THEN** the system binds the item to the selected food and scales its macros from that profile

#### Scenario: No suitable candidate
- **WHEN** no candidate in the shortlist is a suitable match for the recognized item
- **THEN** the system stores the item with `matched = false` and surfaces it in the review UI for the user to resolve, rather than binding it to the highest-ranked candidate

### Requirement: Operator-Run USDA Import
The system SHALL provide an `hcw import-usda` command that downloads the USDA Foundation and SR Legacy datasets and builds the local SQLite FTS5 database. The import SHALL write to a temporary file, validate a minimum expected row count, and only then atomically replace the existing database, so a failed or partial import leaves the previous database in service. The system SHALL NOT run any scheduled or background dataset updater.

#### Scenario: Successful import
- **WHEN** an operator runs the import command and the download and index build succeed
- **THEN** the new database atomically replaces the previous one and the command reports the imported row count

#### Scenario: Failed import leaves previous data serving
- **WHEN** the download fails, or the built database contains fewer rows than the minimum expected count
- **THEN** the command exits non-zero, the temporary file is discarded, and the previously imported database remains in place and queryable

#### Scenario: Search before any import
- **WHEN** a food search runs before any USDA import has been performed
- **THEN** the system returns an empty USDA candidate list together with a flag indicating the reference database is absent, and the enclosing meal analysis still completes with its items recorded as unmatched

### Requirement: Custom User Food Entry
The system SHALL allow users to store custom foods with per-100g macro profiles, for example from packaged food labels, scoped to the owning user.

#### Scenario: Custom food creation
- **WHEN** a user saves a custom food with a name and per-100g macro values
- **THEN** the system stores it scoped to that user and makes it available to subsequent searches and matching

#### Scenario: Cross-user custom food isolation
- **WHEN** a user searches for foods
- **THEN** the results include only their own custom foods and never another user's
