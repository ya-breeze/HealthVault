## Why

Idea #9 (see `docs/investigations/idea-9-food-day-completeness-signal.md`) found, from
production data, that HealthVault's food log cannot tell a fully-logged day from a partially-logged
one: 616 kcal/day logged against a weight-trend-implied ~2,200 kcal/day true intake — roughly 65%
of intake never photographed. Phase 4 (ADR-004's Healthiness Label, and the food dashboard cards
it's paired with) reads that log as if it were complete. Adaptive TDEE (#10) is a harder-blocked
downstream consumer of the same gap. Both would silently compute a real-looking number from data
that is mostly absence, not signal.

A `/grilling` session on 2026-08-24 turned the finding into a design (recorded as a comment on the
idea-forge issue). This change proposes exactly that design: nothing here is a new decision, this
proposal transcribes an already-grilled design into OpenSpec deltas an implementer can build from.

## What Changes

- **Eating Occasion collapsing** — `FoodMeal` rows within 10 minutes of each other on the same
  Logged Day collapse into one Eating Occasion. This is what stops "3 meal rows, 2 of them 3
  minutes apart" from being counted as 3 separate eating events.
- **A `timezone` and a `usual_meals_per_day` key**, added to the existing opaque per-user settings
  blob (`user-settings` capability) — no schema change to that capability, just two new documented
  keys. `timezone` gives the backend a concept of "day" it currently does not have anywhere;
  `usual_meals_per_day` is the per-user auto-pass threshold (defaults to 3).
- **Day Completeness**, computed per completed Logged Day (never for today, which is in progress
  and never asked): zero occasions → **Incomplete** (silent, never prompted — the user certainly
  ate, they just didn't log, and a weekend away would otherwise produce a queue of unanswerable
  prompts); occasions ≥ the threshold → **Complete** (automatic); 1..threshold-1 occasions →
  **Unconfirmed** until the user says otherwise, or **Confirmed Complete** once they do. The
  heuristic only ever decides *whether to ask* — it never overrides what the user says, and an
  auto-Complete day is never asked and (in this change) cannot be downgraded by hand; see
  design.md's "Not doing" for why that's an accepted gap rather than an oversight.
- **A dedicated confirmation table**, not a metric type — the mark is a flag on a date (user +
  local date + confirmed-at), not a time-series measurement, and forcing it through the
  9-touch-point metric-type registry would surface a meaningless boolean in every chart/vitals
  surface built for that registry.
- **Two new backend endpoints**: `GET /api/food/completeness` (a date range → per-day occasion
  count + state, the primitive future rolling-window features query) and
  `POST`/`DELETE /api/food/completeness/{date}/confirm` (set/retract the user's assertion — the
  same control toggles it back off, and a later edit that adds a forgotten meal to an already-
  confirmed day leaves it confirmed *so long as occasions stay below `usual_meals_per_day`*; if
  the edit pushes the day to or past the threshold, it becomes automatically Complete instead,
  which is a strictly stronger state).
- **Inline confirmation control on the food history page only** — no dashboard banner, no
  notification, nothing that chases the user after logging a meal. The page's day-grouping key
  also switches from the browser's local timezone to the new stored `timezone` setting, falling
  back to UTC — a real, immediate behavior change for anyone on a non-UTC browser who hasn't set
  `timezone` yet, not a preservation of today's grouping. Accepted, because this is what resolves
  the existing disagreement between food history's browser-local day grouping
  (`frontend/app/food/history/page.tsx:44`) and the data API's UTC day bucketing
  (`backend/pkg/database/storage_impl.go:155`), at least for food: the two definitions already
  disagreed with each other, so replacing one arbitrary boundary with a consistent one is a net
  improvement even though it isn't behavior-preserving on day one. Setting `timezone` in the new
  settings panel restores a user's own local grouping immediately.
- **A downstream coverage contract**, specified now even though nothing consumes it yet: a
  rolling-7-day feature (ADR-004's Healthiness Label; Adaptive TDEE) SHALL count only Complete and
  Confirmed Complete days, and SHALL say "not enough data" below 3 valid days in the window rather
  than compute a plausible-looking number from too little. This is what makes today's under-13%
  auto-complete rate (see design.md's "Risks" section) an honest "not enough data" instead of a
  misleading average once Phase 4 is built on top of this.

## Not Changing

- **Phase 3** (#8, nutrition targets) — unaffected; it reads profile and weight, no intake input.
- **The general `/api/data/{type}` UTC day-bucketing** used by every other metric's charts
  (`storage_impl.go`) — only food's own day boundary moves to the stored timezone in this change.
  Migrating every chart to per-user-timezone bucketing is a separate, much larger change with its
  own trade-offs (see design.md's "Not doing").
- **No dashboard card, no LLM call, no TDEE computation** ships here — this change is purely the
  completeness signal and its storage/API/UI, which Phase 4 and #10 will each separately propose
  consuming.

## Capabilities

### New Capabilities
- `food-day-completeness`: Eating Occasion collapsing, Day Completeness states, the completeness
  range-query and confirm/unconfirm API, and the downstream coverage contract.

### Modified Capabilities
- `user-settings`: documents the new `timezone` and `usual_meals_per_day` keys in the opaque
  settings blob.
- `food-meal-history`: the Meal History Page's day-grouping key moves from browser-local to the
  stored `timezone` setting, and each day section gains a completeness badge/control per the new
  capability's states.
- `data-model`: the "Food logging tables" requirement's closed enumeration of five tables gains a
  sixth, `FoodDayCompletion`, since it lives in the same `models_food.go` file and is migrated
  alongside the other five.

## Impact

- Backend (Go): `backend/pkg/database/models_food.go` (new `FoodDayCompletion` struct),
  `backend/pkg/database/db.go` (`AutoMigrate` entry), a new occasion-collapsing pure function and
  day-completeness computation (package TBD in design.md), `backend/pkg/server/server.go` (two new
  routes), a new food-completeness handler file alongside `backend/pkg/server/food_meal_detail.go`.
- Frontend (TS/React): `frontend/lib/api.ts` (`UserSettings` keys, new completeness API client
  calls), `frontend/app/food/history/page.tsx` (timezone-aware grouping key, per-day completeness
  badge/control, a settings panel for `timezone`/`usual_meals_per_day`).
- Docs: `CONTEXT.md` (new glossary entries — Eating Occasion, Logged Day, Usual Meals Per Day, Day
  Completeness), new `docs/adr/ADR-006-*.md`, `todo.md` (record this as claimed / done alongside
  the other phases).
- No implementation code ships in this Attempt — see the repo's `openspec/` workflow; this is the
  proposal an implementer's own pass will build from.
