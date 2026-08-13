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

Recognize SHALL include, for every recognized item, an optional per-item estimated nutrient profile (calories, protein, carbohydrates, fat, sugar, sodium, dietary fiber, per 100g) produced in the same model call as recognition, without a separate model call. This profile SHALL be persisted on the item's stored record as soon as the item is created — not held only transiently in the recognition response — so it remains available after a clarification round (see "Bounded Text-Only Clarification Rounds"), whose follow-up call is text-only and cannot regenerate it, and so a later weight edit has a per-100g basis to rescale from (see `food-nutrition-logging` "Item Resolution").

When candidate selection (see `usda-nutrition-database` "Match Selection and Explicit Non-Match") does not resolve an item to any source — whether by an explicit no-match response, an empty candidate shortlist that selection was never run against, or a selection result that fails to bind for any other reason — the system SHALL use this persisted estimated profile, scaled by the item's weight, as that item's macros and set its `macro_source` to `estimated`, provided the profile is present and usable (a valid, non-negative estimate). When candidate selection does find a match, the persisted estimate SHALL be discarded in favor of the matched source's macros. When no candidate selection resolves an item AND no usable estimate is present for it (Recognize produced none, or produced an invalid one), the item SHALL remain `macro_source = none`, unchanged from today.

An `estimated` item's macros are model-produced guesses, not values bound to a database or custom-food record or supplied directly by the user, and SHALL be surfaced in the review UI with a visual treatment that distinguishes it from `reference` and `manual` items, so the user is prompted to verify or correct it.

#### Scenario: No candidate found falls back to a model estimate

- **WHEN** candidate selection finds no suitable custom food, Open Food Facts, or USDA candidate for a recognized item, and Recognize produced a usable estimated profile for it
- **THEN** the system stores that item with `macro_source = estimated` and macros scaled from that persisted estimated profile, instead of `macro_source = none` with no usable macros

#### Scenario: No candidate and no usable estimate remains unresolved

- **WHEN** candidate selection finds no suitable candidate for a recognized item, and Recognize produced no estimated profile for it, or produced one that fails validation
- **THEN** the system stores that item with `macro_source = none` and zeroed macros, exactly as it does today

#### Scenario: A matched candidate takes precedence over the estimate

- **WHEN** candidate selection finds and selects a suitable candidate for a recognized item that also carries an estimated nutrient profile
- **THEN** the system uses the matched candidate's macros and `macro_source = reference`, and does not use the discarded estimate

#### Scenario: An estimate persists through a clarification round

- **GIVEN** a recognized item with a persisted estimated profile whose meal enters `pending_clarification`
- **WHEN** the clarification round concludes and candidate selection still finds no suitable match for that item
- **THEN** the system uses the estimated profile persisted at the item's original creation, since the clarification follow-up call is text-only and cannot produce a fresh one

#### Scenario: An estimated item is visually distinguished in review

- **WHEN** the user opens the confirm-review screen for a meal containing an item with `macro_source = estimated`
- **THEN** that item is shown with a visual treatment distinct from `reference` and `manual` items, indicating its macros are an unverified AI estimate
