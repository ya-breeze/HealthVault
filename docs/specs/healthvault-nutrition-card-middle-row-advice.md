# Nutrition card middle row: LLM advice lines under the Healthiness Label
Idea: ya-breeze/idea-forge#227

## Why

The dashboard's nutrition card (registry id `logging_gap`, titled "Питание" / "Nutrition") is built from three rows. The top row shows today's calories and macros against the Nutrition Target. The bottom row compares 28 days of logged food against the weight trend. The middle row is a four-part initiative: the sustainability warning shipped in `docs/specs/healthvault-nutrition-card-middle-row-he.md`, the deterministic Healthiness Label shipped in `docs/specs/healthvault-nutrition-card-middle-row-th.md`, and then the two parts the owner deferred — the LLM advice lines and a nutrition chat. This change builds the advice lines.

The card can compute a judgment but cannot say what to do about it. A label reading "Fair" tells the reader where they stand and nothing else. ADR-004 (`docs/adr/ADR-004-heuristic-food-healthiness-label.md`) settled how to close that: the heuristic produces the judgment, and the LLM sits strictly downstream of it, turning an already-computed label into one or two readable lines such as "add ~40 g protein/day". The LLM never produces the judgment itself.

Every piece this needs already exists, unused:

- **The LLM client and its credentials.** `vision.Client` (`backend/pkg/vision/vision.go`) already carries three text-only calls — `Translate`, `Describe`, `Clarify` — with an `OpenAIClient`, a scripted `Fake` and an `Unconfigured` implementation beside it. `Translate` is 15 lines: a system prompt, a strict JSON schema, one `c.call`. A fourth text-only method follows an established shape rather than inventing one.
- **A cache precedent for LLM output.** `FoodSearchTranslation` (`backend/pkg/database/models_food.go`) is a per-user table holding one model result, upserted on a unique index, read before the model is called, and refreshed on request via `?refresh=true`. `foodHandlers.translateAndCache` (`backend/pkg/server/food.go`) is the whole pattern in 30 lines, including the rule that a failed cache write counts as a failed generation.
- **The context the advice needs.** The Nutrition Target — calories, protein, carbs, fat and BMR — is computed by `computeNutritionTargetForProfile` (`backend/pkg/server/nutrition_target.go`). The caller's display language comes from `displayLanguageFromSettings` and is reduced to the app's supported primary `en` / `ru` code for this call. The caller's Logged Day is `database.LocalDate` over `callerTimezone`.
- **A reserved response field that turned out to be the wrong shape.** `summaryTodayResponse.Recommendation` (`backend/pkg/server/summary_today.go`) has been `null` since the day it was added, waiting for exactly this. It cannot carry the advice, for two reasons given under `How`, so this change retires it rather than leaving a field that documents a plan nobody will follow.

What has been missing is the set of decisions the idea named: where the cache lives and what invalidates it, what triggers generation, what the row shows when the model is unconfigured or the call fails, and how the prompt keeps the advice's tone consistent with the label. This spec settles all four.

## How

### Scope

This change ships the advice lines. The nutrition chat is excluded and deliberately not filed yet — the owner gated filing it on the advice lines being *used*, not merely shipped, and its persistence model (an ongoing thread versus ephemeral per session) is still an open design question. The owner will decide when use has justified a follow-up idea.

### Where the label comes from, and the ordering this change sits in

ADR-004 puts the LLM downstream of the label, so the advice cannot be generated without one. The Healthiness Label change has landed: `frontend/lib/healthiness.ts` computes the three-level label (Good / Fair / Needs attention) and its reason codes over a rolling 7-day window from per-day macro, sugar and sodium sums returned by `GET /api/food/daily-totals`. Use its exact exported names when wiring the card rather than introducing a parallel label shape.

The backend half depends on none of that, which is why the task order puts it first. The endpoint takes the label and its reason codes as request fields and validates their *shape*, not their vocabulary. It never re-derives the label and never second-guesses it. If the label change's reason codes change later, nothing on the backend needs updating.

