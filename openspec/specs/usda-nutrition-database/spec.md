# usda-nutrition-database Specification

## Purpose
TBD - created by archiving change photo-food-nutrition-logging. Update Purpose after archive.
## Requirements
### Requirement: Local USDA SQLite Storage and FTS5 Candidate Retrieval
The system SHALL maintain a local SQLite database, separate from the application database, containing USDA FoodData Central Foundation and SR Legacy foods with an FTS5 full-text index. A search SHALL return a ranked list of candidate foods with their per-100g macro profiles; it SHALL NOT bind an item to a food on its own.

#### Scenario: Full-text search returns ranked candidates
- **WHEN** searching for a food term such as "grilled chicken breast"
- **THEN** the system queries the FTS5 index and returns up to the configured number of ranked candidates, each with its FDC ID, description, and per-100g macro profile

#### Scenario: Search with no results
- **WHEN** a search term matches no indexed food
- **THEN** the system returns an empty candidate list rather than an unranked or arbitrary fallback

### Requirement: Retrieval Terms Include Preparation and State
The retrieval query for a recognized item SHALL be built from its name together with its preparation and state when those are known. Preparation and state SHALL act only as ranking hints and SHALL NOT be used to filter or exclude candidates, so that an incorrect model guess can never remove the correct food from the shortlist. An unknown preparation or state SHALL contribute no term and SHALL degrade to a name-only query rather than an empty one.

#### Scenario: Preparation improves ranking
- **WHEN** an item's preparation is known and included in the retrieval query
- **THEN** the canonical whole-food entry ranks no worse than it does for the name-only query

#### Scenario: An incorrect preparation guess is not fatal
- **WHEN** the recorded preparation does not match the food's actual preparation
- **THEN** the correct food is still present in the returned candidate shortlist, having been re-weighted rather than filtered out

#### Scenario: Unknown preparation degrades gracefully
- **WHEN** both preparation and state are unknown
- **THEN** retrieval runs on the item name alone and still returns candidates

### Requirement: Match Selection and Explicit Non-Match
The system SHALL resolve a recognized food name to a reference food by offering the retrieved candidate shortlist for selection, and SHALL record an explicit non-match rather than binding to a low-ranked candidate. Custom foods owned by the user SHALL take precedence: a case-insensitive exact name match against the user's custom foods SHALL be selected without consulting the USDA index.

#### Scenario: Custom food takes precedence
- **WHEN** a user has a custom food whose name exactly matches a recognized item name, ignoring case
- **THEN** the system selects that custom food and does not substitute a USDA entry, and the selection is unambiguous because custom food names are unique per user

#### Scenario: Candidate selected from shortlist
- **WHEN** the vision model is given a candidate shortlist for a recognized item and selects one
- **THEN** the system binds the item to the selected food and scales its macros from that profile

#### Scenario: No suitable candidate
- **WHEN** no candidate in the shortlist is a suitable match for the recognized item
- **THEN** the system stores the item with `macro_source = none` and surfaces it in the review UI, which resolves it via `PATCH /api/food/meals/{id}/items/{item_id}`, rather than binding it to the highest-ranked candidate

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

### Requirement: Custom User Food Entry and Correction
The system SHALL allow users to store custom foods with per-100g macro profiles, for example from packaged food labels, scoped to the owning user. Custom food names SHALL be unique per user, and the system SHALL provide update and delete alongside create and list.

Both properties follow from precedence: a custom food shadows every USDA entry sharing its name. Without per-user name uniqueness, two custom foods called "yogurt" make the winning match arbitrary. Without update and delete, a custom food saved with a mistyped macro value permanently poisons matching for that name with no in-app way to correct it.

#### Scenario: Custom food creation
- **WHEN** a user saves a custom food with a name and per-100g macro values
- **THEN** the system stores it scoped to that user and makes it available to subsequent searches and matching

#### Scenario: Duplicate custom food name rejected
- **WHEN** a user saves a custom food whose name matches one they already own, ignoring case
- **THEN** the system returns HTTP 409 and does not create a second entry

#### Scenario: Correct a mistyped custom food
- **WHEN** the owner updates a custom food's macro values
- **THEN** subsequent matches use the corrected values

#### Scenario: Delete a custom food restores USDA matching
- **WHEN** the owner deletes a custom food
- **THEN** it no longer shadows USDA entries and a later search for that name returns USDA candidates

#### Scenario: Cross-user custom food isolation
- **WHEN** a user searches for foods
- **THEN** the results include only their own custom foods and never another user's

#### Scenario: Cross-user custom food mutation
- **WHEN** a user attempts to update or delete a custom food owned by another user
- **THEN** the system returns HTTP 404 and the record is unchanged

