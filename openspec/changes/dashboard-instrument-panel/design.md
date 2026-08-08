## Context

The dashboard (`app/page.tsx`) and data pages (`app/data/[type]/DataTypeClient.tsx`) currently share no header component logic beyond copy-pasted markup, use Tailwind defaults (`blue-500`, `shadow-sm`, `rounded-xl`, Geist font, emoji icons), and every data page runs the same code path regardless of range: fetch raw records for `[from, to]`, hand them to a single `recharts` `<LineChart>`. Verified against `hcw-prod`: `heart_rate` has 587k rows and `steps` has 13k rows over 9 months of real usage. A "Year" query against `heart_rate` today would transfer and attempt to plot ~580k points client-side.

Brainstormed and approved with the user as Direction "A — Instrument Panel": graphite dark base, one fixed accent hue per metric used consistently everywhere, monospace tabular numerals, bordered card grid, custom SVG icons. Scope is the dashboard and data pages; food logging/import/login keep their current content and only pick up the new shared header.

## Goals / Non-Goals

**Goals:**
- One shared header component used by every authenticated page.
- A dashboard vitals grid (8 primary metrics) driven by one design-token file, replacing the 3-stat-card + separate grid layout.
- A zoom-aware chart on data pages where Week/Month/Year never transfer raw records for high-frequency types.
- A single source of truth for per-metric accent colors, imported by both the dashboard and data pages.

**Non-Goals:**
- Redesigning food logging, import, or login page *content* (they only inherit the header).
- Any database schema change — aggregation is computed in SQL at query time, nothing is precomputed or stored.
- Configurable step/activity goals (the mockup's goal-arc idea belonged to a direction the user didn't pick; not part of this change).
- Timezone-aware bucketing. All timestamps are stored and bucketed in UTC, matching the single-user/dogfood deployment; this is a known simplification, not a hidden bug (see Risks).

## Decisions

### Aggregation happens server-side, gated by an optional `?bucket=` param
Rejected alternative: aggregate client-side after fetching raw records for the full range. Ruled out once `hcw-prod` numbers showed `heart_rate` at ~2,200 rows/day — a Year view would need ~580k rows fetched, parsed, and reduced in the browser. Instead `GET /api/data/{type}?bucket=day|week|month` does the `GROUP BY` in SQLite and returns one row per bucket. Omitting `?bucket=` is unchanged (raw records), so this is additive, not breaking — `e2e/tests/data-types.spec.ts`'s existing raw-record assertions keep working; new tests are added for the bucketed path.

### Two aggregation families, dispatched by a static type→family map
`typeRegistry` in `backend/pkg/server/api.go` gets a second field, `family: "cumulative" | "point"`, alongside the existing `table`/`timeCol`. `sleep` is `cumulative` (its meaningful op is a nightly sum of `duration_seconds`) despite reading like a point-in-time series; `blood_pressure` is `point` but has two value columns (`systolic`, `diastolic`) instead of one, handled as a special case in the aggregation query builder rather than a third family, since every other `point` type has exactly one value column.

### Bucketing via SQLite `strftime`, not a Go-side loop
`GROUP BY strftime('%Y-%m-%d', time_col)` for `day`, `strftime('%Y-%W', time_col)` for `week`, `strftime('%Y-%m', time_col)` for `month`, then `SUM`/`AVG`/`MIN`/`MAX` in the same query. Rejected alternative: fetch raw rows and reduce in Go. SQLite's aggregate functions on an indexed time column (every table already has `uniqueIndex:idx_<type>_user_time`) push the reduction to the database instead of loading every row into the Go process — same transfer-size problem as client-side aggregation, one layer down.

### One design-token module, not per-component styling
`app/lib/tokens.ts` (or `app/globals.css` `@theme` block) defines the graphite palette, the 8 metric-color map, and type scale once. The dashboard vitals grid, data-page chart, and shared header all import from it. Rejected alternative: let each component hardcode its own Tailwind classes as today — that's exactly how the app ended up with an unconsidered, inconsistent default look.

### Icon set as inline SVG React components, not an icon library dependency
A handful of icons (camera, pencil, link, import, logout, chevron) are needed. Hand-rolled inline SVG components avoid adding a new frontend dependency for ~6 icons and keep bundle size and stroke-weight consistent with the design tokens.

## Risks / Trade-offs

- **[Risk] UTC-only bucketing shifts "day"/"week" boundaries relative to the user's local midnight** → Mitigation: acceptable for a single-timezone dogfood deployment; documented as a known simplification, not silently wrong. Revisit if the family-sharing feature gains users in another timezone.
- **[Risk] `strftime('%Y-%W', ...)` weeks start on Sunday and ISO-8601 doesn't, so "Week" bucket boundaries won't match a Monday-start calendar** → Mitigation: acceptable since the UI presents bucketed weeks as relative bars ("7 days ago" .. "today"), not calendar-week labels; no user-facing claim of ISO week alignment.
- **[Risk] `blood_pressure`'s two-value-column special case adds branching to the aggregation query builder** → Mitigation: contained to one function (`aggregateBloodPressure` vs `aggregateSingleColumn`); every other type shares one code path.
- **[Risk] Extracting the shared header touches every page's layout in one change, widening the diff** → Mitigation: header extraction is mechanical (move existing markup into one component, no new behavior for food/import/login), reviewed and E2E-tested alongside the rest of the change rather than split into a separate PR that would leave the app with two header implementations in the interim.

## Migration Plan

No data migration. Deploy is a single backend + frontend release, same as any other WIP deploy: `portainer.py git-deploy hcw-wip --branch feature/dashboard-instrument-panel`, validate via E2E against the WIP stack, then merge. Rollback is a normal revert — no persisted state depends on the new `?bucket=` param or the new UI.