The middle row's precedence rule is settled and this change honours it: sustainability warning first, then the label, then the advice. The label renders only when `evaluateSustainability` returns an empty array, and the advice renders only under a rendered label. So a card showing a sustainability warning shows no advice at all, which also disposes of the worst tone conflict available — advice cannot cheerfully suggest a protein target beneath a warning that the reader is eating below their BMR.

### The seam: one new method on `vision.Client`

```go
// AdviceInput is everything the model is told. Label and Reasons come from
// the caller's already-computed Healthiness Label; the target figures come
// from computeNutritionTargetForProfile, never from the request.
type AdviceInput struct {
	Label              string   // "good" | "fair" | "needs_attention"
	Reasons            []string // the label's own reason codes, in heuristic priority order
	MeanCalories       float64
	MeanProteinGrams   float64
	MeanCarbsGrams     float64
	MeanFatGrams       float64
	MeanSugarGrams     float64
	MeanSodiumGrams    float64
	TargetCalories     int
	TargetProteinGrams int
	TargetCarbsGrams   int
	TargetFatGrams     int
	DisplayLanguage    string
}

// Advise turns an already-computed Healthiness Label into one or two short
// lines of advice, in DisplayLanguage. Text-only, no image.
Advise(ctx context.Context, in AdviceInput) ([]string, error)
```

One method, one input struct, a slice of strings out. `Unconfigured` returns `ErrNotConfigured` like its five siblings. `Fake` gains `AdviseResult`, `AdviseErr` and `AdviseCalls` so a test can assert on exactly what was sent and on how many calls were made. `OpenAIClient.Advise` follows `Translate`: a system prompt, a strict JSON schema with a single `lines` array of strings, one `c.call`, one unmarshal.

The seven-day mean figures ride in from the client rather than being re-aggregated server-side. That is the same argument `summaryTargetPayload`'s doc comment already makes about its derivation fields: the numbers in the advice must be the numbers behind the label on screen, and a second server-side aggregation could legitimately return a different mean than the one the label was computed from. It also keeps this change independent of whatever shape the label change gives `GET /api/food/daily-totals`. Extend `HealthinessResult` with a `means` object containing calories, protein, carbs, fat, sugar and sodium per eligible day, computed inside `computeHealthinessLabel` from the exact same window and `isValidDay`-filtered set it already pools for the verdict. The card consumes that object rather than duplicating the eligibility and aggregation rules. The means remain `float64` through `AdviceInput`, because the endpoint accepts finite JSON numbers and the label computes real-valued means; silently truncating or rounding an accepted input would make the prompt differ from the figures behind the label. The target figures are the opposite case and are computed server-side: the client has no business telling the server what the user's target is. An available response therefore echoes the server-effective target figures and display language as `context`; the client uses those values when it associates the result with its render signature. That closes the ordinary race where the summary or `useLanguage()` still holds an older value while the endpoint has already read a newer settings, weight or goal record.

### The prompt, and keeping its tone consistent with the label

ADR-004 flags a "Fair" label with alarmed advice text as a prompt-design problem to solve here. Three prompt rules solve it:

1. **The label is a given, not a question.** The system prompt states that the label was computed by a deterministic heuristic, that it is correct, and that the reply must never restate, dispute, upgrade or downgrade it.
2. **Tone is keyed to the label, in the prompt.** `good` — confirm what is working and offer at most one small refinement. `fair` — one concrete adjustment, in a neutral register. `needs_attention` — direct and specific, still calm; no alarm words, no prognosis.
3. **Fixed prohibitions.** No medical claims, no diagnosis, no supplements, no fasting or cleanse suggestions, no calorie prescription below the stated target, and no measurements, thresholds or quantities except values supplied in the input or simple differences derived directly from them. Derived differences are allowed because advice such as "add ~40 g protein/day" must subtract mean intake from the supplied target. One or two lines, one clause each, at most about 90 characters, written in the caller's display language.

