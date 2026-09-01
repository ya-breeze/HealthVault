# Nutrition card: footnotes behind a hint, on-track line demoted

## Why

On the dashboard, the Nutrition card's bottom half is currently four lines of prose. In the
`on_track` state it reads:

```
✓ Дневник сходится с весом                       ← text-sm font-medium text-text
По полностью заполненным дням записанное сходится с динамикой веса — если разрыв
и есть, он слишком мал, чтобы его различить.     ← text-xs muted, always shown
Учтённые калории оцениваются по фото и могут иметь собственную погрешность.
Эта оценка отдельно не учитывает погрешность коэффициента активности — неверная
оценка активности может выглядеть как неучтённые (или избыточно учтённые)
калории.                                          ← both always shown, every state
```

Two problems, both reported by the user:

1. **The prose is most of the card.** On a phone those three paragraphs wrap to roughly eight
   lines and push the card past the height of the numbers it exists to show. They are caveats —
   true, worth keeping, worth reading *once* — and they carry no per-day information, so they
   cost the same space every day for nothing after the first read.
2. **The status line looks like the headline.** `text-sm font-medium text-text` makes "Дневник
   сходится с весом" the visually loudest thing on the card, above the calorie figure it is a
   footnote to. The gap line is a supplement to today's intake, not the card's subject.

The caveats themselves stay — the estimate genuinely is photo-derived and genuinely doesn't
isolate activity-multiplier error, and `nutrition-card-today-and-on-track.md` put them there
deliberately. What changes is that reading them becomes a choice.

## How

**One ⓘ toggle on the gap line**, at the end of the status row, expanding a block below it that
holds every footnote the card has: `onTrackDetail` (only in the `on_track` state, since it
explains that state specifically) followed by `caveatPhoto` and `caveatActivity`.

Tap-to-expand rather than an `aria-label`/`title` tooltip: the user reads this dashboard on a
phone, where a hover tooltip is unreachable, and that would hide the caveats from exactly the
reader who most needs them. Implemented as a `useState` boolean plus a `<button>` with
`aria-expanded`/`aria-controls`, not `<details>/<summary>` — the card is a `div` grid child with
its own styling and a native disclosure marker would fight it.

**Placement follows the caveats, not the card.** Today the two caveat paragraphs render in every
card state, including `loading`, `retrieval_error` and `nutrition_target_unmet`. Moving them onto
the gap line means they now render only when a gap line renders, i.e. the `ready` states. That is
a fix, not a regression: "logged intake is estimated from photo recognition" qualifies an estimate
the card is not currently showing in any of those three states.

**The status line drops to `text-xs text-text-muted`** — the same weight as the macro row it sits
under, one step below `nutrition-today-calories`'s `text-xl font-bold`. This applies to all four
gap lines (`on_track`, `gap`, `not_enough_data`, the row-level `retrieval_error`) so the row keeps
one voice; the ✓ glyph and the kcal/day range still carry the distinction between them.

The outlier note (`loggingGap.outlierNote`) stays where it is and stays visible. It is conditional
and rare, and unlike the caveats it reports something that actually happened to this user's data.

No new i18n keys and no wording changes; `en.ts` and `ru.ts` gain only the ⓘ button's accessible
label.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Collapse the footnotes behind a ⓘ toggle
- [ ] In `frontend/components/LoggingGapCard.tsx`, add a `hintOpen` `useState(false)` and render
      the two `loggingGap.caveat*` paragraphs only inside the expanded block, removing the
      always-on `<div className="text-xs text-text-muted mt-2 space-y-1">` that sits after
      `renderContent()`
- [ ] Render the ⓘ as a `<button type="button">` at the end of the gap-line row in `renderGap`,
      carrying `aria-expanded`, `aria-controls` pointing at the expanded block's `id`, an
      `aria-label` from a new i18n key, and `data-testid="logging-gap-hint-toggle"`
- [ ] Give the expanded block `data-testid="logging-gap-hint"` and have it render
      `loggingGap.onTrackDetail` first **only** when the gap kind is `on_track`, then
      `caveatPhoto` and `caveatActivity` in both cases
- [ ] Remove the unconditional `<p>{t('loggingGap.onTrackDetail')}</p>` from the `on_track` branch
- [ ] Add `loggingGap.hintToggle` to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`
      ("Show details" / "Подробнее")
- [ ] Add an `InfoIcon` to `frontend/components/icons.tsx` following the existing
      `SVGProps<SVGSVGElement>` icons' shape, or use a text ⓘ if that reads better at 14px
- [ ] Mark completed

### Task 2: Demote the gap line to supporting text
- [ ] Change the `on_track` and `gap` lines in `renderGap` from `text-sm font-medium text-text` to
      `text-xs text-text-muted`, matching the existing `not_enough_data` and `retrieval_error`
      lines, and drop `text-sm` from those two so all four are `text-xs`
- [ ] Confirm the ✓ glyph and the ⓘ button align on the baseline of the shrunken line, and that
      the ⓘ's tap target still clears the 44px minimum the card's other controls use
      (`TapTarget`); if it does not, keep the icon at 14px and pad the button rather than growing
      the icon
- [ ] Mark completed

### Task 3: Update the tests that read the caveats off the card
- [ ] In `e2e/tests/logging-gap.spec.ts`, the "a clear gap renders as a kcal/day range with both
      caveats" test asserts `card` contains both caveat strings; make it click
      `logging-gap-hint-toggle` first and assert the strings inside `logging-gap-hint`
- [ ] Add to the same test that both caveat strings are absent before the toggle is clicked — the
      point of the change is that they are not in the default view
- [ ] Extend the on-track test to click the toggle and assert `logging-gap-hint` contains the
      `onTrackDetail` sentence, and that the sentence is absent before the click
- [ ] Add a case asserting `logging-gap-hint` in the `not_enough_data` state contains the two
      caveats but **not** the `onTrackDetail` sentence
- [ ] Run `make lint`, `make test` and `make test-e2e` against the deployed `hcw-wip` stack
- [ ] Mark completed
