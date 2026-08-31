# Stabilize the flaky e2e tests, and the refresh race one of them was reporting

## Why

Two finished changes are stuck as draft pull requests because their e2e gate failed, and
neither failure had anything to do with the change under test:

- [HealthVault#45](https://github.com/ya-breeze/HealthVault/pull/45) (LLM manual food input) died
  on `completeness.spec.ts:229`, a day-completeness test.
- [HealthVault#47](https://github.com/ya-breeze/HealthVault/pull/47) (camera capture in landscape)
  died on `auth.spec.ts:106`, a token-refresh test.

idea-forge did the right thing in both cases — for #47 it redeployed `main`, re-ran the suite,
saw the failure not reproduce on the base, and refused to excuse it. A gate that cannot tell an
unstable test from a regression has to park the Idea, and it did. The cost is that every change
now pays a lottery on the way out.

Re-running both branches against a free stack put 184 and 183 tests green with zero failures, so
neither branch is broken. Five tests are.

**Measured on `main`, not inferred.** Ten repetitions of the five suspects, then three full-suite
runs:

| Test | Evidence | Failure |
|---|---|---|
| `mobile-nav.spec.ts:297` | 3/10 isolated, 1/3 full runs | `headerControls.length` is 0 |
| `mobile-nav.spec.ts:421` | 2/3 full runs | `submit bar should have a bounding box` |
| `auth.spec.ts:106` | 1/3 full runs, killed #47 | redirected to `/login` |
| `completeness.spec.ts:229` | killed #45; flaked in a #47 re-run | 30 s wait for a response already received |
| `mobile-tap-targets.spec.ts:133` | flaked in an earlier full run | `asserted` is 0 |

Four are test defects of one kind. The fifth — and, once the first fix landed, `mobile-nav.spec.ts:426`
as well — are real bugs in the application that the tests were correctly reporting.

**The bigger bug: a re-login inside one second is dead on arrival.** `mobile-nav.spec.ts:426`
kept failing at the same 2-in-5 rate after its read was synchronized, and the added wait turned
an opaque null bounding box into the real message: `getByTestId('bottom-nav')` — element not
found. `AuthenticatedShell.tsx:106` renders the bar only once `api.me()` resolves, and a
rejection pushes to `/login`, which is where the failing page had ended up. The recorded traffic
for a failing run:

```
200 POST /api/auth/login          login succeeds
401 GET  /api/users/me            401 with the cookies login just set
204 POST /api/auth/refresh        refresh succeeds
401 GET  /api/users/me            still 401 after a good refresh
```

`RequireAuth` (`backend/pkg/server/middleware.go:22`) checks a blacklist that stores the access
token **string** (`kin-core/authdb/blacklist.go:19`), and `auth.GenerateAccessToken`
(`kin-core/auth/token.go:28`) signs claims of only `{UserID, FamilyID, IssuedAt, ExpiresAt}` —
no `jti`, no nonce, timestamps at one-second resolution, deterministic HS256. Two tokens minted
for the same user in the same second are therefore byte-identical. So logout blacklists a string
that the *next* login reproduces exactly, and `Refresh` reproduces it again, which is why the
refresh in the trace changes nothing.

Confirmed directly against the deployed stack, 5 attempts out of 5: login, logout, login again,
then `GET /api/users/me` — tokens identical, response 401 every time. It clears only when the
wall clock ticks into the next second.

This is not a test-only concern. A user who logs out and logs straight back in — wrong account,
switching users — gets a session that 401s until the second rolls over, and the frontend bounces
them back to `/login`. The test found it because `logout ends the session` runs immediately
before it.

**The test defect: reads that never wait.** `expect(...)` polls until its timeout; `count()`,
`boundingBox()`, and `evaluateAll()` sample the DOM once and return whatever is there. Each of
the four calls one of those immediately after a navigation, so each is a race against React
painting, which the suite usually wins:

- `mobile-nav.spec.ts:304` — `evaluateAll` returns `[]`, so line 309 fails.
- `mobile-tap-targets.spec.ts:138` — `count()` returns 0, the loop body never runs, and line 150
  fails on a vacuous pass its author anticipated.
- `mobile-nav.spec.ts:427` — `boundingBox()` returns `null` for a bar that has not laid out.
- `completeness.spec.ts:254` — the inverse: `waitForResponse` is registered *after* `goto` and
  after the meal is already on screen, so when the completeness response arrives first the waiter
  blocks for a second one that never comes.

**The real bug: a duplicate refresh logs the user out.** `coordinatedRefresh`
(`frontend/lib/api.ts:38`) shares one in-flight refresh across same-tab callers via
`refreshPromise`, which is cleared in `.finally()`. The dashboard dispatches several API calls at
once. With an expired access token every one of them 401s, and any whose 401 lands *after* the
shared refresh settles starts another `POST /auth/refresh`. `RotateRefreshToken`
(`backend/pkg/server/auth.go:162`) consumes the presented token, so the second refresh sends a
spent one, gets 401, and `fetchWithAuthRetry` returns the original 401 to a caller that treats it
as "not logged in".

The guard against this — "a refresh completed at or after my request was dispatched, so my 401 is
stale; retry without refreshing" — exists at line 50, but only inside the `navigator.locks`
branch. Web Locks needs a secure context, which the plain-HTTP WIP stack is not, so on that stack
the guard never runs. Lines 54–56 already call this out as a residual risk. Production is HTTPS
and therefore masked today; the residual risk is real for any non-secure origin, and the test is
what it looks like when it fires.

## How

Fix the application bug and the four test defects in one change, because they were found by one
investigation and the e2e evidence for the first is the suite the second stabilizes.

**The refresh guard moves out of the Web Locks branch.** Record the completion time of a
successful refresh in a module-level variable, and check it against `dispatchedAt` before
refreshing in *either* branch. That is the same rule the locks branch already applies through
`localStorage`, applied where it does not depend on a secure context. `localStorage` stays as the
cross-tab channel — the module variable cannot cross tabs — so cross-tab behaviour is unchanged
and same-tab behaviour stops depending on the origin's scheme.

This is deliberately not a rewrite of the refresh coordination. A queue, a single-flight wrapper,
or a retry-with-backoff would all also work, and all of them replace a mechanism that is correct
in the secure-context case with a new one to get wrong. The change is to stop the existing rule
from being conditional.

**The four tests get a waiting read before the non-waiting one.** Not a bare timeout, and not a
`networkidle`: each gets an `expect(...)` that asserts the precondition the following read
depends on, which is the thing that was implicit before. `completeness.spec.ts` instead registers
its response waiter before the navigation that triggers the response, which is the documented
Playwright pattern for that shape.

**The access token gets a `jti`.** `auth.Claims` is exported and embeds `jwt.RegisteredClaims`,
whose `ID` field is the standard `jti`, so HealthVault can mint the token itself with a random
`jti` at both call sites (`auth.go:127` in Login, `auth.go:173` in Refresh) instead of calling
`auth.GenerateAccessToken`. Every issued token is then unique, a blacklist entry revokes exactly
the one session it was created for, and `ParseToken` needs no change because `jti` is already
part of the claims it parses.

Fixing it in kin-core instead would be the tidier home — the flaw is in that library's token
minting, and every project on it has the same hole. It is a separate repository with its own
release, though, and pinning HealthVault to an unreleased version to fix a bug found here trades
one problem for a slower one. Minting locally is a two-line change in this repo that leaves
kin-core free to adopt the same fix on its own schedule. Raising it against kin-core is left to
the operator.

Not chosen: dropping the blacklist check, or blacklisting by user rather than by token. The first
removes real revocation, and the second revokes every session a user has whenever one logs out.

**Excluded: retries.** Raising `retries` in `playwright.config.ts` from 1 would hide exactly the
signal that made this change possible — idea-forge's baseline comparison distinguishes a broken
base from a broken branch, and it can only do that while a failure means something. The two flaky
mobile-nav tests were reported as `flaky` rather than `failed` for months, which is how they
survived; more retries buys more of that.

**Excluded: unsticking the two pull requests.** Both branches are finished and their gates pass
on a free stack. Whether they merge, and whether their Ideas resume through idea-forge or are
marked ready by hand, is a separate decision from this change and is left to the operator.

## Validation Commands

```
make lint
make test
make test-e2e
```

`make test-e2e` alone does not prove a flake is fixed — the suite passed it most of the time
before. The gate for this change is the suite run repeatedly, recorded in Task 4.

### Task 1: Make the same-tab refresh guard independent of Web Locks

- [x] In `frontend/lib/api.ts`, record the completion time of a successful refresh in a
      module-level variable alongside the existing `localStorage` write.
- [x] Check that variable against `dispatchedAt` in the no-Web-Locks fallback path, returning
      `true` without refreshing when a refresh has already completed at or after the caller's
      request was dispatched.
- [x] Check it in the Web Locks path too, before the `localStorage` read, so both paths apply one
      rule.
- [x] Update the comment at lines 54–56, which currently records the now-fixed gap as an accepted
      residual risk.
- [x] Mark completed

### Task 2: Add unit coverage for the duplicate-refresh case

- [x] Add `frontend/lib/api.test.ts` with a stubbed `fetch`.
- [x] Cover the failing case directly: two concurrent calls both 401, the second's 401 resolving
      after the first's refresh has completed, asserting `POST /auth/refresh` is called exactly
      once and both callers are retried.
- [x] Prove the test bites — confirm it fails against the pre-fix code before the fix is applied.
- [x] Cover the case the guard must not break: a 401 dispatched *after* the last refresh
      completed still triggers a new refresh.
- [x] Mark completed

### Task 3: Give the four unsynchronized reads something to wait for

- [x] `mobile-nav.spec.ts:297` — wait for the header's controls to be present before
      `evaluateAll` reads them.
- [x] `mobile-tap-targets.spec.ts:133` — wait for at least one header control before `count()`.
- [x] `mobile-nav.spec.ts:421` — wait for the submit bar and the navigation bar to be visible
      before `boundingBox()` reads either.
- [x] `completeness.spec.ts:229` — register the completeness response waiter before `goto`.
- [x] Keep each test's existing assertions intact; the fixes add waits, never weaken a check.
- [x] Mark completed

### Task 4: Make every issued access token unique

- [x] Add a token minter in `backend/pkg/server` that builds `auth.Claims` with a random `jti`
      and signs it, replacing both `auth.GenerateAccessToken` calls in `auth.go`.
- [x] Cover it: two tokens minted for the same user in the same second differ, and a token
      blacklisted by logout does not revoke a token issued by a later login.
- [x] Prove the test bites — confirm it fails against the pre-fix minting.
- [x] Re-run the live check from `Why` (login, logout, login, `GET /api/users/me`) against the
      deployed branch and record the result here.
- [x] Mark completed

### Task 5: Prove the suite is stable

- [x] Deploy the branch to `hcw-wip` and run `make test-e2e` five times with `--retries=0`.
- [x] Record the pass counts in this task; every run must be zero failed and zero flaky for the
      five tests named in `Why`. Result: **188 passed, 0 failed, 0 flaky, five runs out of five.**
- [x] Report any test that still fails rather than re-running until it passes. The first five-run
      gate, before Task 4 existed, is recorded here rather than discarded: it read 188/187/188/187/188
      with `mobile-nav.spec.ts:426` failing twice. That failure is what Task 4 turned out to be.
- [x] Mark completed

### Task 6: Close what the code review found

The review of this branch surfaced five things. Two are defects in the branch's own work, one is
a pre-existing auth hole the branch's new tests brush against, and two are hygiene. Scope grew,
so this task records it rather than letting the changes land unaccounted for.

- [ ] Revoke the refresh token in `Logout`. It blacklists the access token, which expires in
      15 minutes anyway, and never calls `authdb.RevokeRefreshToken` — so the year-long refresh
      token logout claims to have killed still mints new access tokens for anyone holding the
      cookie value. Cover it: refresh works before logout, 401s after.
- [ ] Make `completeness.spec.ts`'s at-threshold test wait on a positive DOM signal. The response
      waiter it currently awaits resolves on the *first* completeness window, while the page
      setStates only after `Promise.all`, so the absence assertions can still pass on an
      un-rendered page — the exact hazard the comment claims to close.
- [ ] Address `mobile-nav.spec.ts` by test id, not by the `.fixed.z-30` Tailwind pair. The same
      element already carries `data-testid="bottom-action-bar"`.
- [ ] Stop tracking `test-results/.last-run.json`, and ignore Playwright's root-cwd output. It was
      committed in this branch and records `"status": "failed"`.
- [ ] Point `auth_login_limiting_test.go` at the local `generateAccessToken` and drop its
      now-obsolete comment about second-precision collisions — Task 4 removed that hazard, and the
      test currently exercises a token shape production no longer issues.
- [ ] Re-run the five-run `make test-e2e` gate; record the counts.
- [ ] Mark completed
