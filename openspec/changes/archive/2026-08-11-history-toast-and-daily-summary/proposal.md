## Why

The food-logging frontend gives no visual confirmation when a mutation (item add/edit/delete, meal name/date correction, reanalysis, confirmation) succeeds or fails beyond small inline text — easy to miss, especially on mobile. Separately, the meal history page is a flat chronological list with no day boundaries or daily nutrition totals, so a user checking "how did I do today/yesterday" has to add it up by hand.

## What Changes

- Add a toast/notification system (`ToastProvider` + `useToast()`) to the frontend, mounted app-wide.
- Wire toast notifications into the food review page's shared mutation path (`applyMealUpdate` in `ReviewClient.tsx`), so every successful or failed item add/edit/delete, meal name/date edit, reanalysis, and confirmation shows a toast. Existing inline error text is unchanged; the toast is additional feedback, not a replacement.
- Group the meal history list by calendar day (local time), with a per-day header and a per-day nutrition total (calories, protein, carbs, fat) summed over that day's `confirmed` meals only.
- Add `protein_grams`, `carbs_grams`, `fat_grams` to the `GET /api/food/meals` summary response so the frontend has the data needed for per-day totals without an extra round trip per meal.

## Capabilities

### New Capabilities
- `ui-notifications`: A reusable toast/notification system (provider, hook, rendering) and its use on the food review page to confirm or flag the outcome of every mutation issued there.

### Modified Capabilities
- `food-meal-history`: The `GET /api/food/meals` summary gains `protein_grams`, `carbs_grams`, `fat_grams`. The meal history page groups meals by calendar day and shows a per-day nutrition total for that day's confirmed meals.

## Impact

- Backend: `backend/pkg/server/food_meal_detail.go` (`MealSummary` struct, `ListMeals` handler) — additive fields only, no migration (source columns already exist on `FoodMeal`).
- Frontend: `frontend/app/layout.tsx` (mount `ToastProvider`), new `frontend/components/Toast.tsx` (or similar) + context/hook, `frontend/app/food/review/ReviewClient.tsx` (`applyMealUpdate` gains an optional label + toast firing), `frontend/app/food/history/page.tsx` (day grouping + totals), `frontend/lib/api.ts` (`MealSummary` interface gains the three macro fields).
- No breaking changes; both pieces are additive.
