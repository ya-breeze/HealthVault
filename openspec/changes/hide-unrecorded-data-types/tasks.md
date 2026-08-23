## 1. Backend: presence endpoint (`backend/pkg/server/api.go`)

- [x] 1.1 Add `DataTypesPresenceHandler(storage database.Storage) http.HandlerFunc`, a plain function closing over `storage`, following `DataHandler`/`summaryHandler`'s shape (not a new `Storage` interface method).
- [x] 1.2 Resolve the target user via `resolveUser(r, storage, claims.UserID, FamilyIDFromCtx(r))`, returning 401/403 exactly as `DataHandler`/`summaryHandler` do.
- [x] 1.3 Loop over `typeRegistry`, running one indexed existence check (`Count() > 0`, or `SELECT EXISTS(...)` if measurement later shows `Count()` too slow — not needed at this project's scale) per type against `storage.DB()` directly, keyed by the registry's type name.
- [x] 1.4 Return `map[string]bool` as JSON via the existing `writeJSON` helper, with exactly one entry per `typeRegistry` name on success.
- [x] 1.5 Register `GET /api/data-types/presence` in `server.go`, behind the same auth middleware as the other `/api/data/*` routes.
- [x] 1.6 Add `backend/pkg/server/data_types_presence_handler_test.go` covering: unauthenticated request rejected (401), a type with zero rows reports `false`, a type with at least one row reports `true`, the response has exactly one entry per registered type, and family-member resolution via `?user=` behaves the same as `DataHandler`'s.

## 2. Frontend: presence fetch (`frontend/lib/api.ts`, `frontend/app/page.tsx`)

- [x] 2.1 Add an `api.ts` function fetching `/api/data-types/presence` and typing its response as `Record<string, boolean>`.
- [x] 2.2 In `page.tsx`, fetch presence alongside the existing dashboard/settings fetches, gated the same way (no flash of unfiltered cards before it resolves — mirror the existing `settingsStatus` loading-gate pattern).
- [x] 2.3 On fetch failure, treat every type as present (fail open) — do not block the dashboard on this fetch succeeding.
- [x] 2.4 When intersecting the presence map against `PRIMARY_METRICS`/`SECONDARY_TYPES`, treat a type absent from a successful response as present (fail open on incompleteness) — do not implement this as a plain "keep only true-valued keys" intersection, which would get this case wrong.

## 3. Frontend: vitals grid presence filtering (`frontend/app/page.tsx`, `frontend/lib/vitals.ts`)

- [x] 3.1 Exclude zero-presence primary metrics from both the read-only grid and the edit/Customize mode's reorder/show-hide list — they are not part of the customizable set at all, not defaulted to hidden within it.
- [x] 3.2 Exclude zero-presence metrics from the "all cards hidden" (`vitals-grid-empty`) check, so that state only triggers when the user has hidden every metric that *does* have presence.
- [x] 3.3 Add the new empty state: when zero primary metrics have presence, render a placeholder with `data-testid="vitals-grid-empty-no-data"` and copy that does not reference Customize.
- [x] 3.4 Confirm the existing "missing data in the last 7 days" placeholder (for a metric that has presence but no recent rows) is unaffected — presence filtering only removes cards with zero data ever.

## 4. Frontend: More Data presence filtering (`frontend/app/page.tsx`)

- [x] 4.1 Filter `SECONDARY_TYPES` to only those with presence before rendering the More Data pills.
- [x] 4.2 Add `data-testid="more-data"` to the section's wrapper element.
- [x] 4.3 Omit the More Data section entirely (no heading, no empty list) when zero secondary types have presence.

## 5. Localization (`frontend/lib/i18n/en.ts`, `ru.ts`)

- [x] 5.1 Add the new no-data empty-state copy key(s) to both dictionaries, distinct from the existing `dashboard.allCardsHidden` key and its copy.

## 6. Tests (`e2e/tests/dashboard.spec.ts`)

- [ ] 6.1 A primary metric with zero data ever is hidden from the read-only vitals grid.
- [ ] 6.2 A primary metric with data outside the 7-day sparkline window (but at least one row ever) remains shown — the regression case idea-5 got wrong.
- [ ] 6.3 A zero-presence metric does not appear in edit/Customize mode's reorder/show-hide list.
- [ ] 6.4 The new `vitals-grid-empty-no-data` placeholder renders when the user has recorded none of the 8 primary metrics, and is distinct from `vitals-grid-empty` (which still covers "hid every data-bearing card via Customize").
- [ ] 6.5 A secondary type with zero data ever is omitted from More Data; a secondary type with data is shown.
- [ ] 6.6 The More Data section (`more-data` testid) is entirely absent when no secondary type has data.
- [ ] 6.7 A presence-fetch failure leaves every primary and secondary type visible (fail open).
- [ ] 6.8 A successful presence response missing an entry for some type leaves that type visible (fail open on incompleteness) — exercise via a route mock returning a partial map.
- [ ] 6.9 While the presence fetch is pending (delayed route mock), neither the vitals grid nor More Data render any card/entry as filtered-in by presence until it resolves or fails, mirroring the existing settings-load gate.

## 7. Manual verification against WIP

- [ ] 7.1 Deploy the branch to the HealthVault WIP stack per CLAUDE.md's E2E rules, then run the full `e2e/tests/dashboard.spec.ts` suite (including the new cases) against it before requesting review.
