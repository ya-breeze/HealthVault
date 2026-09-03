# Stop the meal history day header from colliding with itself

## Why

On `/food/history/`, a day that carries a "Mark day complete" control renders as three overlapping
fragments instead of a header. Measured on `hcw-wip` at a 390x844 viewport, Russian display
language, against `main` at `33d3a99`:

- the date breaks across two lines — `среда,` / `26 авг.`;
- the control's label wraps to two lines inside its own pill — `Отметить день` / `заполненным`;
- the macro summary wraps *around* the pill, splitting a label from its value —
  `900 ккал · Б 0г · У` / `0г · Ж 0г`.

That day's header block measures **48px tall against 20px** for a day with no control. At a 320px
viewport every day header wraps regardless of the control (40px), and the control day reaches 50px.
English is not exempt: at 390px `Wednesday, Aug 26` already wraps where `Tuesday, Sep 1` does not,
so the defect is a function of label length and appears and disappears from day to day.

The cause is a single flex row. `frontend/app/food/history/page.tsx:224` is one
`flex items-baseline justify-between` holding three units — the `<h2>` date, `DayCompletenessControl`,
and a four-value macro string — inside `max-w-md mx-auto px-6`, which is 342px of content at a 390px
viewport and 272px at 320px. Their combined natural width is roughly 457px. Nothing in the row is
allowed to wrap cleanly, so all three compress at once.

The control is the item that cannot give: it renders a `TapTarget`, whose 48x48 minimum is a
guarantee PR #36 established deliberately and which this change does not touch. It is also the
widest unit in Russian, where "Mark day complete" is `Отметить день заполненным`. Russian is
therefore the binding case, not a secondary one.

The same page carries a second, smaller defect from the same audit. Every meal row is identified by
a full `toLocaleString()` — `26.08.2026, 23:00:00` — inside a group whose header already reads
`среда, 26 авг.`. The date is redundant with the header directly above it and the seconds are noise,
while the row's actually useful number, its calories, sits at the far right in a column that is
empty for any meal that is not yet confirmed.

That timestamp has a correctness bug hiding in it, on the same expression this change rewrites.
Day grouping uses the account's `timezone` setting; the timestamp is formatted with no `timeZone`
at all, so it renders in the *browser's* zone. An account set to `UTC` read from a `+02:00` browser
shows a 23:00 meal as `01:00` under a header naming the previous day. It is invisible on the WIP
stack only because that container also runs UTC.

## How

Split the day header into two rows, and let each row hold units that fit.

- **Row 1 carries the date and the completeness control**, as `flex flex-wrap items-center gap-2`.
  `flex-wrap` is what makes the degradation clean: when the two cannot share a line the control drops
  whole onto its own line rather than being squeezed into the gap. Give the control
  `whitespace-nowrap` so its own label is what forces that wrap — without it the pill shrinks and
  re-wraps internally, which is exactly today's failure. No `justify-between` here: the control sits
  directly beside the date it describes, rather than across a gap from it, and reintroducing that gap
  is what the old layout filled with a squeezed pill.
- **Row 2 carries the day's totals**, calories first and emphasized (`text-sm font-semibold`,
  `tabular-nums`), macros after in the existing muted `text-xs`. This is the same information as
  today's single run-on string, ordered so the number being scanned for reads first. Row 2's natural
  width is about 185px, so it fits on one line at 320px in both languages with room to spare.
- **Meal rows lose the redundant date.** Format `logged_at` as time only — `toLocaleTimeString` with
  `hour`/`minute` and **the account's `timezone`**, the same zone the grouping uses — so the row says
  `23:00`, not `26.08.2026, 23:00:00`, and says it in the zone whose day header it sits under. Give
  the calories figure `tabular-nums` so the column aligns.
- **Add `data-testid` hooks** — `day-header`, `day-total` and its two halves `day-total-calories` and
  `day-total-macros`, plus `meal-meta` on a row's identity line — so the E2E cases can address each
  unit without depending on Tailwind classes, matching how `e2e/tests/mobile-nav.spec.ts` addresses
  the navigation bar. The two halves are addressed separately because the assertion that matters is
  per-unit: each must render on one line of its own, and a container's height alone cannot say which
  of its children wrapped.

**Trade-offs and exclusions.**

- *Rejected: truncating the date with `truncate`.* It keeps the header on one row at any width, but
  a truncated date is unreadable in exactly the case that matters — a long Russian weekday — and the
  page has no other place that names the day.
- *Rejected: shortening the Russian "Mark day complete" label.* It would make the one-row layout fit,
  but it treats a layout defect as a translation problem and leaves the header just as brittle for
  the next long string.
- *Accepted: a day carrying the control is now taller than one without.* Row 1 becomes 48px on those
  days because `TapTarget`'s minimum applies. That is the guarantee working, not a regression, and it
  replaces a 48px row in which the content collided with a 48px row in which it does not.
- *Excluded: the page's length and density.* At 390px this page is 4.31 screens and at 320px 7.94,
  with no visual grouping between days — the audit's third meal-history finding. Density is scoped to
  the deferred visual-language change and is not touched here.
