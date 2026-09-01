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
holds every footnote the card has: `onTrackDetail`, then `caveatPhoto`, then `caveatActivity`.
Each appears only where it says something about the row it is under — see the paragraph on
`caveatActivity` below.

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
under, one step below `nutrition-today-calories`'s `text-xl font-bold`. That becomes the row's
default voice, and the `gap` line is the one outcome that climbs back out of it, keeping
`text-sm font-medium text-text`: an unlogged kcal/day range is a finding the reader is meant to
act on, where "your log matches your weight" is a footnote confirming there is nothing to do.
Demoting the range along with it would have answered the complaint about one line by quieting the
line the card exists for.

**The whole row is the toggle, not just the ⓘ glyph.** `TapTarget`'s 48px minimum applies on touch
either way — `compactOnMouse` releases it only under `pointer: fine` — so the status row is 48px
tall on a phone whichever element is the target, up from roughly 20px. The height is spent
regardless; spending it on a full-width row buys a target that is hard to miss instead of a 14px
glyph floating in one. The card still ends up far shorter, because the eight-odd wrapped lines of
caveats it replaces cost more than the ~28px the row gains. The button takes no `aria-label`: its
accessible name is the status text plus a visually-hidden "show details", which says more than a
bare label would, and `aria-expanded` carries the open/closed state.

**This weakens a disclosure that ADR-010 deliberately made unconditional**, and the change is made
with that understood. ADR-010's Consequences and `docs/specs/logging-gap.md`'s risk list both say
the photo-estimation and activity-multiplier caveats are static "so a user reading only the range
still sees the disclosure" — the over-trust case they were written to prevent. Behind a ⓘ that is
no longer true. The trade is accepted because the failure it guards against is a reader who
over-trusts the range, and eight lines of standing disclaimer is a poor guard against that: copy
that never changes is copy that stops being read, and it was crowding out the numbers it qualifies.
What survives is the affordance — the ⓘ sits on the same line as the range, so the range never
appears as a bare, unqualified figure. ADR-010 carries an `Update` note recording this rather than
being rewritten.

A second consequence of that reasoning: `caveatActivity` renders only for the `gap` and `on_track`
rows, because it qualifies the comparison against the weight trend, and in `not_enough_data` and
the row-level `retrieval_error` that comparison produced nothing. `caveatPhoto` stays
unconditional across the `ready` states — it qualifies today's calorie figure at the top of the
card, which is photo-derived and on screen in all four.

The outlier note (`loggingGap.outlierNote`) stays where it is and stays visible. It is conditional
and rare, and unlike the caveats it reports something that actually happened to this user's data.

No copy is rewritten and none is deleted: `en.ts` and `ru.ts` keep every existing `loggingGap.`
string and gain exactly one, the toggle's visually-hidden label.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Collapse the footnotes behind a ⓘ toggle
- [x] In `frontend/components/LoggingGapCard.tsx`, add a `hintOpen` `useState(false)` and render
      the two `loggingGap.caveat*` paragraphs only inside the expanded block, removing the
      always-on `<div className="text-xs text-text-muted mt-2 space-y-1">` that sits after
      `renderContent()`
- [x] Make the gap-line row itself the disclosure control: a `TapTarget compactOnMouse` wrapping
      the status text and a trailing `InfoIcon`, carrying `aria-expanded`, `aria-controls` from
      `useId()`, a visually-hidden label from a new i18n key, and
      `data-testid="logging-gap-hint-toggle"`
- [x] Give the expanded block `data-testid="logging-gap-hint"` and have it render
      `loggingGap.onTrackDetail` first **only** when the gap kind is `on_track`, then
      `caveatPhoto` unconditionally, then `caveatActivity` only for the `gap` and `on_track` rows
- [x] Remove the unconditional `<p>{t('loggingGap.onTrackDetail')}</p>` from the `on_track` branch
- [x] Add `loggingGap.hintToggle` to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`
      ("Show details" / "Подробнее")
- [x] Add an `InfoIcon` to `frontend/components/icons.tsx` following the existing
      `SVGProps<SVGSVGElement>` icons' shape, or use a text ⓘ if that reads better at 14px
- [x] Mark completed

### Task 2: Demote the gap line to supporting text
- [x] Set `text-xs text-text-muted` on the row, so `on_track`, `not_enough_data` and the row-level
      `retrieval_error` all inherit it and the `on_track` line loses its `text-sm font-medium
      text-text`
- [x] Keep the `gap` line at `text-sm font-medium text-text` as a local override, so a detected
      range stays the loudest thing in the row
- [x] Confirm the row clears `TapTarget`'s 48px touch minimum without the icon growing: the icon
      stays at `w-3.5 h-3.5` and `min-h-12` supplies the height on touch, released under
      `pointer: fine` by `compactOnMouse`
- [x] Mark completed

### Task 3: Record that ADR-010's static-caveat consequence no longer holds
- [x] Add a `> **Update (…)**` note under ADR-010's Consequences bullet about the two static
      caveats, naming this spec, saying plainly that "a user reading only the range still sees the
      disclosure" is now false, and why that was accepted — without rewriting the bullet
- [x] Leave `docs/specs/logging-gap.md` untouched: merged specs are frozen, and this spec is the
      record of the newer decision
- [x] Mark completed

### Task 4: Update the tests that read the caveats off the card
- [x] In `e2e/tests/logging-gap.spec.ts`, the "a clear gap renders as a kcal/day range with both
      caveats" test asserts `card` contains both caveat strings; make it click
      `logging-gap-hint-toggle` first and assert the strings inside `logging-gap-hint`
- [x] Add to the same test that both caveat strings are absent before the toggle is clicked — the
      point of the change is that they are not in the default view
- [x] Extend the on-track test to click the toggle and assert `logging-gap-hint` contains the
      `onTrackDetail` sentence, and that the sentence is absent before the click
- [x] Add a case asserting `logging-gap-hint` in the `not_enough_data` state contains
      `caveatPhoto` but neither `caveatActivity` nor the `onTrackDetail` sentence
- [x] Run `make lint`, `make test` and `make test-e2e` against the deployed `hcw-wip` stack
- [x] Mark completed
