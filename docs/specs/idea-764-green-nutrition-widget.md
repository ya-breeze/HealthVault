# Green nutrition home-screen widget
Idea: https://ideaforge.ikoro.in/idea/764

## Why
The owner accepted variant A and the green palette after reviewing interactive prototypes. Calories are primary; protein and fiber are secondary, while fat and carbs remain visible when space permits. The old widget uses small macro text and leaves empty space at larger launcher sizes.

## How
Implement the accepted compact rows (A), centered calories without the target-text row, thick progress rails, and fixed green day/night colors that automatically follow Android's theme. Use actual launcher bounds to scale/reflow content rather than stretching a small pre-rendered bucket. Keep the existing food-entry, refresh and stale/error/sign-in behavior. Preserve the separate FlexWindow widget's current design and shared pacing helpers.

Calories and macro values/rails turn red only when the consumed amount is strictly above their full-day target; equal to target remains green. Clamp fill to 0..1 and retain meaningful accessible descriptions beyond color. Order compact rows protein, fat, carbs, fiber. Wide layouts retain all four rails. Fiber has no configured target: render its rail neutrally with an unavailable-target indication, never invent a goal or percentage. A future fiber goal is outside this change. Add confirmed-today dietary fiber aggregation to the API and an optional Android field; absent old-server/cache data must remain unknown rather than becoming zero. Amounts retain existing food-model provenance, not guaranteed complete intake.

The accepted visual reference is https://artifacts.ikoro.in/healthvault/widget-764-over-v7/?variant=A&palette=green&align=center . Tiny or large-font surfaces show fewer supporting metrics rather than shrinking an app dashboard into unreadable text. Update picker metadata/preview and English/Russian strings. No production deployment, merge, Play upload, personal-data mutation or unrelated widget feature is authorized here. Build a debug APK for inspection; real-launcher acceptance remains an owner/device check because no Android emulator/device is available on the control host.

## Validation Commands
- `make test-backend`
- `make lint` (Android targets locally skip without SDK; run the shared Android gate below)
- `/data/android-build.py HealthVault-worktrees/idea-764-green-nutrition-widget/android "testDebugUnitTest lintDebug assembleDebug"`
- `make test-e2e BASE_URL=<verified-hcw-wip-url> E2E_ARGS='--retries=0'`

### Task 1: Today's fiber contract
- [ ] Aggregate dietary fiber only from confirmed meals in the existing user's local-day window.
- [ ] Expose dietary_fiber_grams_consumed and test confirmed/status/day/user isolation and API serialization.
- [ ] Accept the additive Android field while preserving old-response/cache compatibility.
- [ ] Mark completed.

### Task 2: Accepted Android widget
- [ ] Implement green automatic day/night themes, centered calorie hero, no target-text row, rows A and all four wide rails.
- [ ] Use full-day strict overrun colors and accessible signals without changing FlexWindow's pacing design.
- [ ] Preserve actions and explicit stale/empty/signed-out/error states; adapt actual sizes and enlarged fonts.
- [ ] Update picker preview/metadata and localized resources; add meaningful boundary/contract tests.
- [ ] Mark completed.

### Task 3: Review and delivery evidence
- [ ] Run backend/static/shared-Android build gates and fix findings.
- [ ] Complete native Review Gate and one best-effort Claude peer review; record actual availability.
- [ ] Open a feature PR, validate against the reserved WIP stack, and attach a debug APK with exact revision evidence.
- [ ] State real-launcher visual acceptance as outstanding for owner/device inspection, separate from build and backend E2E confidence.
- [ ] Mark completed.
