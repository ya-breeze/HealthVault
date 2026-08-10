## MODIFIED Requirements

### Requirement: Owner-Scoped Meal List
The system SHALL expose `GET /api/food/meals`, returning a summary of the authenticated caller's own `FoodMeal` records — of any status — ordered by `logged_at` descending, then `id` descending as a deterministic tie-breaker (`id` alone is a complete, collision-free tie-break, being the primary key — no third field is needed). Unlike `GET /api/data/food_meal`, this endpoint SHALL scope strictly to the caller's own meals (`user_id`), never other family members', so every meal returned is guaranteed openable via `GET /api/food/meals/{id}`.

Each returned summary SHALL contain `id`, `name`, `logged_at`, `status`, `calories`, `protein_grams`, `carbs_grams`, and `fat_grams` — it SHALL NOT include `photo_path`, `raw_response`, `clarify_log`, or any tenant/ownership metadata, none of which the list view needs.

The endpoint SHALL accept an optional `limit` query parameter (a positive integer, capped at 200; default 50 when absent). It SHALL accept an optional `before` query parameter (an RFC 3339 timestamp) and an optional `before_id` query parameter (a UUID). When both are supplied together, they form an exact keyset cursor: results SHALL be those ordered strictly after the row identified by `(before, before_id)` in the list's own `(logged_at, id)` ordering — `(logged_at, id) < (before, before_id)` — which resumes at exactly the next row regardless of how many meals share that `logged_at`, so a tied group can never be split across pages and silently lose part of itself. `before_id` SHALL require `before` to also be present (400 otherwise). `before` supplied without `before_id` SHALL be accepted as a simpler, non-cursor "meals logged before this instant" filter — a plain `logged_at < before` — which is not guaranteed lossless across ties and exists for a caller that just wants a date cutoff rather than exact pagination continuation. A `limit` that is present but not a positive integer ≤ 200, a `before` that is present but not a valid RFC 3339 timestamp, or a `before_id` that is present but not a valid UUID, SHALL be rejected with HTTP 400.

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

### Requirement: Meal History Page
The frontend SHALL provide a meal history page reachable from the dashboard, listing the caller's meals from `GET /api/food/meals` grouped into sections by calendar day (the meal's `logged_at` interpreted in the browser's local time zone), most-recent day first. Each day section SHALL show a header identifying that calendar day, followed by that day's meals in the existing per-meal format (name, logged date/time, status, and calories, blank for a meal whose status is not `confirmed`), each linking to that meal's existing review page (`/food/review/?meal=<id>`). Each day section SHALL also show a daily total — summed calories, protein, carbs, and fat — computed only over that day's `confirmed` meals; a day with no confirmed meals SHALL show a total of zero rather than omitting the total line. The page SHALL offer a "load older" action that re-queries with both `before` and `before_id` set from the oldest meal currently shown, so meals beyond the first page remain reachable via the lossless keyset cursor rather than the plain timestamp filter; meals returned by "load older" SHALL be merged into the correct existing day section or form new day sections as needed, keeping every day's total accurate as more meals load.

#### Scenario: History page is reachable from the dashboard
- **WHEN** an authenticated user is on the dashboard
- **THEN** a link to the meal history page is visible

#### Scenario: History row links to the review page
- **WHEN** a user clicks a meal row in the history list
- **THEN** the system navigates to `/food/review/?meal=<that meal's id>`

#### Scenario: Unconfirmed meal shows no calorie total on its own row
- **WHEN** the history list includes a meal whose status is `pending_review`, `pending_clarification`, `processing`, or `failed`
- **THEN** that row shows no calorie total, since the meal has none computed yet

#### Scenario: Loading older meals
- **WHEN** the user activates "load older" at the bottom of the list
- **THEN** the system fetches the next page using both `before` and `before_id` and appends it, without losing the meals already shown

#### Scenario: Meals are grouped by local calendar day
- **GIVEN** the caller has meals logged on two different calendar days (in the browser's local time zone)
- **WHEN** they view the history page
- **THEN** the meals appear under two separate day headers, most recent day first, each containing only that day's meals

#### Scenario: Daily total sums only confirmed meals
- **GIVEN** a day has one `confirmed` meal and one `pending_review` meal
- **WHEN** the user views that day's section
- **THEN** the displayed daily total (calories, protein, carbs, fat) reflects only the confirmed meal, and the pending meal still appears in the list below it with no calorie total on its own row

#### Scenario: Day with no confirmed meals shows a zero total
- **GIVEN** a day has only meals with status other than `confirmed`
- **WHEN** the user views that day's section
- **THEN** the daily total line is shown with all values at zero, not omitted

#### Scenario: Loading older meals updates day groupings correctly
- **GIVEN** the currently loaded meals end mid-way through a calendar day
- **WHEN** the user activates "load older" and the next page includes more meals from that same day plus meals from an earlier day
- **THEN** the earlier-loaded day's section gains the newly loaded meals (and its total updates to include them), and a new day section appears for the earlier day
