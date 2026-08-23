# Investigation: Hide data types with no data at all — decided in the backend

Research notes for idea-forge plan
`idea-6-hide-data-types-with-no-data-at-all-deci-investigate.md`.
Supersedes idea-5 (`docs/investigations/idea-5-hide-items-without-data.md` on
the unmerged, non-archived branch
`feature/idea-5-hide-items-without-data-in-healthvault`), whose client-side,
7-day-window approach was rejected. Goal here: hide vitals/"more data" types
that have **never** had a record, computed server-side.

## Relevant code

- `frontend/app/page.tsx` — Dashboard. Vitals grid at lines 187-206
  (`data-testid="vitals-grid"`), from `order` (state built by
  `reconcileMetricOrder`, `frontend/lib/vitals.ts`) over the 8
  `PRIMARY_METRICS`. "More Data" pills at lines 253-268
  (`SECONDARY_TYPES.map(...)`, no test id on the section itself today),
  the remaining 18 of the 26 `DATA_TYPES`, computed once by static set
  subtraction — no fetch at all today.
- `frontend/lib/vitals.ts` — `PRIMARY_METRICS` (8 types), `extractVital()`
  (line 118) already returns `null` on `rows.length === 0`, but that only
  swaps in a "no data" placeholder inside `VitalCard`, never hides the card.
- `frontend/lib/api.ts` — `DATA_TYPES` (line ~525, 26 types, frontend mirror
  of the backend registry below).
- `backend/pkg/server/api.go` lines 32-67 — `typeRegistry`, the backend's
  26-type source of truth (`{table, timeCol, family, valueCol}` per type).
- `backend/pkg/database/storage.go` — `Storage` interface. No existing
  "has any record" query; closest pattern is `NeedsAttentionCount` in
  `backend/pkg/server/food_meal_detail.go` (lines 344-367), a single
  `COUNT(*)` per call with no time bound.
- `backend/pkg/database/models.go` — every telemetry table has
  `uniqueIndex:idx_<table>_user_time` covering `(user_id, time_col)`;
  `food_meal` (`models_food.go` line 43) has a plain `index` on `user_id`.
  So an unbounded "does this user have any row of this type" check is
  index-covered for all 26 types — cheap even as 26 separate queries.
- `backend/pkg/server/server.go` lines 82-116 — router registration pattern
  (e.g. `/api/food/meals/needs-attention-count`) to model a new route on.
- Phase 1 hide/show (`c9c955b`, archived at
  `openspec/changes/archive/2026-08-21-dashboard-card-visibility`) —
  `DashboardCardPref`/`reconcileMetricOrder`, orthogonal explicit-preference
  mechanism scoped only to the 8 vitals cards; explicitly does not touch
  More Data. Its loaded-flag gating pattern (`settingsStatus`,
  `page.tsx` lines 29-43, 167-186) is the template for avoiding a flash of
  hidden-then-shown cards here.
- `openspec/specs/dashboard-ui/spec.md` lines 32-34, "Missing data for a
  metric" — codifies today's "show placeholder, don't hide" behavior; will
  need to change.
