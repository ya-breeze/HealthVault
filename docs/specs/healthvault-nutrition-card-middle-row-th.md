# Nutrition card middle row: the deterministic Healthiness Label
Idea: ya-breeze/idea-forge#205

## Why

The nutrition card (registry id `logging_gap`, titled "Питание" / "Nutrition") answers two
questions and leaves the obvious third one blank. The top row answers "did I hit today's target":
consumed calories and macros against the Nutrition Target. The bottom row answers "does my log
agree with the scale": the 28-day Logging Gap, now with its `on_track` state. Between them sits
nothing. Neither row answers the question a user actually asks about food, which is whether what
they have been eating is any good — a target can be hit on 2000 kcal of sugar, and a log can match
the scale perfectly while doing it.

The inputs for that answer are already logged and aggregated nowhere. `FoodMeal.SugarGrams` and
`FoodMeal.SodiumGrams` are written on every confirm, alongside the three macros, for every entry
source — photo, described manual, structured manual, custom food. Outside the per-meal review and
edit screens (`ItemResolver.tsx`, `ManualItemEditor.tsx`) nothing reads them. `GET
/api/food/daily-totals` already walks exactly the rows that carry them, per Logged Day, over a
range the card already requests, and returns only `calories` and `unconfirmed_meals`.

ADR-004 settled how the label is computed and has been waiting on this change to ship: a
deterministic heuristic, not an LLM judgment, because the label renders on every dashboard load and
so must stay free, fast, and reproducible, and because it must work regardless of how a meal was
entered. What ADR-004 deliberately did not settle is the one thing an implementation cannot avoid —
the exact thresholds. This change settles them, in code, with the reasoning written down beside
them, and flips ADR-004 from `Proposed` to `Accepted` because the decision it records finally
ships.

## How

### Scope: the label only

The middle row is a four-part initiative. Part one, a sustainability warning for eating below BMR
or losing weight faster than 1%/week, is specified and in flight on its own branch. This change is
part two: the per-day macro totals the label needs, and the label itself. The LLM advice lines and
the nutrition chat are not in it — see the reasons under "Deliberately not in scope".

The middle row's precedence rule is already settled and this change honours it: the sustainability
warning outranks the label, then the label, then the advice lines. The label renders only when the
sustainability check produces no warning.

### The endpoint gains five sums, and no new endpoint appears

`database.DailyTotal` (`backend/pkg/database/food_daily_totals.go`) gains `protein_grams`,
`carbs_grams`, `fat_grams`, `sugar_grams` and `sodium_grams`, summed over the same `confirmed`
meals `Calories` is already summed over, with the same per-day fill so a day with no meals is a
zero entry rather than an absent one. `DailyTotalsRange`'s `db.Select` list grows by those five
columns; nothing else about the query, the window arithmetic, the 92-day cap or the
`unconfirmed_meals` count changes.

A sibling endpoint was the alternative and is worse: the card would then make five requests where
it makes four, over a date range it has already fetched, to read columns of rows it has already
read. Extending the response is additive — an existing client that does not know the new keys is
unaffected — and it keeps one query where two would otherwise disagree about which meals count.

The five new fields carry **no `omitempty`**, for the reason `summaryTargetPayload` learned the
hard way: zero is a legitimate sum, absence must not double as it, and a client typing the field as
required reads `undefined` and renders `NaN`. A backend test asserts on the raw JSON keys rather
than a decoded struct, since decoding cannot tell a missing key from a zero.

`frontend/lib/api.ts`'s `DailyTotal` interface gains the same five fields, typed as required
numbers.

### The heuristic

A new pure module, `frontend/lib/healthiness.ts`, mirroring `loggingGap.ts`: no fetch, no React,
every step unit-testable on its own. The card wires it up; it owns no I/O.

**Window.** The seven Logged Days ending yesterday in the caller's timezone — the last seven days
of the 28-day gap window the card already resolves, so it is a slice of data already in hand and
costs no request. Today is excluded for the same reason Day Completeness excludes it: the day is
still in progress, and half a day of food is not a light diet.

