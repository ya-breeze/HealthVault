## MODIFIED Requirements

### Requirement: Macro Estimate Fallback for Unmatched Items

Recognize SHALL include, for every recognized item, an optional per-item estimated nutrient profile (calories, protein, carbohydrates, fat, sugar, sodium, dietary fiber, per 100g) produced in the same model call as recognition, without a separate model call. This profile SHALL be persisted on the item's stored record as soon as the item is created — not held only transiently in the recognition response — so it remains available after a clarification round (see "Bounded Text-Only Clarification Rounds"), whose follow-up call is text-only and cannot regenerate it, and so a later weight edit has a per-100g basis to rescale from (see `food-nutrition-logging` "Item Resolution").

An estimated profile is **usable for automatic resolution** when its values pass validation (a valid, non-negative estimate, as before) AND pass all of these plausibility checks: protein, carbohydrates, and fat are each at most 102g/100g and their sum is at most 102g/100g; sugar and dietary fiber together exceed carbohydrates by no more than 2g/100g, checked as a combined sum rather than independently, since both are subsets of carbohydrates; and declared calories are no lower than `atwater - max(25 kcal, atwater×0.15)`, where `atwater = protein×4 + carbs×4 + fat×9`. The 2g allowances absorb rounding while the combined-macro and combined-sugar/fiber bounds reject profiles whose total mass, or whose sugar-plus-fiber content, is physically impossible. The calorie check is one-sided: declared calories exceeding the Atwater figure is not a violation, since sources such as alcohol contribute calories the three tracked macros do not capture. An estimate that is present but fails this plausibility check SHALL be treated as absent for automatic-resolution precedence, but remains persisted so an item already stored with `macro_source = estimated` can continue to rescale consistently on later weight edits.

The system SHALL use the item's persisted usable estimated profile, scaled by the item's weight, as that item's macros and set its `macro_source` to `estimated`, by default whenever a usable estimate is present, EXCEPT when the item is bound to a reference food via a deterministic identity match — an exact case-insensitive match against one of the user's own saved `CustomFood` entries (see `usda-nutrition-database` "Match Selection and Explicit Non-Match"), or a reference explicitly supplied by the caller on a PATCH, item-add, or manual-meal request (see `food-nutrition-logging` "Item Resolution") — in which case that deterministic match SHALL take unconditional precedence over the estimate, exactly as it does today.

When the item has no usable estimate (Recognize produced none, or produced one that fails validation or the plausibility check), the system SHALL fall back to whatever candidate selection resolved for the item (see `usda-nutrition-database` "Match Selection and Explicit Non-Match"): a selected candidate's macros with `macro_source = reference` if one was found, or `macro_source = none` with zeroed macros, unchanged from today, if none was found.

When the item has a usable estimate but candidate selection instead resolved it to a non-deterministic match — a `Select`-picked USDA, Open Food Facts, or frequency/recency-ranked custom-food candidate, none of which are a deterministic identity match — the usable estimate SHALL still take precedence over that matched candidate; the matched candidate's macros are discarded in favor of the estimate.

An `estimated` item's macros are model-produced guesses, not values bound to a database or custom-food record or supplied directly by the user, and SHALL be surfaced in the review UI with a visual treatment that distinguishes it from `reference` and `manual` items, so the user is prompted to verify or correct it.

#### Scenario: No candidate found falls back to a model estimate

- **WHEN** candidate selection finds no suitable custom food, Open Food Facts, or USDA candidate for a recognized item, and Recognize produced a usable estimated profile for it
- **THEN** the system stores that item with `macro_source = estimated` and macros scaled from that persisted estimated profile, instead of `macro_source = none` with no usable macros

#### Scenario: No candidate and no usable estimate remains unresolved

- **WHEN** candidate selection finds no suitable candidate for a recognized item, and Recognize produced no estimated profile for it, or produced one that fails validation or the plausibility check
- **THEN** the system stores that item with `macro_source = none` and zeroed macros, exactly as it does today

#### Scenario: A usable estimate takes precedence over a fuzzy-matched candidate

- **WHEN** candidate selection selects a USDA, Open Food Facts, or ranked-custom-food candidate for a recognized item (a `Select`-picked, non-deterministic match), and the item carries a usable (valid and plausible) estimated profile
- **THEN** the system uses that estimated profile, scaled by weight, and sets `macro_source = estimated`, discarding the matched candidate's macros

#### Scenario: A matched candidate takes precedence over the estimate

- **WHEN** an item is bound to a reference food via an exact case-insensitive match against the user's own saved custom food, or via a caller-supplied `fdc_id`/`off_code`/`custom_food_id` on a PATCH, item-add, or manual-meal request, and the item also carries an estimated nutrient profile
- **THEN** the system uses the deterministically-matched reference's macros and `macro_source = reference`, and does not use the discarded estimate

#### Scenario: An implausible estimate falls back to a fuzzy-matched candidate

- **WHEN** Recognize produces an estimated profile for a recognized item that is valid (non-negative) but fails the plausibility check (e.g. combined protein/carbs/fat exceeds 102g/100g, or declared calories fall below the exact one-sided Atwater threshold), and candidate selection selects a suitable USDA, Open Food Facts, or custom-food candidate for that item
- **THEN** the system treats the estimate as unusable, uses the matched candidate's macros instead, and sets `macro_source = reference`

#### Scenario: An estimate persists through a clarification round

- **GIVEN** a recognized item with a persisted estimated profile whose meal enters `pending_clarification`
- **WHEN** the clarification round concludes and candidate selection still finds no suitable match for that item
- **THEN** the system uses the estimated profile persisted at the item's original creation, since the clarification follow-up call is text-only and cannot produce a fresh one

#### Scenario: An estimated item is visually distinguished in review

- **WHEN** the user opens the confirm-review screen for a meal containing an item with `macro_source = estimated`
- **THEN** that item is shown with a visual treatment distinct from `reference` and `manual` items, indicating its macros are an unverified AI estimate
