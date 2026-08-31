# Use the LLM for manual food input
Idea: ya-breeze/idea-forge#23

## Why

Manual food entry — the path a user takes when they log a meal without a photo — resolves food
through the reference databases first. `frontend/app/food/manual/page.tsx` renders one
`ManualItemEditor` per item, and that component's default tab is "Search food": the user types a
name, presses Search, and `GET /api/food/search` (`backend/pkg/server/food.go`) looks for an
exact custom-food name match, then falls through to a USDA FTS query. The only alternative tab is
"Enter macros", where the user types all seven macro values themselves. Both are database-shaped
work: pick a row, or be the row.

That path is weakest exactly where it is used most. `Search` matches USDA, an American-English
generic-food index, so a Russian query only works if the LLM translation step in
`translateAndCache` happens to produce the right USDA vocabulary term — and even then the user
gets "Egg, quail, whole, raw" as the name of what they ate, in the wrong language, with a portion
weight they have to supply in grams. A dish that is not a single reference food ("борщ со
сметаной и два куска хлеба") has no row to select at all: the user has to split it into items by
hand, guess each one's weight, and search each one separately. The fallback is to type seven
macro numbers per item.

Meanwhile the photo path already solves this problem well. `vision.OpenAIClient.Recognize` splits
a meal into items, names each one in the user's Display Language, estimates each one's weight and
its own per-100g `estimated_profile`, and `resolveItems` (`backend/pkg/server/food_upload.go`)
treats that estimate as the primary macro source — reference-database candidates only bind
unconditionally on an exact custom-food name match, and for a non-English Display Language
`retrieveCandidates` skips USDA and Open Food Facts entirely, because an English-vocabulary index
cannot match a Russian Display Name. All of that machinery is reachable only by uploading a
photo. A user who knows perfectly well what they ate, and can say so in one sentence, is routed
to the worst tool in the app.

The issue asks for the same LLM approach on manual input. Everything needed already exists —
the prompt, the response schema, candidate resolution, the estimate fallback, the clarification
loop, the review screen, the confirm flow. What is missing is a text-only entry point into it.

## How

Add a description-first manual entry path: the user writes what they ate in their own words and
language, the model turns that into Food Items, and the meal lands in `pending_review` on the
existing review screen. The structured form stays, demoted to a secondary option.

**A text-only recognition call.** `vision.Client` gains
`Describe(ctx context.Context, description, displayLanguage string) (*RecognizeResult, error)`.
It returns the same `RecognizeResult` the photo path returns and reuses `recognizeJSONSchema`
unchanged, so every downstream stage — `processRecognition`, `resolveItems`, `persistAnalysis`,
the clarify loop — works on it without modification. It gets its own system prompt,
`describeSystemPrompt`, because the evidence is different: the photo prompt repeatedly instructs
the model to judge from visible evidence, which is meaningless here. The new prompt keeps the
item-splitting and merging rules verbatim (a described stir-fry is still one item; a fillet
beside a side is still two), and replaces the visual instructions with these:

- Read quantities out of the user's own text and convert them to grams ("две сосиски", "тарелка
  борща", "150 г риса"). When the text names no quantity, estimate a typical portion for that
  food and reflect the uncertainty in `confidence`.
- Set `brand` only when the user names one, and `preparation`/`state` only when the text states
  or plainly implies them; otherwise `unknown`.
- Always produce `estimated_profile`. On this path it is usually the *only* macro source — there
  is no photo, and for a non-English Display Language `retrieveCandidates` will not query USDA or
  Open Food Facts at all.
- Ask `clarification_questions` when the text is too vague to size or identify, rather than
  guessing. That routes into the existing `pending_clarification` flow, which is already
  text-only (`vision.Client.Clarify` never re-sends a photo) and therefore already correct for a
  meal that has none.

`languageDirective(displayLanguage)` is appended exactly as `Recognize` appends it, so the
Display Name comes back in the user's language and the Canonical Name in English. Implement
`Describe` on `OpenAIClient` (a single text message, no `image_url` content part, `store:false`
via the shared `call`), on `vision.Fake` (with `DescribeResult`/`DescribeErr`/`DescribeCalls`,
matching the existing fields), and on `vision.Unconfigured` (returns `ErrNotConfigured`).

**The description is persisted on the meal.** `database.FoodMeal` gains
`Description string \`gorm:"type:text" json:"description,omitempty"\``, empty for photo meals and
for structured manual entries. GORM's `AutoMigrate` adds the column; existing rows read back
empty, which is the correct value for them. Persisting it is what makes the meal recoverable: the
analysis call is synchronous and can fail, and without the stored text a failed described meal
would be a dead row with no path forward. It is deliberately left out of
`storage_impl.go`'s `columnAllowlist` for `food_meals`, so the family-visible generic
`GET /api/data/food_meal` path keeps returning only the columns it returns today.

**The endpoint.** `POST /api/food/meals/describe`, in a new `backend/pkg/server/food_describe.go`,
registered next to the existing `/food/meals/manual` line in `server.go` (before the
`/food/meals/{id}` routes, matching how `manual` is already ordered). Body:
`{"description": "...", "name": "...", "logged_at": "..."}` with `name` and `logged_at` optional.
It reads the body through `http.MaxBytesReader` plus `io.ReadAll` before decoding — the same
reasoning `Reanalyze` documents for its 4 KiB cap, since `json.Decoder` stops at the first
complete value and would never trip the limit on trailing padding — with an 8 KiB cap, and
validates the trimmed description at 1 to `maxDescriptionLength` (1000) runes. `logged_at` is
UTC-normalized on write, like every other meal write path. When `name` is omitted, the meal's
`Name` is the description trimmed to 60 runes (with a trailing ellipsis when truncated), so the
history list shows something legible instead of the review screen's fallback text.

The handler follows the photo path's commit-first rule: it creates the `FoodMeal` row with
`Status: processing`, the description, and no `PhotoPath` **before** calling the model, so no
outcome of that call can lose what the user typed. It then runs the analysis with the row's
`UpdatedAt` as the lease token and responds 201 with the resulting meal, using the existing
`reloadIfSuperseded`/`writeReloadedMeal` pair.

**Analysis reuses everything.** `runDescribeAnalysis(ctx, meal, lease)` reads
`DisplayLanguage(h.storage, meal.UserID)`, calls `h.vision.Describe`, and hands the result to the
existing `processRecognition(ctx, meal, recognized, lease, false, displayLanguage)`.
`analyzeDescribedMeal` wraps it with the vision timeout and the `failMeal` fallback, mirroring
`analyzeMeal` — `strict` is `false` here for the same reason it is false for upload and retry:
this path has no prior reviewed content to lose. Passing the user's real Display Language
(rather than `runExpertAnalysis`'s forced `"en"`) is the substance of the fix. For a Russian user
it means the item names come back in Russian, `retrieveCandidates` skips the English-vocabulary
indexes, the fuzzy custom-food match still runs against their own catalog, and macros come from
the model's own `estimated_profile` via `ApplyEstimatedProfile` — which is exactly the behavior
the photo path already has and the search-first form cannot reach.

**Retry.** `RetryMeal` currently 409s on `PhotoPath == ""`. It now branches: a failed or
stale-processing meal with a photo retries through `analyzeMeal` as today; one with no photo but
a stored description retries through `analyzeDescribedMeal`; one with neither still 409s with the
existing message. The optimistic claim, lease handling, and clarify-round reset are unchanged.

**The manual page becomes description-first.** `/food/manual/` leads with a textarea ("Describe
what you ate"), the optional name and time inputs it already has, a live character counter
against the 1000-rune limit, and the same external-provider disclosure the search box carries.
Submitting calls a new `api.describeMeal` and navigates to `/food/review/?meal=<id>`, where the
existing `pending_review` flow — item rows, weight edits, `ItemResolver`, clarification modal,
confirm bar — takes over unchanged. The current item-by-item form moves, behavior untouched,
behind a collapsed disclosure on the same page ("Enter items manually instead"). It is kept, not
deleted, for two reasons: it is the only way to enter exact macros copied off a package label,
and `POST /api/food/meals/manual` is what the E2E suite uses to seed confirmed meals in
`food.spec.ts` and `completeness.spec.ts`. That endpoint's behavior and contract do not change at
all. The review screen additionally shows `meal.description` for a described meal, so the user
can see what the model was given.

New user-facing strings go through the existing `t()` dictionary in both `en.ts` and `ru.ts`, and
`en.ts`'s header comment — which currently records `app/food/manual/page.tsx` and
`ManualItemEditor` as deliberately English-only — is updated to say the description entry path is
translated while the demoted structured form is not. A Russian user typing a Russian description
is the central case of this change; leaving its own input screen untranslated would be an odd
place to stop.

**Deliberately excluded.**

- **Reanalyze for described meals.** `Reanalyze` requires a photo (hint mode re-runs `Recognize`,
  Expert mode calls `EstimateWeights` on the image) and keeps its 409; `ReviewClient` already
  hides the control when `photo_path` is empty, so nothing user-visible regresses. Re-running a
  described meal from an edited description is a coherent follow-up, not part of this change.
- **Changing `POST /api/food/meals/manual`, `GET /api/food/search`, or the translation cache.**
  The structured path and its search box keep working exactly as they do now.
- **Changing the photo path, `resolveItems`, `retrieveCandidates`, or the language gate.** This
  change routes a new input into that machinery; it does not alter it.
- **Asynchronous analysis.** The describe call blocks the request, exactly as photo upload does
  today. A text-only call is cheaper and faster than an image one, so the existing
  `HCW_VISION_TIMEOUT` budget applies unchanged.

**Documentation.** `CONTEXT.md` gains a **Meal Description** term under "Food logging" (the
user's own free-text account of a meal, the input to `Describe`, persisted on the meal and
replayable by retry — distinguished from the photo `Hint`, which only decorates a recognition
call and is never stored). A new `docs/adr/ADR-011-llm-described-manual-food-entry.md` records
the decision that manual entry's primary path is model recognition rather than reference-database
lookup, created as `Proposed` and flipped to `Accepted` as the final commit of the change.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Add the text-only Describe call to the vision package
- [x] Add `Describe(ctx context.Context, description, displayLanguage string) (*RecognizeResult, error)`
      to the `vision.Client` interface in `backend/pkg/vision/vision.go`, with a doc comment
      stating it is text-only, reuses `Recognize`'s response shape and schema, and that
      `displayLanguage` has the same meaning it has on `Recognize`
- [x] Add `describeSystemPrompt` to `backend/pkg/vision/openai.go`: keep
      `recognizeSystemPrompt`'s item-splitting and merging rules, replace the visual-evidence
      instructions with the quantity-reading, portion-estimation, brand/preparation/state, and
      `estimated_profile` rules from `## How`, and keep the `clarification_questions` contract
- [x] Implement `OpenAIClient.Describe`: system message is
      `describeSystemPrompt + languageDirective(displayLanguage)`, user message is the plain
      description text with no `image_url` content part, dispatched through the existing `call`
      with the `food_recognition` schema name and `recognizeJSONSchema`, converted through
      `toRecognizeResult`
- [x] Implement `Unconfigured.Describe` returning `ErrNotConfigured`
- [x] Implement `Fake.Describe` with `DescribeResult`, `DescribeErr`, and a `DescribeCalls`
      slice recording the description and display language, matching the existing fields' shape
- [x] Add a test in `backend/pkg/vision/openai_test.go` asserting a `Describe` request carries
      the description text, carries no image content at all, sets `store:false`, uses the
      `food_recognition` json_schema, and includes the language directive for a non-English
      display language
- [ ] Mark completed

### Task 2: Persist the description on the meal
- [x] Add `Description string \`gorm:"type:text" json:"description,omitempty"\`` to
      `database.FoodMeal` in `backend/pkg/database/models_food.go`, documented as empty for photo
      meals and for structured manual entries
- [x] Confirm `columnAllowlist["food_meals"]` in `backend/pkg/database/storage_impl.go` is left
      unchanged, so the new column is not exposed through the family-visible
      `GET /api/data/food_meal` path
- [x] Confirm nothing in the meal-list or meal-detail response shapes needs widening beyond the
      full `FoodMeal` that `GetMeal` already returns (`MealSummary` stays as it is)
- [x] Mark completed

### Task 3: Add the describe endpoint and its analysis path
- [x] Add `backend/pkg/server/food_describe.go` with `CreateDescribedMeal`, implementing the
      request shape, the 8 KiB body cap via `http.MaxBytesReader` + `io.ReadAll`, the 1 to
      `maxDescriptionLength` (1000) rune validation, UTC normalization of `logged_at`, and the
      60-rune name derivation when `name` is omitted
- [x] Create the meal row (`Status: processing`, description stored, no `PhotoPath`) before any
      model call, then run the analysis using the row's `UpdatedAt` as the lease, and respond 201
      through `reloadIfSuperseded`/`writeReloadedMeal`
- [x] Add `runDescribeAnalysis` and `analyzeDescribedMeal` alongside the existing helpers,
      reading the user's real Display Language via `DisplayLanguage`, calling `vision.Describe`,
      handing off to `processRecognition` with `strict=false`, and falling back to `failMeal` on
      error — documenting why `strict` is false here
- [x] Register `POST /api/food/meals/describe` in `backend/pkg/server/server.go`, next to the
      existing `/food/meals/manual` route and before the `/food/meals/{id}` routes
- [x] Extend `RetryMeal` in `backend/pkg/server/food_retry.go` to re-run the description analysis
      for a photo-less meal that has a stored description, keeping the existing 409 for a meal
      with neither photo nor description
- [x] Mark completed

### Task 4: Backend tests for the describe path
- [x] Add `backend/pkg/server/food_describe_test.go` covering: a successful describe creating a
      `pending_review` meal whose items carry the model's names, weights and estimated macros,
      with the description persisted and the meal aggregate still zero until confirm
- [x] Cover validation: missing/empty description, an over-length description, and an oversized
      request body, each rejected before any `vision.Fake` call is recorded
- [x] Cover a `Describe` failure leaving the meal `failed` with the description still stored, and
      a subsequent `RetryMeal` on that meal re-running `Describe` (assert via `DescribeCalls`)
      rather than returning 409
- [x] Cover the language behavior that motivates this change: with a Russian `display_language`,
      assert `Describe` is called with `"ru"`, that no USDA or Open Food Facts candidate is
      bound, and that items land with `MacroSource` `estimated` from the model's own profile
- [x] Cover a `Describe` response carrying `clarification_questions`, asserting the meal reaches
      `pending_clarification` and that the existing `ClarifyMeal` endpoint carries it through to
      `pending_review` without a photo
- [x] Confirm the existing `food_manual_test.go` suite still passes untouched — the structured
      endpoint's behavior must not change
- [x] Mark completed

### Task 5: Description-first manual entry in the frontend
- [x] Add `description?: string` to the `FoodMeal` interface and a `describeMeal` call to
      `frontend/lib/api.ts`, posting to `/food/meals/describe`
- [x] Rework `frontend/app/food/manual/page.tsx` to lead with the description textarea, the
      optional name and time inputs, a live character counter against the 1000-rune limit, the
      external-provider disclosure, and a submit that navigates to `/food/review/?meal=<id>`
- [x] Move the existing item-by-item form (the `ManualItemEditor` list, its add-item control and
      its own submit) behind a collapsed disclosure on the same page, with its behavior and its
      `POST /api/food/meals/manual` call unchanged
- [x] Show `meal.description` on the review screen for a described meal in
      `frontend/app/food/review/ReviewClient.tsx`, and confirm the reanalyze control stays hidden
      for it (`photo_path` is empty)
- [x] Add the new strings to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`, and update
      `en.ts`'s header comment to record that the description entry path is translated while the
      demoted structured form remains English
- [x] Mark completed

### Task 6: E2E coverage and validation against the deployed stack
- [x] Extend the `Manual meal entry` describe block in `e2e/tests/food.spec.ts` with a test that
      submits a description through the real UI against the deployed stack and waits for the
      review route, asserting it reaches a real outcome (`Review needed`, `Needs clarification`,
      or `Analysis failed`) the way the photo-upload test does, then deletes the meal it created
- [x] Add a test asserting a non-English description is accepted and produces at least one item
      whose name is not forced into English — the behavior the idea reports as broken today
- [x] Update the two existing manual-entry UI tests so they open the structured form's disclosure
      first; leave the `POST /api/food/meals/manual` seeding helpers in `food.spec.ts` and
      `completeness.spec.ts` untouched
- [x] Check whether `e2e/tests/mobile-nav.spec.ts`'s manual-page submit-bar and safe-area tests
      still target the right control after the page rework, and update them if not
- [x] Run `make lint` and `make test`, then deploy the branch to `hcw-wip` and run
      `make test-e2e` against it; fix every failure rather than recording it as pre-existing
- [x] Mark completed

### Task 7: Documentation and the ADR
- [ ] Add a **Meal Description** term to `CONTEXT.md` under "Food logging", including the
      distinction from the photo `Hint` and its `_Avoid_` line, following the file's existing
      entry format
- [ ] Add `docs/adr/ADR-011-llm-described-manual-food-entry.md` with `Status: Proposed`,
      recording why manual entry's primary path is now model recognition rather than
      reference-database lookup, why the structured form is kept rather than removed, and why
      the user's real Display Language is passed here while `runExpertAnalysis` forces `"en"`
- [ ] As the final commit of this change, flip ADR-011's status from `Proposed` to `Accepted`
- [ ] Mark completed
