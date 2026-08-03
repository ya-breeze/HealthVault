## Purpose

Computes 7 primary macros for meal items, allows manual macro edits/entries, and synchronizes aggregate totals into HealthVault's main Nutrition records.

## ADDED Requirements

### Requirement: Macro Calculation and Portion Scaling
The system SHALL compute 7 primary macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) for each recognized item by scaling its per-100g nutritional profile by its gram weight.

#### Scenario: Gram weight scaling
- **WHEN** a food item of 180 grams is matched to a USDA profile with 165 kcal per 100g
- **THEN** the system calculates the item calories as 297 kcal (165 * 1.8) and scales all other 6 macros proportionally.

### Requirement: Syncing Aggregates to Main HealthVault Storage
The system SHALL sum total meal macros upon user confirmation and auto-create or update an entry in the primary `Nutrition` table.

#### Scenario: Confirming meal logging
- **WHEN** the user confirms a `FoodMeal` breakdown
- **THEN** the system aggregates calories and macros across all meal items, updates `FoodMeal` status to `confirmed`, and creates a corresponding record in the main `Nutrition` table.

### Requirement: Manual Food Logging
The system SHALL allow users to manually log meals and nutrient values without requiring a photo.

#### Scenario: Manual meal entry
- **WHEN** a user manually enters meal details, item names, weights, or macro values
- **THEN** the system creates the meal record, scales nutrients, and syncs totals to the main `Nutrition` database.
