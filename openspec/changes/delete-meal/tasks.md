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