- *Excluded: an ADR.* This is one page's layout. It introduces no cross-cutting pattern and stales no
  fact in ADR-008.
- *No new user-visible strings*, so `frontend/lib/i18n/{en,ru}.ts` are unchanged. The change touches
  no Next.js API — it is JSX and Tailwind inside a component that is already `'use client'` — so
  nothing in `node_modules/next/dist/docs/` bears on it.

**Verification.** The gate is the E2E suite, because every claim here is a rendering outcome:
`make lint` covers the backend only and the Vitest suite covers pure functions, not rendering. The
new cases must run in **both languages**, since Russian is the binding case and an English-only test
would pass against the layout that is broken today. Follow `e2e/tests/completeness.spec.ts` for
seeding a day that actually shows the control and for saving and restoring the shared account's
settings, and `e2e/tests/mobile-nav.spec.ts` for asserting on bounding boxes rather than pixel
offsets.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Split the day header into two rows
- [x] In `frontend/app/food/history/page.tsx`, replace the single
      `flex items-baseline justify-between` day-header row with a container holding two rows, and
      give the container `data-testid="day-header"`.
- [x] Row 1: the `<h2>` date and `DayCompletenessControl`, as `flex flex-wrap items-center gap-2`.
      Add a comment recording that `flex-wrap` exists so the control drops whole onto its own line
      rather than compressing into the gap.
- [x] Row 2: the day's totals, with calories first in `text-sm font-semibold` and `tabular-nums`,
      followed by the P/C/F values in the existing muted `text-xs`. Give the row
      `data-testid="day-total"` and each half its own id (`day-total-calories`,
      `day-total-macros`), so a test can say which of the two wrapped.
- [x] Add `whitespace-nowrap` to `DayCompletenessControl`'s button and badge in
      `frontend/components/food/DayCompletenessControl.tsx`, so the control's full label is what
      forces the wrap instead of the pill re-wrapping inside itself.
- [x] Confirm no change to the control's `TapTarget` or its 48x48 minimum.
- [x] Mark completed

### Task 2: Give meal rows a readable identity line
- [x] Replace the meal row's `new Date(meal.logged_at).toLocaleString(dateLocaleFor(language))` with
      a time-only format — `toLocaleTimeString`, `hour` and `minute` only, no seconds and no date.
- [x] Pass the account's `timezone` to that format, the same value `groupByDay` uses, and add a
      comment stating that a timestamp rendered in the browser's zone can name a different day than
      the header it sits under.
- [x] Give the calories figure `tabular-nums` so the right-hand column aligns down the list, and
      `data-testid="meal-meta"` to the identity line so a test can read it.
- [x] Mark completed

### Task 3: Cover the header at narrow viewports in both languages
- [x] Add `e2e/tests/meal-history-layout.spec.ts`, seeding meals with the `createMealAt` helper
      pattern from `completeness.spec.ts` so a day renders with the "Mark day complete" control, and
      cleaning them up afterwards with the same `deleteMeals` pattern.
- [x] Save the account's settings before the run and restore them afterwards, following
      `completeness.spec.ts` — these cases write `display_language`, and the account is shared with
      every other spec file.
- [x] Assert the date, the completeness control, and the totals **do not intersect**, using the
      `intersects` bounding-box helper from `mobile-nav.spec.ts`. Treat this as a backstop, not the
      primary assertion: today's wrapped fragments sit *beside* the pill rather than under it, so
      their boxes stay disjoint while the header still reads as three interleaved pieces.
- [x] Assert the date, the calories total and the macro totals each render on **one line of their
      own**, measured as box height against the element's own line-height. This is the pair of
      assertions that actually fails against today's layout, where the date rendered 2.0 lines and
      the macro summary 2.0 at 390px.
- [x] Assert the completeness control computes `white-space: nowrap` rather than counting its lines:
      it is a TapTarget, so its box is 48px tall by guarantee and a height-based measure would read
      three lines of 16px text however the label renders.
- [x] Run every assertion above at **390x844 and 320x568, in both `en` and `ru`** — four
      combinations. Note in a comment that `ru` is the binding case because
      `Отметить день заполненным` is the widest label the row must hold.
- [x] Assert a meal row's identity line matches a time-only pattern: no four-digit year and no
      seconds field.
- [ ] Confirm the existing `completeness.spec.ts` cases still pass — they address the same control
      this change restyles.
- [ ] Mark completed

### Task 4: Validate against the deployed stack
- [ ] Re-read every modified file for leftover classes from the old single-row layout and for
      consistency with the surrounding code.
- [ ] Run `make lint` and `make test`, and state in the summary that both cover the backend and the
      pure-function suite only — this change's real gate is the E2E run.
- [ ] Deploy the branch to `hcw-wip` and run `make test-e2e` against it.
- [ ] Re-measure the day header at 390px and 320px in both languages and record the after-numbers
      against the before-numbers in this spec's `Why`.
- [ ] Summarize which files changed and what each change does.
- [ ] Mark completed