The reason codes reach the prompt verbatim, so their shape is bounded before they get there: at most six codes, each matching `^[a-z][a-z_]{0,31}$`, deduplicated while preserving first occurrence. That is at most ~200 bytes of lowercase letters and underscores — the same "bound how much caller-controlled text reaches the model" rule `normalizeDisplayLanguage` already applies to the language tag, for the same reason. Do not sort them: `HealthinessResult.reasons` deliberately puts `far` findings before `off` findings and then uses a fixed signal priority, so alphabetical sorting can make the advice lead with the less important reason.

The model's output is post-processed deterministically rather than trusted: trim each line, drop empties, truncate each to 120 runes, keep at most the first two. If nothing survives, the call counts as a failure and nothing is cached.

### The cache: one row per user, keyed on the Logged Day and the complete advice input

```go
// FoodAdvice is a user's cached Healthiness Label advice — one row per user,
// overwritten in place rather than accumulating history.
type FoodAdvice struct {
	models.TenantModel
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	LoggedDay   string    // YYYY-MM-DD in the caller's timezone
	Label       string
	ReasonCodes string // normalized, de-duplicated, comma-joined in priority order
	Language    string
	InputHash   string // SHA-256 of the canonical, normalized AdviceInput
	Lines       string // JSON array of strings
	GeneratedAt time.Time
}
```

A cached row is served when `LoggedDay` and `InputHash` match the request. `InputHash` is the hex SHA-256 of a canonical JSON encoding of the fully normalized `AdviceInput`, after order-preserving reason-code de-duplication and after the server has supplied the Nutrition Target. `Label`, `ReasonCodes` and `Language` remain as explicit columns so the row is inspectable, but the hash covers them plus every mean and target figure. Any mismatch regenerates. That makes the invalidation rule readable in one sentence: **the advice is regenerated when the day rolls over or when anything the model is told changes.**

Ending the label window yesterday does not make its inputs immutable. A confirmed meal from a prior day can still have an item added, edited or deleted, and the Nutrition Target can change after a profile, goal-weight, weight or activity update. The label and reason codes can remain the same across those changes while the useful numerical advice changes, so all mean and target figures belong in `InputHash`. Hash the values the model actually receives, without an extra rounding step, so the cache comparison and prompt cannot disagree.

**Generation is lazy, on read.** There is no scheduler in this backend, and adding one for this would generate advice for a day the user never opens the dashboard on, paying for a call nobody reads. It would also have to guess the label, which is computed on the client. Serialize all advice generation with an advice mutex on `foodHandlers`. After acquiring it, a non-refresh miss re-reads the cache; two tabs loading together must produce one model call rather than both observing the miss. A user-triggered `refresh: true` acquires the same mutex but deliberately skips that second cache read, so every explicit refresh remains an additional call.

### The endpoint: `POST /api/food/advice`

A POST, not a GET with query parameters, for two reasons. The request carries a structured context object — label, reason codes and six numbers — that does not belong in a query string. And generating advice is a real side effect: a paid model call and a cache write. Protect those side effects with `isSameOriginRequest`, just as `foodHandlers.Search` does: a sibling origin can issue a simple credentialed `fetch` with `Content-Type: text/plain` and an exact JSON body, so using JSON does not by itself stop a page from riding the caller's same-site cookie. Reject a browser request whose `Sec-Fetch-Site` is anything other than `same-origin` with 403 before reading or writing the cache or calling the model; retain the helper's existing missing-header allowance for non-browser clients and backend tests.

Request body:

```json
{
  "label": "fair",
  "reasons": ["protein_low", "sugar_high"],
  "window": {
    "mean_calories": 1820, "mean_protein_grams": 74, "mean_carbs_grams": 210,
    "mean_fat_grams": 62, "mean_sugar_grams": 88, "mean_sodium_grams": 3.1
  },
  "refresh": false
}
```

Self-only, with no `?user=` override, matching `GetFoodDailyTotals` and `GetCompleteness`. `label` must be one of the three values; `reasons` must pass the shape rule above; every `window` figure must be a finite number between 0 and 100000. Preserve accepted fractional values through `AdviceInput`. Anything else is a 400 — a malformed body is a client bug, not a state to render.

Response, always 200 when an authenticated same-origin request is well-formed:

