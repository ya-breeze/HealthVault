## Why

The weight chart lags Libra (the app HealthVault already imports CSV exports from): there's no
way to set a goal weight, no BMI reference bands, and no projection of when the current trend will
reach the goal. This is Phase 2 of the four-phase dashboard/food-tracking initiative (Phase 1,
per-card dashboard hide/show, shipped in HealthVault#27; Phases 3-4 depend on this one).

Storage for goal weight was already decided in `docs/adr/ADR-002-goal-weight-as-metric-type.md`:
a new metric type (`weight_goal`, latest-record-wins), not a `UserSettings` field, so goal history
survives revision.

What neither `todo.md` nor ADR-002 accounted for: **there is no write path for any metric type.**
`/api/data/{type}` is `GET` and `DELETE` only — every record arrives via webhook ingest, Health
Connect import, or Libra CSV. Nothing lets a user create one. This also creates a dead end on the
BMI side: ADR-002 says bands are "hidden entirely if no `height` is on file," but a user with no
height has no way to add one. This change therefore includes a new write path and the entry UI
that makes both goal weight and height genuinely settable, not just a metric type that nothing can
populate.

## What Changes

- **New metric type `weight_goal`** — point-in-time, single `kilograms` value, same
  `(user_id, time)` uniqueness as `weight`/`height`. Registered through every existing per-type
  touch point (Go struct, `AutoMigrate`, `typeRegistry`, MCP `typeTimeCol`, frontend
  `DATA_TYPES`/`TYPE_META`, en/ru i18n, `--c-weight_goal` CSS var). Visible as its own
  `/data/weight_goal` page (chart, delete-able record table) but **excluded** from
  `PRIMARY_METRICS`, so it gets no dashboard Vital Card.
- **New write path**: `POST /api/data/{type}`, allowlisted to `weight_goal`, `weight`, `height`
  only (single-value point-in-time types only — `blood_pressure` and `nutrition` are multi-column
  and excluded). Body is a single value + timestamp. Every other registered type continues to
  reject POST. A manual write carries no `source_payload_id` (it has no upstream webhook/import
  payload), mirroring the existing exception already made for user-authored food-logging tables.
- **Reusable "Add record" form** on each allowlisted type's page, plus a "Set goal" shortcut on
  the weight page — this is what actually closes the height dead end, not just the goal-weight
  one.
- **BMI reference bands** on the weight chart: four WHO categories (`<18.5`, `18.5–25`, `25–30`,
  `≥30`), converted to kg via the user's latest `height` record, rendered as `ReferenceArea`s that
  clip to the Y-domain rather than expanding it. Hidden entirely (band + a raw BMI readout) when
  no `height` exists — one condition governs both, so they can't disagree.
- **Goal line**: a `ReferenceLine` at the latest `weight_goal` value. Unlike BMI bands, the
  Y-domain always expands to include it — a known, deliberate trade-off (see Known trade-off
  below).
- **Dashed trend projection**: least-squares regression over the last 30 days of the existing EMA
  series (fixed, independent of the selected zoom), projected to a 12-month horizon. Requires ≥5
  weight records spanning ≥14 days, else "Not enough data to project yet." Flat, diverging, or
  crossing beyond the horizon renders no line plus "Not on track at your current trend" — a
  wrong-direction trend never silently produces a plausible-looking line. Bands and the goal line
  render at every zoom; the projection line itself renders only at Month/Year zoom (a 7-day window
  can't show a months-away crossing), but the ETA text renders at every zoom so no zoom hides the
  answer.
- **Vitest** added to the frontend (currently has no unit-test runner at all). Projection,
  crossing/horizon, BMI conversion, and band computation ship as pure functions with unit tests;
  this also gives the existing `emaSeries`/`computeYDomain`/`rangeForZoom` their first unit
  coverage.
- Doc fixes landing with this change: `CONTEXT.md` glossary entries for **BMI Band**, **Trend
  Projection**, and **Manual Record**; new **ADR-005** for the allowlisted write path; a direct
  correction to **ADR-002** (still `Status: Proposed`), whose Consequences section currently
  misstates ADR-003 as "nutrition targets read Goal Weight only" when ADR-003's actual decision is
  a split (calories/BMR from measured weight, protein g/kg from goal weight); `todo.md` marked as
  claimed by this change.

All chart work stays behind the existing `dataType === 'weight'` guard in
`frontend/app/data/[type]/DataTypeClient.tsx`, so no other metric's rendering path is touched.

## Known trade-off, accepted deliberately

Always expanding the Y-axis to include the goal will flatten the data for users far from their
goal (e.g. 95 kg current, 70 kg goal: a few kg of real variation stretched across a 25 kg axis) —
the same flattening the `weight-chart-scale-and-trend` change (2026-08-19) was written to remove.
A bounded-expansion-then-clip alternative was considered and rejected in favor of always showing
the goal line, since the goal line is the point of this feature. Recorded so it reads as a known
consequence, not a regression rediscovered later.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `data-model`: adds the `weight_goal` type and documents that manually-written records (via the
  new write path) carry no `source_payload_id`.
- `data-api`: adds `POST /api/data/{type}` (allowlisted) and registers `weight_goal` in the
  generic query endpoint's supported-type list.
- `chart-zoom-aggregation`: adds BMI reference bands, a goal-weight reference line, and a dashed
  trend-projection line/ETA text to the `weight` chart.
- `mcp-server`: `weight_goal` becomes queryable through `query_data` like any other registered
  type (registered-type count bump only; no tool behavior change).

## Impact

- Backend (Go): `backend/pkg/database/` (new `WeightGoal` struct, `AutoMigrate` entry),
  `backend/pkg/server/api.go` (`typeRegistry`, new POST handler + allowlist),
  `backend/pkg/mcpserver/tools.go` (`typeTimeCol`).
- Frontend (TS/React): `frontend/lib/api.ts` (`DATA_TYPES`), `frontend/lib/dataTypeMeta.ts`
  (`TYPE_META`, new pure functions for BMI/band/projection math), `frontend/lib/i18n/{en,ru}.ts`,
  `frontend/app/globals.css` (`--c-weight_goal`), `frontend/app/data/[type]/DataTypeClient.tsx`
  (new Add-record form, BMI/goal/projection rendering — all behind the existing `weight` guard).
- New frontend dependency: Vitest (dev-only, test runner).
- `e2e/tests/data-types.spec.ts` extended for the new write path and chart overlays.
- Docs: `CONTEXT.md`, `docs/adr/ADR-005-*.md` (new), `docs/adr/ADR-002-*.md` (correction),
  `todo.md`.
