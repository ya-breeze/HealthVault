# Color the days in the "История питания" card

Idea: ya-breeze/idea-forge#646

## Why

The owner reports that the marks in the dashboard's "История питания" card are hard to read at a
glance. Each of the seven days is a bordered tile holding a small grey `✓`, `!` or `∅`, and the three
summary counts above them are plain text. Every tile looks the same, so telling a good day from a
bad one means reading each glyph. The owner suggested color.

## How

Frontend only. No API, data-model or copy change.

- Add three theme tokens, `--outcome-counted` (green), `--outcome-attention` (amber) and
  `--outcome-empty` (grey, the same as the muted text color), with a light and a dark value, next to the existing `--c-*` tokens.
- Each day tile takes its outcome color: tinted background, colored border, colored glyph. The glyph
  stays, so color is never the only cue (color-blind readers, the `aria-label` is unchanged).
- The glyph grows from `text-base` to `text-lg`, since it is now the main cue.
- Each summary count gets a dot in the same color, so the three counts double as the legend.
- Tile order, `data-testid`, `data-outcome`, link target and the edit-mode controls do not change,
  so the existing e2e assertions keep holding.
- Excluded: a separate legend row, changing the outcome rules in `lib/foodLogHistory.ts`.

## Validation Commands

- `make test-frontend`
- `make lint`

### Task 1: Outcome colors

- [x] Add the three `--outcome-*` tokens to `frontend/app/globals.css`
- [x] Style the day tiles and the summary dots per outcome in `frontend/components/FoodLogHistoryCard.tsx`
- [x] Add a unit test that each outcome maps to a distinct theme token, and that `globals.css` defines each token for light and dark
- [x] Verify in a browser against WIP, light and dark
- [x] Mark completed
