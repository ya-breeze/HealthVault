# Nutrition card: explain the target numbers behind an ⓘ

Idea: ya-breeze/idea-forge#222

## Why

The Nutrition card's top row is four pairs of numbers and nothing else. `LoggingGapCard.tsx`
renders `loggingGap.todayCalories` ("Сегодня 1200 / 2500 ккал") and the three macro strings
("Белки 80/150 г", "Углеводы", "Жиры"). The first number in each pair is today's intake. The
second is the Nutrition Target. Nothing on the dashboard says so, and nothing says where the
target came from. The owner reported exactly that: the card shows numbers without explaining
them, and asked for an ⓘ that reveals the detail.

The second number is the least guessable thing on the card, because the user never typed it. It
is computed on every read (`computeNutritionTargetForProfile`, `backend/pkg/server/nutrition_target.go`)
out of five inputs that live in five different places: the latest `weight` record, the latest
`height` record, birthdate and sex from the profile settings blob, the latest `weight_goal`
record, and an activity multiplier that is either the manual `activity_override` or a tier
inferred from the trailing 28 days of steps (ADR-006). Protein is not sized from the same weight
as calories — ADR-003 splits them deliberately, calories from measured weight and protein from
goal weight — which is precisely the kind of thing a reader cannot reconstruct from a rendered
"150 г". The target also moves on its own: a new weigh-in or a shift in the step average changes
it with no user action, so a reader who does not know its inputs sees an unexplained number
change.

The app already computes and returns all of that. `GET /api/users/me/nutrition-target` answers
with the full derivation — `bmr`, `measured_weight_kg`, `goal_weight_kg`, `height_m`,
`age_years`, `sex`, `activity_multiplier`, `activity_tier` — and, as `api.getNutritionTarget`'s
own comment records, it has no caller in the app today. The information exists and reaches
nobody.

The affordance the idea asks for also already exists on this card. The gap line carries an ⓘ
disclosure (`logging-gap-hint-toggle` / `logging-gap-hint`, added by
`docs/specs/nutrition-card-footnotes-behind-a-hint.md`) holding the photo-estimation and
activity-multiplier caveats. This change gives the top row the same treatment, for the same
reason that spec gave: the explanation is worth reading once, carries no per-day information,
and must not cost the card's height every day.

## How

**A second ⓘ disclosure, on the top row, mirroring the one the gap line already has.** A separate
`useState` boolean and a separate `useId`, not a shared one: the two panels explain different
rows and a reader opening one has not asked for the other. The control is the calorie line — a
`TapTarget compactOnMouse` wrapping the existing `nutrition-today-calories` figure with a
trailing `InfoIcon`, carrying `aria-expanded`, `aria-controls`, an `sr-only` label and
`data-testid="nutrition-today-hint-toggle"`. The calorie pair is the one that raises the
question first, and the whole-row target is what the gap line's spec already settled on: on
touch `TapTarget`'s 48px minimum applies to a bare glyph anyway, so a full-width row buys a
target that is hard to miss for height that is spent regardless. The panel
(`data-testid="nutrition-today-hint"`) renders *below* the macro row, because it explains the
macros too, and stays mounted with the `hidden` attribute so `aria-controls` never dangles —
both exactly as `renderGap` does it.

**The derivation travels in the summary response, not in a second request.** `summaryTargetPayload`
(`backend/pkg/server/summary_today.go`) gains the seven derivation fields; `SummaryTodayHandler`
already holds them in the `values` it computes and currently throws them away. The alternative —
calling the existing, caller-less `api.getNutritionTarget()`, either eagerly or lazily on first
open — was rejected on correctness rather than cost: the target is computed on read, so a second
call can legitimately return a *different* target from the one on screen if a weigh-in or a step
sync lands between the two, and an explanation that contradicts the number it explains is worse
than no explanation. One response means the numbers and their derivation cannot disagree. It
also adds no computation — `computeNutritionTargetForProfile` already ran — and no request.

The new fields carry no `omitempty`, for the reason already written into that struct's doc
comment: absence must mean "no target", which `available` already says, and must not double as
"this number happened to be zero". They are populated only in the `available` branch.

**`getNutritionTarget`'s doc comment stops being true and is corrected.** It currently justifies
keeping the endpoint's client by saying it returns the derivation "that the summary's target
payload deliberately does not carry". After this change it does carry it. The comment is
rewritten to say the route is kept because the backend still serves it; deleting the route and
its client is out of scope here.

