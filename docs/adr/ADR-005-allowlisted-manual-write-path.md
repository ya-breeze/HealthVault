# ADR-005: Manual-Write Endpoint Restricted to an Explicit Allowlist

## Status
Accepted

## Context and Problem Statement

HealthVault's metric records have always been created by ingestion (CSV import, MCP tool
calls, food-photo recognition) — there has never been a user-facing "create a record" endpoint.
This change needs one, at minimum for `weight_goal` (which has no other way to receive data) and
`height` (a pre-existing dead end: logged nowhere manually, blocking BMI bands). Should the new
write path accept any registered metric type, or only some?

## Decision Drivers

- Only `weight`, `height`, and `weight_goal` are simple, single-value-per-timestamp metrics;
  `blood_pressure` and `nutrition` are multi-column and have no single `value` field to hang a
  generic `{value, time}` body on.
- Shipping a rule with an unexplained exception for two types is worse than an explicit, stated
  allowlist.
- The type is already registered and readable via `GET`; a disallowed write on it should read as
  "not writable" (403), not "doesn't exist" (404) — `GET`/`DELETE` already reserve 404 for
  genuinely unregistered types, and this endpoint keeps that meaning intact.
- This is a public API surface change that is hard to reverse once clients (the frontend, a
  future MCP tool, external scripts) start depending on its shape.

## Considered Options

- **A fully generic `POST` across every registered type** — rejected: multi-column types
  (`blood_pressure`, `nutrition`) have no single `value` to write, so the endpoint would still
  need type-specific body shapes, defeating the point of one generic route.
- **A goal-only endpoint (`POST /api/goal`)** — rejected: it would leave `height`'s existing dead
  end unfixed, which is the actual reason a write path exists at all (BMI bands need a way to set
  height).
- **`POST /api/data/{type}`, allowlisted to `weight`/`height`/`weight_goal`** — reuses the
  existing per-type route shape; a single `{value, time?}` body works for all three, since all are
  single positive-quantity point metrics; the allowlist is a small, stated set that can grow later
  without a redesign.

## Decision Outcome

Chosen: **`POST /api/data/{type}`, allowlisted to `weight`, `height`, `weight_goal`**.
Registered-but-not-allowlisted types (e.g. `steps`, `blood_pressure`) return 403; unregistered
types return 404, matching `GET`/`DELETE`'s existing unknown-type behavior. The target user is
always `claims.UserID` — this endpoint does not call `resolveUser` and does not honor `?user=`,
so a family member cannot write into another member's account through it, matching
`DeleteRecordHandler`'s convention.

### Consequences

- Adding a fourth writable type later (another single-value point metric) means adding it to the
  allowlist and its `TYPE_META`/form wiring — no endpoint redesign.
- Multi-column types (`blood_pressure`, `nutrition`) remain ingestion-only until a separate,
  type-specific write contract is designed for them; this ADR does not attempt that.
- The pre-existing `weights`/`heights` tables' `NOT NULL source_payload_id` column had to become
  nullable to support manual (non-ingested) rows — a schema change on tables that predate this
  decision (see this change's `design.md` Migration Plan).
