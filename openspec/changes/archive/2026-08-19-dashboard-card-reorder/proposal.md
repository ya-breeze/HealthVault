## Why

The dashboard's vitals-grid cards render in a fixed order (`PRIMARY_METRICS` in `frontend/lib/vitals.ts`). Users want to put the metrics they care about most at the top instead of living with the developer's default ordering.

## What Changes

- Add a per-user "Edit/Customize" mode to the dashboard: an Edit button reveals move-up/move-down controls on each vitals-grid card; a Done button exits the mode and persists the new order.
- Persist the order server-side, scoped per individual user (not shared across the family), via a new `GET`/`PUT /api/users/me/settings` endpoint pair.
- Introduce a general-purpose per-user settings mechanism on the backend — a new `UserSettings` table storing an arbitrary JSON blob per user — so dashboard card order is its first field, not a special-cased column. This directly addresses the gap `todo.md` already flags (no per-user settings storage exists, blocking ideas like a persisted goal weight).
- No change to which cards exist or their content — this only makes their display order user-configurable. No hide/show visibility toggle in this change.

## Capabilities

### New Capabilities
- `user-settings`: a per-user, family-isolated key-value/JSON settings store with a `GET`/`PUT /api/users/me/settings` API, usable by any feature needing a small persisted per-user preference.

### Modified Capabilities
- `dashboard-ui`: the vitals grid's card order becomes user-customizable (an Edit mode with move-up/move-down controls) instead of always following the fixed `PRIMARY_METRICS` order; unset/new users keep today's default order until they customize it.

## Impact

- **Backend**: new `UserSettings` GORM model (`backend/pkg/database/models.go`), `AutoMigrate` entry, `Storage` interface methods (`GetUserSettings`/`UpsertUserSettings`), new handler + route (`GET`/`PUT /api/users/me/settings`) in `backend/pkg/server/`.
- **Frontend**: `frontend/app/page.tsx` (dashboard) gains an Edit mode; `frontend/lib/vitals.ts`'s `PRIMARY_METRICS` becomes the *default* order, overridden by the fetched per-user order; a small API client addition for the new settings endpoint.
- No new dependencies (no drag-and-drop library).
