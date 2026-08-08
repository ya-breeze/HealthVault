## ADDED Requirements

### Requirement: Owner-Scoped Meal List
The system SHALL expose `GET /api/food/meals`, returning a summary of the authenticated caller's own `FoodMeal` records — of any status — ordered by `logged_at` descending, then `created_at` descending, then `id` descending as a deterministic tie-breaker. Unlike `GET /api/data/food_meal`, this endpoint SHALL scope strictly to the caller's own meals (`user_id`), never other family members', so every meal returned is guaranteed openable via `GET /api/food/meals/{id}`.

Each returned summary SHALL contain only `id`, `name`, `logged_at`, `status`, and `calories` — it SHALL NOT include `photo_path`, `raw_response`, `clarify_log`, or any tenant/ownership metadata, none of which the list view needs.

The endpoint SHALL accept an optional `limit` query parameter (a positive integer, capped at 200; default 50 when absent) and an optional `before` query parameter (an RFC 3339 timestamp), restricting results to meals ordered strictly before that point in the list's own ordering. A `limit` that is present but not a positive integer ≤ 200, or a `before` that is present but not a valid RFC 3339 timestamp, SHALL be rejected with HTTP 400.

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
- **THEN** each entry contains only `id`, `name`, `logged_at`, `status`, and `calories`, and no entry contains `photo_path`, `raw_response`, `clarify_log`, or tenant metadata

#### Scenario: Default and maximum limit
- **WHEN** the caller requests the list with no `limit` parameter
- **THEN** at most 50 meals are returned
- **WHEN** the caller requests `?limit=200`
- **THEN** at most 200 meals are returned

#### Scenario: Invalid limit is rejected
- **WHEN** the caller requests `?limit=-1`, `?limit=0`, `?limit=201`, or `?limit=abc`
- **THEN** the system returns HTTP 400 and does not return a partial or reinterpreted result

#### Scenario: Paging to older meals with `before`
- **GIVEN** the caller has more meals than fit in one page
- **WHEN** they request `?before=<logged_at of the oldest meal in the first page>`
- **THEN** the response contains meals strictly before that point in the list's ordering, letting every meal eventually be reached regardless of how large the history grows

#### Scenario: Invalid before is rejected
- **WHEN** the caller requests `?before=not-a-timestamp`
- **THEN** the system returns HTTP 400

#### Scenario: Unauthenticated list access
- **WHEN** a request without a valid JWT calls `GET /api/food/meals`
- **THEN** the system returns HTTP 401

### Requirement: Meal History Page
The frontend SHALL provide a meal history page reachable from the dashboard, listing the caller's meals from `GET /api/food/meals` with name, logged date/time, status, and calories (blank for a meal whose status is not `confirmed`), each linking to that meal's existing review page (`/food/review/?meal=<id>`). The page SHALL offer a "load older" action that re-queries with `before` set to the oldest meal currently shown, so meals beyond the first page remain reachable.

#### Scenario: History page is reachable from the dashboard
- **WHEN** an authenticated user is on the dashboard
- **THEN** a link to the meal history page is visible

#### Scenario: History row links to the review page
- **WHEN** a user clicks a meal row in the history list
- **THEN** the system navigates to `/food/review/?meal=<that meal's id>`

#### Scenario: Unconfirmed meal shows no calorie total
- **WHEN** the history list includes a meal whose status is `pending_review`, `pending_clarification`, `processing`, or `failed`
- **THEN** that row shows no calorie total, since the meal has none computed yet

#### Scenario: Loading older meals
- **WHEN** the user activates "load older" at the bottom of the list
- **THEN** the system fetches the next page using `before` and appends it, without losing the meals already shown
