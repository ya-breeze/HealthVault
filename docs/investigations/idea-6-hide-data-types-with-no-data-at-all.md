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
  `uniqueIndex:idx_<table>_user_time` with `user_id` as its leading column,
  so a `user_id = ?` presence check is index-covered for all 26 types
  regardless of which column anchors the rest of the index; `food_meal`
  (`models_food.go` line 43) has a plain `index` on `user_id`. One
  exception worth naming: `sleep`'s index is `(user_id, session_end_time)`
  (line 219), not `(user_id, start_time)` even though `start_time` is the
  `timeCol` registered for `sleep` in `typeRegistry` (`api.go` line 39) —
  the presence check is still covered by the `user_id` prefix since it
  never filters on the time column, but this table does not fit the
  "index covers `(user_id, timeCol)`" pattern the other 24 do — `food_meal`
  is the other exception, per its plain `user_id` index noted above.
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
  need to change. The adjacent "Secondary data types remain reachable"
  requirement will too — see "Findings" below.
- `frontend/lib/i18n/en.ts`/`ru.ts` — the dictionaries `page.tsx` reads via
  `t()` (e.g. `dashboard.allCardsHidden`); the new no-data empty-state copy
  needs matching keys here in both locales, per the plan's carried-over
  "Both locales (en + ru)" constraint.
- `e2e/tests/dashboard.spec.ts` — tests vitals-grid states (all-8-render,
  loading/error/retry, Phase 1 reorder/hide-show, all-hidden placeholder)
  via existing testids `vitals-grid`, `vitals-grid-empty`,
  `vitals-grid-loading`, `vitals-grid-error`, `vitals-grid-retry`. One
  existing test (`'secondary "more data" row links to a non-primary data
  page'`, line 111) covers link navigation for one secondary type via its
  `href`, but nothing exercises presence-based hiding, the collapsed-when-
  empty state, or a testid-addressable More Data section — those remain
  uncovered. The testids named in the plan
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
table already carries a `user_id`-leading index (`(user_id, time)` for 24 of
26 types, `user_id` alone for `food_meal`, `(user_id, session_end_time)` for
`sleep`) that makes such a query cheap, and `NeedsAttentionCount` is a close
structural precedent for a new single-purpose, auth-gated aggregate route. The main open implementation
decisions are the concurrency/query shape for checking 26 tables in one
request, and the exact JSON shape of the response — everything else
(fail-open contract, two empty states, composition with Phase 1, `food_meal`
handling) has a clear answer already, either carried over from idea-5's
findings or resolved by design in this plan's "What I want instead."

## Prototype: concrete query and endpoint shape

Not wired into `server.go`'s router or committed as shipping code (per
"Limitations" below, that step waits for an approved OpenSpec change) — this
is a design-level sketch validating that the chosen approach is mechanically
straightforward, and resolving the two decisions the investigation above
left open.

**Correction to Task 1's "suggested next steps" #3**: re-reading
`NeedsAttentionCount` (`food_meal_detail.go` lines 351-365) shows it queries
`h.storage.DB()` directly with GORM, not through a dedicated `Storage`
interface method — no new `Storage` interface method is needed here either.
That said, `NeedsAttentionCount` is a method on `foodHandlers`, a struct type
specific to the food domain; there is no `dataHandlers` struct anywhere in
`backend/pkg/server`. The endpoint's actual siblings — `DataHandler` and
`summaryHandler`, the other `typeRegistry`-consuming handlers in `api.go`,
the file this would live in — are plain functions closing over
`storage database.Storage` and returning an `http.HandlerFunc`, registered
in `server.go` as e.g. `api.HandleFunc("/data/summary",
summaryHandler(storage))`. That closure shape, not `NeedsAttentionCount`'s
struct-method shape, is the precedent to follow for *where this code lives*;
`NeedsAttentionCount` remains the precedent for *querying `DB()` directly
without a new `Storage` method*. `DataHandler`/`summaryHandler` also resolve
the target user via `resolveUser(r, storage, claims.UserID,
FamilyIDFromCtx(r))` (`api.go` lines 343-356) rather than `claims.UserID`
directly, so a family member's data can be viewed via `?user=`; the sketch
below follows that too, since a presence endpoint backing the same dashboard
should stay consistent with its siblings on this point.

