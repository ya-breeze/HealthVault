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
- **Confirm pattern**: mirrors `MealItemRow`'s existing `confirmingDelete` boolean + Confirm/Cancel buttons, rather than a browser `confirm()` dialog or a modal, for visual consistency with the rest of the page.
- **On success**: show a toast ("Meal deleted") via the existing `useToast` hook already used on this page, then `router.push('/food/history')` — there is nothing left to show on the review page once the meal is gone.
- **On failure**: show an inline error (mirroring the page's existing `actionError` pattern) and leave the confirm state active so the user can retry, rather than silently reverting to the non-confirming state.

## Risks / Trade-offs

- [Risk] A user could delete a meal mid-analysis (`processing` or `pending_clarification`) before it's fully reviewed. → Accepted: the backend already allows this (no status gate on delete), and it's a reasonable thing to want (e.g. an accidental upload). No mitigation needed beyond the existing confirm step.
- [Risk] Double-submit if the user double-clicks Confirm. → Mitigated the same way `MealItemRow` already does it: a `deleting` boolean disables the Confirm button while the request is in flight.
