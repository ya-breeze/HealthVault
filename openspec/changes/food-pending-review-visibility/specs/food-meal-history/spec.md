## MODIFIED Requirements

### Requirement: Owner-Scoped Meal List
The system SHALL expose `GET /api/food/meals`, returning a summary of the authenticated caller's own `FoodMeal` records — of any status — ordered by `logged_at` descending, then `id` descending as a deterministic tie-breaker (`id` alone is a complete, collision-free tie-break, being the primary key — no third field is needed). Unlike `GET /api/data/food_meal`, this endpoint SHALL scope strictly to the caller's own meals (`user_id`), never other family members', so every meal returned is guaranteed openable via `GET /api/food/meals/{id}`.

Each returned summary SHALL contain `id`, `name`, `logged_at`, `status`, `calories`, `protein_grams`, `carbs_grams`, and `fat_grams` — it SHALL NOT include `photo_path`, `raw_response`, `clarify_log`, or any tenant/ownership metadata, none of which the list view needs.

The endpoint SHALL accept an optional `limit` query parameter (a positive integer, capped at 200; default 50 when absent). It SHALL accept an optional `before` query parameter (an RFC 3339 timestamp) and an optional `before_id` query parameter (a UUID). When both are supplied together, they form an exact keyset cursor: results SHALL be those ordered strictly after the row identified by `(before, before_id)` in the list's own `(logged_at, id)` ordering — `(logged_at, id) < (before, before_id)` — which resumes at exactly the next row regardless of how many meals share that `logged_at`, so a tied group can never be split across pages and silently lose part of itself. `before_id` SHALL require `before` to also be present (400 otherwise). `before` supplied without `before_id` SHALL be accepted as a simpler, non-cursor "meals logged before this instant" filter — a plain `logged_at < before` — which is not guaranteed lossless across ties and exists for a caller that just wants a date cutoff rather than exact pagination continuation. A `limit` that is present but not a positive integer ≤ 200, a `before` that is present but not a valid RFC 3339 timestamp, or a `before_id` that is present but not a valid UUID, SHALL be rejected with HTTP 400.

The endpoint SHALL accept an optional, repeatable `status` query parameter (e.g. `?status=processing&status=failed`). Each value SHALL be one of `processing`, `pending_clarification`, `pending_review`, `confirmed`, or `failed`. When one or more `status` values are supplied, the response SHALL contain only meals whose `status` matches one of the supplied values, combined with the existing `limit`/`before`/`before_id` paging exactly as when no `status` filter is present. When `status` is omitted, the endpoint SHALL behave exactly as before — meals of any status are returned. A `status` value outside the five recognized values SHALL be rejected with HTTP 400, without returning a partial or reinterpreted result.

#### Scenario: List returns only the caller's own meals
- **GIVEN** two family members each have logged meals
- **WHEN** one of them calls `GET /api/food/meals`
- **THEN** the response contains only that caller's meals, none belonging to the other family member

#### Scenario: Cross-user list access reveals nothing
- **WHEN** a user calls `GET /api/food/meals`
- **THEN** the response never includes another user's meal, even one within the same family, regardless of `limit` or `before`

#### Scenario: List is ordered deterministically
- **GIVEN** the caller has two meals with the same `logged_at`
- **WHEN** they call `GET /api/food/meals` twice with identical parameters
- **THEN** both responses return the two tied meals in the same relative order

#### Scenario: List includes meals of every status
- **GIVEN** the caller has a `processing` meal, a `pending_review` meal, and a `confirmed` meal
- **WHEN** they call `GET /api/food/meals`
- **THEN** all three appear in the response

#### Scenario: Summary excludes internal and detail-only fields
- **WHEN** the caller lists their meals
- **THEN** each entry contains only `id`, `name`, `logged_at`, `status`, `calories`, `protein_grams`, `carbs_grams`, and `fat_grams`, and no entry contains `photo_path`, `raw_response`, `clarify_log`, or tenant metadata

#### Scenario: Default and maximum limit
- **WHEN** the caller requests the list with no `limit` parameter
- **THEN** at most 50 meals are returned
- **WHEN** the caller requests `?limit=200`
- **THEN** at most 200 meals are returned

