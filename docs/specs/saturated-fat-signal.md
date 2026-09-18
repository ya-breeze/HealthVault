# Track saturated fat and make it a Healthiness Label signal

Idea: ya-breeze/idea-forge#628

## Why

HealthVault stores seven per-item nutrients (calories, protein, carbs, fat, sugar, sodium, dietary
fiber) and the Healthiness Label already judges six of them. Saturated fat is not tracked anywhere
in the codebase. The owner flagged it specifically: it is a nutrient worth watching, and the app
already went through this exact expansion once for fiber (`docs/specs/fiber-signal-actionable-chat-sources.md`),
so the same seam — reference import, the recognition model's per-item estimate, manual entry, the
Healthiness Label, and the nutrition chat — has a known shape to extend.

The owner also asked to populate saturated fat from both of HealthVault's two reference sources
(USDA FoodData Central and Open Food Facts) rather than picking one, specifically so the two
sources' saturated-fat values can be compared for accuracy after some real usage. Checked directly
against the code before writing this: `FoodItem.FdcID`, `FoodItem.OffCode` and
`FoodItem.CustomFoodID` are already mutually exclusive per item (`retrieveCandidates` in
`food_upload.go` queries OFF only when a brand was legible, falls through to USDA otherwise, and
`profileForCandidate` resolves from whichever one the bound candidate came from). So no new
provenance column is needed — a saturated-fat item's source is already recoverable from which ID
column is non-null. This spec's How section documents the comparison query rather than adding
storage for something the schema already answers.

## How

### Track the nutrient itself

