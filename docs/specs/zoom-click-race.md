# Two tests that assert on a request their click never sends

## Why

`docs/specs/stabilize-flaky-e2e.md` took five unstable e2e tests to zero failures across ten
consecutive full-suite runs. Validating [HealthVault#45](https://github.com/ya-breeze/HealthVault/pull/45)
against `hcw-wip` immediately afterwards found a sixth and seventh, in a file that branch does not
touch:

| Run | Result |
|---|---|
| 1 | 190 passed |
| 2 | 190 passed |
| 3 | **2 failed** — `data-types.spec.ts:159`, `data-types.spec.ts:197` |
| 4 | 190 passed |
| 5 | **2 failed** — same two |
| 6 | **1 failed** — `data-types.spec.ts:197` |

Both are pre-existing on `main`. Neither #45 nor #49 touches `e2e/tests/data-types.spec.ts` or
`frontend/app/data/[type]/DataTypeClient.tsx`; the last commit to either was #38.

**The click under test does nothing.** `DataTypeClient.tsx:116` is `useState<Zoom>('week')`, so the
page already opens on Week. Both tests then do:

```ts
await page.goto('/data/weight/');
const [req] = await Promise.all([
  page.waitForRequest(r => /\/api\/data\/weight\?.*bucket=day/.test(r.url())),
  page.getByRole('button', { name: 'Week', exact: true }).click(),
]);
```

Clicking the zoom that is already selected sets state to the value it already holds, so React
re-renders nothing and no fetch goes out. The only request that can ever satisfy the waiter is the
one the page issues on mount — and whether that arrives before or after `Promise.all` registers
its listener is a race against `page.goto` resolving. Won, the test passes; lost, it waits the
full 30 s and fails.

The `Promise.all([waitForRequest, click])` shape is the correct Playwright pattern, and it is
correctly applied here. That is what makes this hard to see on inspection: the defect is not the
synchronization, it is that the action being synchronized against is a no-op.

The neighbouring `weight Year-zoom bucketed fetch widens to >= ~2 years` test is not affected,
because Year is not the default: its click really does change state and really does refetch.

Both tests pass 5/5 in isolation — the failure needs the full suite's timing — which is why ten
clean runs of the suite on the previous branch did not surface them.

**A second, unrelated test overstates what it proves.** `TestLogout_RevokesTheRefreshToken`
(`backend/pkg/server/logout_test.go`, added in #49) asserts that a refresh token stops working
after logout. It passes. But `cookies.SetRefreshCookie` scopes the cookie to
`Path=/api/auth/refresh`, so a browser never sends `kin_refresh` to `/api/auth/logout`:
`cookies.GetRefreshToken(r)` returns empty there and the `RevokeRefreshToken` call never runs.
Confirmed against the deployed stack — login, logout, then replaying the refresh token still
returns 204 and mints a new session. The test passes only because `httptest.NewRequest` +
`AddCookie` attaches whatever cookie the test names, ignoring path scoping.

The behaviour is being left as it is: scoping the year-long credential to a single endpoint is
itself the reason it is hard to steal, and widening the path to make logout work would send it on
every request. That decision is recorded in
[idea-forge#181](https://github.com/ya-breeze/idea-forge/issues/181). What must not stand is a
test that reads as proof of a revocation the product does not perform.

## How

**Make the click transition.** Select a different zoom first, let it settle, then assert on the
transition into the zoom under test. `Day` is the natural predecessor for both: it is a real zoom
the page supports, it is not the default, and moving Day → Week is exactly the state change whose
outgoing request these tests exist to inspect.

Waiting for the intermediate zoom's own request before starting the real assertion is what makes
the setup deterministic rather than another race. The alternative — registering the waiter before
`goto` and asserting on the mount request — would also be green, but it would quietly convert
these from "selecting Week widens the fetch" into "the page happens to load on a widened fetch",
and the widening bug they were written for lives in the zoom handler.

Waiting for the intermediate zoom needs a signal that it landed, and the zoom control has none:
selection is expressed only as `bg-border text-accent` on the active button. Pinning a Tailwind
class pair is the brittleness a code review flagged on the previous branch, so the control gets
`aria-pressed` instead. That is the correct markup for a toggle group regardless — today the
selected zoom is conveyed by colour alone, which assistive technology does not announce — and it
gives the test a semantic handle that survives restyling.

Not changed: the `PROJECTION_LOOKBACK_DAYS` structural exclusion in the weight test, the assertion
bounds, or any zoom/fetch behaviour in `DataTypeClient.tsx`. The application logic is correct;
only the tests are wrong about what they exercise.

**Correct the logout test to say what it verifies.** Keep it — the handler logic it covers is
real, and it is the regression guard if the cookie path ever widens — but rename it and document
that it cannot demonstrate browser behaviour, so nobody reads it as evidence that logout ends a
session. This is a comment-and-name change, not a behaviour change.

## Validation Commands

```
make lint
make test
make test-e2e E2E_ARGS=--retries=0
```

The gate for this change is the full suite run repeatedly, not once: both tests pass in isolation
and passed three of six full runs before the fix. Fewer than five consecutive clean runs proves
nothing here.

### Task 1: Make the two zoom tests assert on a real transition

- [x] In `weight Week-zoom bucketed fetch widens to >= 14 days`, select `Day` and wait for its
      request to land before the `Promise.all` that clicks `Week`.
- [x] Do the same in `heart_rate Week-zoom bucketed fetch is not widened`.
- [x] Leave the existing matchers, the `PROJECTION_LOOKBACK_DAYS` exclusion, and the assertion
      bounds untouched — the tests' claims are right, only their setup is.
- [x] Record in the tests why the intermediate zoom is there, so nobody removes it as redundant.
- [x] Add `aria-pressed` to the zoom buttons in `DataTypeClient.tsx`, so the wait has a semantic
      signal rather than a Tailwind class pair, and the control announces its state.
- [x] Mark completed

### Task 2: Prove the fix bites

- [x] Confirm the two tests fail against the unfixed spec under full-suite timing, so the fix is
      shown to address the observed failure and not merely to coexist with it. Done by mechanism
      rather than by repetition, which is stronger: the new premise test asserts that `/data/weight/`
      opens with Week already `aria-pressed="true"`, and it passes. That is precisely why the old
      `click('Week')` issued no request. Six pre-fix full-suite runs (three failed) are recorded in
      `Why` as the observed rate.
- [x] Run the full suite five times with retries genuinely disabled; every run must be zero failed
      and zero flaky. Record the counts. Result: **189 passed, 0 failed, 0 flaky, five runs out of
      five** (188 plus the new premise test).
- [x] Report any test that still fails rather than re-running until it passes. None did.
- [x] Mark completed

### Task 3: Stop the logout test overstating its result

- [x] Rename `TestLogout_RevokesTheRefreshToken` to say what it actually pins, and document that
      `httptest` ignores cookie paths so the test cannot reflect the browser flow.
- [x] Correct the comment in `Logout` (`backend/pkg/server/auth.go`) that claims the revocation
      closes the hole, and point it at idea-forge#181.
- [x] Leave the revocation call in place — it is correct for any caller that does send the cookie,
      and it is what starts working if the cookie path ever widens.
- [x] Mark completed
