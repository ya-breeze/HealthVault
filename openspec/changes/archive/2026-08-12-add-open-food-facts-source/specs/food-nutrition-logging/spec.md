## MODIFIED Requirements

### Requirement: Item Resolution

The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id`, `off_code`, or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name, so the review UI reflects what was actually confirmed rather than the original vision-model guess.

Item resolution SHALL be permitted while the owning meal's status is `pending_review` or `confirmed`. It SHALL be rejected with HTTP 409 while the owning meal's status is `processing`, `pending_clarification`, or `failed`, since those states have no stable, reviewable item set yet.

Whenever a request causes the item to be scaled from a reference food's profile — either by supplying an `fdc_id`/`off_code`/`custom_food_id`, or by changing `weight_grams` alone on an item already bound to one — the weight used for that scaling (the supplied `weight_grams`, or the item's existing weight if none is supplied) SHALL be greater than zero. A request that would scale a reference profile by a zero or negative weight SHALL be rejected with HTTP 400 and SHALL NOT modify the item.

At most one of `fdc_id`, `off_code`, and `custom_food_id` SHALL be supplied in a single request; supplying more than one SHALL be rejected with HTTP 400 and SHALL NOT modify the item, for the same reason a `manual` binding combined with any reference is rejected below — more than one candidate reference source is ambiguous about which one was intended.

The system SHALL apply the item change and, if the owning meal's status is `confirmed`, recompute and persist the meal's macro aggregate from its current items, within a single database transaction (see the "Meal Aggregate Recomputed After Edit While Confirmed" requirement). The response body SHALL be the full updated `FoodMeal`, including its current items and (for a `confirmed` meal) its freshly recomputed aggregate — not the item alone — so a caller can update its full view of the meal from one response.

The item SHALL be loaded and mutated within the same transaction as the write that persists it, not loaded beforehand and mutated in memory — and that write SHALL be conditioned on the item's `updated_at` still matching what was observed when it was loaded within this transaction, so a stale write can never silently overwrite a concurrent request's already-applied change with a stale copy of every other column. A write based on a since-invalidated read is detected either by the conditional write affecting zero rows, or by the database rejecting the write outright because the transaction's own read snapshot is no longer current; either SHALL cause the whole transaction to retry, re-reading the item fresh, up to a small bounded number of attempts — not an immediate rejection. Because the item is reloaded fresh on every attempt, this allows non-conflicting concurrent changes (e.g. a weight edit and an unrelated name correction) to both apply, merged, without either being falsely rejected. Only a write that is still based on a stale read after every retry attempt has been exhausted SHALL be rejected, with HTTP 409, and SHALL NOT apply any part of its change.

#### Scenario: Bind an unresolved item to a food

- **WHEN** the owner patches an item with a chosen `fdc_id`
- **THEN** the system sets `macro_source = reference`, rescales the 7 macros from that profile and the item's weight, includes the item in subsequent aggregates, and returns the full updated meal

#### Scenario: Bind an unresolved item to an Open Food Facts product

- **WHEN** the owner patches an item with a chosen `off_code`
- **THEN** the system sets `macro_source = reference`, rescales the 7 macros from that Open Food Facts product's profile and the item's weight, includes the item in subsequent aggregates, and returns the full updated meal

#### Scenario: Supply macros directly for an unresolved item

- **WHEN** the owner patches an item with direct macro values
- **THEN** the system sets `macro_source = manual` and stores those values as given

#### Scenario: Correct an already-matched item

- **WHEN** the owner patches an item that already has `macro_source = reference` with a different `fdc_id` and a `name`
- **THEN** the system rebinds it, rescales macros from the new profile, and replaces the displayed name with the one supplied

#### Scenario: Binding or rescaling a reference item requires a positive weight

- **WHEN** the owner patches an item with an `fdc_id` and a `weight_grams` of 0 or less, or changes only `weight_grams` to 0 or less on an item already bound to a reference food
- **THEN** the system returns HTTP 400 and does not modify the item

#### Scenario: Renaming an item does not require touching its macros

- **WHEN** the owner patches an item with only a `name` and no `fdc_id`, `off_code`, `custom_food_id`, `manual`, or `weight_grams`
- **THEN** the system updates the name and returns 200 without changing `macro_source` or any stored macro value

#### Scenario: A blank name alone is not something to update

- **WHEN** the owner patches an item with only an empty or whitespace-only `name` and no other field
- **THEN** the system returns HTTP 400 rather than accepting a request with nothing meaningful to apply

#### Scenario: Patch an item of a confirmed meal

- **WHEN** the owner patches an item belonging to a meal whose status is `confirmed`
- **THEN** the system applies the change exactly as it would for a `pending_review` meal, recomputes and persists the meal's aggregate in the same transaction, and returns the full updated meal — it does not return 409

#### Scenario: Patch an item of a meal still being analyzed is rejected

- **WHEN** the owner patches an item belonging to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not modify the item

#### Scenario: Cross-user item patch is rejected

- **WHEN** a user patches an item belonging to a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not modify the item

#### Scenario: Manual macros combined with a food reference is rejected

- **WHEN** the owner patches an item with `manual: true` together with an `fdc_id`, `off_code`, or `custom_food_id`
- **THEN** the system returns HTTP 400 rather than silently preferring one and discarding the other

#### Scenario: More than one reference source in the same request is rejected

- **WHEN** the owner patches an item with more than one of `fdc_id`, `off_code`, and `custom_food_id` set
- **THEN** the system returns HTTP 400 and does not modify the item

#### Scenario: Binding to off_code when no Open Food Facts database has been imported

- **WHEN** the owner patches an item with an `off_code` while no Open Food Facts database has ever been imported
- **THEN** the system returns HTTP 400 reporting the Open Food Facts reference source as unavailable, distinct from the response for an `off_code` that is simply not present in an available index, and does not modify the item

#### Scenario: Two concurrent, non-conflicting patches to the same item both apply

- **GIVEN** an item with an existing binding
- **WHEN** a weight-change PATCH and an unrelated name-correction PATCH for the same item are submitted close enough together that the second request's write is based on a read taken before the first request committed
- **THEN** both changes end up applied — the losing request's retry re-reads the item fresh (now reflecting the winner) and merges its own change on top, rather than being rejected for a conflict that was never real

#### Scenario: A persistently conflicting patch to the same item is eventually rejected

- **GIVEN** an item with an existing binding
- **WHEN** a PATCH request's write is based on a stale read on every one of its retry attempts (e.g. sustained concurrent writes to the same item)
- **THEN** the system rejects it — neither request's write silently overwrites the other's already-committed columns, and the meal is left as whichever attempt actually committed

### Requirement: Item Addition

The system SHALL expose `POST /api/food/meals/{id}/items`, allowing the owner to add a new item to a meal whose status is `pending_review` or `confirmed`. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

Because a newly created item starts with no prior state to fall back on (unlike a PATCH target, which already has a name, weight, and macro source), the request SHALL require a non-blank `name`, plus exactly one of: `manual: true` together with macro values, or an `fdc_id`/`off_code`/`custom_food_id` together with a `weight_grams` greater than zero. A request missing a name, missing both a macro source and a reference, supplying more than one of `fdc_id`/`off_code`/`custom_food_id`, or supplying a reference with no weight or a non-positive weight, SHALL be rejected with HTTP 400.

The system SHALL apply the same transactional aggregate-recompute-on-confirmed behavior as item resolution, and return the full updated `FoodMeal`.

#### Scenario: Add an item bound to a reference food

- **WHEN** the owner posts a new item with a `name`, an `fdc_id`, and a positive `weight_grams` to a `pending_review` meal
- **THEN** the system creates the item with `macro_source = reference`, macros scaled from that profile and weight, and returns the full updated meal including the new item

#### Scenario: Add an item bound to an Open Food Facts product

- **WHEN** the owner posts a new item with a `name`, an `off_code`, and a positive `weight_grams` to a `pending_review` meal
- **THEN** the system creates the item with `macro_source = reference`, macros scaled from that Open Food Facts product's profile and weight, and returns the full updated meal including the new item

#### Scenario: Add a manual item to a confirmed meal

- **WHEN** the owner posts a new item with a `name`, `manual: true`, and macro values to a `confirmed` meal
- **THEN** the system creates the item with `macro_source = manual`, the supplied macro values, recomputes and persists the meal's aggregate to include it, and returns the full updated meal

#### Scenario: Missing name is rejected

- **WHEN** the owner posts a new item with no `name`, or a blank/whitespace-only one
- **THEN** the system returns HTTP 400 and does not create an item

#### Scenario: Reference without a positive weight is rejected

- **WHEN** the owner posts a new item with an `fdc_id` but no `weight_grams`, or a `weight_grams` of 0 or less
- **THEN** the system returns HTTP 400 and does not create an item

#### Scenario: Neither a reference nor manual macros is rejected

- **WHEN** the owner posts a new item with only a `name` and nothing establishing its macro source
- **THEN** the system returns HTTP 400 and does not create an item

#### Scenario: More than one reference source is rejected

- **WHEN** the owner posts a new item with more than one of `fdc_id`, `off_code`, and `custom_food_id` set
- **THEN** the system returns HTTP 400 and does not create an item

#### Scenario: Add an item to a meal still being analyzed is rejected

- **WHEN** the owner posts a new item to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not create an item

#### Scenario: Cross-user item creation is rejected

- **WHEN** a user posts a new item to a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not create an item

### Requirement: Manual Food Logging

The system SHALL expose `POST /api/food/meals/manual`, allowing a user to create a meal with no photo by supplying item names with either a food reference (USDA, Open Food Facts, or custom) plus a weight, or direct macro values. At most one of `fdc_id`, `off_code`, and `custom_food_id` SHALL be supplied per item; supplying more than one SHALL be rejected with HTTP 400 and SHALL NOT create the meal.

#### Scenario: Manual meal entry from food references

- **WHEN** a user manually creates a meal with item names, food references, and gram weights
- **THEN** the system creates the `FoodMeal` with an empty photo path, scales each item's nutrients from its referenced profile, and stores the aggregate

#### Scenario: Manual meal entry from an Open Food Facts reference

- **WHEN** a user manually creates a meal item with an `off_code` and a gram weight
- **THEN** the system scales that item's nutrients from the Open Food Facts product's profile, the same way it does for an `fdc_id` or `custom_food_id` reference

#### Scenario: Manual meal entry from direct macro values

- **WHEN** a user manually creates a meal supplying macro values directly, such as from a package label
- **THEN** the system stores those values as given without requiring a USDA, Open Food Facts, or custom food match

#### Scenario: More than one reference source in a manual item is rejected

- **WHEN** a user manually creates a meal item with more than one of `fdc_id`, `off_code`, and `custom_food_id` set
- **THEN** the system returns HTTP 400 and does not create the meal, rather than silently resolving via one field while persisting all of them

#### Scenario: Manual meal never enters analysis states

- **WHEN** a manual meal is created
- **THEN** its status is `pending_review` or `confirmed`, never `processing`, and no vision model request is made