#### Scenario: Invalid limit is rejected
- **WHEN** the caller requests `?limit=-1`, `?limit=0`, `?limit=201`, or `?limit=abc`
- **THEN** the system returns HTTP 400 and does not return a partial or reinterpreted result

#### Scenario: Paging to older meals with the exact keyset cursor
- **GIVEN** the caller has more meals than fit in one page
- **WHEN** they request `?before=<logged_at of the last meal in the first page>&before_id=<id of that same meal>`
- **THEN** the response contains meals ordered strictly after that row in the list's `(logged_at, id)` ordering, letting every meal eventually be reached regardless of how large the history grows

#### Scenario: A tied-logged_at group larger than the page limit still pages losslessly
- **GIVEN** the caller has more meals sharing one exact `logged_at` than fit in a single page
- **WHEN** they page through with the exact keyset cursor, one page at a time
- **THEN** every meal in that group is returned exactly once, across however many pages it takes, none skipped and none repeated

#### Scenario: before_id without before is rejected
- **WHEN** the caller requests `?before_id=<uuid>` with no `before`
- **THEN** the system returns HTTP 400

#### Scenario: Invalid before or before_id is rejected
- **WHEN** the caller requests `?before=not-a-timestamp`, or a syntactically invalid `?before_id=` alongside a valid `before`
- **THEN** the system returns HTTP 400

#### Scenario: Unauthenticated list access
- **WHEN** a request without a valid JWT calls `GET /api/food/meals`
- **THEN** the system returns HTTP 401

#### Scenario: Macro fields are present alongside calories
- **WHEN** the caller lists their meals
- **THEN** each entry's `protein_grams`, `carbs_grams`, and `fat_grams` reflect that meal's own aggregate values (zero for a meal that has none computed yet, same as `calories` today)

#### Scenario: Filtering by a single status
- **GIVEN** the caller has meals in `processing`, `pending_review`, and `confirmed` status
- **WHEN** they call `GET /api/food/meals?status=pending_review`
- **THEN** the response contains only the `pending_review` meal

#### Scenario: Filtering by multiple statuses
- **GIVEN** the caller has meals in `processing`, `pending_review`, `confirmed`, and `failed` status
- **WHEN** they call `GET /api/food/meals?status=processing&status=failed`
- **THEN** the response contains only the `processing` and `failed` meals

#### Scenario: Status filter combines with paging
- **GIVEN** the caller has more `pending_review` meals than fit in one page
- **WHEN** they page through with `?status=pending_review` plus the keyset cursor
- **THEN** every page contains only `pending_review` meals, and paging reaches all of them exactly once

#### Scenario: Invalid status value is rejected
- **WHEN** the caller requests `?status=bogus`
- **THEN** the system returns HTTP 400 and does not return a partial or reinterpreted result

### Requirement: Needs-Attention Count
The system SHALL expose `GET /api/food/meals/needs-attention-count`, returning a count of the authenticated caller's own meals whose status is `processing`, `pending_clarification`, `pending_review`, or `failed` — the set of meals with no finished, confirmed nutrition totals yet. The response SHALL be a JSON object `{"count": <integer>}`. The endpoint SHALL take no query parameters; the status set counted is fixed. As with the meal list, this endpoint SHALL scope strictly to the caller's own meals (`user_id`).

#### Scenario: Count reflects only meals needing attention
- **GIVEN** the caller has one `processing`, one `pending_review`, one `failed`, and two `confirmed` meals
- **WHEN** they call `GET /api/food/meals/needs-attention-count`
- **THEN** the response is `{"count": 3}`

#### Scenario: Zero when nothing needs attention
- **GIVEN** all of the caller's meals are `confirmed`, or the caller has no meals
- **WHEN** they call `GET /api/food/meals/needs-attention-count`
- **THEN** the response is `{"count": 0}`

#### Scenario: Count is scoped to the caller
- **GIVEN** two family members each have meals needing attention
- **WHEN** one of them calls `GET /api/food/meals/needs-attention-count`
- **THEN** the returned count reflects only that caller's own meals

#### Scenario: Unauthenticated access
- **WHEN** a request without a valid JWT calls `GET /api/food/meals/needs-attention-count`
- **THEN** the system returns HTTP 401
