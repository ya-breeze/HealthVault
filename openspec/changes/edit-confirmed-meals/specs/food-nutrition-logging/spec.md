## MODIFIED Requirements

### Requirement: Item Resolution
The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id` or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name, so the review UI reflects what was actually confirmed rather than the original vision-model guess.

Item resolution SHALL be permitted while the owning meal's status is `pending_review` or `confirmed`. It SHALL be rejected with HTTP 409 while the owning meal's status is `processing`, `pending_clarification`, or `failed`, since those states have no stable, reviewable item set yet.

Whenever a request causes the item to be scaled from a reference food's profile — either by supplying an `fdc_id`/`custom_food_id`, or by changing `weight_grams` alone on an item already bound to one — the weight used for that scaling (the supplied `weight_grams`, or the item's existing weight if none is supplied) SHALL be greater than zero. A request that would scale a reference profile by a zero or negative weight SHALL be rejected with HTTP 400 and SHALL NOT modify the item.

The system SHALL apply the item change and, if the owning meal's status is `confirmed`, recompute and persist the meal's macro aggregate from its current items, within a single database transaction (see the "Meal Aggregate Recomputed After Edit While Confirmed" requirement). The response body SHALL be the full updated `FoodMeal`, including its current items and (for a `confirmed` meal) its freshly recomputed aggregate — not the item alone — so a caller can update its full view of the meal from one response.

The item SHALL be loaded and mutated within the same transaction as the write that persists it, not loaded beforehand and mutated in memory — and that write SHALL be conditioned on the item's `updated_at` still matching what was observed when it was loaded within this transaction. If a concurrent request has modified the same item in between (detected either by the conditional write affecting zero rows, or by the database rejecting the write outright because the read it was based on is no longer current), the system SHALL reject this request with HTTP 409 and SHALL NOT apply any part of its change, rather than silently overwriting the concurrent request's already-applied change with a stale copy of every other column.

#### Scenario: Bind an unresolved item to a food
- **WHEN** the owner patches an item with a chosen `fdc_id`
- **THEN** the system sets `macro_source = reference`, rescales the 7 macros from that profile and the item's weight, includes the item in subsequent aggregates, and returns the full updated meal

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

#### Scenario: Manual macros combined with a food reference is rejected
- **WHEN** the owner patches an item with `manual: true` together with an `fdc_id` or `custom_food_id`
- **THEN** the system returns HTTP 400 rather than silently preferring one and discarding the other

#### Scenario: Two concurrent patches to the same item do not silently clobber each other
- **GIVEN** an item with an existing binding
- **WHEN** two PATCH requests for the same item (e.g. a weight change and a rebind) are submitted close enough together that the second request's write is based on a read taken before the first request committed
- **THEN** exactly one of them applies; the other returns HTTP 409 and does not modify the item — neither request's write silently overwrites the other's already-committed columns

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

This same transaction SHALL re-verify the meal's existence and editable status immediately before the item write, not rely solely on the check made when the request was first received — closing the gap between that earlier check and this write. If the meal no longer exists (e.g. deleted through the generic meal-delete endpoint in that gap), the system SHALL return HTTP 404. If a concurrent operation has changed the meal's status in that same gap (e.g. a Reanalyze claiming it), the system SHALL return HTTP 409, regardless of whether that's detected by the re-verification finding a different status or by the underlying write itself failing because it was based on a since-invalidated read. Neither case SHALL be reported as a generic server error.

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

#### Scenario: An item write reports not found if the meal is deleted mid-flight
- **GIVEN** an owner's request to add, edit, or delete an item passed the initial ownership check
- **WHEN** the meal is deleted (e.g. via the generic meal-delete endpoint) before this transaction's own re-verification runs
- **THEN** the system returns HTTP 404, not a generic server error

#### Scenario: An item write reports a conflict if the meal's status changes mid-flight
- **GIVEN** an owner's request to add, edit, or delete an item passed the initial editable-status check
- **WHEN** a concurrent operation (e.g. Reanalyze) changes the meal's status before this transaction's own write completes
- **THEN** the system returns HTTP 409, not a generic server error, regardless of whether the change is observed directly or only as the underlying write failing against a since-invalidated read

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
