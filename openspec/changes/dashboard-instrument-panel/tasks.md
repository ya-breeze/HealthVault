## 1. Design tokens & icon set

- [ ] 1.1 Add the Instrument Panel token set (graphite palette, 8 metric-color map, type scale) as a shared module (`app/lib/tokens.ts` or a `globals.css` `@theme` block)
- [ ] 1.2 Build the inline-SVG icon set (camera, pencil, link/webhook, import, logout, chevron) as small React components
- [ ] 1.3 Swap Geist for the chosen sans/mono font stack in `app/layout.tsx`

## 2. Shared header

- [ ] 2.1 Extract the header/nav markup (app name, user selector, webhook popover, custom foods/import links, logout) into a shared `Header` component
- [ ] 2.2 Mount `Header` in the root layout (or each page) so dashboard, data pages, food logging, import, and login-authenticated pages all render the same instance
- [ ] 2.3 Remove the now-duplicated inline header markup from `app/page.tsx` and any other page that had its own copy

## 3. Backend: bucketed aggregation endpoint

- [ ] 3.1 Add a `family` field (`cumulative` | `point`) to each `typeRegistry` entry in `backend/pkg/server/api.go`
- [ ] 3.2 Implement `QueryAggregate(table, timeCol, valueCol, family, bucket, userID, TimeRange)` in `backend/pkg/database/storage.go` / `storage_impl.go`, using `strftime`-based `GROUP BY` for `day`/`week`/`month`
- [ ] 3.3 Add the `blood_pressure` two-column special case (`systolic_*`, `diastolic_*`) as a distinct query path from the single-value-column case
- [ ] 3.4 Wire `?bucket=` into `dataHandler`: validate the value (400 on unrecognized or on `food_meal`), call the aggregate query when present, keep the existing raw-record path when absent
- [ ] 3.5 Backend tests: bucketed response shape per family, blood pressure dual columns, 400 on invalid bucket, 400 on `food_meal` + bucket, unchanged raw response when `?bucket=` is omitted

## 4. Frontend: dashboard vitals grid

- [ ] 4.1 Build a `VitalCard` component: current value, 7-day sparkline (SVG), trend indicator, metric accent color from the shared token map
- [ ] 4.2 Fetch 7-day data (bucketed `day`) for the 8 primary metrics and render them as the vitals grid on `app/page.tsx`
- [ ] 4.3 Render the remaining registered types as the existing compact link row (behavior unchanged, restyled to match)
- [ ] 4.4 Handle the no-data-for-a-metric case in `VitalCard` (today's summary-error path generalized per card)

## 5. Frontend: zoom-aware chart

- [ ] 5.1 Add `bucket?: 'day' | 'week' | 'month'` to `api.data()` in `lib/api.ts`
- [ ] 5.2 Build the Day/Week/Month/Year segmented control in `DataTypeClient.tsx`, defaulting to Week
- [ ] 5.3 Implement the cumulative-family chart renderer (line for Day, bars for Week/Month/Year) reading `sum`
- [ ] 5.4 Implement the point-family chart renderer (line for Day, averaged line + min–max band for Week/Month/Year) reading `avg`/`min`/`max`
- [ ] 5.5 Special-case `blood_pressure` rendering: two lines/bands (systolic, diastolic) sharing one chart
- [ ] 5.6 Recompute the stats row (avg/max/total as applicable) from the same bucketed response driving the chart
- [ ] 5.7 Keep the existing raw data table below the chart, restyled to match the new tokens

## 6. Verification

- [ ] 6.1 Update `e2e/tests/dashboard.spec.ts` for the vitals grid structure
- [ ] 6.2 Update `e2e/tests/data-types.spec.ts` for the zoom control and per-family chart rendering
- [ ] 6.3 Run `make lint` / `go vet` / `tsc --noEmit` (per project conventions) and fix findings
- [ ] 6.4 Deploy to `hcw-wip`, run the full E2E suite against it, fix failures before requesting review
