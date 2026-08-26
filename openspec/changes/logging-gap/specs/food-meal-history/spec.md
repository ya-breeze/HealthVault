## ADDED Requirements

### Requirement: Daily Totals endpoint
The system SHALL expose `GET /api/food/daily-totals?from=YYYY-MM-DD&to=YYYY-MM-DD` (requires
authentication, 401 without it), scoped strictly to the caller's own `FoodMeal` records — no
`?user=` override, matching `GET /api/food/completeness`'s convention.

`from` and `to` SHALL both be required (400 if either is missing or malformed). `to` SHALL be
clamped to yesterday in the caller's stored timezone first (per `food-day-completeness`'s Local Day
boundary) when it names today or a future date; only then SHALL `from > to` (using the
possibly-clamped `to`) be validated, and only then SHALL the range be checked against a 92-day
inclusive-day cap — both violations SHALL return 400. This ordering matches
`GET /api/food/completeness`'s existing validation order exactly, so a `from` naming today or a
future date always fails post-clamp rather than resolving to an inverted or empty range.

On success the system SHALL return a JSON array with exactly one entry per day in the resolved
inclusive range, each shaped `{"date": "YYYY-MM-DD", "calories": number}`, summed only over that
Logged Day's `confirmed`-status `FoodMeal` records. A day with no `confirmed` meals SHALL still
appear in the array, with `calories` `0` — the array SHALL NOT omit days with nothing logged.

This endpoint returns `calories` only. No consumer of this change (the Logging Gap computation)
reads protein/carbs/fat; widen the response shape when a concrete consumer needs those fields
rather than pre-building them here.

#### Scenario: Happy path across several days including a zero-meal day
- **GIVEN** the caller has confirmed meals on 3 of the 5 days in a requested range, and no meals at
  all on the other 2
- **WHEN** they call `GET /api/food/daily-totals` for that range
- **THEN** the response contains exactly 5 entries, one per day, with the 2 empty days showing all
  zeros rather than being omitted

#### Scenario: Unconfirmed meals are excluded from the sum
- **GIVEN** a day has one `confirmed` meal (500 kcal) and one `pending_review` meal (300 kcal)
- **WHEN** the caller requests that day's total
- **THEN** the returned `calories` for that day is 500, not 800

#### Scenario: `to` naming today is clamped before range validation
- **GIVEN** the caller's current Logged Day is 2026-08-26
- **WHEN** they call `GET /api/food/daily-totals?from=2026-08-26&to=2026-08-26`
- **THEN** `to` is clamped to 2026-08-25 first, `from` (2026-08-26) is then after the clamped `to`,
  and the system returns HTTP 400

#### Scenario: Range exceeding the cap is rejected
- **WHEN** the caller requests a range spanning more than 92 days (after any clamping)
- **THEN** the system returns HTTP 400

#### Scenario: Cross-user access reveals nothing
- **WHEN** a caller requests `GET /api/food/daily-totals` with any parameters, including any attempt
  at a `?user=` override
- **THEN** the response never includes another user's meals, even within the same family

#### Scenario: Unauthenticated request rejected
- **WHEN** a request to `GET /api/food/daily-totals` carries no valid token
- **THEN** the system returns HTTP 401
