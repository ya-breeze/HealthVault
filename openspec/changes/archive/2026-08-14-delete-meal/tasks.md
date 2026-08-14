## 1. Frontend: delete control on the review page

- [x] 1.1 In `frontend/app/food/review/ReviewClient.tsx`, add delete state (`confirmingDelete`, `deleting`, `deleteError`) and a delete affordance rendered regardless of `meal.status`.
- [x] 1.2 Wire the trash-icon → Confirm/Cancel inline flow, matching `MealItemRow.tsx`'s existing `confirmingDelete` pattern (Confirm disabled while `deleting`).
- [x] 1.3 On Confirm, call `api.deleteRecord('food_meal', mealId)`; on success show a success toast via `useToast` and `router.push('/food/history')`.
- [x] 1.4 On failure, show an inline error and remain in the confirm state (do not reset to the non-confirming state), matching the pattern in `design.md`.

## 2. E2E coverage

- [x] 2.1 Add a test case to the "Meal history" describe block in `e2e/tests/food.spec.ts`: create a manual meal, open its review page, delete it (confirm flow), assert navigation to `/food/history` and that the meal no longer appears there.
- [x] 2.2 Add a test case (or extend an existing mocked-UI describe block) covering the Cancel path: activate delete, click Cancel, assert the meal is unchanged and still present on reload.

## 3. Validation

- [x] 3.1 Run frontend lint/build (`make` target or `npm run build` under `frontend/`) and fix any issues.
- [x] 3.2 Run the new/updated Playwright specs against the deployed WIP stack per the project's E2E workflow.
- [ ] 3.3 Run `openspec validate --specs --strict` after archiving.

## 4. Code review fixes (round 2)

- [x] 4.1 `ClarifyModal`'s full-screen overlay hid the delete control entirely during `pending_clarification` — extracted `DeleteMealControl` and render a second copy inside the modal as an escape hatch.
- [x] 4.2 `handleDelete`'s one-time `await queueRef.current` snapshot missed mutations queued while that await was pending — replaced with `queueDelete`, which claims a queue slot the same way `applyMealUpdate` does.
- [x] 4.3 A 404 on delete (meal already gone) is now treated as success instead of an unrecoverable retry loop.
- [x] 4.4 Added regression coverage for all three: reachability inside the clarify modal, an edit queued after Confirm still ordering behind the delete, and 404-as-success.

## 5. Code review fixes (round 3)

- [x] 5.1 `queueDelete`'s returned promise only awaited the delete request itself, not the queue's tail — a trailing mutation queued right after Confirm (still in flight when the delete settled) kept running after `DeleteMealControl` had already navigated to `/food/history`, surfacing its error toast there. `queueDelete` now also awaits `queueRef.current` after its own request settles.
- [x] 5.2 The review page's own `DeleteMealControl` stayed mounted (invisible but still in tab order) behind `ClarifyModal`'s opaque backdrop, making its confirm/delete flow keyboard-reachable without the user seeing it. It's now unmounted whenever the modal is actually showing (same condition that gates the modal itself), leaving the modal's own copy as the only reachable instance.
- [x] 5.3 `handleDelete`'s 404-treated-as-success and plain-success branches left `deleting` stuck `true` if navigation were ever delayed or interrupted — both now reset it before returning, matching the explicit-error branch.
- [x] 5.4 Added regression coverage for 5.1 (navigation waits for a trailing mutation, asserted via a timing window before/after it settles) and 5.2 (exactly one "Delete meal" button present while the clarify modal is open).

## 6. Code review fixes (round 4)

- [x] 6.1 A mutation queued after Confirm and chained behind the delete is guaranteed to fail once the meal is gone (backend confirms: `PatchMealItem` 404s once its parent meal is hard-deleted) — its "Update failed" toast was real but confusing, surfacing right next to "Meal deleted" for a failure that's just fallout of the user's own action. `queueDelete` now sets a shared `mealGoneRef` before waiting out the queue's tail, and `applyMealUpdate`'s catch suppresses its toast when set.
- [x] 6.2 That same tail-wait had a gap: it only applied when the delete's own request resolved (204), not when it rejected with 404 (treated as success by `DeleteMealControl`) — so the round-3 fix never actually covered the 404-as-success path. `queueDelete` now waits out the tail for both outcomes before resolving/rejecting.
- [x] 6.3 Extracted the queue-slot-claiming mechanism duplicated (and independently bug-prone) across `applyMealUpdate` and `queueDelete` into one shared `claimSlot` primitive both now build on.
- [x] 6.4 Collapsed `DeleteMealControl`'s triplicated success-path statements (toast, navigate, reset `deleting`) down to one shared block reached by both the plain-success and 404-as-success paths.
- [x] 6.5 `openspec/specs.projected/` was never generated for this change across rounds 1–5, which the repo's CI drift check (`.github/workflows/openspec-projected-specs.yml`) treats as a hard failure for any PR touching `openspec/**`. Ran `make projected-specs` and committed the result.
- [x] 6.6 Fixed a wording contradiction in `specs/record-deletion/spec.md`: the pre-existing "Delete failure keeps the user on the review page" scenario didn't exclude 404 and so literally contradicted the newer "404 is treated as success" scenario when read in isolation.
- [x] 6.7 Updated the round-3 "navigation waits for a trailing mutation" e2e test to mock a realistic 404 for the trailing PATCH (matching actual backend behavior) instead of a 200, and assert no "Update failed" toast appears.

### Findings from round 4 explicitly not acted on (with reasoning)

- `MealItemRow.tsx` and `DataTypeClient.tsx` have a pre-existing bug where Cancel after a failed delete leaves the stale error message displayed (the same bug this change's round-2 fix addressed in `DeleteMealControl`, just never applied to the two components it says it mirrors). Real bug, but out of scope: both components predate this change and aren't touched by it. Left as a follow-up for a separate change rather than expanding this PR's scope.
- Generalizing `ClarifyModal`'s `deleteControl` prop into a reusable "stay reachable under any full-screen overlay" mechanism — this page has exactly one such overlay today; see design.md.
- The two `DeleteMealControl` mounts not sharing state across the modal/main-content swap — accepted risk, see design.md.
