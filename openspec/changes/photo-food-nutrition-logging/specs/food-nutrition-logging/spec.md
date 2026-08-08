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

#### Scenario: Unresolved item contributes no macros
- **WHEN** an item is bound to no USDA or custom food and has no user-supplied macros
- **THEN** the system stores it with `macro_source = none` and zeroed macros, and excludes it from the meal aggregate rather than estimating values

### Requirement: Meal Aggregate Totals on Confirmation
The system SHALL sum the 7 macros across every item that has usable macro values when the user confirms a meal, store the aggregate on the `FoodMeal` record, and set its status to `confirmed`. An item has usable macros when its `macro_source` is `reference` (scaled from a bound USDA or custom food) **or** `manual` (supplied directly by the user); only `macro_source = none` is excluded. Aggregation SHALL NOT be restricted to items bound to a reference food, because that would silently zero out meals logged from package labels. The system SHALL NOT create or update any row in the `Nutrition` table.

#### Scenario: Confirming meal logging
- **WHEN** the user confirms a `FoodMeal` breakdown
- **THEN** the system aggregates the 7 macros across every item whose `macro_source` is `reference` or `manual`, stores the totals on the meal, and sets the meal status to `confirmed`

#### Scenario: Confirming a meal of manually entered items only
- **WHEN** a user confirms a meal whose items all carry directly supplied macro values and no reference food binding
- **THEN** the meal aggregate equals the sum of those supplied values and is not zero

#### Scenario: Meal mixing resolved and unresolved items
- **GIVEN** a meal with one `reference` item, one `manual` item, and one `none` item
- **WHEN** the user confirms it
- **THEN** the aggregate includes the first two items and excludes the third, and the meal is still confirmed

#### Scenario: Nutrition telemetry is left untouched
- **WHEN** a user confirms a meal
- **THEN** no row in the `Nutrition` table is created, updated, or deleted, and no `source_payload_id` is minted

### Requirement: Meal Logged Time
Every meal SHALL carry a non-zero `logged_at`, defaulting to the time the upload was received. `POST /api/food/meals/manual` SHALL accept an explicit `logged_at`, and `PUT /api/food/meals/{id}/confirm` SHALL allow correcting it, so a user can record a meal eaten earlier.

#### Scenario: Logged time defaults to upload time
- **WHEN** a user uploads a meal photo without specifying a time
- **THEN** the system sets `logged_at` to the time the upload was received

#### Scenario: Backdated manual meal
- **WHEN** a user creates a manual meal with an explicit `logged_at` of the previous day
- **THEN** the system stores that time, and the meal is returned by a query whose range covers it

### Requirement: Item Resolution
The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id` or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`, before the meal is confirmed. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name, so the review UI reflects what was actually confirmed rather than the original vision-model guess.

#### Scenario: Bind an unresolved item to a food
- **WHEN** the owner patches an item with a chosen `fdc_id`
- **THEN** the system sets `macro_source = reference`, rescales the 7 macros from that profile and the item's weight, and includes the item in subsequent aggregates

#### Scenario: Supply macros directly for an unresolved item
- **WHEN** the owner patches an item with direct macro values
- **THEN** the system sets `macro_source = manual` and stores those values as given

#### Scenario: Correct an already-matched item
- **WHEN** the owner patches an item that already has `macro_source = reference` with a different `fdc_id` and a `name`
- **THEN** the system rebinds it, rescales macros from the new profile, and replaces the displayed name with the one supplied

#### Scenario: Renaming an item does not require touching its macros
- **WHEN** the owner patches an item with only a `name` and no `fdc_id`, `custom_food_id`, `manual`, or `weight_grams`
- **THEN** the system updates the name and returns 200 without changing `macro_source` or any stored macro value

#### Scenario: A blank name alone is not something to update
- **WHEN** the owner patches an item with only an empty or whitespace-only `name` and no other field
- **THEN** the system returns HTTP 400 rather than accepting a request with nothing meaningful to apply

#### Scenario: Patch an item of a confirmed meal
- **WHEN** the owner patches an item belonging to a meal whose status is `confirmed`
- **THEN** the system returns HTTP 409 and does not modify the item

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
