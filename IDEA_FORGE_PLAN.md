# HealthVault Phase 2 — Goal weight, BMI bands, and weight-trend projection

## Overview
**Target project:** [`ya-breeze/HealthVault`](https://github.com/ya-breeze/HealthVault) — not this repo.

**Status on arrival:** already clarified. This came out of a full grilling session, so every design decision below is settled with a rationale. No clarifying round is needed; it can go straight to approach/investigation.

---

## The idea

Bring the weight chart up to parity with Libra (the app HealthVault already imports CSV exports from): let the user set a **goal weight**, shade the chart with **BMI category bands**, and **project the weight trend forward** to when it crosses the goal.

This is Phase 2 of a four-phase dashboard/food-tracking initiative. Phase 1 (per-card dashboard hide/show) shipped in HealthVault#27. Phases 3 and 4 depend on this one.

## Background

- Realizes the `## Weight chart: goal weight, BMI bands, and trend projection` backlog item in HealthVault's root `todo.md`, deferred on 2026-08-19 while scoping `weight-chart-scale-and-trend`.
- Storage question already decided by `docs/adr/ADR-002-goal-weight-as-metric-type.md`: goal weight is a **new metric type** (`weight_goal`, latest-record-wins), *not* a `UserSettings` field — so goal history survives revision, which a JSON-blob overwrite would destroy.

## The discovery that changes the sizing

`todo.md` sized this "small–medium". It isn't, because of one fact neither `todo.md` nor ADR-002 accounted for:

> **There is no write path for any metric type.** `/api/data/{type}` is `GET` and `DELETE` only. Every `weight`, `height`, and other record in HealthVault arrives via webhook ingest, Health Connect import, or Libra CSV. Nothing in the app lets a user create a record.

ADR-002 justified `weight_goal`-as-metric-type on "reuses the existing per-type pattern." That holds for reads; the write side it assumed doesn't exist.

The same gap creates a **dead end on the BMI side**: ADR-002 says bands are "hidden entirely if no `height` is on file" — but a user with no height has no way to add one. As specified, the feature would silently not exist for those users, permanently, with no affordance explaining why.

So this phase necessarily includes a new write path and the entry UI to make it real.

## Settled decisions

### Scope
| | |
|---|---|
| **Shape** | One OpenSpec change covering all three sub-features — they land on the same screen and share one review/e2e pass |
| **Sub-features** | `weight_goal` metric type · four WHO BMI bands · dashed trend projection |

### Data & API
| Decision | Detail |
|---|---|
| New metric type | `weight_goal`. ~9 touch points: Go struct, `AutoMigrate`, `typeRegistry`, `mcpserver/tools.go`, `DATA_TYPES`, `TYPE_META`, en+ru i18n, `--c-weight_goal` CSS var |
| **New write path** | `POST /api/data/{type}`, **allowlisted** to `weight_goal`, `weight`, `height`. Single value + timestamp. Other types keep rejecting POST |
| Upsert | Falls out of the existing unique `(user_id, time)` index. A new goal is written at `time = now`, so "latest wins" is free |
| Visibility | `weight_goal` is first-class — own `/data/weight_goal` page, chart, delete-able record table — but **not** in `PRIMARY_METRICS`, so it gets no Vital Card |

Rejected: a fully generic POST (blood_pressure and nutrition are multi-column and can't use the single-value path, so the rule would ship with unexplained holes) and a goal-only endpoint (leaves the height dead-end intact).

No unit work is needed: storage is hardcoded `Kilograms` / `Meters` with no unit field and no user unit preference anywhere, so BMI is a clean kg/m².

### UI

All chart work stays behind the existing `dataType === 'weight'` guard in `frontend/app/data/[type]/DataTypeClient.tsx`, so no other metric's rendering path is touched. The overlays are additive `ReferenceArea`/`ReferenceLine`/`Line` elements in the branch weight already uses.

| Decision | Detail |
|---|---|
| Manual entry | One **reusable Add-record form** on each allowlisted type's page, plus a "Set goal" shortcut on the weight page. This is what actually closes the height dead-end |
| BMI bands | Four WHO categories — `<18.5` / `18.5–25` / `25–30` / `≥30` — as `ReferenceArea`s, converted to kg via the latest `height` record |
| Band Y-behaviour | Bands **clip** to the Y-domain, never expand it. Four bands span roughly BMI 15–35, which in kg is far wider than any weight chart; fitting them would flatten the data entirely |
| Goal line Y-behaviour | The Y-domain **always expands** to include the goal — see trade-off below |
| BMI readout | On the weight page: value to one decimal plus category name, from the latest raw `weight` and latest `height`. Raw, not smoothed, so the number matches the weight shown beside it |
| Height-absent rule | Bands and readout are both hidden when no `height` exists — one condition, so the two can never disagree |

Rejected: six-band full WHO scale (obese sub-classes add alarming gradients, not information, on a personal chart) and BMI as a derived metric type (would be the first computed-on-read type in a registry where every entry is stored data).

### Projection math
| Decision | Detail |
|---|---|
| Slope | Least-squares regression over the last **30 days of the EMA series**, fixed and **independent of the selected zoom** |
| Minimum data | ≥5 weight records spanning ≥14 days; below that, "Not enough data to project yet" |
| Horizon | 12 months. Flat, diverging, or crossing beyond the horizon → **no line**, and honest text: "Not on track at your current trend" |
| Zoom rules | Bands + goal line render at every zoom. The dashed projection renders at **Month and Year only** — a 7-day window can't show a months-away crossing. The ETA text renders at every zoom, so no zoom hides the actual answer |
| Fetch | Reuses the widened-lookback pattern the weight chart already does. The weight page picks up two extra GETs (latest goal, latest height) since `DataTypeClient` currently fetches only its own type |

The zoom-independent slope matters more than it looks: tying the window to the visible range would make the app answer *"when will I hit my goal?"* differently depending on a view control.

Rejected: a two-point slope (swings the ETA by months between weigh-ins even after EMA smoothing) and unconditional extrapolation (a 6-year projection makes the chart useless, and a wrong-direction trend produces a line that never appears at all).

### Testing

Add **Vitest** to the frontend, which currently has no unit-test runner at all — `emaSeries`, `computeYDomain` and `rangeForZoom` are covered only through Playwright today.

Projection, crossing, horizon, BMI and band conversion get tested as pure functions; their edge cases (flat trend, wrong direction, crossing exactly at the horizon boundary, no height on file) are trivial as unit tests and genuinely impractical to seed through a browser. This also gives the existing `emaSeries` its first unit coverage.

E2E extends `e2e/tests/data-types.spec.ts`, run against the `hcw-wip` stack as usual.

## Known trade-off, accepted deliberately

**Always expanding the Y-axis to include the goal will flatten the data for users far from their goal.** A user at 95 kg with a 70 kg goal has a data range of a few kg stretched across a 25 kg axis, so daily variation compresses toward a flat line at tight zooms — the same flattening the earlier `weight-chart-scale-and-trend` fix was written to remove.

The alternative (bounded expansion, then clip-and-mark at the axis edge) was considered and **rejected in favour of always showing the goal line**, on the grounds that the goal line is the point of the feature. Recorded here so it's a known consequence rather than a regression someone rediscovers later.

## Defect found in existing docs

**ADR-002 misstates ADR-003 and should be corrected.**

ADR-002's consequences claim Phase 3 nutrition targets "read Goal Weight, **not** the latest measured `weight`, as their weight input." ADR-003 decided the opposite — a **split**: calories/BMR from measured weight (using goal weight there would prescribe an unsafe deficit — a 100 kg user with a 70 kg goal would get a calorie target sized for someone who already weighs 70 kg), protein g/kg from goal weight.

`CONTEXT.md` already matches ADR-003. ADR-002 is the stale one, and it's still `Status: Proposed`, so it can be corrected directly rather than annotated with an update note.

## Documentation to land with the change

- `CONTEXT.md` — new glossary entries for **BMI Band**, **Trend Projection**, and **Manual Record** (the last is a genuinely new domain concept: records in HealthVault are no longer all machine-ingested)
- **ADR-005** — the allowlisted write path. Hard to reverse (public API surface), surprising without context (*why only three types?*), and the result of a real trade-off
- **ADR-002** — fix the ADR-003 contradiction above
- `todo.md` — mark the weight-chart backlog item as claimed by this change

## Explicitly out of scope

- Trend lines for other point-in-time metrics (`todo.md` candidate 4 — the EMA code is already metric-agnostic, so this is a "should we", not a "can we")
- Phase 3 profile fields and BMR nutrition targets (ADR-003)
- Phase 4 food cards and LLM recommendations (ADR-004)
- Manual entry for any metric type beyond the three allowlisted

## Follow-on

Phase 3 will need to decide what the protein target does when no `weight_goal` is set — ADR-003 flags this as open. Not this ticket's problem, but this ticket is what makes a goal exist at all.

---

*Created by Claude — decisions from a `/grilling` + `/domain-modeling` session against the HealthVault codebase on 2026-08-21. Codebase facts (registry touch points, missing write path, EMA implementation, test coverage) were verified against source, not assumed.*


### Chosen approach: Single unified change, dependency-ordered (as specified)
One OpenSpec change exactly as scoped in the issue: build the allowlisted write path and weight_goal metric type first (since BMI bands and the goal line both depend on a way to enter height/goal), then layer BMI band + trend-projection overlays onto the existing weight chart, then the Vitest suite for the pure functions, then land CONTEXT.md/ADR-005/ADR-002-fix/todo.md updates and extend the e2e spec. Single PR, single review/e2e pass, matching the issue's stated shape.

## Validation Commands
- `make lint`
- `make test`

### Task 1: Research the problem space
- [x] Identify the constraints, unknowns, and existing code this change touches
- [x] Mark completed

**Findings (verified against source, 2026-08-21):**
- No write path exists for any metric type: `backend/pkg/server/server.go:94-96` registers only `GET /data/{type}` and `DELETE /data/{type}/{id}`; no POST route touches `/data/{type}`. Manual meal entry is a separate, non-generic path (`/food/meals*`).
- Backend metric-type registry touch points: `typeRegistry`/`typeInfo` (`backend/pkg/server/api.go:25,32`), `AutoMigrate` model list (`backend/pkg/database/db.go:46-63`), a Go struct per type in the database package, and `typeTimeCol` in `backend/pkg/mcpserver/tools.go:15` (explicitly commented "keep in sync with typeRegistry").
- Frontend metric-type touch points: `DATA_TYPES` (`frontend/lib/api.ts:525`), `TYPE_META` (`frontend/lib/dataTypeMeta.ts:43`), i18n labels (`frontend/lib/i18n/en.ts:239`, `ru.ts`), CSS var pattern (`frontend/app/globals.css:25,79`, e.g. `--c-weight`).
- Weight-specific chart logic in `frontend/app/data/[type]/DataTypeClient.tsx` is gated behind `dataType === 'weight'` checks (lines 97, 188, 206, 414, 417); the actual pure functions (`computeYDomain`, `emaSeries`, `rangeForZoom`) live in `frontend/lib/dataTypeMeta.ts:130,144,156` and are imported in, not defined in, the client component.
- Frontend has no unit-test runner today — no Vitest/Jest in `frontend/package.json`, confirming Vitest must be added net-new.
- ADR-002/ADR-003 contradiction is real: ADR-002's Consequences section claims nutrition targets read Goal Weight (not measured weight) as their sole input; ADR-003's actual Decision Outcome is a split (calories/BMR from measured weight, protein g/kg from goal weight). ADR-002 is `Status: Proposed`, so it can be corrected directly.
- `todo.md` already carries the backlog item (`todo.md:26,120`), deferred 2026-08-19 while scoping `weight-chart-scale-and-trend`, matching the plan's Background section.
- No discrepancies found between the plan's stated facts and the codebase — Task 2 can proceed on the "Chosen approach" as written.

### Task 2: Implement the chosen approach
- [x] Build the prototype/investigation described under 'Chosen approach' above
- [x] Mark completed

**Scope note:** this repo's `CLAUDE.md` mandates spec-first development — no implementation code
without an approved OpenSpec change, and explicit user approval before writing code ("Never write
implementation code without a corresponding active change" / "wait for explicit user approval
before writing code"). That gate cannot be satisfied inside a single non-interactive loop
iteration, so "build the prototype" for this task means producing the actual OpenSpec change
artifacts the workflow requires, not skipping the gate to hand-write code.

Delivered:
- `openspec/changes/goal-weight-bmi-bands-trend-projection/` — `proposal.md`, `design.md` (write
  endpoint contract, BMI/projection math constants), `tasks.md` (implementation checklist for the
  next phase), and spec deltas for `data-model` (new `weight_goal` type, no-payload-lineage rule
  for manual writes), `data-api` (new `POST /api/data/{type}` requirement, updated
  `GET` type list/count), `chart-zoom-aggregation` (BMI bands, goal line, trend projection
  requirements), and `mcp-server` (registered-type count bump).
- `openspec/specs.projected/` regenerated so reviewers see old→new spec wording before archive.
- `openspec validate goal-weight-bmi-bands-trend-projection --strict` and
  `openspec validate --specs --strict` both pass.
- Pushed and opened spec-only PR: https://github.com/ya-breeze/HealthVault/pull/28

Not done in this task (by design, deferred to after spec approval): the actual Go/TS
implementation, Vitest suite, e2e extension, and doc/ADR edits described in the proposal — all
captured as `tasks.md` in the OpenSpec change, ready to execute once the spec is approved.

### Task 3: Write up what was found
- [ ] Leave a short written summary of findings, limitations, and suggested next steps
- [ ] Mark completed
