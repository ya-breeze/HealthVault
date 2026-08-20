<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# food-nutrition-logging Specification

## Purpose
TBD - created by archiving change photo-food-nutrition-logging. Update Purpose after archive.
## Requirements
### Requirement: Macro Calculation and Portion Scaling

The system SHALL compute 7 primary macros (Calories, Protein, Carbs, Fat, Sugar, Sodium, Dietary Fiber) for each matched item by scaling its per-100g nutritional profile by its gram weight. An item with `macro_source = estimated` uses the same 7-macro scaling, applied to the per-100g profile Recognize produced and persisted for that item rather than a bound reference profile (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items").

#### Scenario: Gram weight scaling
- **WHEN** a food item of 180 grams is matched to a USDA profile with 165 kcal per 100g
- **THEN** the system calculates the item calories as 297 kcal (165 * 1.8) and scales all other 6 macros proportionally

#### Scenario: Weight adjusted after matching
- **WHEN** the user changes an item's weight from 180g to 200g before confirming
- **THEN** the system recalculates all 7 macros for that item from the same per-100g profile and updates the meal aggregate

#### Scenario: Unresolved item contributes no macros
- **WHEN** an item is bound to no USDA, Open Food Facts, or custom food, has no user-supplied macros, and has no usable persisted estimated profile
- **THEN** the system stores it with `macro_source = none` and zeroed macros, and excludes it from the meal aggregate rather than estimating values

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

### Requirement: Meal Logged Time
Every meal SHALL carry a non-zero `logged_at`, defaulting to the time the upload was received. `POST /api/food/meals/manual` SHALL accept an explicit `logged_at`, and `PUT /api/food/meals/{id}/confirm` SHALL allow correcting it, so a user can record a meal eaten earlier.

#### Scenario: Logged time defaults to upload time
- **WHEN** a user uploads a meal photo without specifying a time
- **THEN** the system sets `logged_at` to the time the upload was received

#### Scenario: Backdated manual meal
- **WHEN** a user creates a manual meal with an explicit `logged_at` of the previous day
- **THEN** the system stores that time, and the meal is returned by a query whose range covers it

### Requirement: Item Resolution

The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id`, `off_code`, or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name (its Display Name — see `display-language` "Per-User Display Language Setting"), so the review UI reflects what was actually confirmed rather than the original vision-model guess. This endpoint SHALL NOT accept a `canonical_name` field — the Canonical Name is produced only at recognition time and is not user-editable through this endpoint. When the caller supplies direct macro values for an item that carries a persisted estimated profile (`macro_source = estimated`), the system MAY additionally accept a request to save those values as a new `CustomFood` owned by the caller, using the item's current or supplied name, in the same request — so a correction the user has already typed does not require a separate visit to custom food management to become reusable.

A hand-supplied macro correction that also changes the item's `name` SHALL clear the item's Canonical Name, because such a request replaces the item's identity outright and the recognized English gloss no longer describes it — carrying it forward would pair the new Display Name with a stale, unrelated one, including onto any `CustomFood` created from the same request. The name SHALL be treated as changed only if it actually differs from the item's current name, compared case-insensitively and ignoring surrounding whitespace: a review UI that pre-fills its correction form with the current name and echoes it back unchanged is correcting macros, not renaming, and a pure capitalization fix is not an identity change. Rebinding an item to a different reference food alongside a changed `name` SHALL clear the Canonical Name identically, for the same reason and under the same comparison (see "Correct an already-matched item"). The rule SHALL turn on whether the request changes the item's identity *and* its name, not on which mechanism changed the identity: correcting macros by hand and selecting a different reference food both replace what the item is, and a stale English gloss beneath the new Display Name is equally wrong in either case.

A change that leaves the item's name as it was SHALL leave the Canonical Name intact even when it rebinds — re-matching an item to a better reference row does not disturb the Display Name, so its recognized gloss still describes it. So SHALL a bare rename with no other field (see "Renaming an item does not require touching its macros"), which relabels the same food rather than replacing it; a Canonical Name records what recognition originally identified, and that remains true of a relabelled item.

Item resolution SHALL be permitted while the owning meal's status is `pending_review` or `confirmed`. It SHALL be rejected with HTTP 409 while the owning meal's status is `processing`, `pending_clarification`, or `failed`, since those states have no stable, reviewable item set yet.

Whenever a request causes the item to be scaled from a per-100g profile — either by supplying an `fdc_id`/`off_code`/`custom_food_id`, or by changing `weight_grams` alone on an item already bound to a reference food or already carrying a persisted estimated profile (`macro_source = estimated`) — the weight used for that scaling (the supplied `weight_grams`, or the item's existing weight if none is supplied) SHALL be greater than zero. A request that would scale a reference or estimated profile by a zero or negative weight SHALL be rejected with HTTP 400 and SHALL NOT modify the item.

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
- **THEN** the system rebinds it, rescales macros from the new profile, replaces the displayed name (Display Name) with the one supplied, and clears its Canonical Name

#### Scenario: Rebinding an item without renaming it keeps its Canonical Name

- **WHEN** the owner patches an item that already has `macro_source = reference` with a different `fdc_id` and either no `name` or the name it already had
- **THEN** the system rebinds it and rescales macros from the new profile, and its Canonical Name, if any, is left unchanged

#### Scenario: Binding or rescaling a reference item requires a positive weight

- **WHEN** the owner patches an item with an `fdc_id` and a `weight_grams` of 0 or less, or changes only `weight_grams` to 0 or less on an item already bound to a reference food
- **THEN** the system returns HTTP 400 and does not modify the item

#### Scenario: Weight-only edit rescales an estimated item

- **WHEN** the owner changes only `weight_grams` on an item with `macro_source = estimated`
- **THEN** the system recalculates all 7 macros from that item's persisted per-100g estimated profile and the new weight, the same way it would for a `reference` item bound to a per-100g source

#### Scenario: A correction can be saved as a reusable custom food

- **WHEN** the owner patches an item carrying `macro_source = estimated` with direct macro values and requests that the correction also be saved as a custom food
- **THEN** the system stores the item as `macro_source = manual` with the supplied values, and additionally creates a `CustomFood` owned by the caller with the same Display Name (and Canonical Name, if the item had one) and per-100g values, so a later differently-worded photo of the same dish can match it (see `usda-nutrition-database` "Match Selection and Explicit Non-Match")

#### Scenario: A hand-corrected item that is also renamed loses its Canonical Name

- **WHEN** the owner patches an item that carries a Canonical Name with direct macro values and a `name` that differs from the item's current name
- **THEN** the system stores the correction and clears the item's Canonical Name, so Expert Mode does not show the new Display Name paired with the English gloss of the food it used to be — and any `CustomFood` created from the same request is likewise created without one

#### Scenario: A hand-corrected item whose name is unchanged keeps its Canonical Name

- **WHEN** the owner patches an item that carries a Canonical Name with direct macro values and a `name` equal to the item's current name apart from capitalization or surrounding whitespace — the case a review UI produces when it pre-fills the current name and the user edits only macros
- **THEN** the system stores the correction and leaves the Canonical Name in place

#### Scenario: Renaming an item does not require touching its macros

- **WHEN** the owner patches an item with only a `name` and no `fdc_id`, `off_code`, `custom_food_id`, `manual`, or `weight_grams`
- **THEN** the system updates the Display Name and returns 200 without changing `macro_source`, any stored macro value, or the item's Canonical Name

#### Scenario: A blank name alone is not something to update

- **WHEN** the owner patches an item with only an empty or whitespace-only `name` and no other field
- **THEN** the system returns HTTP 400 rather than accepting a request with nothing meaningful to apply

#### Scenario: A canonical_name field on the request is rejected

- **WHEN** the owner patches an item with a `canonical_name` field present in the request body
- **THEN** the system returns HTTP 400 and does not modify the item, since Canonical Name is not editable through this endpoint

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

### Requirement: Meal Deletion Removes Items and Photo
Deleting a meal SHALL remove its `FoodItem` rows and its stored photo file in the same operation, leaving no orphaned rows or files.

#### Scenario: Delete a meal with items and a photo
- **WHEN** the owner deletes a meal that has items and a stored photo
- **THEN** the system removes the meal row, all of its `FoodItem` rows, and the photo file from disk

#### Scenario: Delete a meal whose photo file is already missing
- **WHEN** the owner deletes a meal whose photo file is absent from disk
- **THEN** the deletion still succeeds and removes the meal and its items

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

This same transaction SHALL re-verify the meal's existence and editable status immediately before the item write, not rely solely on the check made when the request was first received — closing the gap between that earlier check and this write. If the meal no longer exists (e.g. deleted through the generic meal-delete endpoint in that gap), the system SHALL return HTTP 404. The underlying write can also fail outright because the transaction's own read snapshot is no longer current — this does not by itself prove the meal became ineligible, since any unrelated write elsewhere in the database can trigger the identical condition. The system SHALL distinguish the two by re-reading the meal's actual current status once such a failure has rolled back: still eligible SHALL retry the whole transaction (bounded, with a fresh read snapshot) rather than report a false conflict; genuinely no longer eligible, or gone, SHALL return HTTP 409 or 404 respectively. If the meal remains eligible on every re-check yet the write still hits the same stale-snapshot condition on every bounded retry attempt, the system SHALL reject it with HTTP 409 rather than retry indefinitely or fall through to a generic server error — matching the same "stale after every retry is exhausted" contract as the Item Resolution requirement's per-item conflict, since sustained staleness under an otherwise-eligible meal usually means sustained conflicting writes to the very row being changed. Neither case SHALL be reported as a generic server error, and an unrelated write SHALL NOT be reported as this meal being ineligible.

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

#### Scenario: An item write reports a conflict if the meal's status genuinely changes mid-flight
- **GIVEN** an owner's request to add, edit, or delete an item passed the initial editable-status check
- **WHEN** a concurrent operation (e.g. Reanalyze) changes the meal's status before this transaction's own write completes
- **THEN** the system returns HTTP 409, not a generic server error, regardless of whether the change is observed directly or only as the underlying write failing against a since-invalidated read

#### Scenario: An unrelated concurrent write does not falsely block an item write
- **GIVEN** an owner's request to add, edit, or delete an item on a meal that remains eligible throughout
- **WHEN** a write to a *different* meal (or any other unrelated database activity) happens to land in the narrow window between this transaction's read and its own write, causing the same stale-snapshot condition a genuine status change would
- **THEN** this request succeeds normally — the system re-reads this meal's actual status, finds it still eligible, and retries rather than reporting a false conflict

#### Scenario: Bounded-retry exhaustion under an eligible meal is a conflict, not a generic error
- **GIVEN** an owner's request to add, edit, or delete an item on a meal that remains eligible on every re-check
- **WHEN** the transaction's write hits the same stale-snapshot condition on every one of its bounded retry attempts, so the retries are exhausted without ever committing
- **THEN** the system returns HTTP 409, not a generic server error, and applies none of the request's change

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

### Requirement: Item Resolution Mode Controls Are Distinctly Presented

In every item-resolution UI that offers both a search-by-database mode and a manual-macros mode (the meal review page's "Add item" panel and the manual meal-entry form), the mode-selector control(s) SHALL be visually and textually distinguishable from the search-submit control, so a user cannot mistake one for the other. This holds regardless of which mode is active by default, and regardless of which mode-selector control happens to be selected.

Specifically: the mode-selector control(s) SHALL NOT share identical visible text with the search-submit control, and SHALL NOT be styled as the single most visually prominent (solid-filled, colored) control in the panel when the search-submit control is also present — the search-submit control SHALL be the one control styled that way, since it is the only control that triggers a search request.

#### Scenario: Search mode is already selected when the panel opens

- **WHEN** an item-resolution panel opens with its search mode already active
- **THEN** the mode-selector control and the search-submit control are not labeled with identical text, and the search-submit control — not the mode-selector control — is the panel's visually primary (solid-filled) control

#### Scenario: Manual meal-entry search path is reachable and distinguishable

- **WHEN** a user adds an item row on the manual meal-entry form with its default (search) source selected
- **THEN** the row's mode-selector tab and its search-submit button are visually distinguishable, and clicking the search-submit button performs a search

### Requirement: Food Item and Custom Food Carry a Canonical Name

`FoodItem` and `CustomFood` records SHALL each carry an optional Canonical Name field alongside
their existing (Display) name, persisted at creation time from recognition (see
`food-photo-recognition` "Food Recognition and Clarification Questions") or, for a `CustomFood`
created from a correction, copied from the originating item's Canonical Name if it had one. It
SHALL be included in any API response that already includes the record's name, and SHALL be empty
for records created before this change or whose Display Language was English at creation time.

#### Scenario: A custom food created from a non-English recognition keeps its Canonical Name

- **WHEN** a user saves a correction as a new `CustomFood` from an item that carries a Canonical
  Name (see "A correction can be saved as a reusable custom food")
- **THEN** the new `CustomFood` record SHALL store that same Canonical Name

#### Scenario: A pre-existing item has no Canonical Name

- **WHEN** a `FoodItem` or `CustomFood` created before this change, or created while the user's
  Display Language was English, is read
- **THEN** its Canonical Name SHALL be empty, and callers SHALL treat that as "not recorded," not
  as a data error

