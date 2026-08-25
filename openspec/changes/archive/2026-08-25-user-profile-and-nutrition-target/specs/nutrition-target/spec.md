## ADDED Requirements

### Requirement: Step-based activity-level inference
The system SHALL infer a user's activity tier from a trailing 28-calendar-day window of their
`steps` metric, ending the day before the current day (the current day is always excluded — it has
not yet ended, so its count is inherently partial).

Two exclusion rules apply within that window:

1. A day with zero step records SHALL be excluded from the average and the valid-day count
   regardless of where it falls in the window — trailing edge or interior. A day with no records is
   missing data, not a measured zero.
2. A day with fewer than 500 total steps (but at least one record) SHALL be discarded only as part
   of a contiguous run at the trailing (most recent) edge: walking backward from the most recent day
   in the window, such days SHALL be discarded until the first day that has 500 or more total steps
   is reached; trimming SHALL stop there, and no day older than that stopping point SHALL be
   discarded on this basis regardless of its step count.

The remaining days SHALL be averaged directly — the window SHALL NOT be extended with additional
older days to replace ones that were excluded.

The resulting average SHALL map to one of 5 tiers:

| Tier | Trailing 28-day avg steps/day | Multiplier |
|---|---|---|
| Sedentary | < 5,000 | 1.2 |
| Lightly active | 5,000 – 7,499 | 1.375 |
| Moderately active | 7,500 – 9,999 | 1.55 |
| Very active | 10,000 – 12,499 | 1.725 |
| Extra active | ≥ 12,500 | 1.9 |

If fewer than 7 days remain after exclusion, and the user has no `activity_override` set (see
`user-profile`), the system SHALL treat the activity tier as unavailable rather than inferring one
from fewer than 7 days or defaulting to a fixed tier.

#### Scenario: Trailing sync gap and partial day are trimmed, not averaged in

- **WHEN** a user's most recent 2 days have zero step records and the 3rd most recent day has 96
  steps, with normal step counts (multiple hundred to several thousand per day) on all earlier days
  in the 28-day window
- **THEN** the system SHALL discard those 3 days from the average and compute the tier from the
  remaining days only

#### Scenario: A genuinely low-activity day inside the window is kept

- **WHEN** a day 10 days ago has only 300 real steps (a rest day) but every day more recent than it,
  including yesterday, has a normal step count at or above 500
- **THEN** the system SHALL include that 300-step day in the average — trimming does not resume once
  a day at or above the 500-step floor has been reached scanning backward from the most recent day

#### Scenario: An interior zero-record day is excluded, not averaged in as zero

- **WHEN** a day 10 days ago has zero step records (a mid-window sync gap) but every other day in the
  28-day window, including yesterday, has at least 500 steps
- **THEN** the system SHALL exclude that day from both the average and the valid-day count — it SHALL
  NOT be averaged in as a measured 0, even though it is not part of the trailing edge

#### Scenario: Fewer than 7 valid days and no override

- **WHEN** exclusion leaves fewer than 7 valid days in the window and the user has no
  `activity_override` set
- **THEN** the system SHALL treat the activity tier as unavailable for that user

### Requirement: Nutrition Target computation
The system SHALL compute, on every request and without storing the result, a daily Nutrition Target
of calories, protein grams, carbs grams, and fat grams, using:

- BMR via Mifflin-St Jeor: `10 * measured_weight_kg + 6.25 * height_cm - 5 * age_years + sex_term`,
  where `sex_term` is `+5` for `"male"` and `-161` for `"female"`, `measured_weight_kg` is the
  user's latest `weight` record, `height_cm` is the user's latest `height` record converted from
  the stored metres, and `age_years` is the user's calendar age derived from `birthdate`.
- `calories = BMR * activity_multiplier`, using the tier from the step-based inference or the
  user's `activity_override` if set (see `user-profile`, including the explicit override-value to
  multiplier mapping table there).
- `protein_grams = 1.6 * goal_weight_kg`, where `goal_weight_kg` is the user's latest `weight_goal`
  record.
- `remaining_kcal = calories - protein_grams * 4`, split 50/50 by kcal between carbs and fat
  (`carbs_grams = remaining_kcal / 2 / 4`, `fat_grams = remaining_kcal / 2 / 9`), UNLESS this
  produces `fat_grams` below `0.8 * goal_weight_kg`, in which case `fat_grams` SHALL be set to
  `0.8 * goal_weight_kg` and `carbs_grams` SHALL be recomputed as whatever kcal remain after
  protein and the fat floor, divided by 4. If this recomputed `carbs_grams` (or, before any fat
  floor applies, the un-floored `carbs_grams`) would be negative — reachable when `protein_grams *
  4` alone meets or exceeds `calories` — `carbs_grams` SHALL be clamped to 0 rather than returned as
  negative; `protein_grams` and `fat_grams` are unaffected by this clamp and the response is still
  HTTP 200, not a 422.
