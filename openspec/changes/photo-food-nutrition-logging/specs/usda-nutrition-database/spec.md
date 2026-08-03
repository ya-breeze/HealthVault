## Purpose

Manages local SQLite storage of USDA FoodData Central core reference data with FTS5 search, periodic background data updates, and custom user food entries.

## ADDED Requirements

### Requirement: Local USDA SQLite Storage and FTS5 Search
The system SHALL maintain a local SQLite database containing USDA FoodData Central Foundation and SR Legacy foods with an FTS5 full-text index for food matching.

#### Scenario: Full-text search for food items
- **WHEN** searching for a food term like "grilled chicken breast"
- **THEN** the system queries the local USDA SQLite FTS5 table and returns ranked candidate items with per-100g macro profiles within 50 milliseconds.

### Requirement: Periodic USDA Dataset Updates
The system SHALL periodically check for updated USDA FoodData Central releases and refresh the local reference tables in the background.

#### Scenario: Automatic update check
- **WHEN** the monthly background update timer triggers
- **THEN** the system checks for new USDA release datasets, downloads available core data, and updates the local SQLite FTS5 index without interrupting API requests.

### Requirement: Custom User Food Entry and Priority Indexing
The system SHALL allow users to manually store custom foods with custom macro profiles (e.g. packaged food labels) and prioritize user custom foods during search matching.

#### Scenario: Custom food creation and matching priority
- **WHEN** a user saves a custom food entry and subsequently searches for that item or analyzes a photo containing it
- **THEN** the system matches the custom food record ahead of standard USDA reference entries.
