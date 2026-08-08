## MODIFIED Requirements

### Requirement: Item Resolution
The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id` or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name, so the review UI reflects what was actually confirmed rather than the original vision-model guess.

Item resolution SHALL be permitted while the owning meal's status is `pending_review` or `confirmed`. It SHALL be rejected with HTTP 409 while the owning meal's status is `processing`, `pending_clarification`, or `failed`, since those states have no stable, reviewable item set yet.

The system SHALL apply the item change and, if the owning meal's status is `confirmed`, recompute and persist the meal's macro aggregate from its current items, within a single database transaction (see the "Meal Aggregate Recomputed After Edit While Confirmed" requirement). The response body SHALL be the full updated `FoodMeal`, including its current items and (for a `confirmed` meal) its freshly recomputed aggregate — not the item alone — so a caller can update its full view of the meal from one response.

#### Scenario: Bind an unresolved item to a food
- **WHEN** the owner patches an item with a chosen `fdc_id`
- **THEN** the system sets `macro_source = reference`, rescales the 7 macros from that profile and the item's weight, includes the item in subsequent aggregates, and returns the full updated meal

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

#### Scenario: Patch an item of a confirmed meal is now permitted
- **WHEN** the owner patches an item belonging to a meal whose status is `confirmed`
- **THEN** the system applies the change exactly as it would for a `pending_review` meal, recomputes and persists the meal's aggregate in the same transaction, and returns the full updated meal — it does not return 409

#### Scenario: Patch an item of a meal still being analyzed is rejected
- **WHEN** the owner patches an item belonging to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not modify the item

#### Scenario: Cross-user item patch is rejected
- **WHEN** a user patches an item belonging to a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not modify the item

## ADDED Requirements

### Requirement: Item Addition
The system SHALL expose `POST /api/food/meals/{id}/items`, allowing the owner to add a new item to a meal whose status is `pending_review` or `confirmed`. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

Because a newly created item starts with no prior state to fall back on (unlike a PATCH target, which already has a name, weight, and macro source), the request SHALL require a non-blank `name`, plus exactly one of: `manual: true` together with macro values, or an `fdc_id`/`custom_food_id` together with a `weight_grams` greater than zero. A request missing a name, missing both a macro source and a reference, or supplying a reference with no weight or a non-positive weight, SHALL be rejected with HTTP 400.

The system SHALL apply the same transactional aggregate-recompute-on-confirmed behavior as item resolution, and return the full updated `FoodMeal`.

#### Scenario: Add an item bound to a reference food
- **WHEN** the owner posts a new item with a `name`, an `fdc_id`, and a positive `weight_grams` to a `pending_review` meal
- **THEN** the system creates the item with `macro_source = reference`, macros scaled from that profile and weight, and returns the full updated meal including the new item

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

#### Scenario: Add an item to a meal still being analyzed is rejected
- **WHEN** the owner posts a new item to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not create an item

#### Scenario: Cross-user item creation is rejected
- **WHEN** a user posts a new item to a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not create an item

### Requirement: Item Deletion
The system SHALL expose `DELETE /api/food/meals/{id}/items/{item_id}`, allowing the owner to remove an item from a meal whose status is `pending_review` or `confirmed`. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

The system SHALL apply the same transactional aggregate-recompute-on-confirmed behavior as item resolution, and return the full updated `FoodMeal`.

#### Scenario: Delete an item from a confirmed meal
- **WHEN** the owner deletes an item belonging to a `confirmed` meal
- **THEN** the system removes the item, recomputes and persists the meal's aggregate to exclude it, and returns the full updated meal

#### Scenario: Delete an item from a meal still being analyzed is rejected
- **WHEN** the owner deletes an item belonging to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not remove the item

#### Scenario: Delete a nonexistent item
- **WHEN** the owner requests deletion of an item ID that does not exist, or that belongs to a different meal than the one in the URL
- **THEN** the system returns HTTP 404

#### Scenario: Cross-user item deletion is rejected
- **WHEN** a user requests deletion of an item belonging to a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not remove the item

### Requirement: Meal Aggregate Recomputed After Edit While Confirmed
Whenever an item is added, edited, or deleted on a meal whose status is `confirmed`, the system SHALL, within the same database transaction as the item write, reload the meal's current items, recompute its macro aggregate (the same computation used at confirm time), and persist it on the `FoodMeal` record — so the stored totals always equal the sum over the meal's current items, and a failure partway through leaves neither the item change nor a stale aggregate committed. A meal whose status is `pending_review` SHALL NOT have its aggregate recomputed by item writes — it remains as before, computed only once, at confirm.

#### Scenario: Editing an item's weight on a confirmed meal updates the meal total
- **GIVEN** a confirmed meal with a stored total of 500 kcal
- **WHEN** the owner edits one item's weight in a way that changes its calories by +100 kcal
- **THEN** the meal's stored total becomes 600 kcal, visible in the same response

#### Scenario: Adding an item to a confirmed meal updates the meal total
- **GIVEN** a confirmed meal
- **WHEN** the owner adds a new item with usable macros
- **THEN** the meal's stored aggregate immediately includes that item's macros

#### Scenario: Deleting an item from a confirmed meal updates the meal total
- **GIVEN** a confirmed meal whose aggregate includes a particular item's macros
- **WHEN** the owner deletes that item
- **THEN** the meal's stored aggregate no longer includes that item's macros

#### Scenario: Pending-review meals are unaffected
- **GIVEN** a `pending_review` meal
- **WHEN** the owner edits, adds, or deletes an item
- **THEN** the meal's stored aggregate is not recomputed by that write, unchanged from today's behavior

#### Scenario: A failed item write leaves the aggregate untouched
- **GIVEN** a confirmed meal
- **WHEN** an item write's transaction fails partway through (e.g. a storage error during the aggregate update)
- **THEN** neither the item change nor any aggregate change is committed, and the meal is unchanged from before the request

### Requirement: Meal Name and Logged Time Correction
The system SHALL expose `PATCH /api/food/meals/{id}`, allowing the owner to correct a meal's `name` and/or `logged_at` independently of confirming it, while the meal's status is `pending_review` or `confirmed`. At least one of `name` or `logged_at` SHALL be supplied. A supplied `logged_at` SHALL be a non-zero timestamp, preserving the existing invariant that every meal carries a non-zero `logged_at`; a zero-value timestamp SHALL be rejected with HTTP 400 rather than silently accepted. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

#### Scenario: Rename a confirmed meal
- **WHEN** the owner patches a `confirmed` meal with a new `name`
- **THEN** the system updates the meal's name and leaves its items and aggregate unchanged

#### Scenario: Correct the logged time of a confirmed meal
- **WHEN** the owner patches a `confirmed` meal with a new, non-zero `logged_at`
- **THEN** the system updates `logged_at` and the meal is returned by queries whose range covers the new time rather than the old one

#### Scenario: Empty request body is rejected
- **WHEN** the owner patches a meal with neither `name` nor `logged_at` supplied
- **THEN** the system returns HTTP 400

#### Scenario: Zero-value logged_at is rejected
- **WHEN** the owner patches a meal with a `logged_at` of the zero timestamp
- **THEN** the system returns HTTP 400 and does not modify the meal

#### Scenario: Patch a meal still being analyzed is rejected
- **WHEN** the owner patches a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not modify the meal

#### Scenario: Cross-user meal patch is rejected
- **WHEN** a user patches a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not modify the meal