Because `typeRegistry` stores raw table names (not per-type Go structs), a
single generic loop over `typeRegistry` — using GORM's `.Table(name)` rather
than a typed model per type — avoids a 26-way type switch entirely:

```go
// GET /api/data-types/presence — sketch, not wired up.
func DataTypesPresenceHandler(storage database.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromCtx(r)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		targetUser, err := resolveUser(r, storage, claims.UserID, FamilyIDFromCtx(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		present := make(map[string]bool, len(typeRegistry))
		for name, info := range typeRegistry {
			var exists int64
			err := storage.DB().Table(info.table).
				Where("user_id = ?", targetUser.ID).
				Count(&exists).Error
			if err != nil {
				http.Error(w, "query error", http.StatusInternalServerError)
				return
			}
			present[name] = exists > 0
		}

		writeJSON(w, present) // map[string]bool, e.g. {"steps": true, "vo2_max": false, ...}
	}
}
```

This resolves the two open decisions:

- **Query shape**: sequential loop over `typeRegistry`, one indexed
  `COUNT` per type, reusing the existing single-`DB()`-call idiom the
  codebase already has (`NeedsAttentionCount`). Note GORM's `Count()`
  retains a `LIMIT` clause in the generated SQL but doesn't use it to
  short-circuit an aggregate that already returns one row — `Limit(1)`
  paired with `Count()` is a no-op, not a true existence check, so an
  earlier draft of this sketch that chained them has been corrected to a
  plain `Count()`. A real early-exit existence check (e.g. raw
  `SELECT EXISTS(SELECT 1 FROM <table> WHERE user_id = ? LIMIT 1)`, or
  `.Select("1").Limit(1).Find(&rows)` checking `len(rows)`) is available if
  a future measurement shows the full count is too slow on a
  much-larger-than-personal-scale table, but isn't needed here: 26
  sequential round-trips to a local/small SQLite-or-Postgres instance at
  this project's scale (per `/data/CLAUDE.md`, "Scale") is milliseconds of
  total work — not worth the added complexity of goroutines or a
  hand-written `UNION ALL` unless a future measurement says otherwise. This
  also means no new `Storage` interface surface is needed — a plain
  handler function queries `DB()` directly, mirroring `NeedsAttentionCount`
  on that point, while using the closure-over-`storage` shape of its actual
  siblings `DataHandler`/`summaryHandler` for how it's declared and
  registered.
- **Response JSON shape**: `map[string]bool` keyed by the same type names
  the frontend's `DATA_TYPES` (`frontend/lib/api.ts`) already uses — the
  frontend can intersect this map's true-valued keys with `PRIMARY_METRICS`
  and `SECONDARY_TYPES` directly, no translation layer needed. An array of
  present-only type names was considered and rejected: the map form makes
  "type absent from response" (e.g. a partial failure) distinguishable from
  "type present but false," which an array can't express without a second
  field. That distinguishability is only useful if the frontend actually
  acts on it: the handler sketch above always populates one entry per
  `typeRegistry` name on a `200`, so a successful response is contractually
  complete and a missing key can only mean a malformed/partial response,
  not "no data." The frontend's fail-open handling (see "Suggested next
  steps" #4) must therefore treat that case the same as a fetch error —
  show the type — rather than treating an absent key as `false`; simply
  intersecting "true-valued keys" with `PRIMARY_METRICS`/`SECONDARY_TYPES`
  (as this section's opening sentence puts it) is exactly the shortcut that
  would get this wrong, since it silently reads "absent" as "not true."

One thing this sketch surfaces that pure reading didn't: `food_meal`'s
entry has no `valueCol`/`family` set but does have a `table`, so it needs
no special case in this loop — confirming the investigation's earlier note
that presence (unlike bucketed aggregation) treats `food_meal` like any
other registered type.

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
  implementation. The adjacent "Secondary data types remain reachable"
  requirement (same file, just below "Missing data for a metric") also
  needs amending, not just supplementing: it currently reads "Registered
  types outside the 8-metric vitals grid SHALL remain reachable from the
  dashboard as a compact link list, **preserving today's full-catalog
  access**" — and that "full-catalog access" phrase is exactly what
  presence-based hiding of secondary types intentionally breaks.

### Limitations of this investigation

- **No shipped/wired-up code was written.** Per this repo's spec-first
  workflow (`/data/CLAUDE.md`, "OpenSpec — MANDATORY"), implementing and
  routing the actual endpoint requires the OpenSpec propose → approve →
  implement cycle, which is out of scope for an investigate-only
  idea-forge plan. The "Prototype" section above sketches the concrete
  handler code to validate the approach and settle the two open design
  decisions, but it isn't wired into `server.go` or committed as
  production code.
- **Query-shape performance was reasoned about, not measured.** No
  benchmark was run comparing the sequential count-loop above
  against a hand-written `UNION ALL` or 26 goroutines in parallel — at
  this project's personal-deployment scale (per `/data/CLAUDE.md`,
  "Scale") the sequential loop is almost certainly fine and is now the
  recommended default (see "Prototype" above), but no timing was actually
  measured against real data.
