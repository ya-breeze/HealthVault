# Nutrition card: today's intake row, and a positive state when the log agrees with the scale

## Why

The Logging Gap card is silent almost every day, and when it is silent it says the wrong thing.

Its silence rule is one comparison: `computeLoggingGap` returns `not_enough_data` whenever
`Math.abs(value) <= interval`. That interval is dominated by a fixed term — 10% of the Nutrition
Target — so for a 2159 kcal target it is about ±222 kcal/day no matter how much data the user
has. A gap smaller than that is structurally unreportable. Silence is therefore the card's normal
state, not its exceptional one.

The message shown in that state is `loggingGap.notEnoughData` — "Пока недостаточно данных" / "Not
enough data yet". But `not_enough_data` is returned for two opposite situations:

- The hard floor rejected the window: too few weigh-ins, more than three outliers, fewer than
  three fully-logged days, or a stale last weigh-in. Here there really is not enough data.
- The interval covers zero. Here there is plenty of data, and it agrees — the weight trend and
  the food log tell the same story.

A real user hit the second case after two weeks of disciplined logging: implied intake 1515
kcal/day against a mean logged intake of 1617, a gap of −102 inside an interval of ±222. The card
answered "not enough data yet", denying the very logging that produced the agreement. His words:
"воспринимается как минимум confused".

Two things follow, and this change does both.

**The agreeing case needs its own state.** Conflating "I cannot tell" with "I checked, and they
match" is not a wording problem; the card genuinely does not distinguish them today, so no wording
can be correct for both.

**The card needs something to show on a normal day.** A card that occupies a dashboard slot to say
nothing has no reason to be there. Today's intake against the Nutrition Target is the obvious
content: it is what the user wants at a glance, it changes through the day, and — the deciding
factor — it costs no extra request, because `GET /api/summary/today` already returns today's
consumed macros *and* the Nutrition Target in one response. The card currently spends a request on
`GET /users/me/nutrition-target` for the target alone; swapping that call for the summary yields
the today row for free.

This also settles a question raised while reviewing Phase 4 of the food initiative (`todo.md`).
Phase 4 as written adds two more cards — Card A for today versus target, Card B for the
Healthiness Label — which would put three nutrition cards on the dashboard beside the weight card.
ya-breeze rejected that: "иначе будет несколько карточек про еду/вес и юзеру будет неудобно", and
chose a single merged card. This change builds the first and third rows of that card. The middle
row (Healthiness Label plus advice lines) stays Phase 4 work and is out of scope here.

## How

### One card, three rows

The card keeps its registry id `logging_gap` and its file `LoggingGapCard.tsx`, and gains rows:

| row | window | visible |
|---|---|---|
| today's calories and macros against the Nutrition Target | today | whenever a target exists |
| Healthiness Label and advice — **Phase 4, not this change** | 7 days | — |
| the logging-gap line | 28 days | whenever a target exists |

The card's title changes from "Разрыв в учёте" / "Logging Gap" to "Питание" / "Nutrition", because
the gap is no longer the card's subject — it is one line inside it.

**The registry id stays `logging_gap`, and so does the `loggingGap.*` i18n key prefix.** Renaming
the type to `nutrition` would make `reconcileMetricOrder` (`frontend/lib/vitals.ts`) drop the
unknown saved `logging_gap` entry and re-append the new type at the end of the list *visible* —
silently moving the card for anyone who reordered their dashboard, and un-hiding it for anyone who
hid it. The id is internal and nobody sees it. The key prefix is equally internal; renaming it is
deferred to Phase 4, when the middle row lands and the card fully becomes a nutrition card, so
that churn happens once rather than twice.

### `on_track`, a third result from the gap computation

`LoggingGapResult` in `frontend/lib/loggingGap.ts` gains a third member:

```ts
export type LoggingGapResult =
  | { kind: 'gap'; value: number; interval: number }
  | { kind: 'on_track' }
  | { kind: 'not_enough_data' };
```

`computeLoggingGap` keeps its existing checks in their existing order and changes exactly one
outcome:

1. `validDays.length === 0` → `not_enough_data` (unchanged)
2. `!Number.isFinite(value) || !Number.isFinite(interval)` → `not_enough_data` (unchanged)
3. `Math.abs(value) <= interval` → **`on_track`** (was `not_enough_data`)
4. otherwise → `gap` (unchanged)

**Check 2 must keep running before check 3.** When fewer than three distinct EMA days exist,
`slopeStandardError` returns `null`, which the function turns into an infinite `trendErrorKcal` and
therefore an infinite `interval`. `Math.abs(value) <= Infinity` is `true`, so reordering these two
checks would report a genuinely unmeasurable series as "on track" — the exact false reassurance
this change exists to avoid. The existing order already prevents it; a unit test pins it so a later
edit cannot quietly undo it.

The hard-floor path in the card is untouched: `checkHardFloor` returning `true` still yields
`not_enough_data`, which is now an honest message rather than a catch-all.

### Copy: positive, and bounded by what the card actually knows

