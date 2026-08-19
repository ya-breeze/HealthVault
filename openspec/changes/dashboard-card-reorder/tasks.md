## 1. Backend: user-settings storage

- [ ] 1.1 Add `UserSettings` GORM model to `backend/pkg/database/models.go` (`TenantModel` + `UserID uuid.UUID` unique-indexed + `SettingsJSON string`), following the explicit `ID`/`FamilyID` assignment convention (no `BeforeCreate` hook).
- [ ] 1.2 Add `&UserSettings{}` to the `AutoMigrate` call in `backend/pkg/database/db.go`.
- [ ] 1.3 Add `GetUserSettings(userID uuid.UUID) (string, error)` and `UpsertUserSettings(userID, familyID uuid.UUID, settingsJSON string) error` to the `Storage` interface and its implementation (`storage_impl.go`); `UpsertUserSettings` uses a single `clause.OnConflict` upsert on `user_id`, no read-modify-write.

## 2. Backend: API endpoints

- [ ] 2.1 Add a handler for `GET /api/users/me/settings` — resolve `UserID` from `ClaimsFromCtx`, return `{}` if no row exists, otherwise the decoded JSON.
- [ ] 2.2 Add a handler for `PUT /api/users/me/settings` — resolve `UserID`/`FamilyID` from claims, validate the body is JSON, call `UpsertUserSettings`.
- [ ] 2.3 Register both routes in `backend/pkg/server/server.go` alongside the existing `/api/users/me` route; both require the existing auth middleware (401 on missing/invalid auth).
- [ ] 2.4 Backend tests: round-trip GET/PUT, per-user isolation (two users don't see each other's settings), unauthenticated 401, GET before any PUT returns `{}` without creating a row.

## 3. Frontend: API client + default order

- [ ] 3.1 Add a small API client function for `GET`/`PUT /api/users/me/settings` (frontend `lib/`).
- [ ] 3.2 In the dashboard (`frontend/app/page.tsx`), fetch settings on load; derive the rendered card order from `PRIMARY_METRICS` (default) reconciled with any saved `dashboard_order` (filter to known metrics, append newly-added ones missing from the saved order).

## 4. Frontend: edit mode UI

- [ ] 4.1 Add an Edit/Customize toggle button to the dashboard.
- [ ] 4.2 In edit mode, render move-up/move-down buttons per card (disabled at the first/last position respectively); clicking updates the in-memory order immediately.
- [ ] 4.3 Add a Done button that `PUT`s the full updated `dashboard_order` array and exits edit mode.
- [ ] 4.4 Handle the save failing (e.g. network error) — keep edit mode open and surface the error via the existing toast pattern rather than silently discarding the change.

## 5. Verify

- [ ] 5.1 `make lint` / `tsc --noEmit` (frontend) and `go vet` / project lint (backend) — fix any issues.
- [ ] 5.2 Manually verify against the deployed WIP stack: reorder cards, refresh the page, confirm order persisted; log in as a second user and confirm their order is independent.
- [ ] 5.3 Confirm a user with no saved settings still sees the default order with no errors.
