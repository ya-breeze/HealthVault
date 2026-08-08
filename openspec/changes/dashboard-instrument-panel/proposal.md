## Why

The dashboard and per-type data pages use generic default styling (white/gray-900 cards, Tailwind `blue-500` accents, `shadow-sm`, emoji as icons) and present every metric identically regardless of what it means. Data pages are worse than cosmetic: `DataTypeClient.tsx` fetches raw records for the selected range and plots them as a single `recharts` line with no re-aggregation, so a wide range (e.g. a year of `heart_rate`) renders as an unreadable smear of points — confirmed against the real `hcw-prod` database, which holds 587k `heart_rate` rows and 13k `steps` rows over 9 months. A distinctive, information-dense visual system paired with range-aware chart aggregation (matching patterns from Garmin Connect/Google Fit — different zoom levels show different views, not just a rescaled x-axis) makes the data pages usable at every time scale and gives HealthVault its own visual identity instead of a templated one.

## What Changes

- Introduce an "Instrument Panel" design system (dark graphite base, calibrated per-metric colors, monospace tabular numerals, small inline-SVG icon set replacing emoji) and apply it as the app's shared header/nav layout across every page.
- Rebuild the dashboard's summary section as a dense vitals grid: each of the 8 primary metrics (steps, heart rate, sleep, HRV, distance, weight, blood pressure, oxygen saturation) shows its current value, a 7-day sparkline, and a trend indicator. The remaining registered types collapse into a compact link row, unchanged in behavior from today's "All Data Types" grid.
- Replace the data page's single always-raw line chart with a Day / Week / Month / Year segmented control. Each level selects both a time range and an aggregation:
  - **Cumulative types** (`steps`, `distance`, `active_calories`, `total_calories`, `hydration`, `exercise`, `sleep` by night): Day = raw points as a line; Week/Month = per-day (or per-night) sum as bars; Year = per-month sum as bars.
  - **Point-in-time types** (`heart_rate`, `heart_rate_variability`, `weight`, `blood_pressure`, `blood_glucose`, `oxygen_saturation`, `body_temperature`, `skin_temperature`, `respiratory_rate`, `resting_heart_rate`, `vo2_max`, `body_fat`, `lean_body_mass`, `bone_mass`, `speed`, `basal_metabolic_rate`): Day = raw points as a line; Week/Month/Year = a bucketed average line with a shaded min–max band per bucket.
- `GET /api/data/{type}` gains an optional `?bucket=` parameter (`day`, `week`, `month`) that returns pre-aggregated rows (`bucket_start`, `avg`/`sum`, `min`, `max`, `count`) instead of raw records, so Month/Year views don't require transferring hundreds of thousands of raw rows to the browser. Omitting `?bucket=` keeps today's raw-record response unchanged — existing callers (E2E tests, any other consumer) are unaffected.
- Out of scope for this change: food logging flow, import page, and login page keep their current content/behavior — they only inherit the new shared header.

## Capabilities

### New Capabilities
- `dashboard-ui`: the Instrument Panel visual system — shared header/nav, dashboard vitals grid, design tokens (color/type/icon set) — and how it's applied across dashboard and data pages.
- `chart-zoom-aggregation`: the Day/Week/Month/Year zoom control on data pages and the aggregation rules (cumulative vs. point-in-time) that back each zoom level.

### Modified Capabilities
- `data-api`: `GET /api/data/{type}` gains the optional `?bucket=` query parameter described above.

## Impact

- **Frontend:** `app/layout.tsx` (shared header extracted into a component used app-wide), `app/page.tsx` (dashboard rewrite), `app/data/[type]/DataTypeClient.tsx` (zoom control + aggregation-aware chart rendering), `app/globals.css` (design tokens), new components for the vitals card, zoom chart, and icon set, `lib/api.ts` (new `bucket` param on `api.data()`).
- **Backend:** `backend/pkg/server/api.go` (`dataHandler` accepts `?bucket=`), `backend/pkg/database/storage.go` / `storage_impl.go` (new aggregation query alongside existing `QueryRecords`), one new query per cumulative (`SUM`) vs. point-in-time (`AVG`/`MIN`/`MAX`) type family, bucketed by truncating the type's time column to day/week/month in SQLite.
- **E2E:** `e2e/tests/dashboard.spec.ts` and `e2e/tests/data-types.spec.ts` need updates for the new grid/chart structure and zoom control.
- No database schema changes, no new dependencies, no auth/permission changes.
