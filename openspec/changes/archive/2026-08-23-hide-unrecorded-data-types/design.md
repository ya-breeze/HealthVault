## Context

Today `frontend/app/page.tsx` renders all 8 `PRIMARY_METRICS` as vitals cards and all 18 remaining `DATA_TYPES` (`frontend/lib/api.ts`) as "More Data" links, unconditionally — no fetch decides whether a type has ever had data. Phase 1 (`openspec/changes/archive/2026-08-21-dashboard-card-visibility`) added a per-card, user-controlled hide/show preference to the 8 vitals cards only, orthogonal to whether the type has data. The closed idea-5 attempt tried to add data-driven hiding client-side, using a 7-day recency window per type (wrong signal — see proposal) and 18 requests to compute it (too expensive). Full findings: `docs/investigations/idea-6-hide-data-types-with-no-data-at-all.md`.

## Goals / Non-Goals

**Goals:**
- Hide a type everywhere on the dashboard (vitals grid and More Data) if the resolved user has never recorded a row of it, computed over all time.
- One backend request for the whole dashboard, not one per type.
- Preserve Phase 1's user hide/show preference for types that do have data.
- Fail open: any doubt about presence (fetch error, incomplete response) means "show it."

**Non-Goals:**
- Recency-based hiding — the existing 7-day "no data" sparkline placeholder for a metric that has data but not recently is unchanged.
- A settings/customize surface for secondary types — More Data stays a plain filtered link list, no new edit mode.
- Caching/precomputing presence (e.g. in `UserSettings`) — out of scope; a per-load query is the default until measurement says otherwise.

## Decisions

**Endpoint shape**: `GET /api/data-types/presence`, returning `map[string]bool` keyed by the same type names `DATA_TYPES` already uses, one entry per registered type on every `200`. Rejected alternative: an array of present-only type names — a map lets a partial/malformed response (missing key) be distinguished from a genuine `false`, which the frontend's fail-open handling depends on (see below). Placed in the `data-api` capability, alongside `/api/data/summary` as the other dashboard-support aggregate endpoint — not in `dashboard-ui`, since it is a backend HTTP contract like the rest of that capability's requirements.

**Query shape**: a sequential loop over the backend's `typeRegistry`, one indexed `COUNT`-based existence check per type against `storage.DB()` directly (no new `Storage` interface method), mirroring `NeedsAttentionCount`'s precedent for querying `DB()` directly while following `DataHandler`/`summaryHandler`'s closure-over-`storage` shape for where the code lives and how it's registered. 26 sequential indexed round-trips is negligible at this project's personal-deployment scale (see repo-wide `/data/CLAUDE.md`, "Scale") — not worth goroutines or a hand-written `UNION ALL` without a measurement showing otherwise. Resolves the target user via `resolveUser(r, storage, claims.UserID, FamilyIDFromCtx(r))`, matching its siblings' family-member support via `?user=`.

**Presence excludes a type from the customizable set entirely, not just defaults it to hidden.** This is the one open question the investigation explicitly left for this proposal (docs/investigations/…, "Suggested next steps" #5). Two options existed:
1. Treat zero-presence the same as a Phase-1 user-hidden card: still listed (dimmed) in edit mode, so it *could* be toggled visible.
2. Exclude it from edit mode entirely — there is nothing to reorder or reveal, since toggling it visible would still show nothing (no data to render).

Chosen: **option 2.** Option 1 produces a confusing dead end — a user opens Customize, sees "VO2 max" as toggleable, toggles it visible, clicks Done, and it still doesn't appear, because presence (a data fact) can't be overridden by a preference (a display choice). Excluding it entirely means edit mode only ever shows metrics that can actually render, and "all cards hidden" (Phase 1's placeholder) and "nothing recorded" (this change's new placeholder) stay cleanly distinguishable by construction: the former can only occur when at least one metric has presence.

**Two empty states, not one.** `vitals-grid-empty` (existing, Phase 1) means "you have data-bearing metrics but hid all of them via Customize." The new `vitals-grid-empty-no-data` means "you have never recorded any of the 8 primary metrics" — Customize cannot help, so its copy must not point there. These are mutually exclusive by construction (option 2 above: `vitals-grid-empty` can only render when ≥1 metric has presence).

**More Data collapses, not renders empty.** Unlike the vitals grid (which always shows *some* placeholder), the More Data section has no placeholder state today and none is being added — if no secondary type has presence, the section (and its `data-testid="more-data"` wrapper) simply doesn't render. Secondary types have no Phase-1-style user preference layer, so presence is the only filter there.

**Fail-open contract**: a presence fetch error means every type (primary and secondary) is treated as present — identical to today's unconditional-render behavior, so a backend outage degrades to "no hiding" rather than "everything hidden." A `200` response missing an entry for some type is treated identically (shown), not coerced to `false` by a naive "intersect with true-valued keys" implementation — the investigation calls this out explicitly as the easy-to-get-wrong case, since the endpoint contract guarantees a complete map on success but a client-side bug could still read an absent key as `false`.

## Risks / Trade-offs

- **26 sequential DB round-trips per dashboard load, added to the existing dashboard fetches** — accepted at this project's scale; flagged in the investigation as worth revisiting with concurrency or a `UNION ALL` only if a future measurement shows it matters.
- **Excluding zero-presence types from edit mode (option 2) means a user cannot "pre-stage" a type as visible before ever recording it** — accepted; there is nothing meaningful to stage, since the card would still render nothing until data exists, and presence is re-evaluated every load, so the first recorded row naturally makes the type eligible without any user action.
- **No caching of presence** — a user with 26 registered types recording data for the first time in a new type sees it appear on their very next dashboard load (presence is recomputed per load), at the cost of repeating the same 26 checks on every load even though presence flips at most 26 times per user, ever. Deferred; see Non-Goals.

## Migration Plan

Purely additive: a new endpoint, no changed response shape on any existing endpoint, no schema change. Ship backend and frontend together (the frontend change depends on the endpoint existing); no deploy-order constraint beyond that. No rollback hazard — reverting the frontend stops calling the endpoint; reverting the backend makes the frontend's presence fetch fail, which fails open to today's "show everything" behavior by design.
