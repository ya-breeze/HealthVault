## Context

This is a small, frontend-only change with no new architecture, dependency, data model, or migration involved — the backend delete path (`DELETE /api/data/food_meal/{id}`, cascade to items + photo) already exists and is already specified under `record-deletion` and `food-nutrition-logging`. The only open questions are UI placement and interaction details, captured below.

## Goals / Non-Goals

**Goals:**
- Let a user delete a meal from the meal review page (`/food/review/?meal={id}`), the screen they already land on from meal history.
- Reuse the existing confirm-then-delete interaction shape (trash icon → inline Confirm/Cancel) already used for meal-item deletion (`MealItemRow.tsx`) and the generic data-table row delete (`DataTypeClient.tsx`), so the new control looks and behaves like something already in the app rather than a new pattern.
- Reuse the existing `api.deleteRecord('food_meal', id)` helper — no new API client method, no backend change.

**Non-Goals:**
- No quick-delete affordance on the meal history list itself — one entry point (the review page) is enough; adding a second on the list is unnecessary surface area for this change and can be proposed separately if it turns out to be wanted.
- No bulk/multi-meal delete.
- No soft-delete/undo — this reuses the existing hard-delete endpoint as-is.

## Decisions

- **Placement**: the delete control lives at the bottom of the review page's main content (after the meal's status-specific content, alongside where `actionError` is shown), available regardless of meal status (`processing`, `pending_clarification`, `pending_review`, `confirmed`, `failed`) — matching the backend, which has no status restriction on delete (unlike item edits, which are status-gated).
- **Shared component**: the delete control is its own component (`DeleteMealControl`), not inline JSX, because it needs to render in two places — see "Reachability during `pending_clarification`" below.
- **Confirm pattern**: mirrors `MealItemRow`'s existing `confirmingDelete` boolean + Confirm/Cancel buttons, rather than a browser `confirm()` dialog or a modal, for visual consistency with the rest of the page.
- **On success**: show a toast ("Meal deleted") via the existing `useToast` hook already used on this page, then `router.push('/food/history')` — there is nothing left to show on the review page once the meal is gone.
- **On failure**: show an inline error (mirroring the page's existing `actionError` pattern) and leave the confirm state active so the user can retry, rather than silently reverting to the non-confirming state. A 404 (meal already gone, e.g. deleted from another tab) is treated as success instead — a generic error would leave the user stuck retrying a delete that can never succeed.
- **On cancel**: clear any prior delete error along with leaving the confirm state, so returning to the non-confirming view doesn't leave a stale error message displayed.
- **Ordering vs. other mutations**: `DeleteMealControl` doesn't route through `applyMealUpdate`'s queue (there's no resulting `FoodMeal` to reconcile, and the page navigates away on success), but it does claim a slot in `ReviewClient`'s mutation queue via a sibling helper (`queueDelete`) before issuing the delete request — same "queue *issuing*, not just resolution" mechanism `applyMealUpdate` itself uses, not a one-time `await queueRef.current` snapshot. A snapshot only covers mutations already queued at the moment Confirm is clicked; claiming a slot also covers anything queued *while* that wait is still pending (e.g. an item edit committed after Confirm but before an earlier one finishes), since it chains behind the delete's own promise instead of behind the stale snapshot. `queueDelete`'s returned promise additionally waits out `queueRef.current` *after* the delete's own request settles (not just the request itself), so `DeleteMealControl` doesn't navigate away while a mutation chained behind the delete is still in flight — that mutation is guaranteed to fail once the meal is gone, and navigating before it settles let its error toast surface confusingly after "Meal deleted", on `/food/history`.
- **Reachability during `pending_clarification`**: `ClarifyModal` renders a `fixed inset-0` backdrop that fully covers the rest of the page, so the review page's own delete control is present but unreachable while it's open — exactly the status this change's own accepted risk (below) calls out as a case someone would want to delete from. `ClarifyModal` takes an optional `deleteControl` node and renders a second `DeleteMealControl` instance inside itself as an escape hatch, so deleting doesn't require answering questions the user may not want to answer. The review page's own copy is unmounted (not just visually covered) whenever the modal is showing — an invisible-but-mounted copy behind an opaque backdrop is still in DOM/tab order, so a keyboard user tabbing past the backdrop could otherwise reach and activate a delete confirm flow they never saw. Gated on the same condition that decides whether the modal itself renders (`pending_clarification` status *and* pending questions exist), not on bare status, so a `pending_clarification` meal with no pending questions (the modal doesn't render then) still has a reachable copy.

## Risks / Trade-offs

- [Risk] A user could delete a meal mid-analysis (`processing` or `pending_clarification`) before it's fully reviewed. → Accepted: the backend already allows this (no status gate on delete), and it's a reasonable thing to want (e.g. an accidental upload). Mitigated by the confirm step, plus (see above) `ClarifyModal` rendering its own reachable copy of the control so this case doesn't regress into "no way to delete without answering."
- [Risk] Double-submit if the user double-clicks Confirm. → Mitigated the same way `MealItemRow` already does it: a `deleting` boolean disables the Confirm button while the request is in flight.
- [Risk] A mutation queued via `applyMealUpdate` completing after the meal is deleted. → Mitigated by `queueDelete` claiming a queue slot before issuing the delete request, and by its returned promise also waiting for that slot's full tail to settle before `DeleteMealControl` navigates away, covering mutations queued before, during, and immediately after the delete's own request.