**What the panel says.** Four sentences, all interpolated from the user's own values, plus the
one fact about the *first* number that is equally invisible:

- today's figure counts confirmed meals only, so an unconfirmed photo is not in it
  (`database.TodaySummary`'s rule, and the same one the gap's valid-day filter applies);
- calories: basal metabolism `{bmr}` kcal by Mifflin-St Jeor from weight `{weight}` kg, height
  `{height}` cm, age `{age}`, sex `{sex}`, times activity `{multiplier}` (`{tier}`), giving
  `{calories}` kcal;
- protein: 1.6 g per kg of goal weight `{goal}` kg, giving `{protein}` g — named as the goal
  weight, since ADR-003's split is the single most surprising thing here;
- fat and carbs share what the calorie budget has left after protein, half each by energy,
  except that fat never falls below 0.8 g per kg of goal weight, in which case carbs take the
  remainder;
- and a closing line that the target is recomputed on every load, so a new weigh-in, a new goal
  weight or a changed step average moves it.

The fat/carb sentence describes both branches of `computeNutritionTarget` in one sentence rather
than detecting client-side which one fired. Detecting it would mean reimplementing the floor
comparison and its two constants in TypeScript, giving a second copy of the formula that can
drift from the Go one silently; a sentence that is true either way costs a few words and cannot
rot.

**Copy stays on the `loggingGap.` prefix**, unchanged from the reasoning in
`docs/specs/nutrition-card-today-and-on-track.md`: the prefix is internal and renaming it is
still deferred. The `sr-only` toggle label reuses the existing `loggingGap.hintToggle`; the two
buttons' accessible names differ by the row text they wrap.

**Two enumerations need translating, and the backend's tier names are English literals.**
`activity_tier` arrives as one of `Sedentary`, `Lightly active`, `Moderately active`,
`Very active`, `Extra active` (`backend/pkg/server/activity_level.go`), and `sex` as
`male`/`female`. A frontend map turns each into an i18n key, falling back to the raw string for
an unrecognised tier so a future tier renders as itself rather than as a missing key. That map
and the panel's number formatting (weight to one decimal, height metres to whole centimetres,
kcal/grams/age rounded, the multiplier stringified as-is since the five constants stringify
exactly) live in a new `frontend/lib/nutritionTarget.ts`, not inline in the component: this repo
has no component-test harness — every test under `frontend/` is a `lib/*.test.ts` — so a pure
module is the only way this formatting gets unit coverage.

**Where each layer is pinned.** The payload contract is pinned in Go, where a user can be seeded
and the summary's target asserted field-by-field against what `NutritionTargetHandler` reports
for the same user; a mocked e2e fixture would pass even if the backend never sent the fields.
The e2e suite covers the disclosure itself — hidden by default, opening on click, and the two
toggles being independent — against `mockLoggingGapApis`'s fixture, extended with the new
fields.

**Deliberately excluded.** No new endpoint, and no deletion of `/users/me/nutrition-target` or
its client. No explanation of the *gap* line's arithmetic — that row already has its own ⓘ and
its own caveats. No units preference (the app renders kg and kcal throughout; this panel follows
it). No link into the settings screen from the panel; the "complete your profile" state already
owns that job, and this panel is read by users whose profile is complete by definition.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Carry the target's derivation in `GET /api/summary/today`
- [x] Add `MeasuredWeightKg`, `GoalWeightKg`, `HeightM`, `AgeYears`, `Sex`, `ActivityMultiplier`
      and `ActivityTier` to `summaryTargetPayload` in `backend/pkg/server/summary_today.go`, with
      the same JSON names `nutritionTargetValues` uses and no `omitempty` on any of them
- [x] Extend that struct's doc comment so the no-`omitempty` rule covers the new fields, and say
      why the derivation rides along here rather than being fetched separately (one response
      means the explanation cannot describe a target other than the one rendered)
- [x] Populate the new fields in `SummaryTodayHandler` from the `values` it already has, inside
      the existing `unavailableReason == ""` branch only
- [x] In `backend/pkg/server/summary_today_test.go`, extend `summaryTodayTestResponse` and
      `TestSummaryToday_TargetAvailable` to assert every derivation field is populated and
      matches what `NutritionTargetHandler` returns for the same seeded user
- [x] Extend the raw-JSON key-presence assertion (`TestSummaryToday_ZeroTargetFieldIsStillPresent`'s
      approach) to the new keys, so a later `omitempty` cannot silently drop one
