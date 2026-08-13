## MODIFIED Requirements

### Requirement: Macro Calculation and Portion Scaling

The system SHALL compute 7 primary macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) for each matched item by scaling its per-100g nutritional profile by its gram weight. An item with `macro_source = estimated` uses the same 7-macro scaling, applied to Recognize's own estimated per-100g profile for that item rather than a bound reference profile (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items").

#### Scenario: Gram weight scaling
- **WHEN** a food item of 180 grams is matched to a USDA profile with 165 kcal per 100g
- **THEN** the system calculates the item calories as 297 kcal (165 * 1.8) and scales all other 6 macros proportionally

#### Scenario: Weight adjusted after matching
- **WHEN** the user changes an item's weight from 180g to 200g before confirming
- **THEN** the system recalculates all 7 macros for that item from the same per-100g profile and updates the meal aggregate

#### Scenario: Unresolved item with no estimate contributes no macros
- **WHEN** an item is bound to no USDA, Open Food Facts, or custom food, has no user-supplied macros, and Recognize produced no usable estimated profile for it
- **THEN** the system stores it with `macro_source = none` and zeroed macros, and excludes it from the meal aggregate

### Requirement: Meal Aggregate Totals on Confirmation

The system SHALL sum the 7 macros across every item that has usable macro values when the user confirms a meal, store the aggregate on the `FoodMeal` record, and set its status to `confirmed`. An item has usable macros when its `macro_source` is `reference` (scaled from a bound USDA, Open Food Facts, or custom food), `manual` (supplied directly by the user), or `estimated` (scaled from Recognize's own estimated profile when no candidate matched); only `macro_source = none` is excluded. Aggregation SHALL NOT be restricted to items bound to a reference food, because that would silently zero out meals logged from package labels or from a model estimate. The system SHALL NOT create or update any row in the `Nutrition` table.

#### Scenario: Confirming meal logging
- **WHEN** the user confirms a `FoodMeal` breakdown
- **THEN** the system aggregates the 7 macros across every item whose `macro_source` is `reference`, `manual`, or `estimated`, stores the totals on the meal, and sets the meal status to `confirmed`

#### Scenario: Confirming a meal of manually entered items only
- **WHEN** a user confirms a meal whose items all carry directly supplied macro values and no reference food binding
- **THEN** the meal aggregate equals the sum of those supplied values and is not zero

#### Scenario: Meal mixing resolved and unresolved items
- **GIVEN** a meal with one `reference` item, one `manual` item, and one `none` item
- **WHEN** the user confirms it
- **THEN** the aggregate includes the first two items and excludes the third, and the meal is still confirmed

#### Scenario: An estimated item contributes to the aggregate
- **GIVEN** a meal with one `reference` item and one `estimated` item
- **WHEN** the user confirms it
- **THEN** the aggregate includes both items' macros, and the meal is still confirmed

#### Scenario: Nutrition telemetry is left untouched
- **WHEN** a user confirms a meal
- **THEN** no row in the `Nutrition` table is created, updated, or deleted, and no `source_payload_id` is minted