**Eligible days.** A day counts only if it passes the same test the gap's `isValidDay` applies: Day
Completeness is `complete` or `confirmed_complete`, **and** every one of that day's meals reached
`confirmed`. `isValidDay` is exported from `loggingGap.ts` and imported here rather than
reimplemented, so the two rows can never disagree about what "logged" means. The second condition
matters as much here as it does there: a day whose photos failed vision is Complete with macro sums
of zero, and a zero-protein day pooled into the window is a fabricated finding.

**Floor.** At least three eligible days in the seven, the 3-of-7 minimum coverage ADR-007 already
settled, plus a pooled macro energy above zero. Below either, there is no label.

**Pooling, not day-averaging.** Sum each field across the eligible days, then compute shares from
the pooled totals. Averaging per-day shares would give a 400 kcal day the same weight as a 2500
kcal one, which is the wrong answer to "what has this person been eating".

**The denominator is macro energy, not logged calories.** `M = 4·protein_g + 4·carbs_g +
9·fat_g`. `FoodMeal.Calories` is stored independently of the three macros and does not always equal
their 4/4/9 sum — alcohol, fibre, rounding and reference-database inconsistency all separate them.
Shares have to sum to 1, so the denominator has to be the thing they are shares of.

**Five signals, each with three verdicts** (`ok`, `off`, `far`):

| signal | ok | off | far |
|---|---|---|---|
| protein share `4P/M` | 0.15–0.40 | 0.10–0.15 or 0.40–0.45 | <0.10 or >0.45 |
| carb share `4C/M` | 0.25–0.65 | 0.15–0.25 or 0.65–0.72 | <0.15 or >0.72 |
| fat share `9F/M` | 0.20–0.40 | 0.15–0.20 or 0.40–0.48 | <0.15 or >0.48 |
| sugar share `4S/M` | ≤0.15 | 0.15–0.22 | >0.22 |
| mean sodium, g/day | ≤2.3 | 2.3–3.5 | >3.5 |

Boundaries are inclusive on the `ok` side, so a value exactly on a boundary is never the worse
verdict. The thresholds are exported constants with the table above reproduced as their doc
comment; nothing reads them from configuration and nothing tunes them per user.

**Why these numbers, and not the textbook ones.** The macro bands start from the IOM's Acceptable
Macronutrient Distribution Ranges (protein 10–35%, carbs 45–65%, fat 20–35%) and are widened for
one specific reason: this app's own Nutrition Target does not sit inside them.
`computeNutritionTarget` sets protein at 1.6 g per kg of Goal Weight and splits the remaining
calories evenly by energy between carbs and fat, then applies a 0.8 g/kg fat floor that pushes fat
up and carbs down — an ordinary target lands near 22% protein, 39% carbs, 39% fat, and an
aggressive goal weight clamps carbs far lower still. Bands that call the app's own advice unhealthy
are not a heuristic, they are a bug. The widened bands contain every target this app can produce,
which means the label is answering "is this pattern nutritionally sound", not "did you follow your
split" — the top row already answers the latter, and a middle row that repeated it would be worth
nothing.

Sugar is widened for a different reason: `sugar_grams` is **total** sugars (USDA nutrient 2000,
"Sugars, total including NLEA"), not free sugars. WHO's 10%-of-energy limit is a free-sugar limit;
applied to total sugars it flags a diet of fruit and yoghurt. 15% is where total sugars stop being
explicable by whole foods.

Sodium is in grams of **elemental sodium**, not salt — `usda/fdc.go` converts FDC's milligrams to
grams on import. 2.3 g/day is the US dietary upper limit, 5.75 g of salt. It sits at the `ok`
boundary rather than lower because salt added while cooking is invisible to photo recognition and
to most reference rows, so this signal systematically under-reports. That asymmetry is worth
stating plainly: a sodium flag is strong evidence, and the absence of one is not a clean bill. The
hint copy says so.

**Combination.** Any `far` signal, or three or more `off` signals, gives `needs_attention`. One or
two `off` signals give `fair`. All five `ok` gives `good`. The three macro shares are not
independent — they sum to 1, so one being high forces another down — which is why a single `off`
cannot move the label past `fair`, and why the `far` bands are set where a share is extreme enough
that its arithmetic partner is not the explanation.

**Reasons.** Every `off` or `far` signal produces a reason code (`protein_low`, `protein_high`,
`carbs_low`, `carbs_high`, `fat_low`, `fat_high`, `sugar_high`, `sodium_high`). The row renders at
most two: `far` before `off`, ties broken by a fixed signal order — protein, sugar, sodium, fat,
carbs — so the same seven days always produce the same two reasons. Reproducibility is the whole
point of the ADR-004 decision this implements; a reason list that reshuffles on a tie would give
that away for nothing.

