## MODIFIED Requirements

### Requirement: Item Resolution
The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id` or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name, so the review UI reflects what was actually confirmed rather than the original vision-model guess.

Item resolution SHALL be permitted while the owning meal's status is `pending_review` or `confirmed`. It SHALL be rejected with HTTP 409 while the owning meal's status is `processing`, `pending_clarification`, or `failed`, since those states have no stable, reviewable item set yet.

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

#### Scenario: Patch an item of a confirmed meal is now permitted
- **WHEN** the owner patches an item belonging to a meal whose status is `confirmed`
- **THEN** the system applies the change exactly as it would for a `pending_review` meal, and does not return 409

#### Scenario: Patch an item of a meal still being analyzed is rejected
- **WHEN** the owner patches an item belonging to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not modify the item

## ADDED Requirements

### Requirement: Item Addition
The system SHALL expose `POST /api/food/meals/{id}/items`, allowing the owner to add a new item to a meal whose status is `pending_review` or `confirmed`. The request body SHALL accept the same fields and precedence as item resolution: bind to `fdc_id`/`custom_food_id` plus `weight_grams`, or supply macro values directly via `manual`, along with a `name`. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

#### Scenario: Add an item bound to a reference food
- **WHEN** the owner posts a new item with an `fdc_id` and `weight_grams` to a `pending_review` meal
- **THEN** the system creates the item with `macro_source = reference`, macros scaled from that profile and weight, and it appears among the meal's items

#### Scenario: Add a manual item to a confirmed meal
- **WHEN** the owner posts a new item with `manual: true` and macro values to a `confirmed` meal
- **THEN** the system creates the item with `macro_source = manual`, the supplied macro values, and includes it in the meal's aggregate

#### Scenario: Add an item to a meal still being analyzed is rejected
- **WHEN** the owner posts a new item to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not create an item

### Requirement: Item Deletion
The system SHALL expose `DELETE /api/food/meals/{id}/items/{item_id}`, allowing the owner to remove an item from a meal whose status is `pending_review` or `confirmed`. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

#### Scenario: Delete an item from a confirmed meal
- **WHEN** the owner deletes an item belonging to a `confirmed` meal
- **THEN** the system removes the item and it no longer appears among the meal's items or contributes to its aggregate

#### Scenario: Delete an item from a meal still being analyzed is rejected
- **WHEN** the owner deletes an item belonging to a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not remove the item

#### Scenario: Delete a nonexistent or unowned item
- **WHEN** the owner requests deletion of an item ID that does not exist, or belongs to a different meal or a different user
- **THEN** the system returns HTTP 404

### Requirement: Meal Aggregate Recomputed After Edit While Confirmed
Whenever an item is added, edited, or deleted on a meal whose status is `confirmed`, the system SHALL immediately recompute the meal's macro aggregate from its current items (the same computation used at confirm time) and persist it on the `FoodMeal` record, so the stored totals always equal the sum over the meal's current items. This SHALL happen synchronously within the same request as the item write, with no separate save step. A meal whose status is `pending_review` SHALL NOT have its aggregate recomputed by item writes — it remains as before, computed only once, at confirm.

#### Scenario: Editing an item's weight on a confirmed meal updates the meal total
- **GIVEN** a confirmed meal with a stored total of 500 kcal
- **WHEN** the owner edits one item's weight in a way that changes its calories by +100 kcal
- **THEN** the meal's stored total becomes 600 kcal in the same response

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

### Requirement: Meal Name and Logged Time Correction
The system SHALL expose `PATCH /api/food/meals/{id}`, allowing the owner to correct a meal's `name` and/or `logged_at` independently of confirming it, while the meal's status is `pending_review` or `confirmed`. At least one of `name` or `logged_at` SHALL be supplied. The endpoint SHALL be rejected with HTTP 409 for a meal whose status is `processing`, `pending_clarification`, or `failed`.

#### Scenario: Rename a confirmed meal
- **WHEN** the owner patches a `confirmed` meal with a new `name`
- **THEN** the system updates the meal's name and leaves its items and aggregate unchanged

#### Scenario: Correct the logged time of a confirmed meal
- **WHEN** the owner patches a `confirmed` meal with a new `logged_at`
- **THEN** the system updates `logged_at` and the meal is returned by queries whose range covers the new time rather than the old one

#### Scenario: Empty request body is rejected
- **WHEN** the owner patches a meal with neither `name` nor `logged_at` supplied
- **THEN** the system returns HTTP 400

#### Scenario: Patch a meal still being analyzed is rejected
- **WHEN** the owner patches a meal whose status is `processing`, `pending_clarification`, or `failed`
- **THEN** the system returns HTTP 409 and does not modify the meal
