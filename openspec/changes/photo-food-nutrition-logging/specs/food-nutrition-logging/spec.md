## Purpose

Computes 7 primary macros for meal items, supports manual photo-free entry, and manages the meal lifecycle from creation through confirmation and deletion. `FoodMeal` is the source of truth for logged food; the `Nutrition` telemetry table is not written to.

## ADDED Requirements

### Requirement: Macro Calculation and Portion Scaling
The system SHALL compute 7 primary macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) for each matched item by scaling its per-100g nutritional profile by its gram weight.

#### Scenario: Gram weight scaling
- **WHEN** a food item of 180 grams is matched to a USDA profile with 165 kcal per 100g
- **THEN** the system calculates the item calories as 297 kcal (165 * 1.8) and scales all other 6 macros proportionally

#### Scenario: Weight adjusted after matching
- **WHEN** the user changes an item's weight from 180g to 200g before confirming
- **THEN** the system recalculates all 7 macros for that item from the same per-100g profile and updates the meal aggregate

#### Scenario: Unmatched item contributes no macros
- **WHEN** an item has no matched USDA or custom food and no user-supplied macros
- **THEN** the system stores it with `matched = false` and zeroed macros, and excludes it from the meal aggregate rather than estimating values

### Requirement: Meal Aggregate Totals on Confirmation
The system SHALL sum the 7 macros across a meal's items when the user confirms it, store the aggregate on the `FoodMeal` record, and set its status to `confirmed`. The system SHALL NOT create or update any row in the `Nutrition` table.

#### Scenario: Confirming meal logging
- **WHEN** the user confirms a `FoodMeal` breakdown
- **THEN** the system aggregates the 7 macros across all matched meal items, stores the totals on the meal, and sets the meal status to `confirmed`

#### Scenario: Nutrition telemetry is left untouched
- **WHEN** a user confirms a meal
- **THEN** no row in the `Nutrition` table is created, updated, or deleted, and no `source_payload_id` is minted

### Requirement: Manual Food Logging
The system SHALL expose `POST /api/food/meals/manual`, allowing a user to create a meal with no photo by supplying item names with either a food reference (USDA or custom) plus a weight, or direct macro values.

#### Scenario: Manual meal entry from food references
- **WHEN** a user manually creates a meal with item names, food references, and gram weights
- **THEN** the system creates the `FoodMeal` with an empty photo path, scales each item's nutrients from its referenced profile, and stores the aggregate

#### Scenario: Manual meal entry from direct macro values
- **WHEN** a user manually creates a meal supplying macro values directly, such as from a package label
- **THEN** the system stores those values as given without requiring a USDA or custom food match

#### Scenario: Manual meal never enters analysis states
- **WHEN** a manual meal is created
- **THEN** its status is `pending_review` or `confirmed`, never `processing`, and no vision model request is made

### Requirement: Meal Deletion Removes Items and Photo
Deleting a meal SHALL remove its `FoodItem` rows and its stored photo file in the same operation, leaving no orphaned rows or files.

#### Scenario: Delete a meal with items and a photo
- **WHEN** the owner deletes a meal that has items and a stored photo
- **THEN** the system removes the meal row, all of its `FoodItem` rows, and the photo file from disk

#### Scenario: Delete a meal whose photo file is already missing
- **WHEN** the owner deletes a meal whose photo file is absent from disk
- **THEN** the deletion still succeeds and removes the meal and its items