Add `SaturatedFatGrams` (`FoodMeal`, `FoodItem`) and `SaturatedFatPer100g` (`CustomFood`,
`NutrientProfile`, and `FoodItem`'s `EstimatedSaturatedFatPer100g` shadow copy) as an 8th field
everywhere the existing 7 nutrients are declared, following the same `NOT NULL DEFAULT 0`
convention as sugar/sodium/fiber — this codebase has no nullable-nutrient precedent anywhere in
the food-logging domain (`MacroSource`/`HasMacros()` and `HasEstimate` already answer "was this
ever measured", not SQL NULL — see `models_food.go`). Wire it through every place the existing 7
already flow: `SetEstimatedProfile`/`EstimatedProfile`/`CustomFood.Profile()`/
`applyScaledProfile`/`FoodMeal.Aggregate`, the USDA and OFF local SQLite schemas/builders/queries
(`usda/usda.go`, `off/off.go`), the USDA FDC nutrient mapping (`usda/fdc.go`, FDC id 1258,
"Fatty acids, total saturated"), the OFF JSONL import (`off/import.go`, `saturated-fat_100g`,
optional at source like sugar/fiber — absence does not fail the completeness gate), the Luna
prompts and structured-output schema (`vision/openai.go`'s Recognize/Describe prompts and
`estimatedProfileSchema`), all four API request DTOs (`manualMealItemRequest`, `patchItemRequest`,
`createItemRequest`, `customFoodRequest`), the daily-totals aggregation
(`database/food_daily_totals.go`), the generic data-table column allowlist
(`storage_impl.go`'s `columnAllowlist["food_meals"]`), and the two zero-reset spots in
`food_upload.go`/`food_meal_detail.go` that already list all 6 existing macro columns literally.

### Make it a 7th deterministic Healthiness Label signal

Model it the way fiber is modeled: a share of pooled macro energy (`9 × saturatedFat / (4P + 4C +
9F)`), the same denominator sugar's share already uses as a proxy for total energy intake. WHO's
2023 guideline ("Saturated Fatty Acid and Trans-Fatty Acid Intake for Adults and Children: WHO
Guideline") recommends no more than 10% of total energy intake from saturated fat — one
evidence-backed boundary, the same shape EFSA's single fiber boundary took. Do not invent a `far`
band the source doesn't support: add a new single-boundary "upper bound only" band type in
`healthiness.ts` (mirroring `LowerOnlyBands`/`evalLowerOnly`'s existing no-`far` shape, just
inverted — `offHigh: 0.10`, `ok` at or below it, `off` above it, `farBoundary: null`). Append the
signal after fiber in `computeHealthinessLabel`'s fixed `evals` order, so the original six signals'
tie-break order is unchanged. Extend `HealthinessSignalCode`, `HealthinessReasonCode`
(`saturated_fat_high`), `HealthinessDayData`, and `HealthinessResult.means`.

Carry the new mean through `AdviceInput`, `NutritionChatInput`, `foodAdviceWindow`/
`normalizeAdviceRequest`, `nutritionAdviceSignature` (LoggingGapCard's own client-side cache
signature), and the advice cache's `InputHash` — the hash already covers the full marshaled
`AdviceInput`, so adding the field busts stale cached advice automatically, no separate
cache-version bump needed. Extend `explain_nutrition_signal`'s accepted signal set
(`nutritionHistorySignals`, the tool's JSON-schema enum, `nutritionSignalMealValue`/
`nutritionSignalItemValue`) and `normalizeNutritionChatSignals`'s no-`far`-boundary exception
(currently `signal.Code == "fiber"`) to include `saturated_fat`, and raise
`nutritionChatMaxSignals` from 6 to 7.

### Surface it during food review

The owner's own Idea text ("за ним нужно следить") asks for this to be watched, not just computed
into a label. Add it to `MealItemRow`'s existing sodium/fiber review line (`item.macro_source !==
'none'`) rather than a new line, and to all three manual-nutrient-entry tuple arrays
(`ManualItemEditor`, `ItemResolver`, `CustomFoodModal`) and their TS interfaces in `api.ts`
(`NutrientValues`, `ManualMealItemInput`, `PatchItemInput`, `CreateItemInput`, `CustomFood`).
Extend the generic raw-record-table column label (`dataColumnMeta.ts`'s `SHARED_COLUMN_LABEL_KEYS`)
the same way fiber already is. `dataTypeMeta.ts`'s `NUTRITION_MACROS` chart-series selector is a
different, narrower thing — checked during review and confirmed it drives only the `type ===
'nutrition'` wearable-ingestion chart (`models.go`'s `Nutrition` struct), never `food_meal`, so a
`saturated_fat_grams` entry there would offer a chart series ingestion never populates. Not added.

### Legacy rows read as zero, not "unmeasured"

Every meal logged before this change reads `saturated_fat_grams = 0` once the migration backfills
it — the same `NOT NULL DEFAULT 0` convention sugar/sodium/fiber already established, with no
"unknown" state of its own (see the `models_food.go` comment above). For the Healthiness Label this
is asymmetric with fiber: fiber's same default reads as *low* fiber (flagged `off`), while
saturated fat's default reads as *zero* saturated fat (trivially `ok`) — so a user's first 7-day
window after rollout can look reassuring on this one signal specifically because it was never
measured, not because it was actually low. Accepted rather than built around: it ages out of the
rolling 7-day window within a week, and a separate "was this ever measured" state would be new
machinery this app's `MacroSource`/`HasEstimate` fields don't otherwise carry for any of the other
7 nutrients either.

### Comparing USDA vs. OFF accuracy later

No code change ships a comparison tool in this round — the owner asked to compare "after some
time," once enough logged data exists. Recorded here so the query is known rather than
rediscovered: a `FoodItem` row with `macro_source = 'reference'` and `fdc_id IS NOT NULL` came from
USDA; one with `off_code IS NOT NULL` came from OFF. Grouping confirmed items by that split and
comparing their `saturated_fat_grams` (scaled back to per-100g by `weight_grams`) against a
trusted external reference for the same food is enough to answer which source is more accurate —
no schema change needed when that comparison is actually run.

### Documentation

Update ADR-004 with a dated `> **Update:**` note (never rewriting its original text) recording
saturated fat as the 7th signal, its WHO-sourced boundary, and that it is appended last in reason
precedence. Update CONTEXT.md's Healthiness Label glossary entry: "six signals" becomes "seven
signals," and the "fiber is last in reason precedence" sentence is corrected — saturated fat is now
last, fiber second-to-last.

### Out of scope

Trans fat and dietary cholesterol are deliberately not added — trans fat is largely eliminated from
the food supply by regulation already, and current dietary guidance no longer treats dietary
cholesterol as a primary marker. Splitting sugar into "added" vs. "total" is also out of scope; the
existing `sugar_grams` field is untouched.

## Validation Commands

- `make lint`
- `make test`
- `BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS="tests/logging-gap.spec.ts --retries=0"`

### Task 1: Track saturated fat as the 8th nutrient field

- [x] Add `SaturatedFatGrams`/`SaturatedFatPer100g` to `FoodMeal`, `FoodItem` (incl. the
      `EstimatedSaturatedFatPer100g` shadow copy), `CustomFood`, and `NutrientProfile`, and wire
      every helper method that already carries the existing 7 fields
- [x] Extend the USDA and OFF local SQLite schemas, builders, and queries; map FDC nutrient id
      1258 and OFF's `saturated-fat_100g` (optional at source, non-gating). A database promoted by
      an import that ran before this column existed is detected at `Open()` (`PRAGMA table_info`)
      and degrades to serving every other field with `saturated_fat` read as 0, rather than every
      search 500ing until an operator reimports — found blocking in the Review Gate, fixed, and
      covered by a regression test building the pre-upgrade schema by hand in both packages
- [x] Extend `DailyTotal`/`DailyTotalsRange`, the `food_meals` column allowlist, and the two
      zero-reset call sites in `food_upload.go`/`food_meal_detail.go`
- [x] Extend all four request DTOs (`manualMealItemRequest`, `patchItemRequest`,
      `createItemRequest`, `customFoodRequest`) and their handler bodies
- [x] Cover the new field in existing model/import/aggregation Go tests
- [x] Mark completed

### Task 2: Feed it to Luna and the Healthiness Label

- [x] Add `saturated_fat_per_100g` to the Recognize/Describe prompts and
      `estimatedProfileSchema`/`recognizeSchemaEstimatedProfile`/`toEstimatedProfile`
- [x] Add the saturated-fat share signal to `healthiness.ts`: new single-upper-boundary band type,
      WHO's 10%-of-energy boundary, appended after fiber in the fixed `evals` order
- [x] Extend `HealthinessDayData`/`HealthinessResult.means`/`HealthinessSignalCode`/
      `HealthinessReasonCode`, and `LoggingGapCard`'s day-data and advice-window building
- [x] Cover the boundary, the appended tie-break order, and the combination rule in
      `healthiness.test.ts`
- [x] Mark completed

### Task 3: Carry it through advice, chat, and cache invalidation

- [x] Add the saturated-fat mean to `AdviceInput`, `NutritionChatInput`, `foodAdviceWindow`,
      `normalizeAdviceRequest`, and `nutritionAdviceSignature`
- [x] Extend `explain_nutrition_signal`'s enum, `nutritionHistorySignals`,
      `nutritionSignalMealValue`/`nutritionSignalItemValue`, and the no-`far`-boundary exception in
      `normalizeNutritionChatSignals`; raise `nutritionChatMaxSignals` to 7
- [x] Cover request validation, the cache-hash change, and the chat signal's no-`far` rule in Go
      tests
- [x] Mark completed

### Task 4: Surface it in review and manual entry

- [x] Add saturated fat to `MealItemRow`'s existing sodium/fiber review line
- [x] Add it to `ManualItemEditor`, `ItemResolver`, `CustomFoodModal`, and their TS interfaces in
      `api.ts`
- [x] Add it to the generic raw-record-table column label (`dataColumnMeta.ts`) and its i18n keys
      (`item.*`, `resolver.*`, `dataTable.column.*`, `loggingGap.healthinessReason.saturated_fat_high`,
      `loggingGap.healthinessHintNote`) in `en.ts`/`ru.ts` — not `dataTypeMeta.ts`'s
      `NUTRITION_MACROS`, which is scoped to the unrelated wearable-ingestion chart (see "Legacy
      rows read as zero" section above)
- [x] Mark completed

### Task 5: Record and validate the result

- [x] Update ADR-004 and CONTEXT.md without rewriting any merged spec
- [x] Run every validation command against the final branch and deployed WIP stack
- [ ] Run the Review Gate and resolve every valid finding
- [ ] Confirm that no task box remains unticked
- [ ] Mark completed
