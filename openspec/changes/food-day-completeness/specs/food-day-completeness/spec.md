## ADDED Requirements

### Requirement: Eating Occasion collapsing
The system SHALL derive, for any set of a user's `FoodMeal` records on a single Logged Day, a
count of Eating Occasions by sorting the records' `logged_at` timestamps ascending and starting a
new occasion whenever the gap to the immediately preceding timestamp exceeds 10 minutes; records
within 10 minutes of the preceding one SHALL merge into the current occasion. `FoodMeal` records of
every `status` SHALL be included in this collapsing — a meal that failed recognition or is still
processing still represents a logging attempt on that day.

#### Scenario: Two rows minutes apart collapse to one occasion
- **GIVEN** a user logged meals at 14:39 and 14:42 on the same Logged Day
- **WHEN** the system computes that day's Eating Occasion count
- **THEN** those two rows count as a single Eating Occasion

#### Scenario: Rows more than 10 minutes apart are separate occasions
- **GIVEN** a user logged meals at 09:10 and 14:39 on the same Logged Day
- **WHEN** the system computes that day's Eating Occasion count
- **THEN** those two rows count as two separate Eating Occasions

#### Scenario: Occasion count is independent of meal status
- **GIVEN** a user has one `confirmed` meal and one `failed` meal logged 3 minutes apart on the
  same Logged Day
- **THEN** they collapse into a single Eating Occasion, the same as if both were `confirmed`

### Requirement: Local Day boundary
The system SHALL compute a Logged Day for any timestamp as that timestamp converted into the
user's stored `timezone` setting (see `user-settings`) and formatted `YYYY-MM-DD`. A `timezone`
value that is absent, empty, or not a valid IANA zone name SHALL be treated as `UTC` rather than
producing an error. "Today" for a given user SHALL be the current instant converted the same way.

#### Scenario: Missing timezone defaults to UTC
- **GIVEN** a user has no `timezone` key in their settings
- **WHEN** the system computes their Logged Day for a given timestamp
- **THEN** the result is that timestamp's UTC calendar date

#### Scenario: Invalid timezone falls back to UTC
- **GIVEN** a user's `timezone` setting is a string that is not a valid IANA zone name
- **WHEN** the system computes their Logged Day for a given timestamp
- **THEN** the result is that timestamp's UTC calendar date, and no error is raised

#### Scenario: Valid timezone shifts the day boundary
- **GIVEN** a user's `timezone` is `America/Los_Angeles` and they logged a meal at `2026-08-21T02:00:00Z`
- **WHEN** the system computes that meal's Logged Day
- **THEN** the result is `2026-08-20`, not `2026-08-21`

### Requirement: Day Completeness states
For any of a user's completed Logged Days (a Logged Day strictly before that user's current
Logged Day — the current, in-progress day SHALL NEVER be assigned a state), the system SHALL
compute exactly one of four states from that day's Eating Occasion count and the user's
`usual_meals_per_day` setting (a positive integer; absent or invalid SHALL default to 3):

- **Incomplete** — 0 occasions. Computed silently; never surfaced as something requiring user
  input.
- **Unconfirmed** — 1 or more occasions, fewer than `usual_meals_per_day`, and the user has not
  confirmed the day complete.
- **Confirmed Complete** — 1 or more occasions, fewer than `usual_meals_per_day`, and the user has
  confirmed the day complete (see "Day confirmation").
- **Complete** — occasions ≥ `usual_meals_per_day`. Automatic; requires no user action and SHALL
  NOT be alterable by the user in this capability.

`usual_meals_per_day` SHALL be read fresh at computation time, not fixed at any earlier point — a
change to the setting SHALL immediately change the computed state of every past day it affects.

#### Scenario: Zero-occasion day is silently Incomplete
- **GIVEN** a user's completed Logged Day has 0 Eating Occasions
- **WHEN** the system computes that day's state
- **THEN** the result is `incomplete`, and no confirmation action is offered for it anywhere

#### Scenario: Meeting the threshold is automatic
- **GIVEN** a user's `usual_meals_per_day` is 3 and a completed Logged Day has 3 Eating Occasions
- **WHEN** the system computes that day's state
- **THEN** the result is `complete`, with no confirmation required

#### Scenario: Below threshold, unconfirmed
- **GIVEN** a user's `usual_meals_per_day` is 3, a completed Logged Day has 2 Eating Occasions, and
  the user has not confirmed that day
- **WHEN** the system computes that day's state
- **THEN** the result is `unconfirmed`

#### Scenario: Below threshold, confirmed
- **GIVEN** the same day as above, but the user has confirmed it (see "Day confirmation")
- **WHEN** the system computes that day's state
- **THEN** the result is `confirmed_complete`

