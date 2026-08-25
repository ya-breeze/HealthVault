## Context

Full background, evidence, and the grilled decision this change transcribes live on the idea-forge
issue for idea #9 (the "Grilled" comment, 2026-08-24). The short version: 17 days of real food-log
history, 24 meals, zero manual entries, five zero-entry days, and a `≥3 meals` heuristic that a
single sitting can pass twice over (two rows 3 minutes apart). The grilling session's conclusion —
"completeness is a user assertion, gated by a heuristic that decides only *when to ask*" — is the
one thing this design does not re-litigate. What follows settles the three questions the idea
explicitly left open for `opsx:propose`, plus the concrete shapes (types, endpoints, algorithms)
needed to build it.

## Goals / Non-Goals

**Goals:**
- Give the backend a concept of "day" it currently doesn't have (no timezone is stored anywhere).
- Turn "meal count" into "Eating Occasion count" so a single sitting logged as 2-3 rows isn't
  double-counted.
- Let a user assert a partially-logged day is actually complete, and let them retract that.
- Expose a queryable per-day completeness signal a future rolling-window feature (Phase 4, #10)
  can consume without recomputing occasion-collapsing itself.
- Settle the three explicitly-open questions from the grilling comment (below).

**Non-Goals:**
- No dashboard card, no chat/LLM surface, no TDEE math — those are separate proposals that will
  consume this signal.
- No change to the general `/api/data/{type}` UTC day-bucketing used by every other metric's
  charts — see "Not doing" below.
- No retroactive backfill/confirmation UI for the 5 existing zero-entry days — they're Incomplete
  by design and stay silent; nothing prompts for them.

## Decisions

### 1. Eating Occasion collapsing

A pure function over a day's `FoodMeal.LoggedAt` timestamps (all statuses included — a `failed` or
still-`processing` meal still represents a logging attempt, i.e. an eating event, even before its
nutrition is confirmed): sort ascending, start a new occasion whenever the gap to the previous
timestamp exceeds 10 minutes, otherwise merge into the current occasion. Occasion count = number of
resulting groups. This is the fix for the doc's own trap case: 2026-08-21's three rows at
09:1x/14:39/14:42 become 2 occasions, not 3.

### 2. Local Day boundary

A new `timezone` key in the existing opaque `UserSettings` JSON blob (IANA zone name, e.g.
`Europe/Warsaw`). Absent, empty, or a value `time.LoadLocation` rejects is treated as `UTC` — fail
open, matching the "presence" endpoint's precedent (`hide-unrecorded-data-types`'s design.md) of
never hard-failing a read over a malformed per-user preference. A meal's Logged Day is its
`LoggedAt` converted into that zone and formatted `YYYY-MM-DD`. "Today" (excluded from every
completeness computation) is `now()` converted the same way.

