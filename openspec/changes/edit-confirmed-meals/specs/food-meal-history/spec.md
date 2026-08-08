## ADDED Requirements

### Requirement: Owner-Scoped Meal List
The system SHALL expose `GET /api/food/meals`, returning the authenticated caller's own `FoodMeal` records — of any status — ordered by `logged_at` descending. Unlike `GET /api/data/food_meal`, this endpoint SHALL scope strictly to the caller's own meals (`user_id`), never other family members', so every meal returned is guaranteed openable via `GET /api/food/meals/{id}`. The endpoint SHALL accept an optional `limit` query parameter, defaulting to 50 and capped at 200.

#### Scenario: List returns only the caller's own meals
- **GIVEN** two family members each have logged meals
- **WHEN** one of them calls `GET /api/food/meals`
- **THEN** the response contains only that caller's meals, none belonging to the other family member

#### Scenario: List is ordered most-recent-first
- **WHEN** the caller has meals logged at several different times
- **THEN** the response orders them by `logged_at` descending

#### Scenario: List includes meals of every status
- **GIVEN** the caller has a `processing` meal, a `pending_review` meal, and a `confirmed` meal
- **WHEN** they call `GET /api/food/meals`
- **THEN** all three appear in the response

#### Scenario: Default and maximum limit
- **WHEN** the caller requests the list with no `limit` parameter
- **THEN** at most 50 meals are returned
- **WHEN** the caller requests `?limit=500`
- **THEN** at most 200 meals are returned

#### Scenario: Unauthenticated list access
- **WHEN** a request without a valid JWT calls `GET /api/food/meals`
- **THEN** the system returns HTTP 401

### Requirement: Meal History Page
The frontend SHALL provide a meal history page reachable from the dashboard, listing the caller's meals from `GET /api/food/meals` with name, logged date/time, status, and calories (blank for a meal whose status is not `confirmed`), each linking to that meal's existing review page (`/food/review/?meal=<id>`).

#### Scenario: History page is reachable from the dashboard
- **WHEN** an authenticated user is on the dashboard
- **THEN** a link to the meal history page is visible

#### Scenario: History row links to the review page
- **WHEN** a user clicks a meal row in the history list
- **THEN** the system navigates to `/food/review/?meal=<that meal's id>`

#### Scenario: Unconfirmed meal shows no calorie total
- **WHEN** the history list includes a meal whose status is `pending_review`, `pending_clarification`, `processing`, or `failed`
- **THEN** that row shows no calorie total, since the meal has none computed yet
