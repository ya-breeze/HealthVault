## ADDED Requirements

### Requirement: Composite Dish Naming

Recognize SHALL default to naming a visually composite dish (multiple ingredients combined into one served item, e.g. a curry, a stew, a pre-mixed side) as a single recognized item, and SHALL only return multiple items for a photo when it visually observes separately identifiable components (e.g. distinct piles on a plate, or clearly separate foods placed next to each other). This granularity judgment is made by the model on each photo; it is not a user-facing setting and there is no per-upload or per-user toggle for it.

#### Scenario: A single composite dish is recognized as one item

- **WHEN** a photo shows a homogeneous composite dish, such as a mixed vegetable side or a sauced dish, with no visually separate components
- **THEN** Recognize returns exactly one item for that dish, named to reflect the dish as a whole, rather than one item per ingredient it may contain

#### Scenario: A plate with visually separate components is still decomposed

- **WHEN** a photo shows a plate with clearly separate components, such as a portion of rice, a piece of grilled protein, and a side salad plated apart from each other
- **THEN** Recognize returns one item per visually separate component, as it does today

### Requirement: Macro Estimate Fallback for Unmatched Items

Recognize SHALL include, for every recognized item, an optional per-item estimated nutrient profile (calories, protein, carbohydrates, fat, sugar, sodium, dietary fiber) produced in the same model call as recognition, without a separate model call. When candidate selection (see `usda-nutrition-database` "Match Selection and Explicit Non-Match") finds no suitable candidate for an item, the system SHALL use this estimated profile, scaled by the item's weight, as that item's macros and set its `macro_source` to `estimated`. When candidate selection does find a match, the estimated profile SHALL be discarded in favor of the matched source's macros.

An `estimated` item's macros are model-produced guesses, not values bound to a database or custom-food record or supplied directly by the user, and SHALL be surfaced in the review UI with a visual treatment that distinguishes it from `reference` and `manual` items, so the user is prompted to verify or correct it.

#### Scenario: No candidate found falls back to a model estimate

- **WHEN** candidate selection finds no suitable custom food, Open Food Facts, or USDA candidate for a recognized item
- **THEN** the system stores that item with `macro_source = estimated` and macros scaled from Recognize's own estimated nutrient profile for that item, instead of `macro_source = none` with no usable macros

#### Scenario: A matched candidate takes precedence over the estimate

- **WHEN** candidate selection finds and selects a suitable candidate for a recognized item that also carries an estimated nutrient profile
- **THEN** the system uses the matched candidate's macros and `macro_source = reference`, and does not use the discarded estimate

#### Scenario: An estimated item is visually distinguished in review

- **WHEN** the user opens the confirm-review screen for a meal containing an item with `macro_source = estimated`
- **THEN** that item is shown with a visual treatment distinct from `reference` and `manual` items, indicating its macros are an unverified AI estimate
