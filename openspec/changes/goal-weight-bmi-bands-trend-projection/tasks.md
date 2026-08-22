## 1. Backend: `weight_goal` metric type registration

- [x] 1.1 Add `WeightGoal` Go struct in `backend/pkg/database/` alongside `Weight`/`Height`
      (`time`, `kilograms`, `user_id`, `family_id`; unique `(user_id, time)`; no
      `source_payload_id` requirement — see 1.4)
- [x] 1.2 Add `&WeightGoal{}` to the `AutoMigrate(...)` list in `backend/pkg/database/db.go`
- [x] 1.3 Add `"weight_goal"` to `typeRegistry` in `backend/pkg/server/api.go`
      (`table: "weight_goals", timeCol: "time", family: database.AggFamilyPoint, valueCol: "kilograms"`)
- [x] 1.4 Confirm `WeightGoal` rows created via the new write path (task 2) don't require/synthesize
      a `source_payload_id`, matching the existing food-logging exception in `data-model`
- [x] 1.5 Add `"weight_goal"` to `typeTimeCol` in `backend/pkg/mcpserver/tools.go`
- [x] 1.6 Migrate `Weight.SourcePayloadID` and `Height.SourcePayloadID` from `uuid.UUID
      gorm:"not null"` to `*uuid.UUID` (nullable) in `backend/pkg/database/models.go`, so manual
      writes to these two pre-existing types can omit the column without a synthesized value — see
      design.md's Migration Plan

## 2. Backend: allowlisted write path

- [x] 2.1 Add `POST /api/data/{type}` route in `backend/pkg/server/server.go` (registered before
      the existing `GET`/`DELETE` on the same path, same auth middleware)
- [x] 2.2 Implement the handler per design.md's contract: two-step type check — unregistered type
      (not in `typeRegistry`) → 404, matching `GET`/`DELETE`'s existing unknown-type behavior;
      registered but not in the write allowlist (`weight`, `height`, `weight_goal` only) → 403 —
      then parse `{value, time?}` body, default `time` to now, validate `value` is numeric,
      present, and strictly positive (else 400), insert, return 201 with the created row in the
      same shape a GET returns
- [x] 2.3 Handle a write whose `(user_id, time)` collides with an existing row for that type (only
      reachable via an explicit duplicate `time`, since an omitted `time` always defaults to now):
      catch the existing unique `(user_id, time)` constraint violation and return 409, rather than
      letting a raw DB error surface as a 500. The common case (`time = now()` on every write) never
      hits this path — "latest goal wins" already falls out of ordinary insert-then-query-latest
      behavior without it.
- [x] 2.4 Confirm every type outside the allowlist still returns 403 on POST (regression coverage
      for e.g. `steps`, `blood_pressure`)
- [x] 2.5 Confirm the handler resolves the target user as `claims.UserID` directly and does not
      call `resolveUser`/honor `?user=` — a family member cannot write into another member's
      account via this endpoint, matching `DeleteRecordHandler`'s convention

## 3. Frontend: `weight_goal` registry + i18n

- [ ] 3.1 Add `"weight_goal"` to `DATA_TYPES` in `frontend/lib/api.ts`
- [ ] 3.2 Add a `TYPE_META` entry in `frontend/lib/dataTypeMeta.ts` (`{ family: 'point', decimals:
      1 }`, matching the `Weight`/`Height` entries — `TYPE_META` carries no `label`/`unit` field;
      the display label comes from a new `metric.weight_goal` key in `frontend/lib/i18n/en.ts` and
      `ru.ts`, task 3.3)
- [ ] 3.3 Add `weight_goal` labels to `frontend/lib/i18n/en.ts` and `ru.ts`
- [ ] 3.4 Add `--c-weight_goal` CSS var (light + dark) to `frontend/app/globals.css`, following the
      existing `--c-weight` pattern
- [ ] 3.5 Confirm `weight_goal` is excluded from `PRIMARY_METRICS` (no dashboard Vital Card) but
      renders its own `/data/weight_goal` page (chart + delete-able record table) via the existing
      generic `[type]` route

## 4. Frontend: reusable Add-record form

- [ ] 4.1 Build one reusable Add-record form component, mounted on each allowlisted type's page
      (`weight`, `height`, `weight_goal`) — value + optional timestamp, POSTs to the new endpoint,
      refetches the page's data on success
- [ ] 4.2 Add a "Set goal" shortcut on the weight page that opens the same form pre-targeted at
      `weight_goal`