#### Scenario: Today has no state
- **GIVEN** a user's current Logged Day (per their stored timezone) has fewer Eating Occasions
  than `usual_meals_per_day`
- **WHEN** the system computes completeness for a range including today
- **THEN** today is excluded entirely from the result — it is never `unconfirmed` or `incomplete`

#### Scenario: Changing the threshold re-evaluates past days
- **GIVEN** a completed Logged Day has 3 Eating Occasions and the user's `usual_meals_per_day` is 4
  (so the day is currently `unconfirmed`, not yet `complete`)
- **WHEN** the user changes `usual_meals_per_day` to 3
- **THEN** that same day is now computed as `complete`, without any new confirmation action

#### Scenario: An automatically-Complete day cannot be downgraded
- **GIVEN** a completed Logged Day is `complete` (occasions ≥ threshold)
- **WHEN** the system renders or exposes that day's state
- **THEN** no action exists to mark it anything other than `complete` — this capability offers no
  mechanism to override an automatic pass

### Requirement: Day confirmation storage and lifecycle
The system SHALL persist a user's assertion that an Unconfirmed day is complete in a dedicated
table (one row per confirmed `(user, Logged Day)` pair), separate from the health-metric-type
registry described in `data-model` — this is a flag on a date, not a time-series measurement.
Confirming a day already `confirmed_complete` SHALL be idempotent and SHALL NOT create a duplicate
row or change the stored confirmation. A user's confirmation SHALL persist across later edits to
that day's meals (e.g. adding a forgotten meal after confirming) — the confirmation SHALL NOT be
cleared automatically by any such edit, since the day only became more complete, not less. The
same mechanism used to confirm a day SHALL also be able to retract that confirmation, returning the
day to whatever state its current occasion count computes to (`unconfirmed`, or `complete` if
enough occasions now exist).

#### Scenario: Confirming an Unconfirmed day
- **GIVEN** a Logged Day is currently `unconfirmed`
- **WHEN** the user confirms it
- **THEN** the day's state becomes `confirmed_complete` and remains so on subsequent reads

#### Scenario: Confirmation survives a later meal edit
- **GIVEN** a user confirmed a day that had 1 Eating Occasion
- **WHEN** they later add a second meal to that same day, still below `usual_meals_per_day`
- **THEN** the day remains `confirmed_complete`, not reverted to `unconfirmed`

#### Scenario: Retracting a confirmation
- **GIVEN** a Logged Day is `confirmed_complete`
- **WHEN** the user retracts the confirmation
- **THEN** the day's state becomes `unconfirmed` (or `complete`, if its occasion count now meets
  the threshold), and the stored confirmation row no longer exists

#### Scenario: Cannot confirm a zero-occasion day
- **WHEN** a confirmation is attempted against a Logged Day with 0 Eating Occasions
- **THEN** the system SHALL reject it — there is nothing to confirm, and the day stays `incomplete`

#### Scenario: Confirming an already-automatic day is rejected
- **WHEN** a confirmation is attempted against a Logged Day that is already `complete`
  (occasions ≥ threshold)
- **THEN** the system SHALL reject it, since that day requires no confirmation and this capability
  offers no override of an automatic pass

### Requirement: Completeness range query API
The system SHALL expose `GET /api/food/completeness`, requiring authentication (401 if absent),
accepting required `from` and `to` query parameters (each `YYYY-MM-DD`). A missing or malformed
`from`/`to`, or `from` after `to`, SHALL return HTTP 400. A range spanning more than 92 days SHALL
return HTTP 400. A `to` naming the caller's current Logged Day or later SHALL be silently clamped
to the day immediately before it — the endpoint never returns an entry for today or a future day.
The response SHALL be a JSON array with exactly one entry per Logged Day in the resolved inclusive
range, each `{"date": "YYYY-MM-DD", "occasion_count": <integer>, "state": "complete" |
"confirmed_complete" | "unconfirmed" | "incomplete"}`, ordered ascending by date — including days
whose state is `incomplete`, so a caller can see gaps as well as logged days. The endpoint SHALL
scope strictly to the authenticated caller's own data; it SHALL NOT accept a `?user=` override.

#### Scenario: Range includes every day, including gaps
- **GIVEN** a caller has meals on some days in a requested range and none on others
- **WHEN** they call `GET /api/food/completeness?from=<range start>&to=<range end>`
- **THEN** the response contains one entry per calendar day in that range, with `incomplete` entries
  for the days with no meals

#### Scenario: to clamps to yesterday
- **WHEN** a caller requests `to` equal to their current Logged Day (or a later date)
- **THEN** the response covers only up to the day before their current Logged Day, with no error

