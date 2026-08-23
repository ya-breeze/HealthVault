## Why

The dashboard's vitals grid and "More Data" pills currently list every registered data type, including ones the user has never recorded — 26 types shown regardless of whether any of them have ever had a row. This supersedes the closed idea-5 attempt (`docs/investigations/idea-5-hide-items-without-data.md`, unmerged branch `feature/idea-5-hide-items-without-data-in-healthvault`), which hid types client-side based on a 7-day recency window (wrongly hiding types like blood glucose or VO2 max that are logged monthly/yearly) and cost 18 extra per-type API requests per dashboard load.

This change hides only types with **no data at all, ever** — a strictly time-unbounded fact, distinct from the existing 7-day sparkline window — and answers it with one new backend endpoint instead of many client-side requests.

## What Changes

- Add `GET /api/data-types/presence`: one request returning, for every one of the 26 registered types, whether the resolved user has ever recorded a row of that type (`map[string]bool`), computed over all time.
- The vitals grid (8 primary metrics) and the "More Data" list (18 secondary types) each become presence-filtered: a type with zero presence never renders, in either the read-only view or the vitals grid's edit/Customize mode — it is excluded from the customizable set entirely, not merely defaulted to hidden.
- The "More Data" section is omitted entirely (not rendered as an empty list) when no secondary type has ever had data.
- A new empty state, `vitals-grid-empty-no-data`, is added for "this user has never recorded any of the 8 primary metrics" — distinct from the existing `vitals-grid-empty` state ("user explicitly hid every eligible card via Customize"), with copy that does not point at Customize, since Customize cannot fix a lack of data.
- The presence fetch fails open: a fetch error, or a `200` response missing an entry for some type, is treated as "show it" for that type — never as "hide it."
- New locale keys for the no-data empty state copy in both `en` and `ru`.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `data-api`: adds the new `GET /api/data-types/presence` endpoint.
- `dashboard-ui`: the vitals grid and secondary "More Data" list both become presence-filtered (new requirements); "Missing data for a metric" and "Secondary data types remain reachable" are amended to stop describing the pre-presence "show everything, always reachable" behavior; "Customizable vitals grid order and visibility" is amended so a type with zero presence is excluded from the customizable set rather than defaulting to visible-if-referenced.

## Impact

- **Backend**: one new handler in `backend/pkg/server/api.go` (closure over `storage database.Storage`, mirroring `DataHandler`/`summaryHandler`), one new route registration in `server.go`. No schema/migration — every candidate table already has a `user_id`-leading index. No new `Storage` interface method (queries `DB()` directly, mirroring `NeedsAttentionCount`'s precedent for that specific point).
- **Frontend**: `frontend/app/page.tsx` gains one more fetch (alongside the existing dashboard/settings fetches) whose result filters both the vitals grid and the More Data pills; `frontend/lib/i18n/en.ts`/`ru.ts` gain the new empty-state copy; `e2e/tests/dashboard.spec.ts` gains coverage for presence-based hiding, the new empty state, More Data collapsing, and fail-open behavior.
- No breaking change to any existing endpoint or stored shape — this is additive.

## Investigation

Full research, an unwired prototype handler sketch, and the design decisions this proposal is based on are recorded in `docs/investigations/idea-6-hide-data-types-with-no-data-at-all.md`.