- [ ] 4.3 Confirm the form does *not* render on any non-allowlisted type's page

## 5. Frontend: BMI bands + readout

- [ ] 5.1 Add a pure function in `dataTypeMeta.ts` converting the 3 WHO BMI band edges (18.5, 25,
      30) to kg given a height in meters (`kg = bmi * heightMeters^2`)
- [ ] 5.2 Render the 4 resulting bands as `ReferenceArea`s on the `weight` chart, clipped to the
      existing Y-domain (bands never expand it — see design.md), rendered at every zoom level —
      `DataTypeClient.tsx` has two chart branches selected by zoom (the Day-zoom `LineChart` and
      the Week/Month/Year `ComposedChart`), so the bands must be added to both, not just one
- [ ] 5.3 Add a pure `classifyBmi(bmi): category` function in `dataTypeMeta.ts` (lower-inclusive
      boundaries: `[18.5,25)` Normal, `[25,30)` Overweight, `[30,∞)` Obese, else Underweight — see
      design.md) and use it for the BMI readout (1 decimal + category name) computed from latest
      raw `weight` + latest `height`
- [ ] 5.4 Gate both 5.2 and 5.3 behind one shared "does a `height` record exist" condition, exposed
      as its own testable helper rather than inlined in the chart component
- [ ] 5.5 `DataTypeClient.tsx` fetches the latest `height` record when `dataType === 'weight'`
      (new GET alongside the existing weight fetch)

## 6. Frontend: goal line

- [ ] 6.1 Fetch the latest `weight_goal` record when `dataType === 'weight'`
- [ ] 6.2 Render a `ReferenceLine` at the goal value, in both of `DataTypeClient.tsx`'s chart
      branches (Day-zoom `LineChart` and Week/Month/Year `ComposedChart`) — the line must render at
      every zoom level, per the requirement, so neither branch can be skipped
- [ ] 6.3 Fold the goal value into the value set passed to `computeYDomain` for **both** existing
      domain computations — `dayDomain` (feeds the Day-zoom `LineChart`) and `bandDomain` (feeds
      the Week/Month/Year `ComposedChart`) — so the Y-domain always expands to include the goal at
      every zoom, not just the zoom tier touched first (see design.md; this is the accepted
      trade-off)

## 7. Frontend: trend projection

- [ ] 7.0 `DataTypeClient.tsx` fetches a dedicated 30+-day daily-bucketed `weight` range for the
      regression, independent of the active zoom's own bucket fetch — Day zoom fetches no bucketed
      data today, Week zoom's widened lookback only reaches 14 days, and Year zoom's bucket is
      monthly, so none of the three can feed a 30-daily-point EMA window on their own; only Month
      zoom's fetch happens to already cover it. Since the ETA text must render at every zoom level
      (7.5), this fetch cannot be conditional on which zoom is active.
- [ ] 7.1 Add a pure least-squares regression function over `(day_offset, ema_value)` pairs
- [ ] 7.2 Add a pure function selecting the last 30 calendar days of the existing EMA series,
      independent of the active zoom
- [ ] 7.3 Add pure "not enough data" gating: <5 weight records total, or <14-day lifetime span, or
      fewer than 2 EMA points inside the 30-day regression window (task 7.2) itself → no line, "Not
      enough data to project yet" — the lifetime check and the window check are independent; either
      one failing is sufficient
- [ ] 7.4 Add pure crossing/horizon logic: given slope+intercept+goal, compute the crossing date;
      flat/diverging/beyond the 365-day horizon (fixed day count from the regression window's most
      recent day, not calendar-month arithmetic — see design.md) → no line, "Not on track at your
      current trend"
- [ ] 7.4a Add a pure "already at goal" check that runs before 7.4's direction check: direction =
      `sign(goal - windowStartEma)`, where `windowStartEma`/`latestEma` are the oldest/newest EMA
      values in the 30-day regression window from 7.2 (NOT the user's lifetime-earliest raw weight —
      using lifetime history here would let an old, no-longer-relevant weight flip the direction for
      a user whose weight has since crossed to the other side of the goal); if the latest EMA has
      reached or passed the goal in that direction, use "You've reached your goal weight" instead of
      routing a flat/at-goal trend through 7.4's "Not on track" message — see design.md
