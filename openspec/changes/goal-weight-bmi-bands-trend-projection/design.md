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
- Rejected: a fully generic POST across all registered types (multi-column types have no single
  `value` field to hang this contract on) and a goal-only endpoint (`POST /api/goal`) — the latter
  would leave the height dead end for BMI unfixed, which is the actual reason this write path
  exists at all.

### BMI bands and readout: computed client-side from latest raw `height` + `weight`

Four WHO categories, band edges in BMI: `18.5`, `25`, `30` (open below 18.5 and above 30). Convert
each edge to kg via the user's latest `height` record: `kg = bmi * heightMeters^2`. Rendered as
`ReferenceArea`s that are clipped to whatever the chart's Y-domain already is — they must never be
the thing that *sets* the domain, since 4 bands spanning roughly BMI 15-35 is, in kg, far wider
than any real weight chart's data range; letting them expand the domain would flatten the actual
data to a near-flat line.

The BMI readout (value to 1 decimal + category name) uses the latest **raw** `weight` record, not
the EMA-smoothed value, so the number next to "your BMI" matches the same raw weight already shown
elsewhere on the page rather than a smoothed one that could read differently.

Both the bands and the readout are gated on a single condition — "does a `height` record exist" —
so there's no way for one to render while the other silently doesn't.

### Goal line Y-domain: always expands (accepted trade-off, see proposal.md)

Unlike BMI bands, the goal `ReferenceLine`'s value **is** included when computing the chart's
Y-domain (reusing `computeYDomain` from `dataTypeMeta.ts`, called with the goal value folded into
the same min/max input the existing EMA/raw-data domain computation already uses). This is a
one-line change in the value set fed to the existing domain function, not a new domain algorithm.

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
- **Zoom-independence is load-bearing**: tying the regression window to the currently-visible zoom
  range would make "when will I hit my goal?" answer differently depending on a view control,
  which isn't a real change in the underlying trend.
- **Rendering vs. text are decoupled**: the dashed projection `Line` itself renders only at Month
  and Year zoom (a 7-day Week view has no room to show a months-out crossing point), but the ETA
  text renders at every zoom level including Day/Week, so switching zoom never hides the answer to
  the question the feature exists to answer.
- Rejected: a two-point slope (last two weigh-ins) — swings the ETA by months between individual
  weigh-ins even after EMA smoothing, which is worse than the 30-day window's stability.

## Risks / Trade-offs

- **Goal-line Y-domain expansion flattens data for users far from goal** — accepted, see
  proposal.md's "Known trade-off" section. Not re-litigated here.
- **Allowlist is a public API surface change that's hard to reverse once clients depend on it** —
  this is exactly why it gets its own ADR-005 rather than being folded into ADR-002.

## Migration Plan

Additive only: one new table (`weight_goal`), one new route. No existing table, route, or response
shape changes. No backfill needed — an empty `weight_goal` table means "no goal set," which the
frontend already has to handle as its default state.

## Open Questions

None outstanding — this document reflects decisions already settled in a `/grilling` +
`/domain-modeling` session prior to this proposal (see `IDEA_FORGE_PLAN.md` in the originating
branch for that session's full rationale and rejected alternatives).
