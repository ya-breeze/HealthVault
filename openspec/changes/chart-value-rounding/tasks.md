## 1. Shared precision helper

- [x] 1.1 Add `decimals: number` to `TypeMeta` in `frontend/lib/dataTypeMeta.ts` and set it on **every** `TYPE_META` entry (all 24 — none left unset):
  - Carried over from `vitals.ts`: `weight`/`distance`/`sleep` = 1, `steps`/`heart_rate`/`heart_rate_variability`/`oxygen_saturation` = 0.
  - Cumulative types with no existing rule: `active_calories`/`total_calories`/`exercise`/`nutrition` = 0 (kcal/minutes/grams, matches `MacroSummary.tsx`'s existing `Math.round`/`.toFixed(0)`); `hydration` = 1 (liters — sub-liter precision is meaningful, unlike the whole-number group).
  - Point types with no existing rule: sanity-checked individually rather than blanket-defaulted to 0 — `height` = 2 (meters, e.g. 1.75); `blood_glucose`/`body_temperature`/`skin_temperature`/`body_fat`/`lean_body_mass`/`vo2_max`/`bone_mass`/`speed` = 1 (each is either a conventionally-1-decimal unit — mmol/L, °C — or a kg/%/ml-per-kg-per-min/m-per-s quantity where a whole number would hide meaningful variation, same reasoning as `weight`); `blood_pressure`/`respiratory_rate`/`resting_heart_rate`/`basal_metabolic_rate` = 0 (already whole-number in practice, matches `heart_rate`).
- [x] 1.2 Add `formatMetricValue(type: DataType, value: number): string` to `dataTypeMeta.ts`, reading precision from `TYPE_META[type]?.decimals` (fall back to 0 for unregistered types). Uses `toLocaleString` with fixed min/max fraction digits rather than raw `toFixed`, so large values (e.g. steps) keep thousands-grouping — required for task 3.2's "unchanged output" to hold for `steps`, which was already comma-formatted.

## 2. Apply to the chart

- [x] 2.1 In `DataTypeClient.tsx`, add a `tickFormatter` to each `YAxis` using `formatMetricValue`.
- [x] 2.2 Add a `formatter` to each `Tooltip` (covering the raw line/bars, the min-max band, and the averaged point) using `formatMetricValue`. The min-max band's `[number, number]` tuple is rendered as `"low – high"`, each side individually rounded.
- [x] 2.3 Add a tooltip `formatter`/`name` for the weight trend `Line` so its tooltip entry is rounded the same way as the raw series — satisfied by the shared per-chart `formatTooltipValue`, since the trend `Line` already had `name="Trend"` and shares its chart's single `Tooltip`.
- [x] 2.4 Replace the stats-row `stats.avg.toFixed(1)` / `stats.max.toFixed(1)` with `formatMetricValue`. (No sibling min/latest stats exist; `stats.total` — cumulative types only — was left on `.toLocaleString()`, out of this task's literal scope and not the thing the bug report was about.)

## 3. Consolidate the existing duplicate

- [x] 3.1 Refactor `vitals.ts`'s `extractVital()` switch (named `getVital` in design.md; actual function is `extractVital`) to call `formatMetricValue` instead of its inline `toFixed`/`Math.round` per case, removing the now-duplicated precision knowledge.
- [x] 3.2 Verify dashboard vitals-grid card values are unchanged after the refactor (same displayed strings as before) — confirmed manually against `hcw-wip`: steps/heart_rate/hrv/oxygen_saturation/weight/distance/sleep/blood_pressure cards all render identical strings to before (steps keeps its comma grouping via `formatMetricValue`'s `toLocaleString` choice, see 1.2).

## 4. Verify

- [x] 4.1 `make lint` / `tsc --noEmit` in `frontend/` (or project equivalent) — fix any type errors. Both clean; `next build` also clean.
- [x] 4.2 Manually check the weight chart's trend line and Y-axis at Week/Month/Year zoom show 1 decimal, and a whole-number metric (e.g. `heart_rate`) shows no decimals, against the deployed WIP stack.
- [x] 4.3 Confirm existing frontend unit tests (if any cover `vitals.ts`) still pass after the refactor. There are no frontend unit tests in this project (no test runner/config exists) — nothing to run; covered instead by 3.2's manual check and `next build`'s type-check.
