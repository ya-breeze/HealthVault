# Investigation: Dashboard read-path database profile (idea #478)

## Outcome

The current dashboard source queries have a deterministic round-trip baseline:
one settings read, 27 serial Presence `COUNT` statements, eight daily
aggregate statements, and one needs-attention count — 37 SQL statements once
the target user is resolved. Replacing only the Presence computation with one
sorted `UNION ALL` statement of bounded `EXISTS` probes reduces that measured
sequence to 11 statements. The optimization removes 26 of the 27 Presence
round trips; benchmark wall-clock results are supporting evidence, not a
machine-specific acceptance threshold.

This change optimizes the existing authenticated Presence endpoint. The child
change will define and consume the authenticated dashboard read-model response,
including its partial-result and per-source degradation contract. No dashboard
HTTP cutover is part of this change.

## Measured path

`backend/pkg/server/data_types_presence_benchmark_test.go` uses an internal
`server` package benchmark. It passes a resolved user ID directly to the
measured source operations so the profile isolates database work in those
sources; user lookup, fixture setup, and seed writes are outside the timed
region. The benchmark measures each component independently and then runs the
old browser-driven source sequence as `legacy_fresh_load`. It also runs the
same sequence with the optimized Presence helper as `current_fresh_load`.

The test-only statement counter registers GORM query and row callbacks after
setup, so `Rows()`-based steps aggregation is counted as well as ordinary
GORM queries. It reports `sql-statements/op` alongside Go's `ns/op`, `B/op`,
and `allocs/op` without changing production logging.

### Fixtures

| Profile | Settings | Primary vital rows | Food meals | Presence stress |
| --- | ---: | ---: | ---: | --- |
| `empty` | 0 | 0 | 0 | all registered tables empty |
| `populated` | 1 | 14 rows in each of the 8 dashboard data types (112 total) | 3: processing, pending-review, confirmed | normal representative history |
| `high_cardinality_presence` | 1 | the same 112 rows | the same 3 meals | 5,000 additional `steps` rows, for 5,014 total steps rows; the extra rows are outside the aggregate range |

The populated fixture covers `steps`, `heart_rate`, `sleep`,
`heart_rate_variability`, `distance`, `weight`, `blood_pressure`, and
`oxygen_saturation`, plus the Food Meal statuses used by
`needsAttentionStatuses`. The empty and high-cardinality profiles show that
the serial implementation has a fixed 27-statement amplification and also
does more row work when a table contains many matches.

### Reproduction

Run the complete benchmark set with:

```sh
cd backend
go test -tags sqlite_fts5 -run '^$' -bench '^BenchmarkDashboardReadPath' -benchtime=1s ./pkg/server
```

The representative output below was captured on 2026-09-15 on an Intel
i3-10100 host using the populated fixture:

| Benchmark | `ns/op` | SQL statements/op | `B/op` | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `presence_serial_count` | 426,326 | 27 | 90,746 | 1,361 |
| `presence_union_exists` | 233,971 | 1 | 52,337 | 553 |
| `legacy_fresh_load` | 1,436,163 | 37 | 334,538 | 6,383 |
| `current_fresh_load` | 1,301,816 | 11 | 296,195 | 5,575 |

Timings and allocations vary with the Go, SQLite, and host environment. The
statement counts are the reproducible decision signal:

| Measured component | Before | After | Reason |
| --- | ---: | ---: | --- |
| Settings read | 1 | 1 | unchanged |
| Presence | 27 serial `COUNT`s | 1 combined probe | 26 round trips removed |
| Each of 8 daily aggregates | 1 | 1 | unchanged |
| Needs-attention count | 1 | 1 | unchanged |
| Fresh-load source sequence | 37 | 11 | 26 statements removed |

## Query-plan evidence

The combined SQL is built from the sorted `typeRegistry` keys. Each arm binds
the type name and user ID and interpolates only its registry-owned table name:

```sql
SELECT ? AS type_name,
       EXISTS(SELECT 1 FROM steps WHERE user_id = ? LIMIT 1) AS present
UNION ALL
...
```

On the migrated SQLite benchmark database, `EXPLAIN QUERY PLAN` reported a
`SEARCH ... USING COVERING INDEX (...user_id=?)` entry inside the scalar
subquery for every registered table. Representative output is:

```text
SCALAR SUBQUERY 47
SEARCH steps USING COVERING INDEX idx_steps_user_time (user_id=?)
SCALAR SUBQUERY 53
SEARCH weight_goals USING COVERING INDEX idx_weight_goal_user_time (user_id=?)
```

The complete table-to-index set was:

| Registered table | Per-user index used |
| --- | --- |
| `active_calories` | `idx_active_cal_user_time` |
| `basal_metabolic_rates` | `idx_bmr_user_time` |
| `blood_glucoses` | `idx_bg_user_time` |
| `blood_pressures` | `idx_bp_user_time` |
| `body_fats` | `idx_bodyfat_user_time` |
| `body_temperatures` | `idx_bodytemp_user_time` |
| `bone_masses` | `idx_bonemass_user_time` |
| `distances` | `idx_distance_user_time` |
| `exercises` | `idx_exercise_user_time` |
| `food_meals` | `idx_food_meals_user_id` |
| `heart_rates` | `idx_hr_user_time` |
| `heart_rate_variabilities` | `idx_hrv_user_time` |
| `heights` | `idx_height_user_time` |
| `hydrations` | `idx_hydration_user_time` |
| `lean_body_masses` | `idx_lbm_user_time` |
| `nutritions` | `idx_nutrition_user_time` |
| `oxygen_saturations` | `idx_spo2_user_time` |
| `respiratory_rates` | `idx_rr_user_time` |
| `resting_heart_rates` | `idx_rhr_user_time` |
| `skin_temperatures` | `idx_skintemp_user_time` |
| `sleeps` | `idx_sleeps_user_time` |
| `speeds` | `idx_speed_user_time` |
| `steps` | `idx_steps_user_time` |
| `total_calories` | `idx_total_cal_user_time` |
| `vo2_maxes` | `idx_vo2_user_time` |
| `weights` | `idx_weight_user_time` |
| `weight_goals` | `idx_weight_goal_user_time` |

`EXISTS` is a bounded existence probe: SQLite can stop the indexed lookup
after the first matching entry, and the explicit `LIMIT 1` makes that bound
visible in the SQL. The old `COUNT` had to visit all matching entries even
though the response only needed a boolean.

## Cache boundary and invalidation inventory

No cache, materialized table, HTTP cache header, or precomputation was added.
Presence is a cheap read-through signal, and introducing cached state here
would require an invalidation protocol across unrelated mutation paths. Any
later dashboard read-model cache design must account for at least:

- imported telemetry, including Health Connect, Libra, webhook, and other
  import paths;
- manual health-record writes and deletes, including weight, height, and
  weight-goal records;
- Food Meal creation, analysis/retry/clarification, confirmation, edits, and
  deletion, which affect both `food_meal` presence and needs-attention count;
- profile changes and Nutrition Target inputs, including measured weight,
  height, goal weight, birthdate, sex, and activity settings;
- settings changes, especially dashboard preferences and timezone changes,
  which can change the dashboard's interpretation and local-day boundaries.

Those invalidation concerns belong to the later read-model design. This split
keeps Presence read-through only and leaves every health-data mutation path
unchanged.
