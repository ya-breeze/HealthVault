# Interactive disposable E2E for HealthVault

Idea: [Idea 962](https://ideaforge.ikoro.in/idea/962)

## Why

The existing full browser suite already passes in a temporary HealthVault stack through the Useful runner. An interactive agent still has no Make command that starts that stack: `make test-e2e` defaults to hcw-wip when no URL is supplied. Give agents an explicit route to the same isolated profile that IdeaForge uses.

## How

Add `make test-e2e-disposable` as an opt-in entrypoint to the existing Useful `run-checkout` command. Require a private allocation port and session ID. Use the operator-owned HealthVault synthetic profile and the Playwright image already used by the successful full-suite run. Pass the session ID as the default runner owner so a session that holds the worktree lease can snapshot its own checkout. Keep `make test-e2e` and deployed WIP behavior unchanged. The runner supplies the stack-local `BASE_URL`, retains evidence, and cleans up its own resources. This change does not add browser scenarios or change production, WIP, or the IdeaForge gate.

The first pair of parallel full-suite runs exposed timing races in three existing mocked food-edit tests: a delete request can occur before a late `waitForRequest` subscription, and the first PATCH can finish while the test is still clicking Confirm. A later parallel run exposed the same timer race in the adjacent navigation test. Make these existing tests wait for requests before the triggering action and hold mocked PATCH responses until the follow-up actions are ready. In the three queue-order tests, record mutation `fetch` calls synchronously in the browser to assert that follow-up calls have not started while a prior mock response is deliberately held; Playwright request events can arrive later than fetch initiation. The separate navigation test holds its final mocked response and checks that the page stays on review until release. The change stabilizes these existing tests without increasing timeouts or adding scenarios. The final code revision `d3b7bdfcca0935edc3358ff9a15f9714758b6e32` passed all 331 browser tests in disposable run `cd5a5ef81b2843fc923d320e7ded100f`. The run saved its logs and report, removed its containers and network, and left the same WIP backend and nginx containers running.

## Validation Commands

- `make -n test-e2e-disposable DISPOSABLE_HTTP_PORT=48162 DISPOSABLE_SESSION=example-session`
- `make test-e2e-disposable DISPOSABLE_HTTP_PORT=48162 DISPOSABLE_SESSION=example-session`
- `make lint-e2e`
- `git diff --check`

### Task 1: Expose the existing isolated full-suite runner

- [x] Add an explicit Make target with required port and session, synthetic profile, pinned Playwright image, and no WIP fallback.
- [x] Confirm the target runs the existing full suite and leaves saved evidence after owned resources are removed.
- [x] Run project checks and the Review Gate; fix valid findings.
- [x] Mark completed.

### Task 2: Remove observed timing races from existing mocked UI tests

- [x] Register delete-request waits before Confirm and hold the first mocked PATCH until the test is ready to release it.
- [x] Preserve browser-side fetch-order checks in the three queue tests and the held-response navigation check in the adjacent test.
- [x] Re-run the full disposable suite and verify parallel cleanup and WIP isolation.
- [x] Mark completed.
