## Context

The food review page (`frontend/app/food/review/ReviewClient.tsx`) already funnels every mutation — item add/edit/delete, meal name/date edit, reanalyze, retry, clarify, confirm — through one function, `applyMealUpdate`, which exists to serialize *issuing* requests so issue order matches commit order (see its doc comment). It is the single point every mutating child calls back into. There is currently no toast/snackbar system anywhere in the frontend (confirmed by grep across `frontend/app`, `frontend/components`, `frontend/lib`); all feedback is inline text rendered by each component.

The meal history page (`frontend/app/food/history/page.tsx`) renders `GET /api/food/meals` results as a flat list, sorted `logged_at DESC`, keyset-paginated 50 at a time via `before`/`before_id`. `MealSummary` (backend and frontend) currently exposes only `id`, `name`, `logged_at`, `status`, `calories` — no macros.

## Goals / Non-Goals

**Goals:**
- Give the user visual (not just inline-text) confirmation that a food-logging edit succeeded or failed.
- Let the user see, at a glance, day boundaries and daily nutrition totals in their meal history.

**Non-Goals:**
- A general-purpose toast system used beyond the food review page. The provider/hook is app-wide infrastructure (mounted in `app/layout.tsx` so it's available anywhere), but wiring it into *triggers* is scoped to `ReviewClient.tsx`'s mutation path only — no other page's flows are touched in this change.
- Editable/adjustable daily totals, goals, or streaks — this change only sums and displays.
- Per-day totals for non-confirmed meals. Only `confirmed` meals have final nutrition numbers (see existing "Unconfirmed meal shows no calorie total" behavior); a day total including provisional numbers would be misleading and would change as a meal's status changes.
- Server-side day aggregation. Grouping is client-side over the already-fetched page(s), consistent with how the list is already rendered client-side.

## Decisions

**Toast state lives in a React context (`ToastProvider`), mounted once in `app/layout.tsx`.** Alternative considered: a local `useState` inside `ReviewClient.tsx` rendering its own toast stack. Rejected because it would only work on that one page and the proposal frames this as reusable app-wide infrastructure; a context costs little more and avoids re-deriving the same stack/timer logic if a second page wants toasts later.

**`applyMealUpdate` gains an optional second parameter, a short label string, rather than each mutating child calling `useToast()` itself.** `applyMealUpdate` is already the one place that knows whether a mutation resolved or rejected — pushing toast-firing there means each call site adds one string argument instead of importing a hook and duplicating success/failure handling five times (`MealItemRow`, `AddItemForm`, `MealMetaEditor`, `ReanalyzeControl`, plus the inline retry/clarify/confirm calls in `ReviewClient.tsx` itself). A call site that omits the label gets a generic "Saved" toast on success; failures always get a generic "Update failed" (or the thrown error's message where it's a plain string) regardless of label, since the existing inline error text already carries the specific detail.

**Toasts are additive, not a replacement for inline errors.** Existing inline `<p className="text-red-600">{error}</p>` blocks stay exactly as they are. Removing them would lose error text that's still visible after a toast auto-dismisses (~3s) — the toast is a transient nudge, the inline text is the durable record for that field.

**Day grouping happens client-side, keyed by the meal's local calendar date (`new Date(logged_at).toLocaleDateString()`-equivalent day boundary), not UTC.** This matches how the existing row already displays `logged_at` (`.toLocaleString()`), so a meal never appears to "jump days" between the row's own timestamp and the header it's grouped under. Because the list is already ordered `logged_at DESC`, grouping is a single linear pass — no sorting or backend change needed for the grouping itself.

**Daily totals sum only `confirmed`-status meals, using the three new macro fields added to `MealSummary`.** Rejected alternative: fetch full `FoodMeal` (with items) per meal to compute totals — unnecessary, since `FoodMeal` already carries aggregate `calories`/`protein_grams`/`carbs_grams`/`fat_grams` (computed at confirm time, per `food-nutrition-logging`'s "Meal Aggregate Totals on Confirmation" requirement); the list endpoint just needs to expose the three it currently omits.

**Backend change is additive-only:** three new `float64` fields on `MealSummary` and its `ListMeals` mapping, sourced from columns that already exist on `FoodMeal` (`models_food.go:52-54`). No migration, no new query.

## Risks / Trade-offs

- **A day's total can change after page load if the user (re)confirms or edits a meal that lands in an already-rendered day.** Mitigation: the history page already reloads its full state on mutation-free navigation; this is consistent with existing behavior where the list itself doesn't live-update either — acceptable since history is a read-mostly view reached by navigating in, not a page a user watches while editing.
- **A day's total silently omits non-confirmed meals with no indicator on the total line itself.** Mitigation: the day header still lists every meal (confirmed or not) underneath, same as today, so a pending meal is visible — only the summed number excludes it. This matches the existing per-row behavior (blank calories for non-confirmed) and needs no new UI language beyond what's already established.
- **Generic "Saved" toast on unlabeled success could feel low-value if a call site is missed during implementation.** Mitigation: `tasks.md` enumerates every existing `applyMealUpdate` call site explicitly so none are missed silently; a missed label degrades to "Saved" rather than to no toast, so the failure mode is cosmetic, not a silent gap in coverage.

## Migration Plan

No data migration. Deploy is a normal frontend + backend release:
1. Backend: add the three fields (safe to deploy first; frontend ignores unknown-to-it fields until it's updated, and old frontends already ignore fields they don't declare).
2. Frontend: `ToastProvider` in `layout.tsx`, `applyMealUpdate` label wiring, history page grouping.

No feature flag needed — both changes are purely additive UI/response-shape changes with no behavior change for existing callers of `GET /api/food/meals` (extra JSON fields are backward-compatible).

## Open Questions

None outstanding — scope was confirmed with the user (toast on all successful/failed mutations; day totals include calories + macros; confirmed-status meals only).
