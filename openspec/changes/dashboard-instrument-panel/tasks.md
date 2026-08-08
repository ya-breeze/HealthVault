## 1. Design tokens & icon set

- [ ] 1.1 Add the Instrument Panel token set (graphite palette, a 26-type metric-color map — every registered type, not just the 8 vitals-grid metrics — type scale) as a shared module (`app/lib/tokens.ts` or a `globals.css` `@theme` block)
- [ ] 1.2 Build the inline-SVG icon set (camera, pencil, link/webhook, import, logout, chevron) as small React components
- [ ] 1.3 Swap Geist for the chosen sans/mono font stack in `app/layout.tsx`

## 2. Shared header

- [ ] 2.1 Extract the header/nav markup (app name, user selector, webhook popover, custom foods/import links, logout) into a shared `Header` component
- [ ] 2.2 Mount `Header` in the root layout (or each page) so dashboard, data pages, food logging, import, and login-authenticated pages all render the same instance
- [ ] 2.3 Remove the now-duplicated inline header markup from `app/page.tsx` and any other page that had its own copy

## 3. Backend: bucketed aggregation endpoint

- [ ] 3.1 Add a `family` field (`cumulative` | `point`) to each of the 25 non-`food_meal` `typeRegistry` entries in `backend/pkg/server/api.go` — including `height` (`point`) and `nutrition` (`cumulative`), easy to miss since neither is a dashboard vital
- [ ] 3.2 Implement `QueryAggregate(table, timeCol, valueCol, family, bucket, userID, TimeRange)` in `backend/pkg/database/storage.go` / `storage_impl.go`, using `strftime`-based `GROUP BY` for `day`/`month` (no `week` bucket — see design.md)
- [ ] 3.3 Add the `blood_pressure` two-column (`systolic_*`, `diastolic_*`) and `nutrition` seven-column (`sum_calories`, `sum_protein_grams`, ...) special cases as distinct query paths from the single-value-column case
- [ ] 3.4 Wire `?bucket=` into `dataHandler`: validate the value (400 on unrecognized — including `week` — or on `food_meal`), call the aggregate query when present, keep the existing raw-record path when absent
- [ ] 3.5 Backend tests: bucketed response shape per family (including nutrition's 7 columns and blood pressure's 2), 400 on invalid bucket (including `week`), 400 on `food_meal` + bucket, unchanged raw response when `?bucket=` is omitted

## 4. Frontend: dashboard vitals grid

- [ ] 4.1 Build a `VitalCard` component: current value, 7-day sparkline (SVG), trend indicator, metric accent color from the shared token map
- [ ] 4.2 Fetch 7-day data (bucketed `day`) for the 8 primary metrics and render them as the vitals grid on `app/page.tsx`
- [ ] 4.3 Render the remaining registered types as the existing compact link row (behavior unchanged, restyled to match)
- [ ] 4.4 Handle the no-data-for-a-metric case in `VitalCard` (today's summary-error path generalized per card)

## 5. Frontend: zoom-aware chart

- [ ] 5.1 Add `bucket?: 'day' | 'month'` to `api.data()` in `lib/api.ts`
- [ ] 5.2 Build the Day/Week/Month/Year segmented control in `DataTypeClient.tsx`, defaulting to Week; Week and Month both request `bucket=day` (different time ranges), Year requests `bucket=month`
- [ ] 5.3 Implement the cumulative-family chart renderer (line for Day, bars for Week/Month/Year) reading `sum` (or, for `nutrition`, the selected macro's `sum_*` column)
- [ ] 5.4 Implement the point-family chart renderer (line for Day, averaged line + min–max band for Week/Month/Year) reading `avg`/`min`/`max`
- [ ] 5.5 Special-case `blood_pressure` rendering (two lines/bands, systolic + diastolic) and `nutrition` rendering (macro selector, default `calories`)
- [ ] 5.6 Recompute the stats row (avg/max/total as applicable) from the same bucketed response driving the chart
- [ ] 5.7 Keep the existing raw data table below the chart, restyled to match the new tokens

## 6. Verification

- [ ] 6.1 Update `e2e/tests/dashboard.spec.ts` for the vitals grid structure
- [ ] 6.2 Update `e2e/tests/data-types.spec.ts` for the zoom control and per-family chart rendering
- [ ] 6.3 Run `make lint` / `go vet` / `tsc --noEmit` (per project conventions) and fix findings
- [ ] 6.4 Deploy to `hcw-wip`, run the full E2E suite against it, fix failures before requesting review
