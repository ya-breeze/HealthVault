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
Rejected alternative: aggregate client-side after fetching raw records for the full range. Ruled out once `hcw-prod` numbers showed `heart_rate` at ~2,200 rows/day — a Year view would need ~580k rows fetched, parsed, and reduced in the browser. Instead `GET /api/data/{type}?bucket=day|month` does the `GROUP BY` in SQLite and returns one row per bucket. There is no `week`-sized bucket: the UI's Week and Month zoom levels both use `day` buckets and differ only in requested time range (see `chart-zoom-aggregation`'s zoom→bucket table), so a separate week granularity would be dead code with no caller. Omitting `?bucket=` is unchanged (raw records), so this is additive, not breaking — `e2e/tests/data-types.spec.ts`'s existing raw-record assertions keep working; new tests are added for the bucketed path.

### Two aggregation families, dispatched by a static type→family map
`typeRegistry` in `backend/pkg/server/api.go` gets a second field, `family: "cumulative" | "point"`, alongside the existing `table`/`timeCol`. 25 of the 26 registered types get a family — every type except `food_meal`, which never accepts `?bucket=` (see `data-api` spec) and so carries no family at all, not an empty/placeholder one. The two easy to overlook among the 25: `height` is `point` (rarely changes, but shares `weight`'s single-value-column shape), and `nutrition` is `cumulative` despite having seven value columns instead of one — handled the same way as `blood_pressure`'s two-column case below, just with more columns. `sleep` is `cumulative` (its meaningful op is a nightly sum of `duration_seconds`) despite reading like a point-in-time series; `blood_pressure` is `point` but has two value columns (`systolic`, `diastolic`) instead of one, handled as a special case in the aggregation query builder rather than a third family, since every other `point` type has exactly one value column.

### Bucketing via SQLite `strftime`, not a Go-side loop
`GROUP BY strftime('%Y-%m-%d', time_col)` for `day`, `strftime('%Y-%m', time_col)` for `month`, then `SUM`/`AVG`/`MIN`/`MAX` in the same query. Rejected alternative: fetch raw rows and reduce in Go. SQLite's aggregate functions on an indexed time column push the reduction to the database instead of loading every row into the Go process — same transfer-size problem as client-side aggregation, one layer down.

Every type's `timeCol` is covered by that type's `uniqueIndex:idx_<type>_user_time` **except `sleep`**: `typeRegistry["sleep"].timeCol` is `start_time`, but `Sleep`'s unique index (`idx_sleeps_user_time`) is on `(user_id, session_end_time)` — `StartTime` carries no index at all, so `sleep` aggregation does a full table scan. This is deliberately left as-is rather than fixed by adding an index (which would be a schema change, a stated Non-Goal): `hcw-prod` has 709 `sleep` rows total, so a full scan costs nothing in practice. If `sleep` volume ever approaches `heart_rate`'s scale, revisit.

### One design-token module, not per-component styling
`app/lib/tokens.ts` (or `app/globals.css` `@theme` block) defines the graphite palette, type scale, and **one accent color per registered type — all 26, not just the 8 vitals-grid metrics** (`dashboard-ui`'s "Consistent per-metric color" requirement is app-wide: every data page, including the ~18 types with no dashboard card, needs a defined color for its own chart). The dashboard vitals grid, every data-page chart, and the shared header all import from this one module. Rejected alternative: let each component hardcode its own Tailwind classes as today — that's exactly how the app ended up with an unconsidered, inconsistent default look.

### `logged_at` time-column detection is pre-existing, not new work
The `data-api` delta spec's "Generic data query endpoint" requirement carries over a sentence about `logged_at` being recognized by the frontend's time-column detection. That behavior already shipped with the food-logging change (`DataTypeClient.tsx`'s detection array already includes it) — it's repeated here only because a MODIFIED requirement must restate the full requirement text, not because this change adds it. `tasks.md` has no task for it.

### Icon set as inline SVG React components, not an icon library dependency
A handful of icons (camera, pencil, link, import, logout, chevron) are needed. Hand-rolled inline SVG components avoid adding a new frontend dependency for ~6 icons and keep bundle size and stroke-weight consistent with the design tokens.

## Risks / Trade-offs

- **[Risk] UTC-only bucketing shifts "day"/"month" boundaries relative to the user's local midnight** → Mitigation: acceptable for a single-timezone dogfood deployment; documented as a known simplification, not silently wrong. Revisit if the family-sharing feature gains users in another timezone.
- **[Risk] `blood_pressure` and `nutrition`'s multi-value-column special cases add branching to the aggregation query builder** → Mitigation: contained to dedicated functions (`aggregateBloodPressure`, `aggregateNutrition` vs. `aggregateSingleColumn`); every other type shares one code path.
- **[Non-risk, checked] Soft-deleted rows silently inflating a bucket's SUM/AVG/MIN/MAX** — considered and ruled out: `DeleteRecordHandler` → `storage.DeleteRecord` issues a raw `DELETE FROM <table> WHERE id = ? AND user_id = ?` (`storage_impl.go`), not GORM's soft-delete `.Delete()`. No code path in this backend ever populates `deleted_at` on a health-metric row (grepped every `.Delete(` call; the only two touch `FoodItem` and `CustomFood`, both outside `typeRegistry`). Deletion for every aggregatable type is hard-delete, so there is no soft-deleted row for an aggregate query to include, whether or not GORM's soft-delete scope fires on a `.Table()` + map-destination query.
- **[Risk] Extracting the shared header touches every page's layout in one change, widening the diff** → Mitigation: header extraction is mechanical (move existing markup into one component, no new behavior for food/import/login), reviewed and E2E-tested alongside the rest of the change rather than split into a separate PR that would leave the app with two header implementations in the interim.

## Migration Plan

No data migration. Deploy is a single backend + frontend release, same as any other WIP deploy: `portainer.py git-deploy hcw-wip --branch feature/dashboard-instrument-panel`, validate via E2E against the WIP stack, then merge. Rollback is a normal revert — no persisted state depends on the new `?bucket=` param or the new UI.