The `on_track` line reads "Дневник сходится с весом" / "Your log matches your weight", with a
supporting sentence naming the window.

It deliberately does **not** say "всё отлично". The silence condition tests only
`|gap| <= interval`. It says nothing about whether weight is moving toward the goal, or whether
intake is sensible — someone gaining weight while logging honestly reaches the identical state, and
so does someone eating below their BMR. A blanket "all good" would be a claim the card has not
checked. The praise is real but scoped to the one question the card answers: the log and the scale
agree.

### Today's row, from `GET /api/summary/today`

The card fetches four things in parallel. This change replaces one of them:

| before | after |
|---|---|
| `api.data('weight', …)` | unchanged |
| `api.getNutritionTarget()` | `api.getTodaySummary()` |
| `api.getCompleteness(…)` | unchanged |
| `api.getFoodDailyTotals(…)` | unchanged |

`GET /api/summary/today` (`backend/pkg/server/summary_today.go`) already returns everything both
uses need: `calories_consumed`, `protein_grams_consumed`, `carbs_grams_consumed`,
`fat_grams_consumed`, and a `target` object carrying either the computed target or
`{available: false, reason}`.

**One backend change is required after all**, found in code review and scoped in here rather than
deferred, because the frontend cannot be correct without it. `summaryTargetPayload` tagged its four
numeric fields `omitempty`, which drops an int that is zero — and zero is a legitimate target
value: `computeNutritionTarget` clamps carbs to zero whenever protein and the fat floor already
exhaust the calorie budget, which a goal weight typed in pounds against an ordinary TDEE reaches.
The key then vanishes from a response whose target is fully available, the client reads
`undefined` through a field its type declares required, and the dashboard renders
"Carbs 130/NaN g". Absence must mean "no target", which `available` already says; it must not also
mean "this number happened to be zero". The tags are dropped, and a backend test asserts on the raw
JSON keys — decoding into a struct cannot catch this, since a missing key and a zero both arrive
as 0.

Three further consequences worth stating:

- **The unmet-target path stops going through an exception.** `/api/summary/today` never returns
  422 — an unavailable target is a normal state there, reported as `target.available === false`
  with one of the same four `NutritionTargetUnmetReason` codes. The card's
  `nutrition_target_unmet` state is now reached by reading that field rather than by catching
  `NutritionTargetUnmetError`. `api.getNutritionTarget()` and `NutritionTargetUnmetError` stay
  exported and unchanged for their other callers.
- **Today's numbers count confirmed meals only.** `database.TodaySummary` restricts its macro sums
  to `confirmed` meals. A photographed but unconfirmed meal is therefore absent from the row. This
  is the same rule the gap's own `isValidDay` applies, so the two rows cannot disagree about what
  counts as logged.
- **`date` comes from the server.** The response's `date` is the caller's Logged Day as the backend
  resolves it, from the same `timezone` setting the card's window arithmetic uses. The row reports
  the server's date rather than re-deriving one, so the two can never drift apart at a local
  midnight.

When `target.available` is false the whole card falls back to the existing "complete your profile"
state: without a target there is neither a today row nor a gap to compute.

### Which failures cost which row

The four requests go out together but are read individually, through `Promise.allSettled` rather
than `Promise.all`, because they feed two rows with different appetites for failure:

- **The summary failing is the card's failure.** It carries both the today row and the target the
  gap needs, so nothing can be drawn without it.
- **The other three failing costs only the gap line**, which renders "Temporarily unavailable"
  while the today row renders normally. Under a shared `Promise.all` a 500 on the 58-day weight
  history would have discarded today's calories too — the row this change exists to add, thrown
  away over a request it does not depend on.
- **Those three are all-or-nothing between themselves.** The gap is one computation over the three
  together, so a partial set yields a wrong answer rather than a weaker one: a missing
  daily-totals response reads as "no valid days" and a missing weight response as "no weigh-ins",
  and both render as "not enough data yet" — blaming the user's logging for an outage.

### Rendering

`ContentState` collapses its two success-ish states into one `ready` state carrying both rows,
because the today row and the gap line now have independent outcomes:

```ts
type ContentState =
  | { kind: 'loading' }
  | { kind: 'retrieval_error' }
  | { kind: 'nutrition_target_unmet'; reason: NutritionTargetUnmetReason }
  | { kind: 'ready'; today: TodayRow; gap: LoggingGapResult };
```

The today row renders consumed against target calories, a progress bar, and the three macros. The
gap line renders beneath a divider: the existing readout for `gap`, the new line for `on_track`,
the existing "not enough data" for `not_enough_data`. The two existing caveat lines and the outlier
note keep their current position and behaviour.

The progress bar is presentational only — it carries `aria-hidden`, and the numbers beside it are
the accessible content. Exceeding the target is not an error state and is not coloured as one; the
bar clamps at 100% while the numbers keep counting.

### Deliberately not in scope