- **Response caching/invalidation was not investigated.** Presence is a
  slowly-changing fact (it only flips true the first time a user logs a new
  type), which could be cached (e.g. in `UserSettings`, updated on first
  write per type) rather than recomputed on every dashboard load — this
  investigation did not evaluate whether that optimization is worth the
  added complexity versus a straightforward per-load query, and defaults to
  recommending the latter first.
- **Exact copy for the new locale strings was not drafted.** "Relevant
  code" above (line 55) identifies `frontend/lib/i18n/en.ts`/`ru.ts` as the
  dictionaries this change touches, and "Suggested next steps" below calls
  out that new keys are needed in both, but this investigation stops at
  naming the files — it does not draft the actual key name(s) or English/
  Russian copy for the new no-data empty state. Left for the OpenSpec
  proposal.

### Suggested next steps

1. Write and propose an OpenSpec change for `dashboard-ui` covering: the new
   `GET /api/data-types/presence`-style endpoint's contract (map[string]bool
   response, per the "Prototype" section above), updating the "Missing data
   for a metric" requirement, a new requirement for secondary-type (More
   Data) filtering, and the new no-data empty state distinct from Phase 1's
   all-hidden state — per this repo's spec-first workflow, this must be
   approved before implementation.
2. The presence-check query shape (sequential count loop over
   `typeRegistry`) and response JSON shape (`map[string]bool`) are now
   decided by the prototype sketch above; the proposal should still pin
   down the new testids (avoiding collision with the existing
   `vitals-grid-empty`).
3. Implement the backend endpoint as a plain function closing over
   `storage database.Storage` (mirroring `DataHandler`/`summaryHandler` in
   `api.go`, the actual sibling handlers for this route — not a new
   `Storage` interface method), reusing `typeRegistry` as the iteration
   source so a future new data type is automatically covered without
   touching the presence logic, and resolving the target user via
   `resolveUser` for consistency with those siblings' family-member support.
4. Implement frontend integration: one presence fetch in `page.tsx`
   alongside the existing dashboard fetches, gating both the vitals grid
   and More Data pills from the same response, with fail-open behavior on
   both fetch error and a successful-but-incomplete response (a type
   missing from the map on a `200` must be treated as "show it," the same
   as a fetch error — not silently coerced to `false` by an intersection
   over true-valued keys, since the endpoint contract only guarantees
   completeness, not that every client-side consumer honors it), and no
   flash-of-hidden-cards (mirroring Phase 1's `settingsStatus` gating
   pattern). Per the plan's carried-over "Both locales (en + ru)"
   constraint, the new empty-state copy needs matching keys in both
   `frontend/lib/i18n/en.ts` and `ru.ts` (alongside the existing
   `dashboard.allCardsHidden` key it must stay distinct from).
5. Add E2E coverage in `e2e/tests/dashboard.spec.ts` for: a type with no
   data ever hidden from both sections, a type with data outside the 7-day
   sparkline window still shown (the case idea-5 got wrong), the new
   no-data empty state text distinct from the all-hidden one, More Data
   fully collapsing when no secondary type has data, presence-fetch
   failure leaving everything visible, and a presence-hidden card's
   visibility while in Customize/edit mode — `page.tsx`'s existing edit
   mode renders every card including Phase-1-hidden ones so they can be
   re-shown (comment at `page.tsx` lines 163-166), and this investigation
   doesn't resolve whether a never-had-data card should behave the same
   way or stay hidden even in edit mode; that's an open question for the
   OpenSpec proposal, not settled here.
