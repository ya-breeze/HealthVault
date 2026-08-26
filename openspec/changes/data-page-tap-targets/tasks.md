## 1. Failing coverage first

- [ ] 1.1 Add a data-detail case to `e2e/tests/mobile-tap-targets.spec.ts` that logs in, opens a
      seeded `/data/<type>/` route at the existing `MOBILE_VIEWPORT`, and asserts via the existing
      `assertMinTapTarget` helper that each record row's delete control meets 48×48
- [ ] 1.2 Extend that case to activate one row's delete control and assert its confirm and cancel
      controls each meet 48×48
- [ ] 1.3 Add assertions that each of the Day / Week / Month / Year zoom tabs meets 48×48
- [ ] 1.4 Guard the record-row assertions so the case skips (not fails) when the chosen type has no
      record rows, per the design's flake risk
- [ ] 1.5 Run the new case against `hcw-wip` and confirm it FAILS on current code, reporting the
      measured sizes — this is what proves the test is real

## 2. Implementation

- [ ] 2.1 Import `TapTarget` into `frontend/app/data/[type]/DataTypeClient.tsx`
- [ ] 2.2 Migrate the per-record delete control (`aria-label="Delete record"`) to `TapTarget`,
      preserving its `aria-label`, `onClick` and any `data-testid` exactly
- [ ] 2.3 Migrate the pending-delete confirm and cancel controls to `TapTarget`, preserving their
      existing handlers and accessible names
- [ ] 2.4 Migrate the Day / Week / Month / Year zoom tabs to `TapTarget`, keeping the selected-state
      styling and the `Zoom` value each one sets
- [ ] 2.5 Confirm no sizing utility classes were added at any call site — the minimum must come from
      `TapTarget` alone (per the design's first decision)

## 3. Verification

- [ ] 3.1 Run `make lint` (or the frontend's type-check/lint target) and fix anything it reports
- [ ] 3.2 Run the frontend unit tests (`npm test` in `frontend/`) and confirm no regression
- [ ] 3.3 Deploy the branch to `hcw-wip` and wait for it to come up
- [ ] 3.4 Re-run the new e2e case against `hcw-wip` and confirm it now PASSES
- [ ] 3.5 Run the full `mobile-tap-targets.spec.ts` file to confirm the previously-covered routes
      did not regress
- [ ] 3.6 Re-measure the data page at 390×844 and confirm the delete control and zoom tabs report
      ≥48×48, and that no data column lost width
- [ ] 3.7 Confirm the table's column set and horizontal-scroll behaviour are unchanged from `main`

## 4. Finalize

- [ ] 4.1 Self-review the full diff against the proposal's out-of-scope list — no layout or visual
      redesign may have crept in
- [ ] 4.2 Push the branch and open the PR
- [ ] 4.3 Ask the user to run `/code-review` on the branch and address findings
- [ ] 4.4 After approval to finalize, archive the change with `openspec archive data-page-tap-targets --yes`
      and validate with `openspec validate --specs --strict`
- [ ] 4.5 Regenerate the projected-specs artifact as a second commit if this project's generator
      requires it (see `openspec/specs.projected.README.md`)
