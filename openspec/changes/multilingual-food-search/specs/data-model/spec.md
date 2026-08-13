## MODIFIED Requirements

### Requirement: Food logging tables

The system SHALL persist user-authored food logging data in five family-scoped tables. Each SHALL embed the shared tenant model (`id`, `created_at`, `updated_at`, `deleted_at`, `family_id`) and carry a `user_id`. None SHALL carry a `source_payload_id`.

| Table                     | Purpose                                                        | Time anchor    |
|---------------------------|------------------------------------------------------------------|----------------|
| `FoodMeal`                | One logged meal: photo path, status, aggregate macros           | `logged_at`    |
| `FoodItem`                | One food within a meal: reference, weight, 7 scaled macros      | via `meal_id`  |
| `CustomFood`              | A user's own per-100g food profile                              | —              |
| `FoodCalibrationSample`   | A weighed-food ground-truth photo for model benchmarking        | `captured_at`  |
| `FoodSearchTranslation`   | A user's cached free-text-to-USDA-vocabulary query translation  | —              |

`FoodMeal.status` SHALL be one of `processing`, `pending_clarification`, `pending_review`, `confirmed`, `failed`, and `FoodMeal.logged_at` SHALL always be non-zero. Nutrient field names SHALL match the existing `Nutrition` model (`dietary_fiber_grams`, `sodium_grams`) so the two are directly comparable.

`FoodItem` SHALL carry `macro_source`, one of:

| Value       | Meaning                                                     | In the meal aggregate |
|-------------|---------------------------------------------------------------|-----------------------|
| `reference` | Bound to an `fdc_id`, `off_code`, or `custom_food_id`; macros scaled by weight | yes    |
| `manual`    | Macro values supplied directly by the user                    | yes                    |
| `none`      | Unresolved; macros zero, awaiting user resolution              | no                     |

`fdc_id`, `off_code`, and `custom_food_id` SHALL all be nullable, and at most one of the three SHALL be set on a given `FoodItem`. `macro_source` replaces a plain matched/unmatched boolean because "bound to a reference food" and "has usable macros" are different questions, and a manually entered item is the case where they diverge.

`FoodItem` SHALL also carry `preparation`, `state`, and `brand`, each permitted to be empty for unknown. They are persisted rather than merely used in-flight so that a later clarification answer can re-run food lookup without re-analyzing the photo. `brand` additionally determines whether the Open Food Facts index is queried during matching (see `usda-nutrition-database` "Match Selection and Explicit Non-Match").

`CustomFood` SHALL be uniquely indexed on `(user_id, name)`, so that name-based precedence over USDA and Open Food Facts entries has exactly one winner.

`FoodSearchTranslation` SHALL be uniquely indexed on `(user_id, original_query)`, where `original_query` is the trimmed, lowercased free-text search string, so that each user has at most one cached translation per normalized query. It carries no reference-source fields (`fdc_id`, `off_code`, `custom_food_id`) and does not participate in the FoodItem reference-source exclusivity rule below — it caches a translated search term, not a bound reference food.

There SHALL be no unique constraint on `(user_id, logged_at)` for `FoodMeal`, because a user may legitimately log more than one meal at the same recorded time.

#### Scenario: Meal stored with items

- **WHEN** a meal is created with recognized food items
- **THEN** the system SHALL persist the `FoodMeal` row and its `FoodItem` rows with matching `family_id`, the same `user_id` as the parent meal, and a `meal_id` link

#### Scenario: Items are user-scoped, not only family-scoped

- **WHEN** a `FoodItem` row is created
- **THEN** it SHALL carry the owning `user_id`, because the shared tenant model supplies only `family_id` and every ownership rule in this capability is scoped by user

#### Scenario: Two meals at the same logged time

- **WHEN** a user logs two separate meals that carry the same `logged_at` value
- **THEN** both SHALL persist as distinct rows without conflict

#### Scenario: Tenant fields assigned explicitly

- **WHEN** any food logging row is created
- **THEN** the system SHALL assign `id` and `family_id` explicitly, because the shared tenant model provides no `BeforeCreate` hook to populate them

#### Scenario: A FoodItem cannot bind to more than one reference source

- **WHEN** a `FoodItem` is created or updated with more than one of `fdc_id`, `off_code`, and `custom_food_id` set
- **THEN** the system SHALL reject it, since exactly which field is set is what identifies the reference source and more than one set would be ambiguous

#### Scenario: FoodSearchTranslation rows are private to the user who created them

- **WHEN** a `FoodSearchTranslation` row is created
- **THEN** it SHALL carry the owning `user_id`, and a lookup for that cached translation SHALL only match rows owned by the requesting user
