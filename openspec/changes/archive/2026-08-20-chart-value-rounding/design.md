## Context

`DataTypeClient.tsx` renders every chart (`YAxis`, `Tooltip`, `Line` for point-in-time types plus their EMA trend, `Bar` for cumulative types) with no `tickFormatter`/`formatter` props, so Recharts prints raw floats straight from the API/EMA computation.

A precision-per-metric convention already exists, but only inline inside `vitals.ts`'s `getVital()` switch statement (e.g. `weight` → `.toFixed(1)`, `heart_rate` → `Math.round()`), used for the dashboard vitals-grid cards. `DataTypeClient.tsx` separately does `stats.avg.toFixed(1)` / `stats.max.toFixed(1)` for the stats row under each chart — a second, independent hardcoding of "weight-like things get 1 decimal."

`dataTypeMeta.ts`'s `TYPE_META` map already carries per-type chart metadata (`family: 'cumulative' | 'point'`) consumed by `DataTypeClient.tsx`, and is the natural place to add a second per-type fact: display decimal precision.

## Goals / Non-Goals

**Goals:**
- One shared, per-metric decimal-precision source of truth, consumed by both the chart (axis/tooltip/trend line) and the existing vitals-grid/stats-row formatting, replacing today's two independent hardcodings.
- Fix the chart's unrounded rendering for every metric type, not just weight.

**Non-Goals:**
- Changing the EMA trend computation (`emaSeries()`) or backend `AVG`/`MIN`/`MAX` queries — they stay full precision; only rendering changes.
- Changing the *values* of anything already correctly rounded (`vitals.ts`, the stats row) — this is a refactor-to-share-source, not a behavior change for those call sites.
- Locale-aware number formatting (thousands separators etc.) — out of scope, matches current behavior.

## Decisions

**Add a `decimals: number` field to `TYPE_META` in `dataTypeMeta.ts`, plus a `formatMetricValue(type: DataType, value: number): string` helper exported alongside it.**
`TYPE_META` already carries per-type chart metadata used by this exact file, so this keeps all "how do I display/chart type X" facts in one map instead of spreading precision into a third location. `formatMetricValue` centralizes the `toFixed`/`Math.round` choice so callers don't re-derive it.

Precision per type, carried over unchanged from the existing `vitals.ts` switch where one exists: 1 decimal for `weight`, `distance`, `sleep` (hours); 0 decimals (whole number) for `steps`, `heart_rate`, `heart_rate_variability`, `oxygen_saturation`.

That switch only covers the 8 vitals-grid metrics, leaving two groups with no existing precision rule to carry over — both default to 0 decimals unless flagged otherwise, and are enumerated explicitly (not left as an implicit "everything else") so nothing in `TYPE_META` is silently missed:
- **Cumulative-family types** not in `vitals.ts`: `active_calories`, `total_calories` (kcal, whole numbers), `exercise` (`DurationSeconds`, whole number), `nutrition` (grams/calories — matches the existing `Math.round`/`.toFixed(0)` convention already used by `MacroSummary.tsx`/food history). `hydration` (`Liters`) is the one exception in this group — 1 decimal, not 0, since sub-liter precision (e.g. `1.5`) is meaningful and matches `distance`'s treatment as a liquid/distance-style quantity.
  - **Revised during implementation:** this originally said `exercise` would render "as minutes." That was dropped — unlike `distance`/`sleep`, there is no existing "minutes" display convention for exercise anywhere else in the app to match (checked: none found), so introducing one here would be a new, unreviewed unit decision rather than a carry-over. `exercise` stays in its raw storage unit (seconds) at 0 decimals, same as before this change; only its rounding changed, not its unit. See `toDisplayUnit`'s doc comment in `dataTypeMeta.ts`.
- **Point-family types** not in `vitals.ts`: `height`, `blood_pressure`, `blood_glucose`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed` — 0 decimals by default (matches how these read as whole-number-ish metrics with no existing convention to contradict) — **flagged as an open question below** for any metric where 0-decimals isn't obviously right.

**Apply `formatMetricValue` in `DataTypeClient.tsx` at three render points**: `YAxis tickFormatter`, `Tooltip formatter`, and (for point-in-time types) the trend `Line`'s own tooltip entry — all read from the already-full-precision `value`/`trend` fields, so this is purely a render-prop addition, no data pipeline change.

**Refactor `vitals.ts`'s `getVital()` to call `formatMetricValue` instead of its inline `toFixed`/`Math.round` per case**, and refactor the stats-row `.toFixed(1)` calls in `DataTypeClient.tsx` to the same helper. This removes the two pre-existing duplicated copies of the precision rule rather than adding a third.

## Risks / Trade-offs

- **[Risk]** Assigning a default 0-decimals precision to point-in-time metrics that never had an explicit rule (e.g. `blood_glucose`, `body_temperature`, `vo2_max`) is a judgment call, not a carry-over of an existing decision. → Mitigation: call out the exact default list in tasks.md for a quick human sanity-check during implementation; easy to bump any one type to 1 decimal without touching the mechanism.
- **[Risk]** Refactoring `vitals.ts` and the stats row alongside the chart fix broadens the diff slightly beyond "just the chart." → Mitigation: the refactor is a mechanical extraction (same output values, same tests should pass unchanged) — the alternative (a third hardcoded copy in the chart) is the thing explicitly being avoided per the proposal.

## Open Questions

- Confirm the default 0-decimals assumption for the point-family types with no pre-existing `vitals.ts` precision rule (`height`, `blood_pressure` sub-values already whole numbers in practice, `blood_glucose`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed`) — several of these (`body_fat` %, `vo2_max`, `speed`) plausibly want 1 decimal rather than 0; flagged in tasks.md rather than blocking the proposal.