```json
{"available": true,  "lines": ["...", "..."], "generated_at": "2026-09-05T06:12:00Z", "context": {"target_calories": 2500, "target_protein_grams": 110, "target_carbs_grams": 278, "target_fat_grams": 105, "display_language": "en"}}
{"available": false, "reason": "unconfigured"}
{"available": false, "reason": "unavailable"}
```

The `available` / `reason` shape is `summaryTargetPayload`'s, and for its reason: a deployment with no OpenAI key is a normal, expected state, not an error. The available variant's `context` reports the four target figures and the normalized `en` / `ru` display language the server actually supplied to `AdviceInput`; it is required on both cache hits and fresh generations so the client never attributes advice generated from a newer target or language to an older card state. `unconfigured` is returned when the client is `vision.Unconfigured` (checked by `errors.Is(err, vision.ErrNotConfigured)`), `unavailable` for any other failure — an API error, a timeout, an empty result after post-processing, or a failed cache write. A failure is never cached, so the next load retries.

`refresh: true` skips the cache read and regenerates, then upserts. A failed refresh leaves the existing row untouched and answers `unavailable`, exactly as `translateAndCache` leaves a stale translation in place.

### Rendering

The advice is a fifth request and must not delay the four the card already makes. It goes out from its own `useEffect` after the main load resolves. Key that effect on a stable signature of the complete normalized request context — the label, priority-ordered reason codes and all six mean figures — plus the target figures and the current `language` from `useLanguage()`. Use the context value rather than treating the `display_language` captured by one main load as a reactive language source. When an available response arrives, build its stored signature from the request's label, reasons and means plus the response's server-effective target and language context; do not stamp it with the target and language captured by the request. Render it only when that stored signature still equals the current one. Also clear the stored result and refresh error in the effect before starting or skipping the next request. The render-time equality check matters because `useEffect` runs after paint, so an effect-only reset can still show the previous result for one render under the new label, figures or language. Without either guard, the previous result can remain indefinitely when the replacement returns `unavailable`; without the server context, an initial English render can briefly claim a Russian result, or a stale target can claim advice generated from a newer one. The card is fully readable before the advice arrives, and the row grows when it lands.

The block renders under the label, inside the middle row, only when `available` is true and at least one line came back. Testids follow the existing names: `nutrition-advice` on the block, `nutrition-advice-line` on each line, `nutrition-advice-refresh` on the "get advice" control, `nutrition-advice-error` on the failure line.

What the row shows in each state:

- **Cached or freshly generated lines** — the lines, plus the refresh control.
- **`unconfigured`** — nothing at all, and no refresh control. A deployment without a key must not advertise a feature it does not have.
- **`unavailable` on load** — nothing. The reader did not ask for advice, and an error line for something they never requested is noise.
- **`unavailable` after the user pressed refresh** — one line, `loggingGap.adviceUnavailable`. They asked, so they get an answer.

The refresh control is a `TapTarget`, disabled while a request is in flight, showing `loggingGap.adviceRefreshing` for the duration. New keys keep the `loggingGap.` prefix, for the reason already recorded in `frontend/lib/i18n/en.ts`: the prefix is internal and renaming it is deferred to one future change. `loggingGap.adviceDetail` joins the existing hint disclosure whenever advice is on screen, and says the plain truth — the lines are written by an AI model from the label, the reason codes and the Nutrition Target; they are cached for the current day and regenerate when those inputs change or the reader requests a refresh; they are not medical advice.

### Retiring `summaryTodayResponse.recommendation`

The reserved field cannot carry this. `GET /api/summary/today` is on the dashboard's critical path, and the advice needs a label the server does not have — the label is computed on the client, from data the summary endpoint does not fetch. Putting an LLM call behind that response would also put model latency in front of the card's top row, which is the exact cost ADR-004 refused for the label itself.

So the field goes: removed from `summaryTodayResponse`, from its `nil` assignment, from `TodaySummary` in `frontend/lib/api.ts`, from the e2e fixture, and from the assertion in `summary_today_test.go`. Nothing reads it — the frontend types it as the literal `null`. Leaving a permanently-null field in place would document a plan that this spec has just decided against.