#### Scenario: Malformed range is rejected
- **WHEN** a caller requests a missing `from`/`to`, an unparseable date, `from` after `to`, or a
  range spanning more than 92 days
- **THEN** the system returns HTTP 400 and returns no data

#### Scenario: Scoped to the caller only
- **GIVEN** two family members each have food logs
- **WHEN** one of them calls `GET /api/food/completeness`
- **THEN** the response reflects only their own Logged Days, with no `?user=` parameter honored

#### Scenario: Unauthenticated access
- **WHEN** a request without a valid JWT calls `GET /api/food/completeness`
- **THEN** the system returns HTTP 401

### Requirement: Day confirmation API
The system SHALL expose `POST /api/food/completeness/{date}/confirm` and
`DELETE /api/food/completeness/{date}/confirm`, both requiring authentication (401 if absent) and
scoped strictly to the caller (no `?user=` override — this is a personal assertion, not shared
family data). `{date}` SHALL be `YYYY-MM-DD`; a malformed value, or a value naming the caller's
current Logged Day or a future day, SHALL return HTTP 400.

`POST` SHALL confirm the named day if it is currently `unconfirmed`, returning HTTP 201 with
`{"date", "state": "confirmed_complete", "confirmed_at"}`. If the day is already
`confirmed_complete`, `POST` SHALL be idempotent and return HTTP 200 without creating a duplicate
record. If the day is `incomplete` (0 occasions) or already `complete` (automatic), `POST` SHALL
return HTTP 400 — neither is confirmable.

`DELETE` SHALL remove any existing confirmation for the named day and return HTTP 204, whether or
not a confirmation existed beforehand (idempotent — retracting an already-unconfirmed day is not an
error).

#### Scenario: Confirming an eligible day
- **GIVEN** the caller's day `2026-08-17` is currently `unconfirmed`
- **WHEN** they `POST /api/food/completeness/2026-08-17/confirm`
- **THEN** the response is HTTP 201 and the day's state becomes `confirmed_complete`

#### Scenario: Re-confirming is idempotent
- **GIVEN** the caller's day `2026-08-17` is already `confirmed_complete`
- **WHEN** they `POST /api/food/completeness/2026-08-17/confirm` again
- **THEN** the response is HTTP 200 and no duplicate confirmation is created

#### Scenario: Cannot confirm zero-occasion or automatic days via the API
- **WHEN** the caller `POST`s confirm against a day with 0 occasions, or a day already `complete`
- **THEN** the system returns HTTP 400

#### Scenario: Retracting a confirmation
- **GIVEN** the caller's day `2026-08-17` is `confirmed_complete`
- **WHEN** they `DELETE /api/food/completeness/2026-08-17/confirm`
- **THEN** the response is HTTP 204 and the day's state reverts to whatever its occasion count
  computes to

#### Scenario: Retracting a non-existent confirmation is not an error
- **GIVEN** the caller's day `2026-08-17` has no confirmation on file
- **WHEN** they `DELETE /api/food/completeness/2026-08-17/confirm`
- **THEN** the response is HTTP 204

#### Scenario: Cannot confirm or retract today or a future day
- **WHEN** the caller targets their current Logged Day or a later date with either `POST` or
  `DELETE`
- **THEN** the system returns HTTP 400

#### Scenario: Unauthenticated access
- **WHEN** a request without a valid JWT calls either endpoint
- **THEN** the system returns HTTP 401

### Requirement: Downstream coverage contract
Any feature that aggregates a metric over a rolling 7-Logged-Day window using food-log data (e.g.
a Healthiness Label, an adaptive-TDEE computation) SHALL count only `complete` and
`confirmed_complete` days as valid for that computation, excluding `unconfirmed` and `incomplete`
days entirely. If fewer than 3 of the most recent 7 Logged Days are valid, such a feature SHALL
state that there is not enough data rather than compute and display a value derived from the fewer-
than-3 valid days. A feature satisfying this contract SHALL be able to state its actual coverage
(e.g. "3 of the last 7 days") rather than presenting a computed value as if the window were fully
logged.

#### Scenario: Enough coverage to compute
- **GIVEN** a rolling-7-day window has 4 `complete`/`confirmed_complete` days and 3
  `unconfirmed`/`incomplete` days
- **WHEN** a downstream feature computes its metric over that window
- **THEN** it uses only the 4 valid days and MAY state coverage as "4 of the last 7 days"

#### Scenario: Below the minimum coverage
- **GIVEN** a rolling-7-day window has only 2 `complete`/`confirmed_complete` days
- **WHEN** a downstream feature would compute its metric over that window
- **THEN** it SHALL report that there is not enough data instead of showing a value
