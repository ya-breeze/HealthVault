## Context

`GET /api/food/meals` (`backend/pkg/server/food_meal_detail.go`, `ListMeals`) already returns the
caller's own meals of any status, ordered `logged_at DESC, id DESC`, with `limit`/`before`/
`before_id` keyset paging. The frontend history page (`app/food/history/page.tsx`) is its only
consumer today. There is no dashboard-facing summary and no status filter — a caller that wants
"how many meals still need attention" has no option but to page through the entire history and
count client-side, which is both slow (multiple round trips for a long history) and wrong once
the history exceeds one page fetched by the dashboard on load.

`database.MealStatus{Processing,PendingClarification,PendingReview,Confirmed,Failed}` are the five
existing status constants (`backend/pkg/database/models_food.go`). "Needs attention" is everything
except `confirmed` — a meal in any of the other four states has no finished nutrition totals yet
and either needs the user to look at it (`pending_review`, `pending_clarification`, `failed`) or is
still in flight (`processing`).

## Goals / Non-Goals

**Goals:**
- Let the dashboard show a real-time count of meals needing attention with a single cheap request.
- Let any future caller filter the meal list by status without paging the full history.

**Non-Goals:**
- No push/websocket updates — the count is fetched on dashboard load like every other vital, not
  live-updated while the tab is open.
- No per-status breakdown on the dashboard (e.g. "2 processing, 1 failed") — a single combined
  count and a link into history, where the existing status labels already disambiguate.
- No change to how meal status transitions or is computed — this only adds read paths.

## Decisions

**Two separate endpoint additions, not one.** `status` filter on `GET /api/food/meals` and a
dedicated `GET /api/food/meals/needs-attention-count` endpoint solve different problems and are
both cheap to add:
- The `status` filter (repeatable query param, `?status=processing&status=failed`) is a natural,
  minimal extension of the existing list endpoint's query surface — it reuses the same `WHERE`
  builder and keyset paging, so a caller who *does* want the rows (not just a count) for a specific
  status subset gets an efficient, page-able list instead of over-fetching and filtering
  client-side.
- The count is intentionally its own endpoint rather than `len(list with status filter)`, because
  the dashboard needs a single accurate number regardless of history size, and computing that via
  the list endpoint would require either an unbounded `limit` (defeats the point of a keyset-paged
  list) or multiple round trips to page through and sum. A `SELECT COUNT(*) ... WHERE status IN
  (...)` is one query, one row, no pagination concerns.

**Count endpoint takes no parameters — the "needs attention" status set is fixed server-side**
(`processing`, `pending_clarification`, `pending_review`, `failed`), not caller-specified. This
endpoint has exactly one consumer (the dashboard indicator) and one meaning; a generic
`?status=` count endpoint would be speculative generality for a need that doesn't exist yet. If a
second consumer needs a different status set later, extend then.

**Response shape:** `{"count": <int>}` — matches the single-purpose nature of the endpoint; no
need for the fuller `MealSummary` list shape.

**Status filter validation:** an unrecognized `status` value returns 400, same treatment as an
invalid `limit`/`before`/`before_id` today — consistent with the existing endpoint's "reject,
don't silently ignore" behavior.

**Dashboard placement:** the indicator renders as its own compact banner/pill above the "Log food"
section (not inside the vitals grid, which is fixed to the 8 primary health metrics per
`dashboard-ui`'s existing "Dashboard vitals grid" requirement, and a meal-attention count isn't a
vital). It is omitted entirely when the count is 0, so a user with nothing pending sees no new UI —
consistent with `VitalCard`'s existing "no data" pattern of showing state only when there's
something to show.

## Risks / Trade-offs

[Two extra queries on every dashboard load (vitals + this count)] → Mitigation: the count query is
a single indexed `WHERE user_id = ? AND status IN (...)` aggregate, negligible next to the 8
parallel vitals queries already issued on dashboard load.

[Count can go stale while the dashboard tab stays open and a meal transitions status in the
background — e.g. `processing` → `pending_review` via the async analysis path] → Mitigation:
same staleness profile as every other value on the dashboard today (vitals, sparklines); no
existing dashboard data auto-refreshes. Out of scope per Non-Goals; the count is correct as of
page load.

## Open Questions

None — scope is narrow enough that implementation can proceed directly from this design.
