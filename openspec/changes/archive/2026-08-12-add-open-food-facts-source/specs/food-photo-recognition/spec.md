## ADDED Requirements

### Requirement: Recognized Item Brand

Each recognized item SHALL carry a `brand` alongside its name, preparation, and state. It SHALL be the product/manufacturer brand as legibly shown on packaging visible in the photo, or empty when no brand is legible or the food is unpackaged/homemade.

This exists so that later food-reference matching (see `usda-nutrition-database` "Match Selection and Explicit Non-Match") has a real signal for distinguishing among differently-branded packaged products, which candidate selection cannot otherwise do: selection is a text-only model call with no access to the photo, so a generic name alone cannot tell which of several branded products with materially different macros was actually photographed.

#### Scenario: Brand extracted when a label is visible

- **WHEN** the vision model recognizes a packaged food whose brand is legible in the photo
- **THEN** the returned item includes a non-empty `brand` alongside its name, preparation, state, weight, and confidence

#### Scenario: No brand to extract

- **WHEN** the vision model recognizes an unpackaged or homemade food, or a package whose brand is not legible
- **THEN** the returned item's `brand` is empty rather than guessed, and the item is still returned with its name and weight

#### Scenario: Brand is persisted

- **WHEN** an item is stored
- **THEN** its `brand` is persisted with it, so that a later clarification answer or reanalysis can re-run food lookup without re-extracting it from the photo
