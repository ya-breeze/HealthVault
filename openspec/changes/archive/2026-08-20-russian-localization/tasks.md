## 1. Backend data model

- [x] 1.1 Add nullable `CanonicalName string` column to `FoodItem` and `CustomFood`
      (`backend/pkg/database/models_food.go`); update doc comments so `Name` is documented as the
      Display Name going forward
- [x] 1.2 Confirm `AutoMigrate` picks up both new columns (`backend/pkg/database/db.go`); add/adjust
      a `db_test.go` case asserting a pre-existing row reads back with an empty `CanonicalName`

## 2. Display Language setting

- [x] 2.1 Document `display_language` as a recognized (but not schema-enforced) key in the
      `UserSettings` JSON blob — no backend code change needed since it's opaque JSON; add a
      backend test asserting an arbitrary `display_language` value round-trips through
      `GET`/`PUT /api/users/me/settings` unchanged
- [x] 2.2 Add a small backend helper to read a user's current `display_language` (default `"en"`
      when absent) for reuse by recognition and matching code (2.x, 4.x below)

## 3. Recognition: Display Name + Canonical Name

- [x] 3.1 Extend the vision prompt (`backend/pkg/vision/openai.go`) to accept a target language
      parameter and request `display_name` + `canonical_name` per recognized item
- [x] 3.2 Add a `CanonicalName string` field to `vision.Item` (`backend/pkg/vision/vision.go`,
      alongside the existing `Name` field, which becomes the Display Name); extend response
      parsing to populate both, leaving `CanonicalName` empty when the target language is English
      rather than duplicating `Name`
- [x] 3.3 Thread the caller's `display_language` (via 2.2) into the initial-analysis
      (`backend/pkg/server/food_upload.go`), reanalyze (`backend/pkg/server/food_reanalyze.go`),
      and clarification-round (`backend/pkg/server/food_clarify.go`) call sites
- [x] 3.4 Persist `DisplayName` (into `Name`) and `CanonicalName` on `FoodItem` creation
      (`backend/pkg/server/food_upload.go`, `backend/pkg/server/food_item.go` — both already
      assign `item.ID = uuid.New()` explicitly since `TenantModel` has no `BeforeCreate` hook;
      follow the same pattern for the new field, no new assignment concern introduced)
- [x] 3.5 Backend test: non-English recognition produces distinct Display/Canonical names;
      English recognition leaves Canonical Name empty

## 4. Matching: fuzzy custom-food match + non-English skip

- [x] 4.1 Implement a small hand-rolled normalized Levenshtein-similarity function (no new
      dependency — see design.md decision 5); unit-test it directly with a handful of near-miss
      string pairs
- [x] 4.2 Replace the exact-name custom-food match inside `retrieveCandidates`
      (`backend/pkg/server/food_upload.go`) with the fuzzy-similarity match against the threshold
      from design.md; `resolveItems`'s `exactMatch[i]` flag (same file) keeps meaning "bind
      unconditionally" — just fed by fuzzy instead of exact now. Keep
      `rankedCustomFoodCandidates` (the existing frequency/recency-ranked tier, same file) as the
      fallback when no candidate clears the threshold, unchanged
- [x] 4.3 Add the non-English gate inside `retrieveCandidates`: skip the Open Food Facts/USDA
      branches entirely when `display_language != "en"`, leaving the item's candidate set empty
      (falls through to the macro-estimate fallback in `resolveItems`) when no custom-food match
      was found either
- [x] 4.4 Backend tests: fuzzy match hits on a near-miss name; non-English item skips OFF/USDA
      querying even with a brand present; existing exact-match and frequency/recency scenarios
      from `usda-nutrition-database/spec.md` still pass

## 5. Item resolution API surface

- [x] 5.1 Reject a `canonical_name` field on `PATCH /api/food/meals/{id}/items/{item_id}` with
      HTTP 400 (`backend/pkg/server/food_item.go`)
