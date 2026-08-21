## Context

Weight-chart zoom/EMA behavior is already specified in `openspec/specs/chart-zoom-aggregation/spec.md`
(landed by the `weight-chart-scale-and-trend` change, archived 2026-08-19). This change extends
that same capability rather than introducing a new one, plus a small, genuinely new capability on
the write side: HealthVault has never had a user-facing "create a record" path before.

The sizing surprise driving most of this document: ADR-002 assumed the write side already existed
("reuses the existing per-type pattern" — true for reads, not for writes). This document exists to
pin the two things that would otherwise get improvised mid-implementation: the write endpoint's
exact contract, and the projection/BMI math's exact constants.

## Goals / Non-Goals

**Goals:**
- A real, working path for a user to set a goal weight and a height, closing both the goal-weight
  metric type's actual dead end and the pre-existing BMI-bands dead end.
- BMI bands, a goal line, and a trend projection that behave predictably at every zoom level.
- The write path allowlist is intentionally narrow, with a stated reason, not a silent restriction.

**Non-Goals:**
- A fully generic POST for every metric type (rejected — `blood_pressure`/`nutrition` are
  multi-column and can't use a single-value body; shipping the rule with an unexplained exception
  for two types is worse than an explicit allowlist).
- Trend lines/projection for any metric other than `weight`.
- Unit conversion or a user unit preference — storage is hardcoded `Kilograms`/`Meters` with no
  unit field anywhere in the schema, so BMI is a clean kg/m² with no conversion layer needed.

## Decisions

### Write endpoint: `POST /api/data/{type}`, allowlisted, single value + timestamp

```
POST /api/data/{type}          type ∈ {weight, height, weight_goal}
{ "value": <float64>, "time": "<RFC3339, optional, defaults to now>" }
→ 201 { ...the created record, same shape as a GET row... }
```

- `{type}` not in the allowlist → 403 (not 404 — the type is real and readable, just not
  writable), matching the existing `typeRegistry` 404-for-unknown-type behavior staying reserved
  for genuinely unregistered types.
- Value is required; a missing/non-numeric value → 400.
- Upsert is not implemented as a special case: the existing unique `(user_id, time)` index on
  each of these three tables makes a same-instant write collide naturally, and every write from
  this endpoint uses `time = now()` when the caller omits it, so "latest goal wins" falls out of
  ordinary insert-then-query-latest behavior — no new upsert logic needed.
- Target user is always `claims.UserID` — this endpoint does **not** call `resolveUser` (the
  helper `GET /api/data/{type}` uses to honor a family-sharing `?user=<member>` override) and does
  not accept `?user=` at all. This matches `DeleteRecordHandler`'s existing convention: mutations
  on this path family are always scoped to the caller, even though the sibling `GET` is
  family-sharing-aware. Without this, a family member could write a manual weight/height/goal
  record into another member's account by adding `?user=` to the request.
- Rejected: a fully generic POST across all registered types (multi-column types have no single
  `value` field to hang this contract on) and a goal-only endpoint (`POST /api/goal`) — the latter
  would leave the height dead end for BMI unfixed, which is the actual reason this write path
  exists at all.

### BMI bands and readout: computed client-side from latest raw `height` + `weight`

Four WHO categories, band edges in BMI: `18.5`, `25`, `30`. Boundaries are lower-inclusive,
matching WHO convention: Underweight `(-∞, 18.5)`, Normal `[18.5, 25)`, Overweight `[25, 30)`,
Obese `[30, ∞)` — a BMI of exactly 18.5, 25, or 30 belongs to the higher category. This applies
identically to the band `ReferenceArea` edges and to the BMI readout's category-name lookup, so
the two can never disagree at a boundary value. Convert each edge to kg via the user's latest
`height` record: `kg = bmi * heightMeters^2`. Rendered as `ReferenceArea`s that are clipped to
whatever the chart's Y-domain already is — they must never be the thing that *sets* the domain,
since 4 bands spanning roughly BMI 15-35 is, in kg, far wider than any real weight chart's data
range; letting them expand the domain would flatten the actual data to a near-flat line. The bands
render at every zoom level (Day/Week/Month/Year) — unlike the dashed trend projection, they have
no zoom restriction.

The BMI readout (value to 1 decimal + category name) uses the latest **raw** `weight` record, not
the EMA-smoothed value, so the number next to "your BMI" matches the same raw weight already shown
elsewhere on the page rather than a smoothed one that could read differently. The category-name
lookup (BMI value → one of the four category names above) SHALL be its own pure function, exported
alongside the band-edge conversion, so both are unit-testable independent of the chart component.

Both the bands and the readout are gated on a single condition — "does a `height` record exist" —
so there's no way for one to render while the other silently doesn't.

### Goal line Y-domain: always expands (accepted trade-off, see proposal.md)

