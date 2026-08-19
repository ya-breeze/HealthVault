## 1. Shared precision helper

- [ ] 1.1 Add `decimals: number` to `TypeMeta` in `frontend/lib/dataTypeMeta.ts` and set it on **every** `TYPE_META` entry (all 24 — none left unset):
  - Carried over from `vitals.ts`: `weight`/`distance`/`sleep` = 1, `steps`/`heart_rate`/`heart_rate_variability`/`oxygen_saturation` = 0.
  - Cumulative types with no existing rule: `active_calories`/`total_calories`/`exercise`/`nutrition` = 0 (kcal/minutes/grams, matches `MacroSummary.tsx`'s existing `Math.round`/`.toFixed(0)`); `hydration` = 1 (liters — sub-liter precision is meaningful, unlike the whole-number group).
  - Point types with no existing rule: `height`, `blood_pressure`, `blood_glucose`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `basal_metabolic_rate`, `body_fat`, `lean_body_mass`, `vo2_max`, `bone_mass`, `speed` = 0 by default — sanity-check each against realistic values (e.g. `body_fat`/`vo2_max`/`speed` likely want 1 decimal, not 0) rather than blanket-defaulting.
- [ ] 1.2 Add `formatMetricValue(type: DataType, value: number): string` to `dataTypeMeta.ts`, reading precision from `TYPE_META[type]?.decimals` (fall back to 0 for unregistered types) and applying `toFixed`/`Math.round` accordingly.

## 2. Apply to the chart

- [ ] 2.1 In `DataTypeClient.tsx`, add a `tickFormatter` to each `YAxis` using `formatMetricValue`.
- [ ] 2.2 Add a `formatter` to each `Tooltip` (covering the raw line/bars, the min-max band, and the averaged point) using `formatMetricValue`.
- [ ] 2.3 Add a tooltip `formatter`/`name` for the weight trend `Line` so its tooltip entry is rounded the same way as the raw series.
- [ ] 2.4 Replace the stats-row `stats.avg.toFixed(1)` / `stats.max.toFixed(1)` (and any sibling min/latest calls) with `formatMetricValue` so the chart and the stats row can no longer drift apart.

## 3. Consolidate the existing duplicate

- [ ] 3.1 Refactor `vitals.ts`'s `getVital()` switch to call `formatMetricValue` instead of its inline `toFixed`/`Math.round` per case, removing the now-duplicated precision knowledge.
- [ ] 3.2 Verify dashboard vitals-grid card values are unchanged after the refactor (same displayed strings as before).

## 4. Verify

- [ ] 4.1 `make lint` / `tsc --noEmit` in `frontend/` (or project equivalent) — fix any type errors.
- [ ] 4.2 Manually check the weight chart's trend line and Y-axis at Week/Month/Year zoom show 1 decimal, and a whole-number metric (e.g. `heart_rate`) shows no decimals, against the deployed WIP stack.
- [ ] 4.3 Confirm existing frontend unit tests (if any cover `vitals.ts`) still pass after the refactor.
