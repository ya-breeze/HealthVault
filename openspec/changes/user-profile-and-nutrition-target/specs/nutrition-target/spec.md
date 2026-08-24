## ADDED Requirements

### Requirement: Step-based activity-level inference
The system SHALL infer a user's activity tier from a trailing 28-calendar-day window of their
`steps` metric, ending the day before the current day (the current day is always excluded — it has
not yet ended, so its count is inherently partial).

Within that window, the system SHALL trim incomplete days from the trailing (most recent) edge only:
walking backward from the most recent day in the window, each day with zero step records, or with
fewer than 500 total steps, SHALL be discarded; trimming SHALL stop at the first day that has 500 or
more total steps, and no day older than that stopping point SHALL be discarded regardless of its
step count. The remaining days SHALL be averaged directly — the window SHALL NOT be extended with
additional older days to replace ones that were trimmed.

The resulting average SHALL map to one of 5 tiers:

| Tier | Trailing 28-day avg steps/day | Multiplier |
|---|---|---|
| Sedentary | < 5,000 | 1.2 |
| Lightly active | 5,000 – 7,499 | 1.375 |
| Moderately active | 7,500 – 9,999 | 1.55 |
| Very active | 10,000 – 12,499 | 1.725 |
| Extra active | ≥ 12,500 | 1.9 |

If fewer than 7 days remain after trimming, and the user has no `activity_override` set (see
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

#### Scenario: Fewer than 7 valid days and no override

- **WHEN** trimming leaves fewer than 7 valid days in the window and the user has no
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
  user's `activity_override` if set (see `user-profile`).
- `protein_grams = 1.6 * goal_weight_kg`, where `goal_weight_kg` is the user's latest `weight_goal`
  record.
- `remaining_kcal = calories - protein_grams * 4`, split 50/50 by kcal between carbs and fat
  (`carbs_grams = remaining_kcal / 2 / 4`, `fat_grams = remaining_kcal / 2 / 9`), UNLESS this
  produces `fat_grams` below `0.8 * goal_weight_kg`, in which case `fat_grams` SHALL be set to
  `0.8 * goal_weight_kg` and `carbs_grams` SHALL be recomputed as whatever kcal remain after
  protein and the fat floor, divided by 4.

#### Scenario: Standard computation

- **WHEN** a user with measured weight 91.1 kg, height 1.78 m, birthdate implying age 35, sex
  male, goal weight 80 kg, and an inferred "Moderately active" tier requests their target
- **THEN** the system SHALL return calories, protein_grams, carbs_grams, and fat_grams computed
  per the formulas above, with `protein_grams = 128` (80 * 1.6)

#### Scenario: Fat floor engages for an aggressive deficit

- **WHEN** a user's remaining-calorie 50/50 split would put `fat_grams` below `0.8 * goal_weight_kg`
- **THEN** the system SHALL set `fat_grams` to the floor and recompute `carbs_grams` from the
  remaining kcal, rather than returning a fat target below the floor

### Requirement: Nutrition Target endpoint
The system SHALL expose `GET /api/users/me/nutrition-target` (requires authentication), following
the existing `summaryHandler` pattern: no request body, resolves the authenticated user via
`ClaimsFromCtx`, returns a plain JSON object. On success it SHALL return HTTP 200 with `calories`,
`protein_grams`, `carbs_grams`, `fat_grams`, and the inputs used (`measured_weight_kg`,
`goal_weight_kg`, `height_m`, `age_years`, `sex`, `activity_multiplier`, `activity_tier`).

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