- `calories`, `protein_grams`, `carbs_grams`, and `fat_grams` SHALL each be rounded to the nearest
  whole unit (kcal for `calories`, grams for the other three) before being returned.

#### Scenario: Standard computation

- **WHEN** a user with measured weight 91.1 kg, height 1.78 m, birthdate implying age 35, sex
  male, goal weight 80 kg, and an inferred "Moderately active" tier (multiplier 1.55) requests
  their target
- **THEN** the system SHALL return HTTP 200 with `calories = 2873`, `protein_grams = 128`,
  `carbs_grams = 295`, and `fat_grams = 131`

#### Scenario: Fat floor engages for an aggressive deficit

- **WHEN** a user's remaining-calorie 50/50 split would put `fat_grams` below `0.8 * goal_weight_kg`
- **THEN** the system SHALL set `fat_grams` to the floor and recompute `carbs_grams` from the
  remaining kcal, rather than returning a fat target below the floor

#### Scenario: Protein alone exceeds the calorie target

- **WHEN** a user's `protein_grams * 4` is greater than or equal to their `calories`
- **THEN** the system SHALL return HTTP 200 with `protein_grams` and `fat_grams` (at its floor)
  computed normally, `carbs_grams = 0`, and SHALL NOT return a 422 for this input combination

### Requirement: Nutrition Target endpoint
The system SHALL expose `GET /api/users/me/nutrition-target` (requires authentication), following
the existing `summaryHandler` pattern for its no-request-body, plain-JSON-response shape. Unlike
`summaryHandler`, it SHALL NOT accept a `?user=<username>` parameter and SHALL always compute the
target for the authenticated caller only (`ClaimsFromCtx`), since its inputs include the caller's
own `UserSettings` fields, which are self-only per `user-settings`. On success it SHALL return HTTP
200 with `calories`, `protein_grams`, `carbs_grams`, `fat_grams`, and the inputs used
(`measured_weight_kg`, `goal_weight_kg`, `height_m`, `age_years`, `sex`, `activity_multiplier`,
`activity_tier`).

When an input required by the "Nutrition Target computation" requirement is unavailable, the system
SHALL return HTTP 422 with `{"error": "<reason>"}`, checking in this order and reporting the first
unmet reason found:

1. `missing_profile` — no valid `birthdate` or no valid `sex` (see `user-profile`).
2. `missing_measurements` — no `weight` record or no `height` record exists.
3. `missing_goal_weight` — no `weight_goal` record exists.
4. `insufficient_activity_data` — the activity tier is unavailable (see the inference requirement
   above) and no `activity_override` is set.

There SHALL be no partial-target response: if any of the four reasons apply, the system SHALL
return 422 rather than a JSON object with some fields present and others omitted.

#### Scenario: Full target returned

- **WHEN** an authenticated user with a complete profile, measured weight and height, a goal
  weight, and either sufficient step history or an activity override requests their target
- **THEN** the system SHALL return HTTP 200 with all four target fields

#### Scenario: No goal weight set

- **WHEN** an authenticated user has a complete profile and measurements but no `weight_goal`
  record
- **THEN** the system SHALL return HTTP 422 with `{"error": "missing_goal_weight"}`, not a partial
  target omitting protein/carbs/fat

#### Scenario: No profile set

- **WHEN** an authenticated user has never saved a `birthdate` or `sex`
- **THEN** the system SHALL return HTTP 422 with `{"error": "missing_profile"}`

#### Scenario: No measured weight or height

- **WHEN** an authenticated user has a complete profile and a goal weight but no `weight` or no
  `height` record
- **THEN** the system SHALL return HTTP 422 with `{"error": "missing_measurements"}`

#### Scenario: Activity tier unavailable and no override

- **WHEN** an authenticated user has a complete profile, measurements, and a goal weight, but fewer
  than 7 valid trailing step days and no `activity_override`
- **THEN** the system SHALL return HTTP 422 with `{"error": "insufficient_activity_data"}`

#### Scenario: Unauthenticated request rejected

- **WHEN** a request to `GET /api/users/me/nutrition-target` carries no valid token
- **THEN** the system SHALL return HTTP 401
