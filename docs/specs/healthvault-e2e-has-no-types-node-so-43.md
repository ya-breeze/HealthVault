# Repair validation for the e2e Node typing gate
Idea: ya-breeze/idea-forge#269

## Why

The original e2e type-check produced 43 diagnostics because `e2e/package.json` lacked Node declarations even though the suite uses `process.env`, `__dirname`, `Buffer`, `node:http`, `node:net`, `path`, and `fs`. That permanent noise prevented TypeScript from exposing real defects in a Playwright suite that creates and deletes Food Meals, rewrites settings, and writes records against hcw-wip.

Candidate `c312752` already contains the core solution: `e2e/package.json` declares `@types/node` and a local `typescript`, `e2e/tsconfig.json` enables a strict no-emit check, `tests/helpers/target.ts` uses `NodeJS.ProcessEnv`, the real `CanvasRenderingContext2D` cast diagnostic in `tests/food.spec.ts` is resolved, and the `Makefile` makes `lint` depend on `lint-e2e` before running `go vet -tags sqlite_fts5 ./...`. Its implementation record says the type-check and all three project validation commands passed.

Independent reconciliation nevertheless found that the required `go-vet` validation failed for `c312752` while the base passed. The branch is also based on `2961873`, while `main` has advanced through subsequent backend and e2e changes. The candidate therefore cannot ship on its earlier validation record: the exact failure must be reproduced on the current combined tree, repaired without losing the valid typing work, and reviewed and validated again.

## How

Preserve the candidate's dependency, lockfile, strict TypeScript configuration, `NodeJS.ProcessEnv` signature, focused canvas-stub correction, and `lint-e2e` integration. Reconcile the branch with current `main`, then reproduce `make lint` and distinguish a failure in the `lint-e2e` prerequisite from the backend recipe in `Makefile`. The backend check must retain the repository's `sqlite_fts5` build tag because the USDA index depends on SQLite FTS5.

Repair the concrete diagnostic at its source. If current e2e additions reveal TypeScript errors, use accurate types or implementation changes rather than weakening `strict`, excluding files, adding `any`, or introducing `@ts-ignore` or `@ts-expect-error`. If the backend vet step fails, make the smallest correctness-preserving Go change required on the reconciled tree and add focused coverage when behavior changes. If the failure comes from dependency installation or the `.install-stamp` prerequisite, correct the package-lock or Make dependency relationship while keeping `make lint` reproducible from a clean checkout. Do not remove either the Node type-check or Go vet merely to make the gate green.

The end state remains a zero-diagnostic e2e type-check gated by `make lint`, followed by a clean backend vet. Type-checking `frontend/`, changing Playwright retries or workers, restructuring unrelated tests, and changing application behavior are deliberately excluded. Validation must target the existing non-production hcw-wip default; this change does not create or modify a public hostname, Cloudflare Access policy, dogfood stack, or production stack.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Reconcile and reproduce the failed validation
- [ ] Reconcile candidate `c312752` with current `main`, preserving the existing changes in `Makefile`, `e2e/package.json`, `e2e/package-lock.json`, `e2e/tsconfig.json`, `e2e/tests/helpers/target.ts`, and `e2e/tests/food.spec.ts`.
- [ ] Run `make lint` on the reconciled tree and capture the first actionable failure, identifying whether it comes from dependency installation, `npm run typecheck --silent`, or `cd backend && go vet -tags sqlite_fts5 ./...`.
- [ ] Confirm the failure is specific to the candidate or its integration with current `main`, using the passing base as the comparison rather than treating unrelated pre-existing output as part of this change.
- [ ] Mark completed

### Task 2: Repair the candidate-specific failure
- [ ] Fix the reproduced diagnostic at its source with the smallest scoped change, retaining the local TypeScript compiler, Node 22 declarations, strict no-emit configuration, and the `lint-e2e` gate.
- [ ] If the failure is in newly reconciled e2e code, fix every resulting TypeScript diagnostic without `any`, suppression comments, relaxed compiler options, or exclusions from `e2e/tsconfig.json`.
- [ ] If the failure is in Go code, preserve the existing behavior, run vet with `GO_TAGS := sqlite_fts5`, and add or update a focused test if the repair changes executable behavior.
- [ ] If the failure is in dependency setup, keep `e2e/package.json` and `e2e/package-lock.json` synchronized and ensure the absolute `e2e/node_modules/.install-stamp` prerequisite remains valid for a clean checkout.
- [ ] Confirm `resolveTarget` remains typed as `NodeJS.ProcessEnv` and that the proxy callbacks in `e2e/tests/auth.spec.ts` rely on the resolved `node:http` inference unless an explicit annotation is genuinely required.
- [ ] Mark completed

### Task 3: Review the repaired integration
- [ ] Review the complete diff from current `main` for accidental reversions, unrelated behavior changes, weakened type coverage, stale lockfile data, and Makefile dependency or ordering mistakes.
- [ ] Confirm `lint` still runs both `lint-e2e` and `go vet -tags $(GO_TAGS) ./...`, and that `lint-e2e` still invokes the canonical `typecheck` script.
- [ ] Fix every correctness or specification-fidelity issue found during review before rerunning validation.
- [ ] Mark completed

### Task 4: Validate the final result
- [ ] Run `make lint` and confirm both the e2e TypeScript check and backend Go vet finish successfully with zero diagnostics.
- [ ] Run `make test` and fix any candidate-caused backend or frontend regression.
- [ ] Run `make test-e2e` against its guarded hcw-wip default and fix any candidate-caused failure without targeting production.
- [ ] Recheck the final diff after validation so generated files, temporary deliberate errors, and local test artifacts are not included.
- [ ] Mark completed