- `e2e/tests/dashboard.spec.ts` — tests vitals-grid states (all-8-render,
  loading/error/retry, Phase 1 reorder/hide-show, all-hidden placeholder)
  via existing testids `vitals-grid`, `vitals-grid-empty`,
  `vitals-grid-loading`, `vitals-grid-error`, `vitals-grid-retry`. **Nothing
  tests More Data today.** The testids named in the plan
  (`vitals-grid-empty-no-data`, `more-data`) **do not exist anywhere in the
  current tree** — they're carried over from the idea-5 plan document as
  proposed names, not leftover dead code from a merged attempt. Note
  `vitals-grid-empty` is already taken for a different meaning ("user hid
  every card via Customize"), so a new no-data empty state needs its own,
  non-colliding id.

## Constraints and unknowns

- **No zero-value ambiguity carries over from idea-5**: `/api/data/{type}`
  already normalizes "no matching rows" to `[]`, distinct from a legitimate
  zero-valued row. A presence signal (boolean per type) is a strictly
  simpler derivative of that same fact, just unbounded in time instead of
  windowed.
- **"Ever" vs. "7 days"**: this is the one behavioral question idea-5 left
  unresolved that idea-6 resolves by design — presence must be computed
  over all time, not the dashboard's 7-day sparkline window. This means the
  presence query cannot reuse `QueryRecords`/`QueryAggregate`'s
  `TimeRange`-scoped shape as-is; it needs an unbounded existence check
  (`EXISTS`/`LIMIT 1`/`COUNT(*) > 0`) per type.
- **Shape of the new endpoint**: a dedicated `GET /api/data-types/presence`
  (or similar) returning `{type: hasData}` for all 26 types is cleaner than
  extending `/api/data/summary` (`api.go` ~line 243), which is a fixed
  3-metric (steps/avgHR/sleep) shape not naturally generalizable to 26
  unbounded existence checks. A dedicated endpoint also composes with the
  `typeRegistry` directly (iterate all registered types), stays
  independently testable/cacheable, and is reusable from a future
  Customize/settings page without coupling to the dashboard payload's own
  evolution — matching the "Chosen approach" already selected in the plan.
- **Query cost**: 26 indexed per-user-per-table lookups is cheap at this
  project's scale (small, personal deployments — see repo-wide
  `/data/CLAUDE.md`), but a single request doing 26 sequential round-trips
  to the DB is still 26x more DB calls than any other endpoint in this
  codebase makes. Worth deciding at implementation time whether to run them
  concurrently (goroutines) or as one query (e.g. `UNION ALL` of 26
  single-column `EXISTS` subqueries) — no precedent for either in the
  current codebase, so this is a genuine design choice, not a "copy the
  existing pattern" one.
- **Fail-open contract**: per the plan, a failed presence fetch must never
  read as "no data" for any type — the frontend must treat a presence
  fetch error as "show everything," matching idea-5's fail-open finding but
  now for a single request instead of per-type ones (simpler: one
  fetch to gate instead of two independent `vitalsLoaded`/`secondaryLoaded`
  flags).
- **Two distinct empty states**: "user hid every card via Customize"
  (existing `vitals-grid-empty`, Phase 1) vs. "nothing has ever been
  recorded" (new). These need different copy/guidance (Customize can't fix
  the latter) and must not collide on the same testid — confirmed as a real
  design need, not just a plan aspiration, since Phase 1's placeholder
  already exists and already means the first thing.
- **Interaction with Phase 1 hide/show**: presence-based hiding and
  explicit user hide/show are orthogonal filters (data fact vs. user
  preference) and can compose as sequential filters, same conclusion idea-5
  reached — no new conflict found for the backend-endpoint version of this.
- **`food_meal`**: its presence check is a plain `COUNT`/`EXISTS` on
  `food_meals` filtered by `user_id` only (no bucket/aggregation concerns
  apply to a presence check, unlike its `?bucket=` incompatibility for the
  data endpoint) — no special-casing needed beyond what `NeedsAttentionCount`
  already demonstrates.
- **Frontend integration point**: the presence fetch needs to run alongside
  the existing dashboard fetches in `page.tsx` and gate both the vitals
  grid (intersected with `PRIMARY_METRICS`) and the More Data pills
  (intersected with `SECONDARY_TYPES`) from the same one response, replacing
  idea-5's two separate `Promise.all` fetches with a single call.

## Conclusion

The chosen approach (dedicated backend presence endpoint) is viable and
addresses both problems idea-5's client-side prototype ran into: it answers
"has this type ever had a row" directly instead of proxying through a 7-day
window, and it costs one request instead of eighteen. The backend has no
existing unbounded-existence query to reuse verbatim, but every candidate
table already carries a `(user_id, time)` index that makes such a query
cheap, and `NeedsAttentionCount` is a close structural precedent for a new
single-purpose, auth-gated aggregate route. The main open implementation
decisions are the concurrency/query shape for checking 26 tables in one
request, and the exact JSON shape of the response — everything else
(fail-open contract, two empty states, composition with Phase 1, `food_meal`
handling) has a clear answer already, either carried over from idea-5's
findings or resolved by design in this plan's "What I want instead."

## Findings write-up

### What was found

