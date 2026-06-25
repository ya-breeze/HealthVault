## Why

Users have no way to remove erroneous or unwanted records from their health data vault. A bad sensor reading, a duplicate import, or a measurement they simply want to discard requires manual database intervention today.

## What Changes

- Users can delete individual health data records directly from the data-type table view in the UI.
- Deletion is permanent (hard delete) with a two-step confirm flow to prevent accidents.
- All 24 health data types support deletion uniformly via the existing `typeRegistry` pattern.

## Capabilities

### New Capabilities

- `record-deletion`: Per-row hard delete of health data records, initiated from the frontend table with an inline confirm step, executed via a new authenticated `DELETE /api/data/{type}/{id}` endpoint that enforces user ownership.

### Modified Capabilities

(none)

## Impact

- **Backend**: New `DELETE /api/data/{type}/{id}` route in `server.go`; new `DeleteRecord` method on the `Storage` interface and its implementation.
- **Frontend**: `DataTypeClient.tsx` gains a delete icon column and per-row confirm state.
- **No schema migration**: Hard delete requires no new columns.
- **No changes to `QueryRecords`**: Soft-delete filtering is not needed since rows are permanently removed.
