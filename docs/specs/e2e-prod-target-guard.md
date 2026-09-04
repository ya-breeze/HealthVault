# E2E can never run against the prod stack

## Why

A bare `npx playwright test` in `e2e/` runs most of the suite against **hcw-prod**, the
stack holding the only copy of the owner's real health data.

The suite has no single notion of "the target". Eleven files resolve `BASE_URL`
independently, and they disagree:

| default | files |
|---|---|
| `192.168.1.54:8888` — **prod** | `playwright.config.ts`, `auth`, `cf-access`, `dashboard`, `data-types`, `food`, `settings` |
| `192.168.1.54:8892` — wip | `chart-day-boundary`, `logging-gap`, `helpers/food-day` |
| `localhost:3000` — a dev server | `import` |

`playwright.config.ts` is the one that decides where the *browser* goes, and it defaults
to prod. Every spec that drives the app through the page rather than through `request` —
`settings`, `mobile-tap-targets`, `mobile-nav`, `food`'s UI paths — therefore points at
prod with nothing to stop it. `food.spec.ts` creates and deletes meals and custom foods;
`dashboard.spec.ts` POSTs to the webhook and rewrites user settings; `data-types.spec.ts`
writes records. Against prod those are not test fixtures, they are edits to real data.

`make test-e2e` passes `BASE_URL=…:8892` and is safe. The exposure is the direct
invocation — which is exactly what anyone debugging a single spec reaches for, and what
`--ui`, `--debug`, and every IDE "run test" button do.

This was found while fixing something else: `meal-history-layout.spec.ts` aborted with
`unauthorized` because the browser had logged into prod while the helper's API calls went
to wip. The mismatch is what made it visible. Had both halves defaulted to 8888 the run
would have completed — against prod, silently. The guard added there covers two files by
comparing browser host to API host; it cannot see the single-target case at all.

Repo policy is that tests never read or write real data, and `hcw-prod` is `class: prod`.
Nothing in the tree enforces either.

## How

Resolve the target **once**, in `e2e/tests/helpers/target.ts`, and have every file import
it — `playwright.config.ts` included, so the browser and the API calls cannot disagree by
construction. The default becomes the wip stack.

The module refuses to resolve a prod target at all:

```ts
export function resolveTarget(env: Record<string, string | undefined>): string
```

throws when the resolved target is `192.168.1.54:8888` or `healthvault.ikoro.in` (the
Cloudflare hostname that tunnels to the same container — a deny-list of one port would
miss it). The two are matched differently on purpose: the LAN address needs its port,
because hcw-wip is `:8892` on the same IP, while the hostname is matched without one,
because that domain serves nothing but prod. Importing the module is the first thing
Playwright does, so a poisoned `BASE_URL` fails before a browser starts, not midway
through a run that has already written three meals.

It also strips a trailing slash, since callers build request URLs by concatenating
`${BASE_URL}/api/...`.

Taking `env` as a parameter rather than reading `process.env` inside is what makes the
guard testable without spawning a child process or mutating the ambient environment; the
module-level constant is just `resolveTarget(process.env)`.

**No escape hatch.** An `ALLOW_PROD=1` opt-out would reintroduce the failure it prevents,
because the situation that needs it — "I only want to look at prod, I won't change
anything" — is indistinguishable at the guard from the one that loses data. A genuine
read-only prod smoke test is a different artifact with a different lifecycle; it can be
written deliberately, with its own spec, and this refusal is what will prompt that
conversation instead of letting the question go unasked.

`import.spec.ts`'s `localhost:3000` default disappears with the rest. It was pointing at a
dev server nobody runs during the gate; under `make test-e2e` it already received the wip
URL from the environment, so nothing about its behaviour changes.

Excluded: the per-file `USER`/`PASS` constants, duplicated the same way. They carry no
data-loss risk and folding them in would touch the same eleven files for an unrelated
reason. `helpers/food-day.ts` keeps re-exporting `BASE_URL` so its consumers are untouched.

## Validation Commands

```
make lint
make test
make test-e2e E2E_ARGS=--retries=0
```

### Task 1: Resolve the target in one place

- [x] Add `e2e/tests/helpers/target.ts` exporting `resolveTarget(env)` and `BASE_URL`
- [x] Default to `http://192.168.1.54:8892` when `BASE_URL` is unset
- [x] Throw for `192.168.1.54:8888` on any scheme or path, and for `healthvault.ikoro.in`
      on any port — while leaving `192.168.1.54:8892` alone
- [x] Strip a trailing slash, so `${BASE_URL}/api/...` never becomes `//api/...`
- [x] Error message names the stack, says what `make test-e2e` does, and does not suggest a bypass
- [x] Mark completed

### Task 2: Point every caller at it

- [x] `playwright.config.ts` takes `baseURL` from the module
- [x] Replace the `process.env.BASE_URL || …` line in all ten spec/helper files with the import
- [x] `helpers/food-day.ts` re-exports `BASE_URL` so `completeness` and `meal-history-layout` need no edit
- [x] `assertSameStack`'s comment no longer describes disagreeing defaults, because there is now one
- [x] No `process.env.BASE_URL` remains outside `target.ts`
- [x] Mark completed

### Task 3: Cover the guard

- [x] `e2e/tests/e2e-target.spec.ts` asserts the unset default, an explicit wip URL, the
      trailing-slash strip, both prod spellings rejected, the neighbouring wip port still
      allowed, and a non-URL rejected
- [x] Confirm `BASE_URL=http://192.168.1.54:8888 npx playwright test --list` fails before
      collecting tests, and record the output in the PR
- [x] Mark completed

### Task 4: Validate

- [x] `tsc --noEmit` on `e2e/` reports no diagnostic this change did not already have.
      It is not clean and was not before: `e2e/package.json` has no `@types/node`, so
      `process`, `path` and `__dirname` are untyped across 40-odd pre-existing errors.
      Installing it would fix them and is a separate change; the bar here is introducing
      no new ones — which ruled out typing the guard's parameter as `NodeJS.ProcessEnv`
- [x] `make lint` and `make test` pass (go vet clean, all Go packages ok, 176 Vitest cases)
- [ ] `make test-e2e E2E_ARGS=--retries=0` passes against `hcw-wip` on this branch
- [ ] Mark completed
