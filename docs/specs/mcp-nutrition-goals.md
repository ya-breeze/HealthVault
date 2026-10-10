# Read current nutrition goals through personal MCP
Idea: https://ideaforge.ikoro.in/idea/1010

## Why
ChatGPT can read recorded food and activity, but cannot read the connected user's nutrition goals. It needs the current application targets to compare intake with an explicit reference instead of inventing personal targets.

## How
Add one read-only tool, `get_nutrition_goals`, with no input parameters. Resolve the fixed owner on every call under the existing backup barrier. Read settings once and reuse the current nutrition-target computation. Return its values and basis, availability reason, calculation time, resolved timezone and local date. Return fiber independently with configured/default/unavailable provenance using the existing fiber rules. Missing macro targets are null, not zero. A default fiber value remains an application default even without a profile.

These are current calculated targets, not historical targets or a prescribed calorie deficit. Preserve existing formulas and activity inference. Exclude goal editing, new targets for other nutrients, OAuth, analysis/advice/scoring tools, and historical goal reconstruction. ChatGPT performs comparisons and recommendations from existing evidence. No settings, records or advice caches are written. Merge and production rollout require separate owner approval after reviewed WIP validation; refreshed ChatGPT acceptance follows rollout outside this implementation gate.

## Validation Commands
`make test-backend`
`make lint`
`make test-e2e E2E_ARGS=--retries=0` against hcw-wip

### Task 1: Expose current goals
- [x] Add the fixed-owner read-only tool and advertise its current-only semantics.
- [x] Reuse shared macro computation and fiber resolution with explicit sources, units and missing-data reasons.
- [x] Document the contract and ChatGPT refresh check.
- [x] Mark completed.

### Task 2: Verify the contract
- [x] Test HTTP target parity, owner isolation, future rows, fresh settings and missing goals/data.
- [x] Test independent configured/default/unavailable fiber and read-only behavior.
- [x] Verify discovery, identity rejection and database error handling.
- [x] Run backend tests, static checks and native plus cross-platform Review Gate.
- [x] Mark completed.

### Task 3: Validate WIP
- [x] Verify hcw-wip class, pinned runtime source identity and ten-tool MCP discovery.
- [x] Verify goals against the existing HTTP endpoint and preserve synthetic WIP data after the probe.
- [x] Run the full deployed browser suite and record results.
- [x] Mark completed.

Validation evidence: backend suite and `make lint` passed on 2026-10-10; Android lint skipped because no SDK is installed. Native correctness, standards and spec/tests reviews completed. Claude peer completed; legacy Local timezone parity and missing identity-test findings were fixed and verified. WIP deployed application revision `d3e6528946f4a72c192a2d65bfbef7096f8d905d`; actual backend binary SHA-256 `947732ef7a31c48418c300dfc01956b2f78349acf838f445501cf674025a34ed` matched the pinned build, and nginx configuration matched source. MCP 1.3.0 exposed ten tools. Protocol probes passed HTTP goals parity, owner isolation, fresh settings and read-only checks; synthetic records were removed and original settings/record baselines restored. Full deployed Chromium suite: 325 passed, 1 skipped, retries disabled, one worker (5.5 minutes). Production rollout and refreshed owner ChatGPT acceptance remain separate approved steps.
