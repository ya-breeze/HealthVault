# Larger home-screen widget layouts for roomy launcher cells

Idea: 764 (Idea Store)

## Why

The owner's Samsung home-screen screenshot shows both placed widgets with small text and large empty
areas. The 2x2 widget draws the calorie value at 26sp and the macro labels at 7sp; the 2x1 widget
draws calories at 16sp and shows only the letters P/C/F for macros, with no grams.

The cause is the breakpoint set. `SizeMode.Responsive` hands the composition the matched bucket
size, not the real cell size, and the buckets that match a Samsung 2x2 (about 156x167dp) and 2x1
(about 156x72dp) are the 110x110dp and 109x48dp minima. Every size was tuned for those minima,
so a roomier launcher gets the minimum layout stretched over a bigger card.

The owner reviewed 1:1 phone-scale mockups over the real screenshot and chose variant **B** for 2x2
and variant **G** for 2x1.

## How

Add two breakpoints, `ROOMY_SHORT` at 140x68dp and `ROOMY_COMPACT` at 150x158dp, to the existing
responsive set. On Android 12 and later the platform picks, among the declared buckets that fit the
cell, the one with the smallest squared distance to it, not the largest one. A Samsung 2x1 now
lands on `ROOMY_SHORT` and a Samsung 2x2 on `ROOMY_COMPACT`, while launchers with smaller cells keep
today's `SHORT` and `COMPACT` layouts. Because matching is by distance, a wide-but-short 2x2 cell
(about 156x110dp) would sit closer to `ROOMY_SHORT` than to the 110x110 bucket and lose its 2x2
composition. A third new bucket at 150x110dp maps to the existing `COMPACT` layout and keeps those
cells where they are. `WIDE_SHORT` (230x48) and `WIDE` (230x110) remain the closest match for wider
cells. A unit test mirrors the platform's distance rule so these outcomes are checked, not assumed.

**2x2 (`ROOMY_COMPACT`, variant B).** No header row. A 36sp calorie value with its unit, then
"of 2140 kcal" with the pace glyph and percentage on the right, an 8dp calorie bar, and three macro
rows at the bottom: letter, flexible bar, grams at 14sp, pace glyph.

**2x1 (`ROOMY_SHORT`, variant G).** Two columns. The left one holds a 26sp calorie value,
"of 2140" with the calorie pace glyph, and a 6dp calorie bar. The right one holds three macro rows:
letter, grams at 12sp, target grams in muted 10sp, pace glyph, with a 3dp bar under each row.

Both layouts keep the existing semantics: whole-card **Log food** tap, stale `!` on the calorie
value, no fabricated denominator or bar without a target, glyph beside every colored bar. Pace glyphs
sit outside the flexible part of each row so that, if a larger system font makes a row too wide, the
target text clips before the glyph. Both compositions fill their minimum bucket at the default font
scale, so from font scale 1.1 the roomy breakpoints render the existing `SHORT` and `COMPACT`
compositions, which already handle large text. The 2x2 hero is 36sp rather than the mockup's 38sp,
and the row gaps are tighter, for the same reason.

Excluded: the FlexWindow widget, the 4x2 layout, the picker preview, and scaling fonts continuously
with the real cell size. The picker preview stays representative of the 2x1 minimum.

This project has no automated way to render a launcher. JVM tests cover breakpoint selection and
fallback; visual acceptance needs the owner to sideload the debug APK on the Samsung phone.

## Validation Commands

- `/data/android-build.py HealthVault-worktrees/idea-764-roomy-widget/android "testDebugUnitTest lintDebug assembleDebug"`
- `make test`
- `make lint`

### Task 1: Roomy breakpoints

- [x] Declare `ROOMY_SHORT` (140x68dp) and `ROOMY_COMPACT` (150x158dp) in the responsive size set and in `summaryWidgetLayout`
- [x] Render the existing `SHORT`/`COMPACT` compositions for the roomy breakpoints at font scale 1.1 and above
- [x] Add unit tests for the new breakpoint boundaries, their precedence against `WIDE_SHORT`/`WIDE`, and the large-font fallback
- [x] Add a 150x110dp bucket mapped to `COMPACT` so distance-based matching keeps wide-but-short 2x2 cells on their layout
- [x] Add a unit test that mirrors the platform's closest-fitting-bucket rule for Samsung, minimum, and wide cells
- [x] Mark completed

### Task 2: Variant B and G compositions

- [x] Implement the 2x2 variant B composition with the missing-target and stale states
- [x] Implement the 2x1 variant G composition with the missing-target and stale states
- [x] Add the "of N kcal" and "of N" strings in English and Russian
- [x] Mark completed