Unlike BMI bands, the goal `ReferenceLine`'s value **is** included when computing the chart's
Y-domain (reusing `computeYDomain` from `dataTypeMeta.ts`, called with the goal value folded into
the same min/max input the existing EMA/raw-data domain computation already uses). This is a
one-line change in the value set fed to the existing domain function, not a new domain algorithm —
but it is **one such change in two places**, not one: `DataTypeClient.tsx` already computes two
independent domains selected by zoom (`dayDomain` for Day zoom's `LineChart`, `bandDomain` for the
Week/Month/Year `ComposedChart`), so the goal value must be folded into both, and the
`ReferenceLine`/BMI-band `ReferenceArea`s must be added to both chart branches. "Renders at every
zoom level" (goal line and BMI bands alike) is only true if both branches carry the overlay —
covering just one leaves the other zoom tier silently missing it.

### Trend projection: least-squares over a fixed 30-day EMA window, 12-month horizon

```
window = last 30 calendar days of the existing EMA series (dataTypeMeta.ts's emaSeries),
         fixed regardless of the chart's selected zoom
slope, intercept = ordinary least-squares fit of (day_offset, ema_value) over that window
```

- **Minimum data**: fewer than 5 weight records, or a span under 14 days between the earliest and
  latest record — render "Not enough data to project yet," no line, no ETA.
- **Direction check**: if the slope doesn't move toward the goal (flat, or moving away), or the
  projected crossing date is beyond the 12-month horizon — render no line, and "Not on track at
  your current trend." This is a deliberate refusal to extrapolate: an unconditional projection
  either produces a 6-year line (useless) or, for a wrong-direction trend, simply never crosses
  the goal at all, which needs to say so rather than draw nothing with no explanation.
- **Horizon is a fixed day count, not calendar-month arithmetic**: "12 months" = 365 days from the
  most recent day in the 30-day EMA window (the regression's last `day_offset`). A crossing at
  `day_offset <= 365` from that point is within the horizon; beyond it is not. Pinning this as a
  day count (rather than leaving it to calendar-month arithmetic, which varies 28-31 days per
  month) gives the boundary-exactly-at-the-horizon unit test a single unambiguous expected value.
- **Zoom-independence is load-bearing**: tying the regression window to the currently-visible zoom
  range would make "when will I hit my goal?" answer differently depending on a view control,
  which isn't a real change in the underlying trend.
- **Rendering vs. text are decoupled**: the dashed projection `Line` itself renders only at Month
  and Year zoom (a 7-day Week view has no room to show a months-out crossing point), but the ETA
  text renders at every zoom level including Day/Week, so switching zoom never hides the answer to
  the question the feature exists to answer.
- **The 30-day EMA window needs its own fetch, independent of the active zoom's bucket**: today,
  `DataTypeClient.tsx` only fetches daily-bucketed weight data for Week and Month zoom (Day zoom
  fetches no bucketed data at all — `chartRows` is empty — and Year zoom's bucket is monthly, not
  daily). Since the ETA text must render at every zoom level, the projection cannot rely on
  whatever bucketed data a given zoom already happens to have loaded. The weight page adds one more
  dedicated fetch — a 30+-day daily-bucketed range, independent of `zoom`/`bucket` — purely to feed
  the regression, alongside the existing per-zoom chart fetch and the goal/height fetches from
  tasks 5.5 and 6.1.
- Rejected: a two-point slope (last two weigh-ins) — swings the ETA by months between individual
  weigh-ins even after EMA smoothing, which is worse than the 30-day window's stability.

## Risks / Trade-offs

- **Goal-line Y-domain expansion flattens data for users far from goal** — accepted, see
  proposal.md's "Known trade-off" section. Not re-litigated here.
- **Allowlist is a public API surface change that's hard to reverse once clients depend on it** —
  this is exactly why it gets its own ADR-005 rather than being folded into ADR-002.

## Migration Plan

Mostly additive: one new table (`weight_goal`), one new route. No existing route or response shape
changes. No backfill needed — an empty `weight_goal` table means "no goal set," which the frontend
already has to handle as its default state.

One exception is not purely additive: the pre-existing `weights` and `heights` tables have a
`NOT NULL` `source_payload_id` column (every row today is ingested, never manually written). To
honor the "manual writes SHALL NOT require or synthesize a `source_payload_id`" rule for these two
types, that column becomes nullable (`*uuid.UUID` in Go) on both existing tables. This is a
column-relaxation migration, not a purely additive one — no existing row is affected (they all
already have a non-null value) and no data loss occurs, but it is a schema change on tables that
predate this change, so it needs its own migration step rather than falling out of `AutoMigrate`
adding a new table.

## Open Questions

None outstanding — this document reflects decisions already settled in a `/grilling` +
`/domain-modeling` session prior to this proposal (see `IDEA_FORGE_PLAN.md` in the originating
branch for that session's full rationale and rejected alternatives).
