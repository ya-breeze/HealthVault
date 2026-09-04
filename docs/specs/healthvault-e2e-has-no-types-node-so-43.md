# e2e: give the suite `@types/node` and a type-check that reports zero
Idea: ya-breeze/idea-forge#269

## Why

`e2e/package.json` declares one dependency, `@playwright/test`. The specs under `e2e/tests/` use far more than that. `auth.spec.ts` imports `node:http` and `node:net`. `food.spec.ts` imports `path`, calls `await import('fs')` at lines 373 and 1910, reads `__dirname` at five call sites, and builds a `Buffer` at line 375. `import.spec.ts` imports `path`. Sixteen files read `process.env.HCW_USER` and `process.env.HCW_PASS`, and `tests/helpers/target.ts:101` passes `process.env` straight into `resolveTarget`. None of those names have types in this package.

A type-check over the suite therefore reports 43 diagnostics on `main`, and every one is the same missing dependency: `TS2307` for the node module imports, `TS2580` for `process`, `TS2304` for `__dirname`, and `TS7006` where a callback parameter loses its type because the module it came from did not resolve. The suite still runs, because Playwright transpiles TypeScript without checking it. That is exactly the problem. A real type error introduced tomorrow arrives as diagnostic number 44 and nobody sees it.

The noise has already changed the code. `tests/helpers/target.ts:54-56` carries a comment explaining that `resolveTarget` takes a `Record<string, string | undefined>` instead of the natural `NodeJS.ProcessEnv` only because "this package has no `@types/node`, so that namespace does not resolve here". A missing devDependency is shaping a public signature.

Now is the moment because this suite mutates a live stack. It creates and deletes meals, rewrites user settings, and writes records against hcw-wip. `tests/helpers/target.ts` exists solely to keep it away from hcw-prod. A suite with that reach deserves a check that fails on a typo, and `make lint` today runs `go vet` over the backend and nothing else.

## How

Add `@types/node` and `typescript` to `e2e/package.json` devDependencies, add an `e2e/tsconfig.json`, fix whatever real diagnostics the typing reveals, and wire the check into `make lint`.

Both new dependencies are needed. `@types/node` supplies the missing declarations. `typescript` has to be pinned in the lockfile too, because a lint gate must not resolve its own compiler over the network at run time — `npx tsc` with no local install does exactly that. Pin `@types/node` to the major version of the Node that actually runs the suite: run `node -v` in the environment where `make test-e2e` runs and match it. Do not copy `frontend/package.json`'s `^20` by default; `frontend/Dockerfile` builds on `node:22`, so the two are already out of step and the e2e number should follow the runtime, not the neighbour.

The `tsconfig.json` is new, so its settings are a decision rather than an inheritance. Start from `strict: true`, `noEmit: true`, `esModuleInterop: true` (`food.spec.ts:2` and `auth.spec.ts:3` use default imports of CommonJS modules), `skipLibCheck: true`, `target: ES2022`, `lib: ["ES2022", "DOM", "DOM.Iterable"]` (specs pass browser-side callbacks to `page.evaluate`, so `window` and `navigator` must resolve), `types: ["node"]`, and `module: "commonjs"` with `moduleResolution: "node10"`, which is closest to how Playwright loads these files. Include `tests/**/*.ts` and `playwright.config.ts`. If one of those settings is the sole cause of a diagnostic that cannot otherwise be removed, change it and say in a comment which setting moved and why.

Expect the idea's estimate of the residual work to shrink. The three `TS7006` parameters in `auth.spec.ts`'s `startLocalProxy` — `req`, `res` at line 45 and `proxyRes` at line 54 — are implicitly `any` only because `node:http` does not resolve. Once it does, `http.createServer` and `http.request` infer them, and no annotation is needed. Annotate them only if a diagnostic survives; a hand-written annotation that duplicates an inferred type is worse than none.

Restore `NodeJS.ProcessEnv` on `resolveTarget`, and delete the comment paragraph that explains its absence. This is safe for `tests/e2e-target.spec.ts`, which calls `resolveTarget({})` and `resolveTarget({ BASE_URL: … })` with object literals; `ProcessEnv` is an index-signature type those literals satisfy.

Wire the check into `make lint` only if the count reaches zero. A gate expected to be red teaches everyone to ignore it, which is the failure this change is trying to end. If some diagnostic cannot be removed, leave the `lint` target untouched, add the check as a separate target, and state in the pull request which diagnostic blocked it. The wiring costs one thing worth naming: `make lint` gains the `e2e/node_modules/.install-stamp` prerequisite the `test-e2e` target already uses, so on a fresh checkout `make lint` runs `npm ci` and needs the network. That is acceptable — `make test-e2e` already has the same dependency, and the stamp makes it a one-time cost.