### ADRs

No new ADR. ADR-004 already decided everything architectural here — heuristic label, LLM downstream of it, cached daily generation, a user-triggered refresh. The prompt design and the cache key are implementation detail, which belongs in this `How` section.

ADR-004 is already `Accepted`; the Healthiness Label change flipped it when the deterministic half shipped. As the last commit, add a short `> **Update (`docs/specs/healthvault-nutrition-card-middle-row-advice.md`, <date>):**` note recording that this change ships the cached LLM advice lines while the chat remains deferred. Do not rewrite the accepted decision or its earlier update.

### What the owner still has to do

Nothing gates this change, but one check is the owner's alone. The tests here drive `vision.Fake`, and the e2e suite mocks `POST /api/food/advice`, so no OpenAI key is needed to validate the change. Confirming that the real model produces advice in the right tone and the right language requires `HCW_OPENAI_API_KEY` in the deployed stack's environment, which lives in a file this pipeline cannot read. Read the generated lines on the deployed stack once, in both languages, and tune the prompt's tone rules if they read wrong.

### Deliberately excluded

- **The nutrition chat.** Its own idea, gated by the owner on these advice lines being used rather than merely shipped, and carrying an unresolved design question about persistence.
- **Server-side rate limiting on refresh.** The control is disabled while in flight, and this deployment serves one user. A limiter would be complexity that only pays off at a concurrency this app does not have.
- **Caching failures.** A failed call is retried on the next load. A negative cache would save a handful of calls a year and would hide a newly-fixed configuration for the rest of the day.
- **Advice under a sustainability warning.** The precedence rule already excludes it, and that is the right answer rather than a limitation.
- **Re-deriving the label server-side.** ADR-004 puts the judgment in the heuristic. The endpoint takes the label as given.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: The Advise call on the vision client
- [x] Add `AdviceInput` and `Advise(ctx context.Context, in AdviceInput) ([]string, error)` to the `Client` interface in `backend/pkg/vision/vision.go`, documenting that it is text-only, that `Label` and `Reasons` are already-computed inputs it must never dispute, and that the returned lines are in `DisplayLanguage`
- [x] Implement `Advise` on `Unconfigured` in `backend/pkg/vision/unconfigured.go`, returning `ErrNotConfigured` like its siblings
- [x] Add `AdviseResult []string`, `AdviseErr error` and `AdviseCalls []AdviceInput` to `Fake` in `backend/pkg/vision/fake.go`, recording every call's full input
- [x] Implement `OpenAIClient.Advise` in `backend/pkg/vision/openai.go` following `Translate`'s shape: an `adviceJSONSchema` with a single required `lines` array of strings and `additionalProperties: false`, an `adviceSystemPrompt`, one `c.call` with schema name `nutrition_advice`, one unmarshal
- [x] Write `adviceSystemPrompt` with the three tone rules from `How`: the label is a given and must not be restated or disputed; tone keyed per label value (`good` / `fair` / `needs_attention`); and the fixed prohibitions — no medical claims, no diagnosis, no supplements, no fasting, no calorie prescription below the stated target, and no quantities except supplied values or simple differences derived directly from them, one or two lines of at most one clause and about 90 characters each, written in the caller's display language
- [x] Post-process the model's output inside `Advise`: trim each line, drop empties, truncate each to 120 runes, keep at most the first two, and return an error when nothing survives
- [x] Extend `backend/pkg/vision/openai_test.go` in the style of its existing text-only cases: assert the request body carries no image content, that the label, reason codes, target figures and language all reach the prompt, and that a four-line model reply is truncated to two
- [x] Mark completed

### Task 2: The advice cache table
- [ ] Add `FoodAdvice` to `backend/pkg/database/models_food.go` with `UserID` (unique index), `LoggedDay`, `Label`, `ReasonCodes`, `Language`, `InputHash`, `Lines` and `GeneratedAt`, following `FoodSearchTranslation`'s shape
- [ ] Document on the model that the row is overwritten in place rather than accumulating history, that `Lines` is a JSON array of strings, and that freshness requires the Logged Day plus a hash of the complete normalized `AdviceInput` because prior-day food and Nutrition Target inputs remain editable
- [ ] Register `&FoodAdvice{}` in `db.AutoMigrate` in `backend/pkg/database/db.go`, beside `&FoodDayCompletion{}`
- [ ] Mark completed

