## 1. Backend — Storage layer

- [ ] 1.1 Add `DeleteRecord(tableName string, id uuid.UUID, userID uuid.UUID) error` to the `Storage` interface in `pkg/database/storage.go`
- [ ] 1.2 Implement `DeleteRecord` in `pkg/database/storage_impl.go` using a raw `db.Exec("DELETE FROM <table> WHERE id = ? AND user_id = ? AND deleted_at IS NULL", id, userID)` and return 404-sentinel if `RowsAffected == 0`

## 2. Backend — HTTP handler and route

- [ ] 2.1 Add `deleteRecordHandler(storage)` in `pkg/server/api.go`: validate `{type}` against `typeRegistry`, parse `{id}` as UUID, call `storage.DeleteRecord`, return 204 on success or 404 if not found
- [ ] 2.2 Register `DELETE /api/data/{type}/{id}` on the protected `api` subrouter in `pkg/server/server.go`

## 3. Backend — Tests

- [ ] 3.1 Add unit tests for `DeleteRecord` in `pkg/database/db_test.go`: own record deleted (204-equivalent), other user's record not deleted (0 rows affected), non-existent ID (0 rows affected)
- [ ] 3.2 Add HTTP handler tests (or extend existing handler tests) for the new DELETE route: 204 success, 404 wrong user, 404 unknown type, 401 unauthenticated

## 4. Frontend — API client

- [ ] 4.1 Add `deleteRecord(type: string, id: string): Promise<void>` method to the api client in `frontend/lib/api.ts` (or wherever `api.data` is defined), calling `DELETE /api/data/{type}/{id}` and throwing on non-204

## 5. Frontend — DataTypeClient UI

- [ ] 5.1 Add `pendingDeleteId` state (`string | null`) to `DataTypeClient.tsx`
- [ ] 5.2 Add an "Actions" column header to the table header row
- [ ] 5.3 In each table row: if `r.id !== pendingDeleteId`, render a trash icon button that sets `pendingDeleteId = r.id`; if `r.id === pendingDeleteId`, render a highlighted row with Confirm and Cancel buttons
- [ ] 5.4 Confirm handler: call `api.deleteRecord(type, pendingDeleteId)`, on success filter the record from `records` state and reset `pendingDeleteId` to null; on error show an error state
- [ ] 5.5 Cancel handler: reset `pendingDeleteId` to null
- [ ] 5.6 Ensure clicking trash on a new row while another is in confirm state resets `pendingDeleteId` to the new row's id (the single-state-variable approach handles this automatically)

## 6. Verification

- [ ] 6.1 Run `make lint` (or equivalent) in the backend; fix any issues
- [ ] 6.2 Run backend tests: `make test` or `go test ./...`
- [ ] 6.3 Deploy to WIP stack and manually verify: delete a weight record, confirm it disappears from the table and does not reappear on reload
- [ ] 6.4 Run E2E tests against the WIP stack
