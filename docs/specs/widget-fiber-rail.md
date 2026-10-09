# Align compact fiber with the other nutrients

## Why
The compact home widget omits the fiber rail. Its amount follows the icon instead of aligning with the other amounts on a 2x2 launcher surface.

## How
Render the existing MetricRail and spacer for every compact nutrient row. Fiber has no configured target, so use the existing neutral rail without inventing a goal or progress fraction. This replaces the earlier compact icon/value-only choice. Keep the shared number and unit columns, sizing budgets, themes, unknown values and wide layout. The picker preview already shows this arrangement.

## Validation Commands
`/data/android-build.py HealthVault-worktrees/widget-fiber-rail/android "testDebugUnitTest lintDebug assembleDebug"`

`git diff --check`

### Task 1: Restore the compact fiber rail and alignment
- [ ] Render fiber with the same weighted rail and spacer as the other compact rows.
- [ ] Update the Android documentation to describe the neutral compact fiber rail.
- [ ] Run Android tests, lint, APK build and the Review Gate; record device validation limits.

The control host has no launcher rendering test or connected phone. JVM tests and a debug APK do not confirm pixels on the owner's 2x2 launcher. Phone acceptance remains a separate delivery check.
