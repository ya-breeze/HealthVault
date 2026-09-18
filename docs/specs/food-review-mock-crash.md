# Fix food.spec.ts "Fat (g)" locator collision with "Saturated fat (g)"

## Why

A prior session's `component-reference-shadow` fork reported 30/272 E2E tests failing on a clean
`main` checkout, with a React error boundary showing "This page couldn't load" on
`/food/review/?meal=mock-meal-id`. Re-running the full suite against a fresh `hcw-wip` deploy of
`main` (this branch, unmodified, before any fix) reproduces only **2** failures, both in
`food.spec.ts`, both a Playwright strict-mode violation — not a React crash and not a page-load
failure. No error boundary exists anywhere in this frontend (`frontend/app` and
`frontend/components` have no `error.tsx`/`global-error.tsx`/`ErrorBoundary`), so a genuine
render-crash claim doesn't match the codebase. The most likely explanation for the earlier report
is a stale or mid-deploy `hcw-wip` build at the time it ran (see
`project_hcw_wip_contention` in this environment's shared-stack notes) rather than a real,
persistent bug — this spec fixes the one failure mode that does reproduce and does not chase the
unreproduced one.

The real root cause: the saturated-fat-signal work (merged PR #80) added a "Saturated fat (g)"
labeled input to `ManualItemEditor.tsx` and `CustomFoodModal.tsx`, alongside the pre-existing
"Fat (g)" input. Two assertions in `food.spec.ts` (lines 883 and 968) locate the fat field with
`page.locator('label:has-text("Fat (g)") input')`. Playwright's `:has-text()` is a
case-insensitive substring match, and "Saturated fat (g)" contains the substring "fat (g)", so
the locator now resolves to two elements instead of one and Playwright's strict mode fails the
assertion — the exact failure reproduced by `make test-e2e -g 'shows sodium and fiber...'`
against `hcw-wip`.

## How

Change both locators from the ambiguous substring match to the exact-match form Playwright's own
error output already identifies as unambiguous:
`page.getByRole('spinbutton', { name: 'Fat (g)', exact: true })`. No application code changes —
this is a test-only fix, since the two labels coexisting is correct, intended UI (both fields are
real and both are meant to be independently editable).

Excluded: investigating the originally-reported 30-test/error-boundary failure further. It did
not reproduce against a freshly deployed `hcw-wip`, and inventing a fix for a symptom that isn't
present would be working from an unverified premise. If it recurs, the next session should first
rule out `hcw-wip` contention (another session's concurrent deploy) before assuming an app bug.

## Validation Commands

```
cd /data/HealthVault && make test-e2e BASE_URL=http://192.168.1.54:8892
```

### Task 1: Fix the locator collision

- [x] Change `food.spec.ts:883`'s locator from `page.locator('label:has-text("Fat (g)") input')`
      to `page.getByRole('spinbutton', { name: 'Fat (g)', exact: true })`
- [x] Change `food.spec.ts:968`'s locator the same way
- [x] Re-run the full E2E suite against `hcw-wip` and confirm 0 failures (272/272, minus the 1
      pre-existing skip)
- [x] Mark completed