### Task 3: The POST /api/food/advice endpoint
- [ ] Create `backend/pkg/server/food_advice.go` with the request and response types from `How`, and register `api.HandleFunc("/food/advice", fh.PostFoodAdvice).Methods("POST")` in `backend/pkg/server/server.go` beside the other `/food/*` routes
- [ ] Require `isSameOriginRequest` before any cache access or model call, answering 403 for a browser `Sec-Fetch-Site` other than `same-origin`; preserve the helper's missing-header allowance for non-browser clients and tests
- [ ] Validate the request: `label` one of `good` / `fair` / `needs_attention`; at most six `reasons`, each matching `^[a-z][a-z_]{0,31}$`, de-duplicated without changing their first-occurrence priority order; every `window` figure finite and between 0 and 100000 and preserved as a `float64`. Answer 400 for anything else, and 401 when `ClaimsFromCtx` is nil
- [ ] Resolve the caller's Logged Day via `callerTimezone` plus `database.LocalDate`, their display language via the settings blob (reduced to the supported primary `en` / `ru` code), and their Nutrition Target via `computeNutritionTargetForProfile`; when the target is unavailable, answer `{"available": false, "reason": "unavailable"}` without calling the model
- [ ] Build the normalized `AdviceInput`, preserving the heuristic's reason priority order, hash its canonical JSON encoding, and serve the cached row only when both `LoggedDay` and `InputHash` match; otherwise call `Advise` inside `h.visionTimeout` and upsert the row with `clause.OnConflict` on `user_id`, the way `translateAndCache` does
- [ ] Add an advice mutex to `foodHandlers` and hold it across every `Advise` call; after acquiring it, repeat the cache lookup for non-refresh misses so concurrent first loads make one paid call, while explicit refresh requests skip the repeated lookup and are serialized but not coalesced
- [ ] Honour `refresh: true` by skipping the cache read and regenerating; on failure leave any existing row untouched and answer `unavailable`
- [ ] Map failures: `errors.Is(err, vision.ErrNotConfigured)` to `reason: "unconfigured"`, everything else — API error, timeout, empty result, failed cache write — to `reason: "unavailable"`, logging each at warn level with the user id, and never caching a failure
- [ ] Remove `Recommendation` from `summaryTodayResponse` and its `nil` assignment in `backend/pkg/server/summary_today.go`, update that struct's doc comment to say the advice moved to its own endpoint and why, and drop the `Recommendation` field and assertion from `backend/pkg/server/summary_today_test.go`
- [ ] Add `backend/pkg/server/food_advice_test.go` covering: a cache hit serving without calling `Advise` (a `Fake` whose `AdviseErr` fails the test if called, mirroring `TestFoodSearch_CacheHitSkipsTranslation`); a miss generating and persisting a row; regeneration on each of a changed Logged Day, label, reason codes, language, mean figure and target figure; a `far` reason followed by an `off` reason reaching `Advise` in that priority order; a fractional mean reaching `Advise` unchanged; both cache-hit and generated available responses carrying the server-effective target and normalized display-language context; two concurrent non-refresh misses producing one `Advise` call; `refresh: true` regenerating over a fresh row; `vision.Unconfigured` yielding `unconfigured`; an `Advise` error yielding `unavailable` with no row written; a rejected label and a malformed reason code each yielding 400; a same-site or cross-site browser request yielding 403 with no `Advise` call or cache write; and no claims yielding 401
- [ ] Mark completed

