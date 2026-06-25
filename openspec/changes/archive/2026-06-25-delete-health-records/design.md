## Context

HealthVault stores 24 health data types in SQLite via GORM. All data tables embed `TenantModel` (which includes `gorm.DeletedAt` for soft-delete support, though it is unused today). Records are queried through a single generic `QueryRecords(tableName, timeCol, userID, tr)` method that uses raw `.Table()` calls.

The frontend displays records in a table at `/data/[type]` with date-range filtering. Record IDs are present in API responses but filtered from the display columns — they are available to the frontend without any API change to `GET /api/data/{type}`.

Currently there is no delete endpoint and no delete UI.

## Goals / Non-Goals

**Goals:**
- Allow authenticated users to permanently remove individual health records they own.
- Uniform support across all 24 data types via the existing `typeRegistry` pattern.
- Two-step confirm flow in the UI to prevent accidental deletion.

**Non-Goals:**
- Bulk or range deletion (date-range wipe, import-batch rollback).
- Undo / soft delete / recycle bin.
- Admin deletion of other users' data.
- Deletion of family members' records (only the record's owner can delete).

## Decisions

### Hard delete over soft delete

**Decision:** Use `DELETE FROM <table> WHERE id = ? AND user_id = ?` (permanent removal) rather than setting `deleted_at`.

**Rationale:** Soft delete would require patching `QueryRecords` to add `AND deleted_at IS NULL` — that method uses raw `.Table(tableName)` with `[]map[string]any` results, so GORM's auto-filter does not apply. Health data originates on the user's device/Google Fit; "delete from vault" semantics are correct. Hard delete is simpler, has no migration cost, and avoids a latent bug class around soft-deleted rows appearing in queries.

**Alternatives considered:** Soft delete with `QueryRecords` patch — adds complexity for no user-visible benefit given the data source.

### Ownership enforced in SQL WHERE clause

**Decision:** Include `user_id = ?` in the DELETE statement rather than fetching the record first and checking ownership in Go.

**Rationale:** A single round-trip; if the record doesn't exist or belongs to another user, `RowsAffected == 0` and we return 404. Avoids a TOCTOU window and keeps the handler simple.

### New `Storage.DeleteRecord` method

**Decision:** Extend the `Storage` interface with `DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error`.

**Rationale:** Consistent with the existing `QueryRecords` pattern (table-name-driven, generic across all types). The compile-time interface check `var _ Storage = (*storageImpl)(nil)` will catch missed implementations.

### Inline confirm row state (no modal)

**Decision:** Clicking the trash icon flips the row into a "pending confirm" state (highlighted background, Confirm + Cancel buttons). No modal dialog.

**Rationale:** Modals interrupt flow; inline state is contextual and faster. The two-step friction is sufficient to prevent misclicks while remaining lightweight. A single `pendingDeleteId` state variable (or `null`) tracks which row is armed — at most one row can be in confirm state at a time.

## Risks / Trade-offs

- **No undo:** Hard delete is permanent. Mitigated by the explicit confirm step and the fact that source data lives on the device.
- **Concurrent family access:** If two family members view the same user's data simultaneously, one deleting a row the other is viewing produces a stale UI in the second session. Mitigated by the inline removal from local state on success — the second user's table will refresh on next load.
- **Raw table names in SQL:** `tableName` comes from the validated `typeRegistry`, not user input, so SQL injection is not possible (same guarantee as `dataHandler`).

## Migration Plan

No schema migration required. Hard delete operates on existing columns. Deploy is a standard redeploy of the git-based WIP stack.
