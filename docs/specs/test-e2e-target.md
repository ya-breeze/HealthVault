# Add a test-e2e Makefile target that runs against hcw-wip
Idea: ya-breeze/idea-forge#42

## Why

HealthVault has a full Playwright suite — 10 spec files under `e2e/` — and no `make` target runs
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
to the same shape, plus a fix Diary's version also needs but doesn't have: HealthVault's `e2e/`
has its own `node_modules`, so a target that just calls `npx playwright test` fails with
`MODULE_NOT_FOUND` on a worktree where nobody has run `npm install` yet, which is the normal state
of a fresh feature branch. `test-e2e` should self-heal that instead of documenting it as a manual
step someone has to remember.

## How

Add a `test-e2e` target to the top-level `Makefile`, matching Diary's shape:

```make
.PHONY: test-e2e
test-e2e: $(ROOT_DIR)e2e/node_modules/.install-stamp
	@cd $(ROOT_DIR)e2e && BASE_URL=$(or $(BASE_URL),http://192.168.1.54:8892) npx playwright test --reporter=line

$(ROOT_DIR)e2e/node_modules/.install-stamp: $(ROOT_DIR)e2e/package-lock.json
	@cd $(ROOT_DIR)e2e && npm ci
	@touch $(ROOT_DIR)e2e/node_modules/.install-stamp
```

`$(ROOT_DIR)` is the Makefile's existing `dir $(realpath ...)` variable, already used by every other
target (`build`, `test-backend`, `test-frontend`, `run-backend`) so each keeps working regardless of
the directory `make` is invoked from. The `e2e/` targets follow the same convention rather than
relying on bare relative paths that only resolve when `make` is run from the repo root.

`8892` is `hcw-wip`'s HTTP port (`jq '.deployments["hcw-wip"].http_port' /data/data.json`).
`BASE_URL` overrides it, so `make test-e2e` still works against any other reachable stack — a
different WIP stack — without editing the Makefile. Do not point it at `hcw-prod`: the suite is
not read-only, several specs issue real `POST`/`PUT`/`DELETE` requests (creating and deleting food
log entries, changing settings), and `hcw-prod` holds real, irreplaceable data per this
environment's stack-class rules.

This matters beyond the Makefile target itself: `playwright.config.ts` and 5 of the 10 spec files
each independently fall back to `http://192.168.1.54:8888` — `hcw-prod`'s port — when `BASE_URL` is
unset, and `hcw-prod` seeds the same `alice`/`pass1` credentials the suite logs in with. `make
test-e2e` always sets `BASE_URL` explicitly, so it never hits any of these fallbacks, but anyone
invoking `npx playwright test` directly from `e2e/` without `BASE_URL` set still hits `hcw-prod`
silently, with valid credentials, running the same mutating requests this change exists to make
safe to run routinely. Rewriting every one of those fallbacks is a larger change than one Makefile
target — see "Two things this change deliberately does not touch" below; this is a third,
pre-existing one, called out here rather than left implicit.

The prerequisite is a stamp file (`e2e/node_modules/.install-stamp`), not the `e2e/node_modules`
directory itself. `npm ci` writes into `node_modules` incrementally, so a directory-as-target
would leave a *directory* with a fresh mtime even when `npm ci` dies partway through (network
drop, disk full) — Make would then treat the failed install as satisfied on the next run and skip
straight to a confusing `MODULE_NOT_FOUND` instead of retrying. A stamp file avoids that: it is
only `touch`ed after `npm ci` exits 0, so a failed install leaves no stamp (or a stale one) and the
next `make test-e2e` retries the install. This makes the dependency-install step conditional the
way Make already understands — `make test-e2e` installs on the first run in a fresh worktree
(where the stamp doesn't exist) and skips straight to the tests on every run after, unless
`package-lock.json` changes. This is why the target is not simply "run `npm ci` unconditionally
before every test run": that would work, but would add several seconds of no-op network/install
cost to every local iteration.

Three things this change deliberately does **not** touch:

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
- **`playwright.config.ts`'s and individual specs' own `BASE_URL` fallbacks are not rewritten.**
  Several of them default to `hcw-prod`'s port when `BASE_URL` is unset — a pre-existing risk this
  change doesn't introduce and, being spread across the config and 5 spec files, is a larger fix
  than one Makefile target. `make test-e2e` itself is unaffected since it always sets `BASE_URL`
  explicitly; a follow-up idea should hunt down and align every in-repo default instead.

Out of scope: Playwright's browser binaries (not `e2e/node_modules`, which only holds the npm
packages) are a one-time, environment-wide install already covered by the `e2e-validate` skill's
existing guidance (`npx playwright install chromium`) — this change does not duplicate that inside
the Makefile, since it isn't a per-project or per-worktree concern.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e` (requires `hcw-wip` deployed and reachable at the URL in `data.json`)

### Task 1: Add the test-e2e target
- [ ] Add the `test-e2e` target and the `e2e/node_modules/.install-stamp` target to the top-level
      `Makefile`, exactly as described in `## How`
- [ ] Confirm `test`'s recipe and prerequisites are unchanged — `test-e2e` must not be reachable
      from `make test`
- [ ] Mark completed

### Task 2: Verify from a clean worktree
- [ ] From a worktree with no `e2e/node_modules` present, run `make test-e2e` against `hcw-wip`
      and confirm it installs dependencies once, then runs the suite to completion with a
      successful (`0`) exit status and no `MODULE_NOT_FOUND`. A failing run — including a failure
      that looks pre-existing or unrelated to this change — does not satisfy this task; per this
      environment's E2E validation rule, a failure found while validating a change must be fixed,
      not waved through, so treat any failure as a finding to fix or to raise explicitly rather
      than a reason to tick this box anyway
- [ ] Run `make test-e2e` a second time in the same worktree. Record `stat -c %Y
      e2e/node_modules/.install-stamp` before and after this run and confirm it is unchanged, and
      confirm no `npm ci`/`added N packages` output appears — this is the concrete check that
      `npm ci` did not re-run, not just that the run looked fast
- [ ] Touch `e2e/package-lock.json` (or bump a dependency version) and run `make test-e2e` a third
      time; confirm `npm ci` runs again and the stamp's mtime updates — this is the path that
      makes the target self-heal a dependency change, not just a missing `node_modules`
- [ ] Confirm the `BASE_URL` override itself: `hcw-wip` and `hcw-prod` are the only two HealthVault
      deployments in `data.json`, and `hcw-prod` is off-limits (see `## How`), so there is no
      second reachable stack to point at. Instead run
      `make -n test-e2e BASE_URL=http://example.invalid:9` (Make's dry-run flag, which prints the
      recipe without executing it) and confirm the printed command line carries
      `BASE_URL=http://example.invalid:9` rather than the `192.168.1.54:8892` default — this
      proves the `$(or $(BASE_URL),...)` substitution picks up an override without running the
      suite against anything
- [ ] Mark completed
