# Align compact fiber with the other nutrients

## Why
The compact home widget omits the fiber rail. Its amount follows the icon instead of aligning with the other amounts on a 2x2 launcher surface.

## How
Render the existing MetricRail and spacer for every compact nutrient row. Fiber has no configured target, so use the existing neutral rail without inventing a goal or progress fraction. This replaces the earlier compact icon/value-only choice in `widget-prototype-fidelity.md` and `widget-alignment.md`. Keep the shared number and unit columns, sizing budgets, themes, unknown values and wide layout. The picker preview already shows this arrangement.

## Validation Commands
`/data/android-build.py HealthVault-worktrees/widget-fiber-rail/android "testDebugUnitTest lintDebug assembleDebug"`

`git diff --check`

### Task 1: Restore the compact fiber rail and alignment
- [x] Render fiber with the same weighted rail and spacer as the other compact rows.
- [x] Update the Android documentation to describe the neutral compact fiber rail.
- [x] Run Android tests, lint, APK build and the Review Gate; record device validation limits.

The control host has no launcher rendering test or connected phone. JVM tests and a debug APK do not confirm pixels on the owner's 2x2 launcher. Phone acceptance remains a separate delivery check.

## Validation evidence
Shared Android build completed successfully: 84 JVM tests, zero failures/errors/skips; lint zero errors; debug APK produced. Native correctness, standards and spec/tests reviews found no blocking findings. Claude peer attempt found no correctness defect, but skipped review subagents; record peer unavailable because structured events do not show the required review subagent verdict. README already states that fiber has no target and its rail stays neutral. No widget rendering E2E suite or connected launcher exists here; no phone installation, merge, Play publication or backend deployment occurred. Real launcher acceptance remains unverified.
