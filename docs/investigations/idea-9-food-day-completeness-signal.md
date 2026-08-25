# Investigation: Food log completeness signal (Phase 4 prerequisite)

Research notes for idea-forge plan
`idea-9-healthvault-food-log-completeness-signal-investigate.md`. Filed as a
Phase 4 (ADR-004) prerequisite finding, then turned into a design via a
`/grilling` session on 2026-08-24 (recorded as a comment on the idea-forge
issue, quoted in full inside the plan file).

## Outcome

This investigation concluded in a proposed OpenSpec change,
`openspec/changes/food-day-completeness/` (`proposal.md`, `design.md`, spec
deltas for a new `food-day-completeness` capability plus modifications to
`user-settings` and `food-meal-history`, and an implementer `tasks.md` with
every box left unticked). `openspec validate food-day-completeness --strict`
passes. No production code was written or wired up — per this repo's
spec-first workflow, that is deliberately out of scope for this Attempt and
is left for a later, separate implementation pass against the approved spec.

The chosen design: completeness is a **user assertion**, gated by a
heuristic that only decides *when to ask*, never what the answer is.
`FoodMeal` rows within 10 minutes collapse into one **Eating Occasion**;
a completed Logged Day is **Complete** (auto, ≥ threshold occasions),
**Unconfirmed**/**Confirmed Complete** (1..threshold-1 occasions, before/
after the user confirms), or **Incomplete** (0 occasions, silent, never
prompted). The threshold is a new per-user `usual_meals_per_day` setting
(default 3); a new per-user `timezone` setting gives the backend its first
concept of "day" at all. Storage is a dedicated confirmation table (user +
local date + confirmed-at), not the metric-type registry. Two new endpoints
(`GET /api/food/completeness`, `POST`/`DELETE .../confirm`) and an inline
control on the food history page are the only UI surface — no dashboard
banner, no notification. Downstream rolling-window features (ADR-004,
Adaptive TDEE/#10) must count only Complete/Confirmed-Complete days and say
"not enough data" below a 3-of-7 floor rather than compute a plausible
number from too little.

## Relevant code

- `backend/pkg/database/storage_impl.go:155` — `/api/data/{type}` buckets
  by **UTC** day (`strftime('%Y-%m-%dT00:00:00Z')`). This is one of the two
  conflicting "day" definitions found; the design leaves it as-is (see
  "Not doing" below) and only changes food history's own grouping.
- `frontend/app/food/history/page.tsx:44` — food history groups meals by
  **browser-local** day (`getFullYear/getMonth/getDate`). This is the other
  conflicting definition, and the one the design changes: it moves to a
  server-computed `Intl.DateTimeFormat('en-CA', { timeZone: tz })` key using
  the new stored `timezone` setting.
- `backend/pkg/database/models_food.go` — where the new `FoodDayCompletion`
  struct lands (design.md's "Storage" section has the concrete GORM tags:
  `user_id` + `local_date` unique index, `confirmed_at`).
- User settings blob (`user-settings` capability, consumed via
  `frontend/lib/api.ts`'s `updateSettings` read-modify-write helper) —
  currently holds only `dashboard_order` and `display_language`; the design
  adds `timezone` and `usual_meals_per_day` as two more opaque keys, no
  schema change to the capability itself.
- `backend/pkg/server/food_meal_detail.go` — sibling food-domain handler
  file; the new completeness handlers land alongside it per the proposal's
  "Impact" section.
- `backend/pkg/server/server.go` (lines 82-116 per the idea-6 investigation
  above) — router registration pattern the two new routes follow.
- `FoodMeal.LoggedAt` and `Status` — the occasion-collapsing input. All
  statuses count (including `failed`/`processing`), since a logging
  *attempt* is still an eating event even before nutrition is confirmed.

## Constraints and unknowns

- **No timezone exists anywhere in the schema today.** `user_settings`
  holds only `dashboard_order` and `display_language`. Every downstream
  decision (Eating Occasion boundaries, "today" exclusion, the Logged Day
  key) depends on this being added first — it's the one piece of new state
  every other decision sits on top of.
- **Two live, conflicting day definitions**, confirmed by reading both call
  sites (not inferred): UTC in the general data API, browser-local in food
  history. The discrepancy is currently invisible only because all 24
  production meals fall inside 05:00–17:59 UTC — no meal has ever crossed
  either boundary. The design resolves this for food only; the general
  `/api/data/{type}` UTC bucketing used by every other metric's charts is
  explicitly out of scope (see design.md "Not doing" — re-bucketing 25
  other metric types by a timezone that can change mid-history is its own
  change with its own trade-offs).
- **Meal count is not occasion count.** Verified against real data: on
  2026-08-21, two of three meal rows are 3 minutes apart (14:39, 14:42) —
  one sitting logged as two rows. A naive `≥3 meals` rule passes that day
  by, in effect, double-counting a single occasion. The 10-minute collapse
  window is what fixes this; no precedent for the exact window existed in
  the codebase, so 10 minutes is a design choice, not a measured optimum.
- **Whole-missing days vs. partly-logged days are different failure
  modes**, and conflating them was the original design's blind spot: 5 of
  17 days have zero entries. A zero-entry day is never prompted (the user
  certainly ate; asking would produce an unanswerable queue after any
  weekend away) — only a 1..threshold-1-occasion day gets a control.
- **Retroactivity of the threshold** (open question #3 from the grilling
  comment): settled in design.md by reading `usual_meals_per_day` fresh on
  every computation rather than snapshotting it at confirm time, matching
  how BMI bands and the weight-trend projection already treat their own
  inputs elsewhere in the app — one consistent mental model for "does
  changing a setting rewrite history," rather than a new one-off rule.
- **Whether an auto-Complete day can be downgraded by hand** (open question
  #1): settled as **no, not in this change** — an auto-Complete day exposes
  no control at all, in either direction, which is what "the heuristic
  decides only when to ask" means by construction. Accepted as a known gap;
  nothing in the 17-day sample (2/17 auto-Complete at threshold 3) suggests
  it's urgent to solve now.
- **The minimum coverage floor** (open question #2): settled as 3-of-7,
  reusing the number the grilling comment's own "Accepted consequence"
  paragraph already used as an example, so every future consumer (ADR-004,
  #10) inherits one number instead of independently re-deriving or
  disagreeing on it.
- **A `timezone` change deletes existing confirmations, rather than leaving
  them pinned.** `LocalDate` is computed once at confirm time against
  whatever `timezone` was current then, while the occasion count behind it
  is recomputed fresh on every read using the *current* `timezone`. Leaving
  a stored confirmation in place after a `timezone` change would silently
  match it against a different set of meals than the ones the user actually
  reviewed — the same date string means a different 24-hour window under
  the old and new zone. So changing `timezone` SHALL delete all of that
  user's existing confirmations instead, per design.md's "Storage" and
  "Risks" sections — a visible, bounded cost (re-confirm a handful of days)
  rather than a confirmation silently misattaching itself to the wrong
  day's data.
- **API scoping**: `GET /api/food/completeness` and the confirm endpoints
  are scoped strictly to `claims.UserID`, no `?user=` family-member
  override — this is a personal assertion, not shared family data,
  following the manual-write endpoint's convention from ADR-005 rather than
  `DataHandler`/`summaryHandler`'s `resolveUser` pattern used elsewhere.

## Findings write-up

### What was found

- The production evidence (616 kcal/day logged vs. ~2,200 kcal/day
  weight-trend-implied intake, ~65% of intake never photographed) holds up,
  but three corrections surfaced only during grilling and are not visible
  from the raw 14-day table in the original finding: the habit is 17 days
  old (not established), 100% photo / 0% manual despite a manual path
  existing, and meal count alone is a weaker signal than it first appears
  because of same-sitting double-counting.
- A boolean/heuristic completeness flag (the originally-proposed approach)
  was rejected during grilling in favor of a user-assertion model gated by
  a heuristic that only decides when to ask. This is a materially different
  shape than what the finding originally sketched, and it changes the
  downstream contract from "trust a computed flag" to "trust the flag only
  when confirmed, otherwise show reduced coverage."
- No existing table, endpoint, or settings key covers any part of this —
  genuinely new surface area on both backend and frontend, consistent with
  how idea-6's presence endpoint was also greenfield.
- The two pre-existing, silently-conflicting day-boundary definitions
  (UTC in the data API, browser-local in food history) were confirmed by
  reading both call sites directly, not inferred from documentation — this
  is real, live behavior, currently masked only by the small sample of
  production data never crossing either boundary.

### Limitations of this investigation

- **No shipped/wired-up code was written.** Per this repo's spec-first
  workflow (`/data/CLAUDE.md`, "OpenSpec — MANDATORY"), implementing the
  new table, endpoints, and frontend surface requires the OpenSpec propose
  → approve → implement cycle, out of scope for an investigate-only
  idea-forge plan.
- **The 10-minute Eating Occasion window is a design choice, not a measured
  one.** It resolves the one trap case found in the 17-day sample
  (2026-08-21's two rows 3 minutes apart), but no sensitivity analysis was
  done against a larger or different eating-pattern dataset — a user with
  habitually long meals (bar tab across 40 minutes with two separate
  orders) could still be miscounted either direction.
- **The design rests on an explicit, named, unverified assumption**: that
  photo capture stays the primary logging path and the user's logging
  habit matures over time, so the low current auto-complete rate
  (2/17 at threshold 3, see design.md's "Risks" section) self-corrects rather than
  permanently starving Phase 4 of usable days. This investigation did not
  and could not test whether that assumption holds — it can only be
  observed over the following weeks of real usage.
- **Locale copy for the new UI (confirmation control, settings panel) was
  not drafted.** The proposal names the file (`food/history/page.tsx`) and
  the two new settings fields but leaves exact English/Russian strings to
  the implementation pass, consistent with how idea-6's write-up also left
  locale strings undrafted.
- **No benchmark or load consideration for the new endpoints** — at this
  project's personal-deployment scale (`/data/CLAUDE.md`, "Scale") this is
  not expected to matter, but it was reasoned about, not measured.

### Suggested next steps

1. Implement against `openspec/changes/food-day-completeness/tasks.md` in
   order: the `timezone`/`usual_meals_per_day` settings keys and the
   occasion-collapsing pure function first (design.md decisions #1-#2),
   since every other piece — the completeness computation, the storage
   table, both endpoints, and the frontend grouping key — depends on those
   two existing.
2. When implementing the frontend day-grouping change
   (`food/history/page.tsx:44`), verify against a manually-constructed test
   case that spans a real timezone offset (not just UTC), since the
   production dataset never exercises a day boundary crossing today —
   the existing UTC-vs-browser-local discrepancy is real but currently
   silent, and it would be easy to ship a fix that looks correct against
   this data and is still wrong at a boundary.
3. Once shipped, watch the auto-complete rate over the following weeks
   against the assumption named above (photo logging habit matures) —
   if it doesn't rise, that's a signal the heuristic-gated-assertion design
   itself needs revisiting, not just the threshold value.
4. Phase 4 (ADR-004) and Adaptive TDEE (#10) should each consume the
   `GET /api/food/completeness` range endpoint and the 3-of-7 coverage
   floor directly rather than re-deriving their own completeness logic —
   both are named as blocked consumers in the proposal's "Why" section.
5. Revisit open question #1 (auto-Complete override) only if real usage
   shows false-positive Complete days are common — design.md deliberately
   leaves this unsolved rather than inventing a mechanism the 17-day sample
   gives no evidence is needed yet.
