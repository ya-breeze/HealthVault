## Why

Today the only way to learn that a logged meal still needs attention — it's still `processing`,
came back `pending_review` or `pending_clarification`, or `failed` analysis — is to open the meal
history page and scan status labels one by one. Nothing on the dashboard, the first page after
login, indicates that any meal needs a look. A meal can sit unconfirmed indefinitely without the
user noticing, which is also the meal's only path to final calorie/macro totals (`food-meal-history`'s
"Daily total sums only confirmed meals" scenario) — so an unnoticed pending meal quietly drops out
of the day's nutrition totals too.

## What Changes

- Extend `GET /api/food/meals` with an optional `status` query parameter accepting one or more
  status values (`processing`, `pending_review`, `pending_clarification`, `failed`, `confirmed`),
  so a caller can request only meals in specific statuses instead of paging the full history
  client-side to find them.
- Add a lightweight `GET /api/food/meals/needs-attention-count` endpoint returning a single count
  of the caller's own meals whose status is `processing`, `pending_review`, `pending_clarification`,
  or `failed` — the set that has no finished, confirmed totals yet. A lightweight endpoint is used
  instead of computing the count from the (potentially multi-page) filtered list, since the
  dashboard only ever needs the number, not the rows.
- Add a small "N meals need attention" indicator to the dashboard, visible only when the count is
  greater than zero, linking to `/food/history/`.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `food-meal-history`: `GET /api/food/meals` gains an optional `status` filter; a new
  `GET /api/food/meals/needs-attention-count` endpoint is added.
- `dashboard-ui`: the dashboard gains a conditional "needs attention" indicator sourced from the
  new count endpoint.

## Impact

- Backend: `backend/pkg/server/food_meal_detail.go` (`ListMeals`, plus a new handler for the count
  endpoint), `backend/pkg/server/server.go` (route registration).
- Frontend: `frontend/lib/api.ts` (`listMeals` gains a `status` option; a new API call for the
  count), `frontend/app/page.tsx` (dashboard indicator).
- No schema or migration changes — both endpoints read existing `FoodMeal` rows.
