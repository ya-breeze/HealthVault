## 1. Backend data model

- [ ] 1.1 Add nullable `CanonicalName string` column to `FoodItem` and `CustomFood`
      (`backend/pkg/database/models_food.go`); update doc comments so `Name` is documented as the
      Display Name going forward
- [ ] 1.2 Confirm `AutoMigrate` picks up both new columns (`backend/pkg/database/db.go`); add/adjust
      a `db_test.go` case asserting a pre-existing row reads back with an empty `CanonicalName`

## 2. Display Language setting

- [ ] 2.1 Document `display_language` as a recognized (but not schema-enforced) key in the
      `UserSettings` JSON blob — no backend code change needed since it's opaque JSON; add a
      backend test asserting an arbitrary `display_language` value round-trips through
      `GET`/`PUT /api/users/me/settings` unchanged
- [ ] 2.2 Add a small backend helper to read a user's current `display_language` (default `"en"`
      when absent) for reuse by recognition and matching code (2.x, 4.x below)

## 3. Recognition: Display Name + Canonical Name

- [ ] 3.1 Extend the vision prompt (`backend/pkg/vision/openai.go`) to accept a target language
      parameter and request `display_name` + `canonical_name` per recognized item
- [ ] 3.2 Extend response parsing (`backend/pkg/vision/vision.go`) to read both fields; when the
      target language is English, leave `canonical_name` empty rather than duplicating the name
- [ ] 3.3 Thread the caller's `display_language` (via 2.2) into the initial-analysis, reanalyze,
      and clarification-round call sites (`backend/pkg/server/food_upload.go`,
      `backend/pkg/server/food.go`)
- [ ] 3.4 Persist `DisplayName` (into `Name`) and `CanonicalName` on `FoodItem` creation
      (`backend/pkg/ingest/ingest.go` pattern — remember explicit `ID`/`FamilyID` assignment,
      `TenantModel` has no `BeforeCreate` hook)
- [ ] 3.5 Backend test: non-English recognition produces distinct Display/Canonical names;
      English recognition leaves Canonical Name empty

## 4. Matching: fuzzy custom-food match + non-English skip

- [ ] 4.1 Implement a small hand-rolled normalized Levenshtein-similarity function (no new
      dependency — see design.md decision 5); unit-test it directly with a handful of near-miss
      string pairs
- [ ] 4.2 Replace the exact case-insensitive custom-food match in
      `backend/pkg/server/food_item.go` (or wherever "Match Selection" is implemented) with the
      fuzzy-similarity match against the threshold from design.md; keep the existing
      frequency/recency-ranked tier as the fallback when no candidate clears the threshold
      unchanged
- [ ] 4.3 Add the non-English gate: skip Open Food Facts/USDA querying entirely when
      `display_language != "en"`, falling through to the macro-estimate fallback
- [ ] 4.4 Backend tests: fuzzy match hits on a near-miss name; non-English item skips OFF/USDA
      querying even with a brand present; existing exact-match and frequency/recency scenarios
      from `usda-nutrition-database/spec.md` still pass

## 5. Item resolution API surface

- [ ] 5.1 Reject a `canonical_name` field on `PATCH /api/food/meals/{id}/items/{item_id}` with
      HTTP 400 (`backend/pkg/server/food_item.go`)
- [ ] 5.2 Ensure `canonical_name` is included (when non-empty) in `FoodItem`/`CustomFood` JSON
      responses used by meal detail, confirmation, and the Custom Food catalog endpoints
- [ ] 5.3 When saving a correction as a new `CustomFood` (existing "save as reusable" flow), copy
      the originating item's `CanonicalName` onto the new `CustomFood` record
- [ ] 5.4 Backend tests for 5.1–5.3

## 6. Frontend: i18n scaffolding

- [ ] 6.1 Add `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts` string dictionaries plus a
      `t()` lookup function (see design.md decision 6 — no `next-intl`, no route restructuring)
- [ ] 6.2 Add a React Context provider that loads `display_language` from
      `GET /api/users/me/settings` on mount and exposes the active dictionary + `t()`
- [ ] 6.3 Wrap the app root (`frontend/app/layout.tsx`) with the provider

## 7. Frontend: language switcher

- [ ] 7.1 Add a Display Language switcher control (e.g. in an existing settings/account screen)
      that calls `PUT /api/users/me/settings` and updates the Context provider's state immediately
- [ ] 7.2 Translate the initial set of static UI strings covered by 6.1's dictionaries (scope:
      navigation, food-logging screens — full app-wide coverage is not required for this change,
      per design.md Non-Goals)

## 8. Frontend: Display Name / Canonical Name rendering

- [ ] 8.1 Update meal confirmation screen to render `display_name` (falling back to legacy `name`
      for pre-change API responses if needed) instead of a single name field
- [ ] 8.2 Update food history detail view similarly
- [ ] 8.3 Update Custom Food catalog screen similarly

## 9. Frontend: Expert Mode toggle

- [ ] 9.1 Add a reusable Expert Mode toggle component (local, non-persisted state)
- [ ] 9.2 Wire it into the confirmation screen: show `canonical_name` alongside `display_name`
      when on, and only when `canonical_name` is non-empty
- [ ] 9.3 Wire it into the food history detail view
- [ ] 9.4 Wire it into the Custom Food catalog screen

## 10. Validation

- [ ] 10.1 `make lint` / `make check` (or project equivalents) pass on both backend and frontend
- [ ] 10.2 Manual/E2E pass: log a photo with Display Language set to Russian, confirm Display
      Name shows in Russian, Expert Mode reveals the English Canonical Name, and history/catalog
      screens reflect the same
- [ ] 10.3 Manual/E2E pass: existing English-Display-Language flow is unchanged (no Canonical
      Name shown, reference-DB matching still runs as before)