- [x] Add a case asserting the derivation fields stay zero/empty when `available` is false
- [x] Mark completed

### Task 2: Type the derivation on the client
- [x] Add the seven fields to the `available: true` branch of `TodaySummaryTarget` in
      `frontend/lib/api.ts`, typed against the Go tags, with `sex` as `'male' | 'female'`
- [x] Rewrite `api.getNutritionTarget`'s doc comment: the claim that the summary payload
      "deliberately does not carry" the derivation is no longer true, so the route's client is
      kept only because the backend still serves the route
- [x] Extend `TodayRow` in `frontend/components/LoggingGapCard.tsx` to carry the derivation
      alongside the consumed and target values, and populate it where `todayRow` is built
- [x] Mark completed

### Task 3: A pure module for the panel's labels and formatting
- [x] Add `frontend/lib/nutritionTarget.ts` exporting a tier-name-to-i18n-key map for the five
      names in `backend/pkg/server/activity_level.go`, a sex-to-key map, and a function turning
      an available target into the interpolation values the panel needs
- [x] Have the tier lookup fall back to the raw backend string when the name is unrecognised, so
      a tier added later renders as itself instead of a missing key
- [x] Format weight and goal weight to one decimal, height as whole centimetres from metres,
      kcal/grams/age rounded, and the multiplier stringified directly
- [x] Add `frontend/lib/nutritionTarget.test.ts` covering all five tiers, both sexes, the unknown
      tier fallback, the metres-to-centimetres conversion, and that each of the five multiplier
      constants stringifies exactly (`1.2`, `1.375`, `1.55`, `1.725`, `1.9`)
- [x] Mark completed

### Task 4: Render the top row's ⓘ disclosure
- [x] In `LoggingGapCard.tsx`, add a `todayHintOpen` `useState(false)` and a second `useId`, kept
      separate from `hintOpen`/`hintId` so the two panels open independently
- [x] Make the calorie line a `TapTarget compactOnMouse` with a trailing `InfoIcon`, an `sr-only`
      `loggingGap.hintToggle` label, `aria-expanded`, `aria-controls` and
      `data-testid="nutrition-today-hint-toggle"`, leaving `data-testid="nutrition-today-calories"`
      on the figure itself so existing assertions keep working
- [x] Render the panel after the macro row as an always-mounted div with `id`, `hidden={!open}`,
      `data-testid="nutrition-today-hint"` and the same `text-xs text-text-muted` voice the gap
      hint uses
- [x] Fill the panel with the five sentences from `## How`, interpolating the values from
      `frontend/lib/nutritionTarget.ts`
- [x] Comment why the derivation is read from the summary rather than fetched, and why the two
      disclosures do not share state
- [x] Mark completed

### Task 5: Copy in both languages
- [x] Add the panel's keys to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts` under the
      `loggingGap.` prefix: the confirmed-meals sentence, the calorie derivation sentence, the
      protein sentence, the fat/carb sentence, and the recomputed-on-every-load sentence
- [x] Add the two sex labels and the five activity-tier labels in both files
- [x] Keep the calorie sentence's placeholders and the Russian wording consistent with the card's
      existing voice ("ккал", "г", "кг"), and name the goal weight explicitly in the protein
      sentence
- [x] Confirm the two dictionaries still have identical key sets
- [x] Mark completed

### Task 6: End-to-end coverage
- [ ] Extend `mockLoggingGapApis`'s available-target fixture in `e2e/tests/logging-gap.spec.ts`
      with the seven derivation fields
- [ ] Add a test asserting `nutrition-today-hint` is hidden on load, becomes visible after
      clicking `nutrition-today-hint-toggle`, and then contains the fixture's BMR, activity
      multiplier, tier label and goal weight
- [ ] Assert in the same test that opening the top row's hint leaves `logging-gap-hint` hidden,
      and that opening the gap hint leaves `nutrition-today-hint` hidden
- [ ] Assert the panel names the confirmed-meals rule, since that is the sentence explaining the
      *first* number
- [ ] Mark completed

### Task 7: Validate against the deployed stack
- [ ] Run `make lint` and `make test`, and fix everything they report
- [ ] Deploy the branch to the WIP stack and run `make test-e2e` against it, fixing every failure
      rather than recording it as pre-existing
- [ ] Re-read the card in both languages on the deployed stack and confirm the panel's numbers
      match the row's target numbers exactly
- [ ] Mark completed