- The backend's `typeRegistry` (`api.go`) is the natural source of truth to
  drive a presence endpoint from — iterating its 26 entries and, per entry,
  running `SELECT EXISTS(SELECT 1 FROM <table> WHERE user_id = ? LIMIT 1)`
  (or an equivalent `Count() > 0`) against each registered `table`, is
  index-covered for every type today (no schema/migration work implied).
- No existing endpoint or `Storage` interface method already returns this
  shape — this is genuinely new surface area, not an extension of the
  dashboard payload or `/api/data/summary`, confirming "dedicated endpoint"
  over "extend an existing response" is the right shape at this codebase's
  current structure.
- The two testids named in the plan (`vitals-grid-empty-no-data`,
  `more-data`) are proposed, not pre-existing — there is nothing to migrate
  or reconcile from a prior merged implementation; idea-5's attempt never
  merged or archived, so this is greenfield on both the frontend and
  backend.
- `openspec/specs/dashboard-ui/spec.md`'s current "Missing data for a
  metric" requirement documents the opposite of what's wanted here (show
  placeholder, don't hide) and will need an OpenSpec change to update it,
  plus new requirements for secondary-type filtering, the presence
  endpoint's contract, and the new empty state — per this project's
  spec-first rule, that change needs proposing and approving before any
  implementation.

### Limitations of this investigation

- **No prototype was built.** Per the plan's Task 2 ("Build the
  prototype/investigation described under 'Chosen approach'"), the
  intended output of this investigation-track plan is the write-up itself,
  not shipped code — implementing the actual endpoint and frontend wiring
  requires the OpenSpec propose → approve → implement cycle mandated by
  this repo's workflow (`/data/CLAUDE.md`, "OpenSpec — MANDATORY"), which is
  out of scope for an investigate-only idea-forge plan.
- **Query-shape performance was reasoned about, not measured.** No
  benchmark was run comparing "26 sequential EXISTS queries" vs. "one
  `UNION ALL` query" vs. "26 goroutines in parallel" — at this project's
  personal-deployment scale (per `/data/CLAUDE.md`, "Scale") any of the
  three is almost certainly fine, but the choice is left to implementation
  time rather than decided here.
- **Response caching/invalidation was not investigated.** Presence is a
  slowly-changing fact (it only flips true the first time a user logs a new
  type), which could be cached (e.g. in `UserSettings`, updated on first
  write per type) rather than recomputed on every dashboard load — this
  investigation did not evaluate whether that optimization is worth the
  added complexity versus a straightforward per-load query, and defaults to
  recommending the latter first.

### Suggested next steps

1. Write and propose an OpenSpec change for `dashboard-ui` covering: the new
   `GET /api/data-types/presence`-style endpoint's contract, updating the
   "Missing data for a metric" requirement, a new requirement for
   secondary-type (More Data) filtering, and the new no-data empty state
   distinct from Phase 1's all-hidden state — per this repo's spec-first
   workflow, this must be approved before implementation.
2. In that proposal, decide explicitly: the presence-check query shape
   (sequential vs. `UNION ALL` vs. parallel), the exact response JSON shape
   (map of type→bool vs. array of present types), and the new testids
   (avoiding collision with the existing `vitals-grid-empty`).
3. Implement the backend endpoint plus a `Storage` interface method (e.g.
   `PresentDataTypes(userID uuid.UUID) (map[string]bool, error)`), reusing
   `typeRegistry` as the iteration source so a future new data type is
   automatically covered without touching the presence logic.
4. Implement frontend integration: one presence fetch in `page.tsx`
   alongside the existing dashboard fetches, gating both the vitals grid
   and More Data pills from the same response, with fail-open behavior on
   fetch error and no flash-of-hidden-cards (mirroring Phase 1's
   `settingsStatus` gating pattern).
5. Add E2E coverage in `e2e/tests/dashboard.spec.ts` for: a type with no
   data ever hidden from both sections, a type with data outside the 7-day
   sparkline window still shown (the case idea-5 got wrong), the new
   no-data empty state text distinct from the all-hidden one, More Data
   fully collapsing when no secondary type has data, and presence-fetch
   failure leaving everything visible.