Deliberately excluded: type-checking `frontend/`. It has a `tsconfig.json` and its own `typescript` dependency, but `make lint` never invokes `tsc` there either. That is a second gap with its own trade-offs, and folding it in would hide this change's result behind an unrelated wall of diagnostics. Also excluded: any change to what the specs test, how they are structured, or the `retries`/`workers` settings in `playwright.config.ts`.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Add the typing dependencies
- [ ] Run `node -v` in the environment that runs `make test-e2e` and record the major version.
- [ ] Add `@types/node` to `devDependencies` in `e2e/package.json`, pinned to that major version.
- [ ] Add `typescript` to `devDependencies` in `e2e/package.json`, so the type-check never resolves a compiler over the network.
- [ ] Run `npm install` in `e2e/` to refresh `e2e/package-lock.json`, and commit both files.
- [ ] Confirm `npm ci` in `e2e/` still succeeds from the refreshed lockfile.
- [ ] Mark completed

### Task 2: Add a tsconfig and a type-check script
- [ ] Create `e2e/tsconfig.json` with `strict`, `noEmit`, `esModuleInterop`, `skipLibCheck`, `target: ES2022`, `lib: ["ES2022", "DOM", "DOM.Iterable"]`, `types: ["node"]`, `module: "commonjs"`, and `moduleResolution: "node10"`.
- [ ] Set `include` to `tests/**/*.ts` and `playwright.config.ts`, and `exclude` to `node_modules`.
- [ ] Replace the placeholder `scripts.test` in `e2e/package.json` with a `typecheck` script that runs `tsc --noEmit`, so the check has one canonical spelling.
- [ ] Run the type-check and record the new diagnostic count and the full list.
- [ ] Mark completed

### Task 3: Fix the diagnostics the typing reveals
- [ ] Re-run the type-check and confirm the `TS2307`, `TS2580` and `TS2304` diagnostics for `node:http`, `node:net`, `path`, `fs`, `process`, `__dirname` and `Buffer` are all gone.
- [ ] Check whether `req`, `res` (`e2e/tests/auth.spec.ts:45`) and `proxyRes` (`e2e/tests/auth.spec.ts:54`) still report `TS7006`; annotate them with `http.IncomingMessage`, `http.ServerResponse` and `http.IncomingMessage` only if they do.
- [ ] Fix every remaining diagnostic in `e2e/tests/` and `e2e/playwright.config.ts`, treating each as a real defect rather than something to suppress.
- [ ] Use no `@ts-ignore`, no `@ts-expect-error`, and no `any` to reach zero; if a diagnostic resists, change the code or the tsconfig setting and explain the change in a comment.
- [ ] Mark completed

### Task 4: Restore `NodeJS.ProcessEnv` on `resolveTarget`
- [ ] Change the `env` parameter of `resolveTarget` in `e2e/tests/helpers/target.ts:60` from `Record<string, string | undefined>` to `NodeJS.ProcessEnv`.
- [ ] Delete the doc-comment paragraph at `e2e/tests/helpers/target.ts:54-56` that explains the missing `@types/node`, and keep the paragraph above it explaining why the function takes `env` at all.
- [ ] Confirm `e2e/tests/e2e-target.spec.ts` still type-checks, since every case there passes an object literal.
- [ ] Mark completed

### Task 5: Gate on the check and validate
- [ ] Confirm the type-check reports zero diagnostics.
- [ ] Add a `lint-e2e` target to the `Makefile` that depends on `$(ROOT_DIR)e2e/node_modules/.install-stamp` and runs the `typecheck` script in `e2e/`.
- [ ] Make `lint` depend on both the existing `go vet` step and `lint-e2e`, add `lint-e2e` to `.PHONY`, and comment why the e2e check is gated rather than advisory.
- [ ] If, and only if, a diagnostic could not be removed, leave `lint` unchanged, keep `lint-e2e` as a standalone target, and state the blocking diagnostic in the pull request description.
- [ ] Verify the gate bites: introduce a deliberate type error in a spec, confirm `make lint` fails, then revert it.
- [ ] Run `make lint`, `make test`, and `make test-e2e` against hcw-wip, and confirm all three pass.
- [ ] Mark completed
