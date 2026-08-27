# Add a test-e2e Makefile target that runs against hcw-wip
Idea: ya-breeze/idea-forge#42

## Why

HealthVault has a full Playwright suite — 9 spec files under `e2e/` — and no `make` target runs
it. The Makefile only defines `all`, `build`, `test`, `test-backend`, `test-frontend`, `lint`, and
`run-backend`, so the suite is only ever run by hand. `docker-compose` is not available in this
environment's containers, so the suite has to run directly against an already-deployed stack, not
a locally-started one.

This matters beyond convenience: ya-breeze/idea-forge#40 taught ralph to run a project's `test-e2e`
target when one is configured. Without the target existing, this repo is silently skipped by that
automation — every future change lands without the E2E signal `e2e-validate` is supposed to
provide, and the gap looks like "no E2E tests exist" rather than what it actually is, which is
"the tests exist but nothing invokes them."

Diary already has this target in this shape (`BASE_URL` defaulting to its own WIP stack,
overridable), which is the pattern this environment has settled on. This change brings HealthVault
to the same shape, plus the fix Diary's version doesn't need: HealthVault's `e2e/` has its own
`node_modules`, so a target that just calls `npx playwright test` fails with `MODULE_NOT_FOUND` on
a worktree where nobody has run `npm install` yet, which is the normal state of a fresh feature
branch. `test-e2e` should self-heal that instead of documenting it as a manual step someone has to
remember.

## How

Add a `test-e2e` target to the top-level `Makefile`, matching Diary's shape:

```make
.PHONY: test-e2e
test-e2e: e2e/node_modules
	@cd e2e && BASE_URL=$(or $(BASE_URL),http://192.168.1.54:8892) npx playwright test --reporter=line

e2e/node_modules: e2e/package-lock.json
	@cd e2e && npm ci
	@touch e2e/node_modules
```

`8892` is `hcw-wip`'s HTTP port (`jq '.deployments["hcw-wip"].http_port' /data/data.json`).
`BASE_URL` overrides it, so `make test-e2e` still works against any other reachable stack —
`hcw-prod` for a read-only smoke check, or a different WIP stack — without editing the Makefile.

The `e2e/node_modules` prerequisite is a real Make file target, not a phony one: its recipe runs
`npm ci` and then `touch`es the directory so its mtime is newer than `package-lock.json`. That
makes the dependency-install step conditional the way Make already understands — `make test-e2e`
installs on the first run in a fresh worktree (where `e2e/node_modules` doesn't exist) and skips
straight to the tests on every run after, unless `package-lock.json` changes. This is why the
target is not simply "run `npm ci` unconditionally before every test run": that would work, but
would add several seconds of no-op network/install cost to every local iteration.

Two things this change deliberately does **not** touch:

- **`test` stays exactly as it is.** `test-e2e` is a separate target, not folded into `test` and
  not made a prerequisite of it — `test` must keep working with no deployed stack (e.g. in CI, or
  a sandboxed agent with no network path to `192.168.1.54`), and idea-forge's ralph selects
  `test-e2e` by name when it wants the E2E signal specifically.
- **Seeded users are not hardcoded into the target or into any spec file.** The suite already logs
  in using credentials the specs assume exist; those credentials come from whichever stack
  `BASE_URL` points at, via that stack's `HCW_SEED_USERS` env var (`hcw-wip` currently seeds
  `TestFamily:alice:pass1` — see `deployments["hcw-wip"].env` in `data.json`). `test-e2e` only
  supplies the URL. If a future stack is pointed at with different seeded credentials than the
  specs expect, that is a seed-data mismatch to fix in `data.json` or the specs, not something
  this Makefile target should paper over.

Out of scope: Playwright's browser binaries (not `e2e/node_modules`, which only holds the npm
packages) are a one-time, environment-wide install already covered by the `e2e-validate` skill's
existing guidance (`npx playwright install chromium`) — this change does not duplicate that inside
the Makefile, since it isn't a per-project or per-worktree concern.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e` (requires `hcw-wip` deployed and reachable at the URL in `data.json`)

### Task 1: Add the test-e2e target
- [ ] Add the `test-e2e` and `e2e/node_modules` targets to the top-level `Makefile`, exactly as
      described in `## How`
- [ ] Confirm `test`'s recipe and prerequisites are unchanged — `test-e2e` must not be reachable
      from `make test`
- [ ] Mark completed

### Task 2: Verify from a clean worktree
- [ ] From a worktree with no `e2e/node_modules` present, run `make test-e2e` against `hcw-wip`
      and confirm it installs dependencies once, then runs the suite to completion (pass or a
      pre-existing failure unrelated to this change — either way, no `MODULE_NOT_FOUND`)
- [ ] Run `make test-e2e` a second time in the same worktree and confirm it does not re-run `npm
      ci` (no network/install step observed the second time)
- [ ] Mark completed
