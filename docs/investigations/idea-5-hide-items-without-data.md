# Investigation: Hide items without data in HealthVault

Research notes for idea-forge plan `idea-5-hide-items-without-data-in-healthvault-investigate.md`.
Goal under investigation: show only vitals/"more data" types that actually have recorded data.

## Relevant code

- `frontend/app/page.tsx` — Dashboard. Vitals grid at lines 187-206
  (`data-testid="vitals-grid"`). "More Data" pill list at lines 253-268
  (`SECONDARY_TYPES.map(...)`), section heading `t('dashboard.moreData')`.
- `frontend/components/VitalCard.tsx` — renders one vitals-grid card; when
  `result` is `null` it already renders a "no data" placeholder (line 141,
  `t('vitals.noData')`) instead of hiding the card.
- `frontend/lib/vitals.ts` — `PRIMARY_METRICS` (8 types, lines 14-23) and
  `extractVital()` (line 118), which returns `null` when `rows.length === 0`
  (line 119).
- `frontend/lib/api.ts` — `DATA_TYPES` (lines 525-532, 26 types total,
  including `food_meal`); `api.data()` (line 404) fetches
  `GET /api/data/{type}?from=&to=&bucket=day`.
- `backend/pkg/server/api.go` — `DataHandler` (line 190); explicitly
  normalizes a no-match query to `[]` (lines 234-236).

## Payload shape / presence signal

The `/api/data/{type}` response is a plain JSON array of row objects. When
there are zero matching records the backend normalizes to `[]` — an
unambiguous "no data" signal that is never conflated with a legitimate
zero-valued row (e.g. "0 steps" still returns a non-empty row with `sum: 0`).
So `rows.length === 0` reliably means "no data recorded in the requested
window," distinct from "value is 0." This confirms the chosen approach's
assumption holds for every type observed.

## Current type-list mechanism

- `PRIMARY_METRICS` = 8 hardcoded types (steps, heart_rate, sleep,
  heart_rate_variability, distance, weight, blood_pressure,
  oxygen_saturation) → vitals grid. Only these are fetched today; presence is
  computed via `extractVital()` returning `null`, but that result currently
  only swaps in a placeholder string — it never hides the card.
- `SECONDARY_TYPES` = `DATA_TYPES` minus `PRIMARY_METRICS` (18 types) → "More
  Data" pills, computed once by static set subtraction. **No data fetch and
  no presence check exists for these today** — they always render.

## Related prior work

`dashboard-card-visibility` (Phase 1, commit c9c955b, archived at
`openspec/changes/archive/2026-08-21-dashboard-card-visibility`) added
user-controlled per-card hide/show + reordering for the 8 vitals cards,
persisted in `UserSettings.dashboard_order`
(`DashboardCardPref`/`reconcileMetricOrder` in `frontend/lib/vitals.ts`). It
explicitly scoped out the needs-attention banner, Log Food row, and More Data
row (`todo.md` lines 21-22). This is an orthogonal mechanism (explicit user
preference, not data presence) but touches the same components/registry, so
this change needs to define the interaction — see below.

