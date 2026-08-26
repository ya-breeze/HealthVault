## 1. Failing coverage first

- [x] 1.1 Add a data-detail case to `e2e/tests/mobile-tap-targets.spec.ts` at the spec file's
      existing `MOBILE_VIEWPORT` (375×667) that logs in and opens a `/data/<type>/` route
- [x] 1.2 Mock `**/api/data/**` for that case so the record table always renders a fixed set of rows,
      matching how every other case in the file mocks its API — no dependence on seeded data and no
      skip guard
- [x] 1.3 Assert via the existing `assertMinTapTarget` helper that each record row's delete control
      meets 48×48
- [x] 1.4 Extend the case to activate one row's delete control and assert its confirm and cancel
      controls each meet 48×48
- [x] 1.5 Assert that each of the Day / Week / Month / Year zoom tabs meets 48×48
- [x] 1.6 Add a `/data/nutrition` case asserting each macro selector tab meets 48×48
- [x] 1.7 Run the new cases against `hcw-wip` and confirm they FAIL on current code, reporting the
      measured sizes — this is what proves the tests are real
      → both failed: `record delete control width` **14** (expected ≥48);
      `Calories macro tab height` **26.5** (expected ≥48)

## 2. Implementation

- [x] 2.1 Migrate the per-record delete control (`aria-label="Delete record"`) to `TapTarget`,
      preserving its `aria-label`, `onClick` and any `data-testid` exactly
- [x] 2.2 Migrate the pending-delete confirm and cancel controls to `TapTarget`, preserving their
      existing handlers and accessible names
- [x] 2.3 Migrate the Day / Week / Month / Year zoom tabs to `TapTarget`, keeping the selected-state
      styling and the `Zoom` value each one sets
- [x] 2.4 Migrate the nutrition macro tabs (`NUTRITION_MACROS.map`) to `TapTarget`, keeping the
      selected-state styling and the macro each one sets
- [x] 2.5 Confirm no sizing utility classes were added at any call site — the minimum must come from
      `TapTarget` alone (per the design's first decision)
      → the `DataTypeClient.tsx` diff is exactly ten tag swaps; no `className` value changed

> Note: `DataTypeClient.tsx` already imports `TapTarget` (used by the "Set goal" / "Set height"
> controls), so no new import is needed.

## 3. Verification

- [x] 3.1 Run `make lint` and fix anything it reports → passes, but it is **backend-only**
      (`go vet ./...`) and covers none of this change. Ran `npx tsc --noEmit` in `frontend/`
      instead as the real static check for it: clean.
- [x] 3.2 Run the frontend unit tests (`npm test` in `frontend/`) and confirm no regression
      → 3 files, 57 tests, all passing
- [x] 3.3 Deploy the branch to `hcw-wip` and wait for it to come up
- [x] 3.4 Re-run the new cases against `hcw-wip` and confirm they now PASS
- [x] 3.5 Run the full `mobile-tap-targets.spec.ts` file to confirm the previously-covered routes
      did not regress → 8/8 passed
- [x] 3.6 Re-measure `/data/weight`, `/data/steps` and `/data/nutrition` at **both 375×667 and
      390×844** and confirm: delete control, zoom tabs and macro tabs all report ≥48×48; every data
      column's width and the table's `scrollWidth` are unchanged from the pre-change baseline
      (weight 110/102/104/110/86, scrollWidth 513; steps 74/110/104/106/110/86, scrollWidth 590);
      and the macro tabs still fit their `flex-wrap` container without clipping
      → all six combinations match the baseline exactly. Delete 14×20 → 48×48; zoom tabs
      48/52/59/52 × 48; macro tabs all ≥48×48, container reports no clipping.
- [x] 3.7 Confirm the table's column set and horizontal-scroll behaviour are unchanged from `main`
      → verified by A/B on one build (neutralising only what `TapTarget` adds). Also checked the
      transient pending-delete state: Actions grows 86 → 155 and scrollWidth 513 → 581, but that is
      **identical before and after** this change — confirm/cancel were already wider than 48, only
      their height (24 → 48) was deficient. The four data columns never move.

## 4. Finalize

- [x] 4.1 Self-review the full diff against the proposal's out-of-scope list — no layout or visual
      redesign may have crept in, and `AddRecordForm`'s inputs must be untouched
      → caught and removed an accidentally committed `e2e/node_modules` symlink (`.gitignore` has
      `node_modules/`, which does not match a symlink); now excluded locally. Final diff touches
      only the two intended source files plus the OpenSpec artifacts.
- [x] 4.2 Push the branch and open the PR
- [ ] 4.3 Ask the user to run `/code-review` on the branch and address findings
- [ ] 4.4 After approval to finalize, archive the change with `openspec archive data-page-tap-targets --yes`
      and validate with `openspec validate --specs --strict`
- [ ] 4.5 Regenerate the projected-specs artifact as a **second, separate commit** — the
      `openspec-projected-specs` workflow regenerates and hard-fails on any diff under
      `openspec/specs.projected/`, so this is mandatory, not conditional
