## Context

HealthVault has no per-user settings storage today — `backend/pkg/database/models.go` has no `User`/`Settings` model of its own (identity comes from `kin-core`'s `User`/`Family`), and `todo.md` explicitly calls this out as blocking ideas like a persisted goal weight. The dashboard (`frontend/app/page.tsx`) renders `PRIMARY_METRICS` (`frontend/lib/vitals.ts`) — a hardcoded, comment-labeled "in display order" array — directly, with no per-user override point.

Auth already resolves both `UserID` and `FamilyID` per request via JWT claims (`ClaimsFromCtx`/`FamilyIDFromCtx` in `backend/pkg/server/middleware.go`), and every existing table uses `kin-core`'s `TenantModel` (family-scoped) plus an explicit `UserID` column for per-user ownership within a family (see `Weight`, `Height`, etc. in `models.go`).

## Goals / Non-Goals

**Goals:**
- A minimal, reusable per-user settings store (`UserSettings`), with dashboard card order as its first real field.
- Per-user (not per-family) card order, matching how the JWT already resolves an individual `UserID`.
- No read-modify-write race window on the settings row.

**Non-Goals:**
- A general key-value settings API with arbitrary keys/namespaces — over-engineering for a single-user-per-household app. One JSON blob per user is enough headroom for a handful of future preferences.
- Family-level or shared settings.
- Hiding/showing cards, or changing which metrics are eligible — only their order.
- Drag-and-drop UI (explicitly deferred per the earlier design discussion — no new frontend dependency).

## Decisions

**New `UserSettings` GORM model, one row per user, storing a single JSON blob:**
```go
type UserSettings struct {
    models.TenantModel                     // ID, FamilyID (family-scoped like every other table)
    UserID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
    SettingsJSON string  `gorm:"not null;default:'{}'"`
}
```
`UserID` is uniquely indexed — exactly one settings row per user. `SettingsJSON` is an opaque JSON object (e.g. `{"dashboard_order": ["weight","steps",...]}`) decoded/encoded by the handler, not modeled as SQL columns — this is what makes it reusable for future preferences without a migration each time, while staying a single small table rather than a generic key-value schema (rejected as unneeded complexity for a handful of fields).

Per the shared `models.TenantModel` convention: `ID` and `FamilyID` must be assigned explicitly on create (no `BeforeCreate` hook) — same pattern as `backend/pkg/ingest/ingest.go`.

**`GET /api/users/me/settings` returns the current user's decoded settings (or `{}` defaults if no row exists yet — no row is created on GET).**
**`PUT /api/users/me/settings` replaces the entire blob** (full-document upsert, not a partial/merge update) via GORM's `clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"settings_json", "updated_at"})}`. The frontend always fetches-then-PUTs the whole object, so the server never does a read-modify-write on this row — it's a single atomic upsert statement. This sidesteps the SQLite write-race class documented for other tables (GORM `Transaction()` alone isn't sufficient there); a single-statement upsert has no window for two requests to interleave a stale read.

Both endpoints require authentication (401 if missing/invalid) — there's no ownership check beyond "the JWT's own UserID," since a user can only ever read/write their own settings row (no `{id}` path param to spoof).

**Frontend**: `frontend/lib/vitals.ts` keeps `PRIMARY_METRICS` as the default order. The dashboard fetches `GET /api/users/me/settings` once on load; if `dashboard_order` is present, it's used to sort/reorder the same fixed metric set (unknown/removed metric keys are dropped, newly added metrics not yet in a saved order are appended at the end) rather than replacing the metric list itself. Edit mode operates on the in-memory ordered array; Done sends the full updated `dashboard_order` array via `PUT`.

## Risks / Trade-offs

- **[Risk]** A user's saved order references a metric type that's later removed from `PRIMARY_METRICS` (or vice versa, a new metric is added later and isn't in anyone's saved order). → Mitigation: reconciliation happens client-side at render time (filter saved order to known metrics, append any missing ones) — the stored JSON never needs migrating when `PRIMARY_METRICS` changes.
- **[Risk]** Opaque JSON blob loses SQL-level query/validation on its contents. → Mitigation: acceptable for a small, frontend-owned preferences blob with no server-side business logic depending on its contents; the alternative (a dedicated `dashboard_order` column) would need a new migration for every future preference, which is exactly what this design avoids.
- **[Trade-off]** No optimistic-concurrency check on the upsert (last write wins) — acceptable since this is a single user's own preference edited from typically one device at a time in one sitting, not concurrently-written telemetry.
