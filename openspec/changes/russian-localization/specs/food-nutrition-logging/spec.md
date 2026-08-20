## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Item Resolution

The system SHALL expose `PATCH /api/food/meals/{id}/items/{item_id}`, allowing the owner to bind an item to an `fdc_id`, `off_code`, or `custom_food_id`, supply macro values directly, change its weight, or correct its displayed `name`. This applies to any item regardless of its current `macro_source` — an item that is already matched is not locked once bound, since a matched-but-wrong food (e.g. the vision model guessing "dark berries" for what are actually cherries) is exactly as much a review concern as an unresolved one. When the caller supplies a `name` alongside a binding, the system SHALL store it as the item's new displayed name (its Display Name — see `display-language` "Per-User Display Language Setting"), so the review UI reflects what was actually confirmed rather than the original vision-model guess. This endpoint SHALL NOT accept a `canonical_name` field — the Canonical Name is produced only at recognition time and is not user-editable through this endpoint. When the caller supplies direct macro values for an item that carries a persisted estimated profile (`macro_source = estimated`), the system MAY additionally accept a request to save those values as a new `CustomFood` owned by the caller, using the item's current or supplied name, in the same request — so a correction the user has already typed does not require a separate visit to custom food management to become reusable.

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
- **THEN** the system rebinds it, rescales macros from the new profile, and replaces the displayed name (Display Name) with the one supplied — its Canonical Name, if any, is left unchanged

#### Scenario: Binding or rescaling a reference item requires a positive weight

- **WHEN** the owner patches an item with an `fdc_id` and a `weight_grams` of 0 or less, or changes only `weight_grams` to 0 or less on an item already bound to a reference food
- **THEN** the system returns HTTP 400 and does not modify the item

#### Scenario: Weight-only edit rescales an estimated item

- **WHEN** the owner changes only `weight_grams` on an item with `macro_source = estimated`
- **THEN** the system recalculates all 7 macros from that item's persisted per-100g estimated profile and the new weight, the same way it would for a `reference` item bound to a per-100g source

#### Scenario: A correction can be saved as a reusable custom food

- **WHEN** the owner patches an item carrying `macro_source = estimated` with direct macro values and requests that the correction also be saved as a custom food
- **THEN** the system stores the item as `macro_source = manual` with the supplied values, and additionally creates a `CustomFood` owned by the caller with the same Display Name (and Canonical Name, if the item had one) and per-100g values, so a later differently-worded photo of the same dish can match it (see `usda-nutrition-database` "Match Selection and Explicit Non-Match")

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