### Rendering, and when the row is silent

The label renders between the today row and the divider above the gap line. `ContentState`'s
`ready` variant carries a third member, `healthiness: HealthinessResult | null`.

The row renders **nothing at all** — no line, no divider, no placeholder — when there is no label:
fewer than three eligible days, zero macro energy, or a failure of the fetches that feed it. The
card already has a bottom row whose whole job is to report the state of the food log, and a second
"not enough data yet" line stacked above it would say the same thing twice in the state a new user
spends their first weeks in. This is the one place where silence is right, because something else
on the same card is already speaking.

The label shares the gap line's fetch group. `/api/food/daily-totals` and `/api/food/completeness`
feed both, so when that group fails the middle row disappears along with the gap line's value,
while the top row still renders from the summary. The card's existing `Promise.allSettled` split
already produces this; the label reads the same settled results.

**Precedence.** The label renders only when the sustainability warning produces no warning. That
change is on its own branch and may or may not have merged when this one lands, so the
implementation checks: if the middle row already computes a sustainability warning, gate the label
on it producing none; if it does not exist yet, the label is the whole middle row and the
sustainability change adds the gate when it arrives. Either way, both changes touch the same rows
of `LoggingGapCard.tsx`, so whichever lands second rebases onto the first.

### Copy

The card's copy standard holds: say only what was measured. The line reads "Last 7 days: Good" /
"За 7 дней: хорошо", and when the label is not `Good` it names its reasons — "Last 7 days: Fair —
protein is low, sugar is high". It does not say "healthy" or "unhealthy", because five numbers off
a food log cannot support either word. Keys keep the `loggingGap.` prefix, which stays internal and
unrenamed for the reason the previous change recorded.

The existing hint disclosure gains one sentence, shown only when the label is present: the label
covers macro balance, total sugars and sodium on fully-logged days only; total sugars includes the
sugars in fruit and dairy; and salt added while cooking is usually missing from the log, so no
sodium flag is not the same as low sodium.

### Deliberately not in scope

- **The LLM advice lines** under the label, and the nutrition chat below them. They are the next
  two parts of this initiative and are described in this idea's deferred scope. The advice lines
  need a cache, a new `vision.Client` method, a refresh endpoint and a prompt; the chat is
  explicitly conditioned on the advice lines being *used*, not merely shipped, so it cannot land
  now under any reading. `summaryTodayResponse.Recommendation` stays `null` and stays reserved.
- **The sustainability warning.** Specified and being built separately; this change only leaves
  room for its precedence.
- **Making the thresholds configurable.** A per-user threshold is a per-user label, which is not a
  label. If a band turns out wrong in use, the fix is to change the constant and say why.
- **Renaming the `logging_gap` registry id or the `loggingGap.*` key prefix.** Renaming the id
  makes `reconcileMetricOrder` drop the saved entry and silently re-show a hidden card; the prefix
  is internal and its churn is not worth a second pass.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Per-day macro, sugar and sodium sums on the daily-totals endpoint
- [ ] Add `ProteinGrams`, `CarbsGrams`, `FatGrams`, `SugarGrams` and `SodiumGrams` to
      `database.DailyTotal` in `backend/pkg/database/food_daily_totals.go`, tagged
      `protein_grams`/`carbs_grams`/`fat_grams`/`sugar_grams`/`sodium_grams` with no `omitempty`,
      and document why zero must serialize
- [ ] Sum them in `DailyTotalsRange` over the same `confirmed`-status meals `Calories` is summed
      over, extending the `db.Select` column list, and keep the zero-filled entry for a day with no
      meals
- [ ] Extend `backend/pkg/server/food_daily_totals_test.go`: per-day macro sums are correct; a
      non-`confirmed` meal contributes to `unconfirmed_meals` and to none of the five sums; a day
      with no meals returns zeros for all of them; and, asserting against the raw JSON rather than a
      decoded struct, all five keys are present on a day whose sums are zero
- [ ] Add the same five fields to the `DailyTotal` interface in `frontend/lib/api.ts` as required
      numbers, with a comment pointing at `database.DailyTotal`