**Not doing**: migrating the general `/api/data/{type}` UTC day-bucketing (`storage_impl.go`,
used by every chart's Week/Month/Year aggregation) to the same per-user timezone. The grilling
comment names that inconsistency as evidence, but this change's job is food's day boundary — every
other metric's bucketing is unaffected. Revisiting that is its own change if it's ever wanted, with
its own trade-off about whether re-bucketing 25 other metric types by a timezone that can change
mid-history is worth it.

### 3. Day Completeness states

For a completed Logged Day (`local_date < today` in the user's zone):

| Occasions | Confirmed? | State |
|---|---|---|
| 0 | — | **Incomplete** |
| 1..threshold-1 | no | **Unconfirmed** |
| 1..threshold-1 | yes | **Confirmed Complete** |
| ≥ threshold | (irrelevant) | **Complete** |

`threshold` = the user's `usual_meals_per_day` setting (positive integer; absent/invalid → 3).
Today has no state — it's excluded from range results entirely, not returned as some 5th value.

**Settling open question #3 (retroactivity)**: `threshold` is read fresh on every computation, not
snapshotted at confirm time. Changing "how many times a day I usually eat" immediately re-evaluates
every past day's auto-Complete classification. This matches how BMI bands and the weight trend
projection already treat their own inputs (recomputed on read, not memoized at write time) — one
mental model for "does changing a setting rewrite history" across the app, and it's simpler to
implement than tracking a threshold-as-of per day.

**Settling open question #1 (can an auto-Complete day be downgraded?)**: **No, not in this change.**
An auto-Complete day exposes no control — full stop, matching "the heuristic decides only *when to
ask*," which by construction means a passing day is never asked, in either direction. A user who
logged 3 occasions but knows they missed a large dinner has no way to flag it here. Accepted as a
known gap, not solved by inventing an override that would blur "heuristic decides when to ask" into
"heuristic decides when to ask, except when it's wrong, which the user judges" — a materially
different, unbuilt feature. Revisit only if false-positive Complete days turn out to be common
enough to matter; nothing in the 17-day sample (2/17 auto-Complete at threshold 3) suggests urgency.

**Settling open question #2 (minimum coverage floor)**: **3 of the most recent 7 calendar days**
must be Complete or Confirmed Complete before a rolling-7-day downstream feature computes and shows
a number; below that, it states "not enough data" instead (see the new capability's "Downstream
Coverage Contract" requirement). Chosen because it's the exact number already used as an example in
the grilling comment's own "Accepted consequence" paragraph, so codifying it rather than leaving a
placeholder avoids every future consumer (ADR-004, #10) re-deriving or disagreeing on the same
number independently.

### 4. Storage

New table, not a metric type — the grilling comment already settled this ("does not belong in the
metric-type registry, which would duplicate the type name across nine files and surface a boolean
in charts and vitals cards where it is meaningless"). One row = one confirmed day:

```go
// FoodDayCompletion is the user's assertion that a partially-logged day (an
// Unconfirmed day, per the food-day-completeness capability) is actually
// complete. Row presence is the assertion; deleting the row retracts it.
// Not a metric type: this is a flag on a date, not a time-series
// measurement, and doesn't belong in typeRegistry.
type FoodDayCompletion struct {
	models.TenantModel
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_food_day_completion_user_date"`
	LocalDate   string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_food_day_completion_user_date"` // YYYY-MM-DD in the user's stored timezone at confirm time
	ConfirmedAt time.Time `gorm:"not null"`
}
```

Retracting a confirmation SHALL use a hard (`Unscoped()`) delete, not GORM's default soft delete.
`TenantModel` carries a `DeletedAt gorm.DeletedAt` column, and the unique index on
`(UserID, LocalDate)` has no `deleted_at` clause — the same conflict already hit and fixed for
`CustomFood` (`backend/pkg/server/food_custom.go`, `DeleteCustomFood`). A plain `Delete()` would
leave a soft-deleted row occupying that `(user, date)` slot forever, so re-confirming the same
Logged Day after retracting it would either violate the unique constraint or (if the insert path
is an upsert) silently revive the soft-deleted row without clearing `deleted_at`, leaving a day
that looks confirmed in the database but reads back as `unconfirmed` to any default-scoped query.

`LocalDate` is computed once at confirm time using whatever `timezone` is current then. Because the
day-completeness *range query* recomputes each day's occasion count fresh, on every call, using the
*current* `timezone` (per "Local Day boundary" above), a stored `LocalDate` string pinned to an old
timezone can end up matched against a different set of meals than the ones the user actually
confirmed once `timezone` changes — the same date string means a different 24-hour window under the
old and new zone. Leaving the confirmation in place would silently misattribute it to whichever
meals now fall under that string, not merely "stay pinned harmlessly" as a naive reading of "the
date doesn't shift" might suggest. To avoid that: **changing `timezone` SHALL delete all of that
user's existing `FoodDayCompletion` rows** as part of the settings update, using the same hard
(`Unscoped()`) delete required above for retraction — this bulk delete hits the identical
`(UserID, LocalDate)` unique index with no `deleted_at` clause, so a plain `Delete()` here would
leave every row soft-deleted and block re-confirming any of those dates going forward, exactly the
`CustomFood` trap repeated a second time in the same change. Every previously confirmed day reverts
to whatever state its occasion count computes to under the new timezone, rather than risking a
stale confirmation attaching itself to the wrong day's data. (The threshold,
`usual_meals_per_day`, has no equivalent hazard and keeps recomputing freely on every read, per
above — it is not stored per-day the way a confirmation is.)

### 5. API

`GET /api/food/completeness?from=YYYY-MM-DD&to=YYYY-MM-DD` — both required; 400 on missing/malformed
dates. Validation order matters: `to` is clamped to yesterday in the caller's stored timezone
*first* (when it names today or later — the caller isn't required to know the server's day-boundary
computation precisely to ask for "up to whatever's final"), and only then is `from > to` (using the
now-possibly-clamped `to`) and the >92-day span checked, both 400. This means a `from` that names
today or a future date always fails the `from > to` check post-clamp — e.g. `from=<today>&to=<today>`
clamps `to` to yesterday, at which point `from` is after `to` and the request is rejected, rather
than silently resolving to an inverted or empty range. Response: a JSON array, one entry per day in
the resolved inclusive range
(including Incomplete zero-occasion days — the array is the coverage primitive; a consumer needs to
see the gaps, not just the days with meals):

```json
[{"date": "2026-08-17", "occasion_count": 1, "state": "unconfirmed"}, ...]
```

Scoped strictly to the caller (`claims.UserID`, no `?user=` — this is a personal assertion, not
shared family data, matching the manual-write endpoint's convention from ADR-005).

`POST /api/food/completeness/{date}/confirm` — 400 on a malformed date or a date that is today/
future in the caller's zone. 400 if the day's current state is not `unconfirmed` *and* not already
`confirmed_complete` — i.e. 0 occasions or already-auto-Complete are both rejected (nothing to
confirm); re-confirming an already-`confirmed_complete` day is idempotent (200, no new row). A fresh
confirmation returns 201 with `{date, state, confirmed_at}`.

`DELETE /api/food/completeness/{date}/confirm` — removes the row if present; idempotent 204 whether
or not one existed (deleting an absent confirmation isn't an error — mirrors this project's other
DELETE endpoints not distinguishing "already gone" from "just removed").

### 6. Frontend surface

The confirmation control and the two new settings both live on `/food/history` — nowhere else. No
dashboard badge, no toast, no post-meal-log prompt (the grilling comment is explicit: "it does not
chase the user").

- Day-grouping key changes from `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}` (browser-local,
  `page.tsx:44`) to `Intl.DateTimeFormat('en-CA', { timeZone: tz }).format(d)` (`tz` = the stored
  `timezone` setting, default `'UTC'`) — `en-CA` formats as `YYYY-MM-DD` directly, matching the
  backend's `LocalDate` format so the two are trivially comparable.
- Each day section (which, by construction, only exists for days with ≥1 meal — a zero-occasion
  Incomplete day never appears here, since there's nothing to group into a section for it) shows,
  from `GET /api/food/completeness` covering the loaded range: nothing extra for `complete`, a
  small "Complete" badge for `confirmed_complete` with an unconfirm affordance, and a "Mark day
  complete" button for `unconfirmed`. Today's section (if visible) shows neither, since the range
  endpoint never returns an entry for it.
- The existing "load older" pagination on this page has no depth limit (`PAGE_SIZE = 50` meals per
  page, `frontend/app/food/history/page.tsx`), so the loaded range can exceed the endpoint's 92-day
  cap — trivially for a sparse logger, and eventually for anyone who pages back far enough. The
  frontend SHALL split the loaded range into consecutive ≤92-day windows and issue one
  `GET /api/food/completeness` call per window (in parallel), merging the results by date, rather
  than one call for the whole loaded span. A single oversized call is not an option: it would 400
  and blank out completeness for every visible section, not just the days beyond the cap.
- A small settings panel (collapsed by default) at the top of the page for `timezone` (a `<select>`
  populated from `Intl.supportedValuesOf('timeZone')` when available, prefilled — not auto-saved —
  from the browser's own zone via `Intl.DateTimeFormat().resolvedOptions().timeZone`) and
  `usual_meals_per_day` (a number input, min 1, default 3), saved through the existing
  `api.updateSettings` read-modify-write helper (`frontend/lib/api.ts`) so neither field can clobber
  `dashboard_order`/`display_language`.

## Risks / Trade-offs

- **A `timezone` change clears all existing confirmations** (per "Storage" above) — a user who
  confirms several days and then changes `timezone` loses those confirmations outright rather than
  having them silently reattach to the wrong days. Accepted: `timezone` changes are expected to be
  rare (set once, near account creation) and re-confirming a handful of days is a small, visible
  cost compared to a confirmation silently misattaching itself to meals the user never reviewed.
  The "changed" check compares the raw stored string, so a first-time explicit save of `"UTC"` over
  a previously-absent or invalid value also triggers this cascade even though the effective zone
  (UTC either way) didn't actually change. Accepted as the same trade-off: it's a rare, one-time
  write for most users, and detecting "no *effective* change" would require resolving and comparing
  zones in a store that's otherwise schema-free by design.
- **Today's under-13%-auto-complete rate means Phase 4 will mostly show "not enough data" at
  first** — named and accepted in the grilling comment itself as the honest output for 17 days of a
  logging habit still forming, not a defect to design around here.
- **No override for a false-positive auto-Complete day** — see open question #1 above; accepted
  as a real, currently-unaddressed gap rather than solved with an ad hoc mechanism.

## Migration Plan

Purely additive: new table, two new keys in an already-opaque settings blob, two new endpoints, no
change to any existing endpoint's response shape. The one existing behavior that does change, for
every user regardless of whether they ever touch the new setting: food history's day-grouping key
moves from the browser's local timezone (`getFullYear/getMonth/getDate`) to the stored `timezone`
setting, default UTC. For anyone on a non-UTC browser who hasn't set `timezone` yet, this is a real,
immediate shift in which calendar day a meal near midnight lands under — **not** a preservation of
current behavior, despite that being the more natural-sounding claim. Accepted: it trades one
arbitrary boundary (an unspecified browser zone) for another (UTC), and is what resolves the
pre-existing disagreement between food history's browser-local grouping and the data API's UTC
bucketing (`storage_impl.go`) named in the "Not doing" note above — the two definitions were
already inconsistent with each other, so this picks one and applies it consistently rather than
leaving a user's own history split across two boundaries. Anyone who wants the old browser-local
split back can set `timezone` to their own zone via the new settings panel immediately after this
ships. No rollback hazard: reverting the frontend stops calling the
new endpoints and grouping reverts to browser-local; reverting the backend leaves `FoodDayCompletion`
as an unused table and the frontend's completeness fetch failing closed (badge/control simply don't
render — food history's meal list itself doesn't depend on the new endpoint).
