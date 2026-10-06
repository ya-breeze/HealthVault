# Align nutrition columns and fill launcher sizes

## Why
The owner's launcher widgets need aligned labels, bars and numbers. Variable amount widths currently move each compact bar's right edge. Calorie-only widgets underuse their bounds, while wide widgets leave space that can support larger nutrient values and icons.

## How
Give nutrient labels and numbers shared column widths, with right-aligned numbers and a separate gram column. Keep compact fiber's icon/value without a fake target rail. In wide layouts align the two equal-width columns and center their content vertically within the available rows. Increase calorie-only typography and calorie rail thickness, budget against actual size, four-digit calorie values and enlarged system fonts. Increase wide nutrient typography and icon sizes while preserving the 110dp minimum height. Keep fixed green automatic themes, strict overruns, accessibility, refresh and food actions, and FlexWindow unchanged.

## Validation Commands
`/data/android-build.py HealthVault-worktrees/widget-alignment/android "testDebugUnitTest lintDebug assembleDebug"`

`git diff --check`

### Task 1: Align and resize home-widget content
- [x] Implement shared label/number/unit columns and proportional size-specific typography.
- [x] Verify compact and minimum-height wide size budgets and variable digit lengths.
- [x] Complete native Review Gate and one Claude peer attempt.

Build/model checks are not native pixel verification. Real launcher/theme/font acceptance remains outstanding until the owner installs the release. This slice changes Android only.

## Validation evidence
Shared Android tests passed (78 tests, zero failures/errors); lint zero errors; debug APK built. Native correctness, standards and spec/tests reviews clean after fixing narrow-grid width overflow. Claude peer unavailable due weekly quota (API429), attempted once. Real launcher rendering remains outstanding. No merge or Play publication occurred in this slice.