- [ ] Mark completed

### Task 2: The heuristic library
- [ ] Export `isValidDay` from `frontend/lib/loggingGap.ts` and note in its doc comment that the
      Healthiness Label shares it so the two rows agree on what counts as logged
- [ ] Add `frontend/lib/healthiness.ts`: the seven-day window resolved as the last seven days of
      the gap window, the eligible-day filter, the three-day floor, pooled sums, the macro-energy
      denominator, the five signals with the threshold table from `## How` as exported constants,
      the combination rule, and the deterministic reason ordering
- [ ] Return a discriminated result — a label of `good` / `fair` / `needs_attention` plus up to two
      reason codes, or `null` when there is no label — and document each threshold's origin
      (AMDR and where it was widened, total-versus-free sugars, elemental sodium and its
      under-reporting) in the constants' doc comment
- [ ] Add `frontend/lib/healthiness.test.ts` covering: fewer than three eligible days yields no
      label; days failing `isValidDay` are excluded from the pool, not merely from the count; zero
      macro energy yields no label; each band boundary lands on the `ok` side; a pooled window
      built from this app's own Nutrition Target split (about 22/39/39) is `good`; one `off` signal
      is `fair`; three `off` signals are `needs_attention`; any `far` signal is `needs_attention`;
      the sugar and sodium boundaries; and that the reason list is capped at two and ordered
      deterministically
- [ ] Mark completed

### Task 3: Render the middle row
- [ ] Carry the label on `ContentState`'s `ready` variant in
      `frontend/components/LoggingGapCard.tsx`, computed from the daily-totals and completeness
      responses the card already fetches — no fifth request
- [ ] Render the label line between the today row and the gap line's divider, with its reasons when
      the label is not `good`, and render nothing at all — no line and no divider — when there is
      no label or when the daily-totals/completeness fetch group failed
- [ ] Gate the label on the sustainability warning producing no warning if that code is already
      present in this file; if it is not, render the label as the whole middle row and leave the
      gate to the sustainability change
- [ ] Add the label's explanatory sentence to the existing hint disclosure, shown only when the
      label is present
- [ ] Give the new elements `data-testid` attributes consistent with the existing names
      (`nutrition-healthiness`, `nutrition-healthiness-label`)
- [ ] Mark completed

### Task 4: Copy in both languages
- [ ] Add the label line, the three label words, the eight reason phrases and the hint sentence to
      `frontend/lib/i18n/ru.ts` and `en.ts`, keeping the `loggingGap.` prefix
- [ ] Keep the copy to what was measured: no "healthy", no "unhealthy", no claim about sodium the
      log cannot support
- [ ] Confirm the two dictionaries have identical key sets
- [ ] Mark completed

### Task 5: End-to-end coverage
- [ ] Extend `LoggingGapFixture.dailyTotals` in `e2e/tests/logging-gap.spec.ts` with the five new
      fields, defaulting to zero, and confirm the existing fixtures still assert what they did
      (zero macro energy means no label, so no existing expectation changes)
- [ ] Add fixtures and tests for a `good` window, a `needs_attention` window, and a window with
      only two eligible days that renders no middle row at all
- [ ] Extend the live-backend contract test so it proves the real `/api/food/daily-totals` answers
      with the five new keys the card now reads
- [ ] Mark completed

### Task 6: Documentation
- [ ] Update `CONTEXT.md`'s **Healthiness Label** entry with the concrete seven-day window, the
      eligible-day rule it shares with the Logging Gap, and the five signals — without copying the
      threshold numbers, which live in code
- [ ] Update `todo.md`'s Phase 4 section: the middle row's label half is shipped, the advice lines
      and the chat are not
- [ ] Mark completed

### Task 7: Validate against the deployed stack, then accept ADR-004
- [ ] Run `make lint` and `make test` and fix everything they report
- [ ] Deploy the branch to `hcw-wip` and run `make test-e2e` against it; fix every failure rather
      than recording it as pre-existing
- [ ] As the last commit, flip `docs/adr/ADR-004-heuristic-food-healthiness-label.md` from
      `Proposed` to `Accepted`, and add an `> **Update:**` note recording that the heuristic half
      shipped here, where the thresholds live, and that the LLM-downstream half it also decides is
      still unbuilt
- [ ] Mark completed