- [ ] 7.4b Extend `bucketBandData` with synthetic future-dated rows (populated only in a new
      `projection` field; `avg`/`range`/`trend` left undefined) spanning from the last real bucket
      to the computed crossing date, capped at the 365-day horizon — at the active zoom's bucket
      granularity (daily at Month, monthly at Year) — so the dashed line has X-axis positions to
      plot on the categorical axis. Confirm Month zoom's visible X-range is allowed to expand
      beyond its normal ~30-day window to fit the crossing point — see design.md
- [ ] 7.5 Render the dashed projection `Line` only at Month/Year zoom, using the extended data from
      7.4b; render the ETA text (or the "not enough data" / "not on track" / "you've reached your
      goal" message) at every zoom level
- [ ] 7.6 Confirm the projection line/text never render at all when no `weight_goal` is set

## 8. Testing: Vitest setup + unit coverage

- [ ] 8.1 Add Vitest to `frontend/package.json` (dev dependency + `test` script), minimal config
- [ ] 8.2 Unit tests for `emaSeries`, `computeYDomain`, `rangeForZoom` (first coverage ever, per
      the `weight-chart-scale-and-trend` change's deferred follow-up)
- [ ] 8.3 Unit tests for the BMI band-edge conversion (task 5.1) and the `classifyBmi` category
      lookup (task 5.3), including the exact-boundary values BMI 18.5, 25, and 30 (each SHALL
      classify into the higher category per design.md's lower-inclusive convention)
- [ ] 8.4 Unit tests for the regression + 30-day-window selection (task 7.1, 7.2)
- [ ] 8.5 Unit tests for the "not enough data" gate (task 7.3): exactly 4 records, exactly 5
      records spanning <14 days, exactly 5 spanning >=14 days, and a user meeting both lifetime
      thresholds but with fewer than 2 EMA points inside the last 30 calendar days (old data, no
      recent activity)
- [ ] 8.6 Unit tests for crossing/horizon logic (task 7.4): flat trend, wrong-direction trend,
      crossing just inside the 365-day horizon, crossing at exactly day 365 (within horizon),
      crossing at day 366 (beyond horizon)
- [ ] 8.7 Unit tests for the shared height-exists gate (task 5.4): bands and readout both
      suppressed with zero `height` records, both present with one — independent of the E2E
      coverage in 9.2, per IDEA_FORGE_PLAN.md's testing section calling this out as a pure-function
      case impractical to seed reliably through a browser
- [ ] 8.8 Unit tests for the "already at goal" check (task 7.4a): flat trend with latest EMA at the
      goal (displays "You've reached your goal weight", not "Not on track"), flat trend with latest
      EMA still far from the goal (displays "Not on track" as before), latest EMA already past
      the goal on the far side (still classified as reached, not "not on track"), and a case where
      the user's lifetime-earliest raw weight sits on the opposite side of the goal from the current
      30-day window (e.g. old weight 60kg, current flat trend 90kg, goal 75kg) — must display "Not
      on track", not falsely report "You've reached your goal weight"; and a case where
      `windowStartEma == goal` exactly but `latestEma` has since diverged away from goal — must
      display "Not on track" (per design.md's undefined-direction fallthrough), not falsely report
      "You've reached your goal weight"

## 9. E2E

- [ ] 9.1 Extend `e2e/tests/data-types.spec.ts`: create a `weight_goal` record via the new
      Add-record form, confirm it appears on `/data/weight_goal` and as a goal line on the weight
      chart
- [ ] 9.2 E2E coverage for the height dead end being closed: create a `height` record via the same
      form, confirm BMI bands + readout appear on the weight chart afterward and are absent before
- [ ] 9.3 E2E coverage confirming POST to a non-allowlisted type (e.g. `steps`) is rejected
- [ ] 9.4 Run full suite against `hcw-wip`

## 10. Docs

- [ ] 10.1 `CONTEXT.md`: add glossary entries for **BMI Band**, **Trend Projection**, **Manual
      Record**
- [ ] 10.2 New `docs/adr/ADR-005-<slug>.md` for the allowlisted write path (`Status: Proposed`
      until this change merges)
- [ ] 10.3 Correct `docs/adr/ADR-002-goal-weight-as-metric-type.md`'s Consequences section to match
      ADR-003's actual split decision (calories/BMR from measured weight, protein g/kg from goal
      weight), not the current "goal weight only" misstatement
- [ ] 10.4 Mark the weight-chart backlog item in `todo.md` as claimed by this change

## 11. Verification

- [ ] 11.1 `make lint`
- [ ] 11.2 `make test`
- [ ] 11.3 `npx tsc --noEmit` and `npm run build` in `frontend/`
- [ ] 11.4 `openspec validate --strict` for this change
