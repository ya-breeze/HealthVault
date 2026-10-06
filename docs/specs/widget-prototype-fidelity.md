# Restore accepted home-widget appearance

## Why
The owner's real launcher screenshot does not match the accepted Idea 764 variant A. On Android 12+, the card uses Glance's widgetBackground derived from an unspecified secondaryContainer, so it renders purple despite the explicit green background. Padding, calorie-rail emphasis, gram typography and compact fiber placement also differ from the accepted prototype.

## What
Use the exact green day/night background. Restore proportional inset spacing and make the calorie rail thicker than nutrient rails. Center calorie-only compositions vertically. Keep large nutrient numbers with small muted gram units. In compact variant A, place fiber's icon and amount together without a rail; wide layouts retain four rails. Preserve strict overrun colors, automatic system theme, accessibility, refresh/food actions, enlarged-font reduction and FlexWindow. Do not invent missing fiber values or a fiber target.

## How
Use GlanceTheme.colors.background on the home card. Budget spacing from actual dp bounds in GreenWidgetSizing, with separately sized calorie and nutrient rails. Split amount and unit rendering. Preserve the existing summary API/cache contract. The source reference is https://artifacts.ikoro.in/healthvault/widget-764-over-v7/?variant=A&palette=green&align=center .

## Validation Commands
`/data/android-build.py HealthVault-worktrees/widget-prototype-fidelity/android "testDebugUnitTest lintDebug assembleDebug"`

`git diff --check`

Run shared Android unit tests, lint and APK build. Review the complete diff in all native angles and attempt one Claude peer review. Unit tests are not rendering evidence: real launcher/theme/font acceptance remains required and must not be described as passed. No backend or production deployment is included in this correction.

### Task 1: Restore accepted home-widget styling
- [x] Correct background, proportional spacing, rail hierarchy and compact fiber placement.
- [x] Separate gram units and preserve overrun/accessibility behavior.
- [x] Run Android build/checks and Review Gate; record their actual limits.

## Validation evidence
Shared Android: 74 tests passed, zero failures/errors; lint zero errors; debug APK built. Final build log: /tmp/widget-fidelity-build-final.log. Native correctness, standards and spec/tests review angles clean. Claude peer unavailable: weekly quota (API429), attempted once. No real-launcher rendering verification occurred. Production API at healthvault.ikoro.in independently returns numeric fiber; an older phone cache remains a hypothesis requiring a successful phone refresh. No backend changes, merge or Play publication in this correction.