The current spec (`openspec/specs/dashboard-ui/spec.md` lines 32-34, "Missing
data for a metric") codifies today's behavior as "indicate no data rather
than rendering empty/broken sparkline" (show placeholder, don't hide). That
requirement will need to change, plus a new requirement is needed for
secondary-type filtering.

## Constraints and unknowns

- **No zero-value ambiguity** — absence is always `[]`; presence is any
  non-empty array, even where the aggregated value is 0.
- **Window ambiguity** — "no data" is only evaluated over the dashboard's
  7-day sparkline window today. Hiding based on that window could hide a type
  the user logged 8+ days ago; need to decide between the existing 7-day
  window vs. an "ever any data" check (which needs a different, unbounded
  query — especially for the 18 secondary types that aren't fetched at all
  today).
- **New fetch cost** — secondary types need 18 additional API calls per
  dashboard load unless batched or handled server-side; need to decide
  sequencing relative to the existing `Promise.all` for `PRIMARY_METRICS`,
  and fail-open vs. fail-closed behavior if a per-type presence fetch errors.
- **Interaction with Phase 1 hide/show** — a card the user explicitly hid
  must stay hidden. A card the user left visible but that has no data is the
  new case to define: hide outright, or still surface it (e.g. dimmed) in
  Edit mode so it stays discoverable, mirroring how Phase 1 already shows
  user-hidden cards dimmed in Edit mode.
- **No pagination/lazy-load risk** — all fetches are single bounded-range
  queries, so client-side filtering won't hide something mid-load beyond the
  existing loading-state gating already in `page.tsx`.
- **`food_meal` is special** — it uses `logged_at` as its time column and
  does not support the `?bucket=` param used by other types; any presence
  check for it needs the unbucketed `/data/food_meal` path.

## Conclusion

The chosen approach (filter client-side on the existing per-type payload) is
viable: the backend already gives an unambiguous no-data signal (`[]`) for
every type. The main implementation gaps are (1) secondary types are
currently never fetched, so presence-checking them means adding fetches, and
(2) the "no data" window/scope and its interaction with Phase 1's
user-controlled visibility need explicit decisions before implementation.

## Findings write-up

### What was built

A throwaway prototype (commit `2455e9d`, `frontend/app/page.tsx`) implements
the chosen approach end-to-end on the dashboard:

- **Vitals grid** — cards are filtered to `PRIMARY_METRICS` where
  `extractVital()` returns non-null (reusing the fetch already in place, no
  new API calls). A user-hidden card (Phase 1) stays hidden regardless of
  data; a visible-but-dataless card is now omitted from the read-only view.
  In Edit mode, dataless cards still render (dimmed via the existing
  `editing` path) so they stay discoverable and re-orderable, mirroring how
  Phase 1 already shows user-hidden cards in Edit mode.
- **More Data pills** — a new presence-only fetch (`Promise.all` over
  `SECONDARY_TYPES`, unbucketed `api.data()`, 7-day window) drives filtering
  the pill list down to types with at least one row. Unlike the vitals grid,
  this section has no Customize/Edit escape hatch, so a dataless type is
  dropped outright rather than shown dimmed — there is nothing actionable for
  the user to do about it here. The whole section (heading + pills) disappears
  if nothing has data, rather than showing an empty pill row.
- **Fail-open loading** — both filters default to "show everything" until
  their respective fetch resolves (`vitalsLoaded` / `secondaryLoaded`), so
  nothing flashes hidden-then-visible on first paint.
- **New empty state** — a distinct `dashboard.noVitalsData` message
  ("no data yet" for a grid the user did *not* hide) separate from the
  existing `allCardsHidden` message ("you hid everything, use Customize"),
  since Customize cannot fix a lack of recorded data.

### Findings

- The core assumption behind the chosen approach holds: `[]` from
  `/api/data/{type}` is an unambiguous, universal "no data" signal, never
  conflated with a legitimate zero-valued row.
- Client-side filtering composes cleanly with the existing Phase 1
  hide/show feature — the two are orthogonal (explicit user preference vs.
  data presence) and can be applied as sequential filters without conflict.
- Secondary-type presence requires new fetches (18 extra calls today); the
  prototype batches them in one `Promise.all` reusing the same 7-day window
  as the vitals grid, so no additional network round-trip depth is added
  versus the primary-metrics fetch.
- `food_meal`'s unbucketed endpoint works fine for a presence-only check
  (row count > 0), sidestepping its `bucket=day` incompatibility since
  presence checking never needs aggregation.

### Limitations

- **Window scope is arbitrary.** Presence is evaluated over the dashboard's
  7-day sparkline window, so a type logged only 8+ days ago reads as "no
  data" and stays hidden even though the user has historical data for it.
  The prototype does not implement an "ever any data" alternative (which
  would need a different, unbounded query, especially costly for the 18
  secondary types).
- **Extra network cost for secondary types is unconditional.** All 18
  `SECONDARY_TYPES` are now fetched on every dashboard load purely to decide
  visibility, even though most users will only ever populate a handful.
  There's no caching or server-side precomputation of "which types have any
  data," which would avoid this per-load cost.
- **Spec is not updated.** `openspec/specs/dashboard-ui/spec.md` (lines
  32-34) still codifies the old "show placeholder, don't hide" behavior for
  missing vitals data; this prototype contradicts it and was built without
  an approved OpenSpec change, per this investigation's scope (research
  only, not implementation).
- **No tests were added** for the new filtering logic, empty states, or the
  Edit-mode dimmed-but-dataless card behavior — the prototype is illustrative
  only.
- **Not evaluated:** interaction with slower/failing networks beyond the
  basic `.catch(() => [])` fallback (treated as "no data" on error, which is
  fail-closed for that one type but fail-open for the whole section while
  loading — the two failure modes aren't unified).

### Suggested next steps

1. Decide the window-scope question explicitly (7-day sparkline window vs.
   "ever any data") before writing a real OpenSpec proposal — this is the
   single biggest behavioral judgment call left open.
2. If secondary-type presence checking proceeds, consider a lighter-weight
   backend signal (e.g. a single `/api/data/presence` endpoint returning
   which types have any data) instead of 18 client-side fetches, to avoid
   the unconditional per-load cost noted above.
3. Write the OpenSpec change updating `dashboard-ui/spec.md`'s "Missing data
   for a metric" requirement, plus a new requirement for secondary-type
   filtering and the Edit-mode dataless-card behavior, and get it approved
   before implementing for real.
4. Add test coverage for: empty-vitals-with-data-elsewhere state, the new
   `noVitalsData` vs. `allCardsHidden` message split, Edit-mode dimmed
   dataless cards, and More Data section fully collapsing.
5. Discard or rework the throwaway prototype commit — it was built to
   validate feasibility, not as the shipped implementation.