- [x] 5.2 Ensure `canonical_name` is included (when non-empty) in `FoodItem`/`CustomFood` JSON
      responses — falls out automatically from the new struct field (`json:"canonical_name,omitempty"`)
      for `GetMeal`/`ConfirmMeal` (`backend/pkg/server/food_meal_detail.go`), `PatchMealItem`
      (`backend/pkg/server/food_item.go`), and `ListCustomFoods` (`backend/pkg/server/food_custom.go`);
      verify none of these hand-roll a response struct that would silently drop it
- [x] 5.3 When saving a correction as a new `CustomFood` (existing "save as reusable" flow), copy
      the originating item's `CanonicalName` onto the new `CustomFood` record
- [x] 5.4 Backend tests for 5.1–5.3

## 6. Frontend: i18n scaffolding

- [x] 6.1 Add `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts` string dictionaries plus a
      `t()` lookup function (see design.md decision 6 — no `next-intl`, no route restructuring)
- [x] 6.2 Add a React Context provider that loads `display_language` from
      `GET /api/users/me/settings` on mount and exposes the active dictionary + `t()`
- [x] 6.3 Wrap the app root (`frontend/app/layout.tsx`) with the provider

## 7. Frontend: language switcher

- [x] 7.1 Add a Display Language switcher control (e.g. in an existing settings/account screen)
      that calls `PUT /api/users/me/settings` and updates the Context provider's state immediately
      — implemented as a `<select>` in `components/Header.tsx`, visible on every authenticated page
- [x] 7.2 Translate the initial set of static UI strings covered by 6.1's dictionaries (scope:
      navigation, food-logging screens — full app-wide coverage is not required for this change,
      per design.md Non-Goals)

## 8. Frontend: Display Name / Canonical Name rendering

- [x] 8.1 Update meal confirmation screen to treat `name` as the Display Name (unchanged wire
      field) and additionally render `canonical_name` under Expert Mode (see 9.x) — no API field
      rename, `name` is already what's rendered today
- [x] 8.2 Update food history detail view similarly — `/food/review/?meal=<id>` (ReviewClient.tsx)
      is the same screen/component for both the just-uploaded confirmation flow and clicking into a
      past meal from `/food/history`; the list screen itself only shows meal-level fields (no items)
- [x] 8.3 Update Custom Food catalog screen similarly

## 9. Frontend: Expert Mode toggle

- [x] 9.1 Add a reusable Expert Mode toggle component (local, non-persisted state) —
      `components/food/ExpertModeToggle.tsx`
- [x] 9.2 Wire it into the confirmation screen: show `canonical_name` alongside `name` (the
      Display Name) when on, and only when `canonical_name` is non-empty
- [x] 9.3 Wire it into the food history detail view — same component as 8.2/9.2, see note there
- [x] 9.4 Wire it into the Custom Food catalog screen

## 10. Validation

- [x] 10.1 `make lint` / `make check` (or project equivalents) pass on both backend and frontend —
      `make lint` (`go vet -tags sqlite_fts5 ./...`) clean; frontend `npx tsc --noEmit` and
      `npm run build` (static export) both clean
- [x] 10.2 Manual/E2E pass: log a photo with Display Language set to Russian, confirm Display
      Name shows in Russian, Expert Mode reveals the English Canonical Name, and history/catalog
      screens reflect the same — done live against `hcw-wip` (real OpenAI call): uploaded
      `e2e/tests/fixtures/meal.jpg`, answered clarification in Russian, got item
      `name: "Гречневая каша с курицей"` / `canonical_name: "Buckwheat porridge with chicken"`;
      confirmed via screenshot that Expert Mode reveals "По-английски: Buckwheat porridge with
      chicken" only when toggled on; confirmed no `fdc_id`/`off_code` bound (OFF/USDA correctly
      skipped for non-English). Found and fixed a real bug in the process: `LanguageProvider`'s
      settings fetch only ran once on root-layout mount (while still on `/login`, unauthenticated),
      never retrying after login — language stayed stuck on English all session. Fixed by refetching
      on every pathname change (commit 9493a9d); re-verified fixed via screenshot.
- [x] 10.3 Manual/E2E pass: existing English-Display-Language flow is unchanged (no Canonical
      Name shown, reference-DB matching still runs as before) — full existing `e2e/tests/food.spec.ts`
      suite (44 tests, unrelated to language) run against `hcw-wip`: all passed, no regressions