- **The Healthiness Label and LLM advice** (Phase 4's middle row). ADR-004 still has two open
  questions — the heuristic's thresholds and the chat's persistence model — and neither is settled
  here. ADR-004 gets an `> **Update:**` note recording that the card layout is now one card rather
  than Card A plus Card B; its `Status: Proposed` is not flipped, because nothing in it ships here.
- **Migrating the card off `useEffect`.** `frontend/AGENTS.md` points at the bundled Next.js 16
  guides, whose client-fetching guidance recommends the `use` API or SWR over `useEffect`. Every
  card in this app fetches through `useEffect`, and this change extends the existing effect in
  `LoggingGapCard.tsx` rather than introducing a second pattern into the same file. Migrating the
  app's client fetching is a separate, app-wide change.
- **Renaming the `logging_gap` registry id or the `loggingGap.*` key prefix**, for the reasons
  given above.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Add the `on_track` result
- [x] Add `{ kind: 'on_track' }` to `LoggingGapResult` in `frontend/lib/loggingGap.ts` and return
      it from `computeLoggingGap` when `Math.abs(value) <= interval`, leaving the preceding
      `validDays.length === 0` and `Number.isFinite` guards returning `not_enough_data`
- [x] Update the function's doc comment so it explains the three outcomes and why the finiteness
      check must precede the interval comparison
- [x] Add unit tests in `frontend/lib/loggingGap.test.ts`: a gap inside the interval returns
      `on_track`; a gap outside it still returns `gap`; `se === null` returns `not_enough_data` and
      **not** `on_track`; an empty valid-day set returns `not_enough_data`
- [x] Mark completed

### Task 2: Fetch today's summary instead of the bare target
- [x] Add a `TodaySummary` interface and an `api.getTodaySummary()` method to `frontend/lib/api.ts`
      for `GET /api/summary/today`, typed against `summaryTodayResponse` in
      `backend/pkg/server/summary_today.go`, with `target` as a discriminated
      available/unavailable shape
- [x] Replace the `api.getNutritionTarget()` call in `LoggingGapCard.tsx` with
      `api.getTodaySummary()`, taking the gap's target calories from `target.calories`
- [x] Reach the `nutrition_target_unmet` state from `target.available === false` and its `reason`,
      and delete the now-unreachable `NutritionTargetUnmetError` catch from this card; leave
      `api.getNutritionTarget()` and `NutritionTargetUnmetError` exported and unchanged
- [x] Mark completed

### Task 3: Render the today row and the on-track line
- [x] Restructure `ContentState` to the `ready` shape described in `## How`, carrying both the
      today row and the gap result
- [x] Render the today row: consumed against target calories, an `aria-hidden` progress bar that
      clamps at 100%, and the three macros against their targets
- [x] Render the gap line beneath a divider, covering `gap`, `on_track` and `not_enough_data`, and
      keep the outlier note and both caveat lines where they are
- [x] Give the new elements `data-testid` attributes consistent with the existing
      `logging-gap-*` names
- [x] Mark completed

### Task 4: Copy in both languages
- [x] Retitle `loggingGap.title` to "Питание" / "Nutrition" in `frontend/lib/i18n/ru.ts` and
      `en.ts`
- [x] Add keys for the today row (calorie line, macro labels) and for the `on_track` line and its
      supporting sentence, in both files, keeping the `loggingGap.*` prefix
- [x] Re-read `loggingGap.loading`, whose current text names the gap specifically, and reword it if
      it no longer describes a card that is loading three rows
- [x] Confirm the two dictionaries have identical key sets
- [x] Mark completed

### Task 5: End-to-end coverage
- [x] Extend `e2e/tests/logging-gap.spec.ts` so it asserts the today row renders for a seeded user
      with a Nutrition Target, and that the card's title is the new one
- [x] Add or adjust coverage so the `on_track` line and the `not_enough_data` line are
      distinguishable by `data-testid`, and neither spec asserts the old conflated behaviour
- [x] Mark completed

### Task 6: Validate against the deployed stack
- [x] Run `make lint` and `make test` and fix anything they report
- [x] Deploy the branch to `hcw-wip` and run `make test-e2e` against it; fix every failure rather
      than recording it as pre-existing
- [x] Add the `> **Update:**` note to `docs/adr/ADR-004-heuristic-food-healthiness-label.md`
      recording the one-card decision, leaving its `Status` as `Proposed`
- [x] Update `todo.md`'s Phase 4 section so it describes the merged card and notes which rows this
      change already shipped
- [x] Mark completed

### Task 7: Apply the code review's findings
- [x] Drop `omitempty` from `summaryTargetPayload`'s four numeric fields and assert in
      `summary_today_test.go`, against the raw JSON rather than a decoded struct, that all four
      keys are present whenever `available` is true
- [x] Read the four fetches with `Promise.allSettled` so a failure of the three gap-only requests
      costs the gap line rather than the whole card, and cover both halves of that split in
      `e2e/tests/logging-gap.spec.ts`
- [x] Rewrite the `on_track` supporting sentence, which claimed both a 28-day span the mean is not
      taken over (the floor is 3 valid days) and an *absence* of unlogged calories the interval can
      only call unmeasurable
- [x] Correct `getNutritionTarget`'s comment, which claimed other callers it no longer has
- [x] Mark completed