### Task 4: The client and the card
- [ ] Add the request and response types plus `api.getNutritionAdvice(...)` to `frontend/lib/api.ts`, discriminated on `available` the way `TodaySummaryTarget` is, so no caller can read `lines` or the required server-effective `context` without checking first; remove `recommendation` from `TodaySummary` and its doc-comment paragraph
- [ ] Extend `HealthinessResult` in `frontend/lib/healthiness.ts` with a typed `means` object for calories, protein, carbs, fat, sugar and sodium per eligible day; compute it from the same window and `isValidDay`-filtered set already used for the verdict, and add unit coverage proving ineligible/out-of-window days are excluded and fractional means are preserved
- [ ] Use `HealthinessResult` for the label, reason codes and all six mean figures rather than re-deriving any of them in the card
- [ ] Add a second `useEffect` in `frontend/components/LoggingGapCard.tsx`, keyed on a stable signature of the full normalized advice request plus the target figures and the current `language` from `useLanguage()`, that requests advice only when the sustainability warnings are empty and a label is on screen; on an available response, build the stored signature from the request label/reasons/means and the response's server-effective target/language context, render only a result whose stored signature still matches, clear any prior advice and refresh error before requesting or whenever those preconditions stop holding, keep it out of the existing four-request load so the card renders before the advice arrives, and give this effect its own `cancelled` flag following the main effect's pattern
- [ ] Render the advice block under the label with testids `nutrition-advice` and `nutrition-advice-line`, and a `TapTarget` refresh control with testid `nutrition-advice-refresh`, disabled while a request is in flight
- [ ] Implement the four display states from `How`: lines plus control; nothing at all for `unconfigured`; nothing for `unavailable` on load; a single `nutrition-advice-error` line for `unavailable` after a user-triggered refresh
- [ ] Show `loggingGap.adviceDetail` inside the existing hint disclosure whenever advice lines are on screen
- [ ] Mark completed

### Task 5: Copy in both languages
- [ ] Add `loggingGap.adviceRefresh`, `loggingGap.adviceRefreshing`, `loggingGap.adviceUnavailable` and `loggingGap.adviceDetail` to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`, keeping the `loggingGap.` prefix
- [ ] Write `loggingGap.adviceDetail` to say what was actually done: the lines are written by an AI model from the label, its reason codes and the Nutrition Target, they are cached for the current day and regenerate when those inputs change or the reader requests a refresh, and they are not medical advice
- [ ] Confirm the two dictionaries have identical key sets
- [ ] Mark completed

### Task 6: End-to-end coverage, documentation and validation
- [ ] Route `**/api/food/advice` in `mockLoggingGapApis` (`e2e/tests/logging-gap.spec.ts`), defaulting to `{"available": false, "reason": "unconfigured"}` so every existing fixture keeps its current assertions, and drop `recommendation: null` from the summary fixture
- [ ] Add a case where the endpoint returns two lines with context matching the mocked summary target and current display language: assert both `nutrition-advice-line` elements are visible and that the block sits under the label
- [ ] Add stale-context cases where an available response reports, in turn, a different target and a different display language from the card currently rendered, asserting its advice never appears in either case; these must fail if either part of the response-context signature guard is deleted
- [ ] Add a refresh case whose two available responses carry matching context: click `nutrition-advice-refresh`, assert a second request was issued with `refresh: true`, and assert the newly returned line replaces the old one
- [ ] Add an `unconfigured` case asserting `nutrition-advice` and `nutrition-advice-refresh` are both absent, and an `unavailable`-on-load case asserting the same
- [ ] Add an `unavailable`-after-refresh case asserting `nutrition-advice-error` is visible
- [ ] Add the precedence regression: with a sustainability warning firing, assert `nutrition-advice` is absent and no request to `/api/food/advice` was made
- [ ] Update `todo.md`'s Phase 4 section to record the advice lines as shipped and to name the nutrition chat as the one remaining part of the middle row
- [ ] As the last commit, add an `> **Update:**` note to the already-`Accepted` `docs/adr/ADR-004-heuristic-food-healthiness-label.md` naming this spec, recording that the cached advice lines shipped and that the chat remains deferred; do not rewrite the accepted decision or its earlier update
- [ ] Run `make lint` and `make test` and fix everything they report
- [ ] Deploy the branch to the WIP stack and run `make test-e2e` against it, fixing every failure rather than recording it as pre-existing
- [ ] Mark completed
