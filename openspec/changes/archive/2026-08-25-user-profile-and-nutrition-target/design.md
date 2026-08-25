## Context

ADR-003 fixed the formula split (calories/BMR from measured weight, protein g/kg from Goal Weight)
but left three things as numbers for "Phase 3's own `opsx:propose`" to pick: the activity-tier
count/multipliers, the protein-with-no-goal fallback, and the g/kg/split figures themselves. The
originating grilling session settled the protein rate (1.6 g/kg), the carb/fat split (50/50 with a
fat floor), and the no-goal fallback (require a goal, no fallback) explicitly. It settled the
*mechanism* for activity level (infer from steps, trailing 28-day window, discard incomplete edge
days, manual override) but not the specific tier boundaries, multiplier values, or the exact
edge-day trim rule — those are this document's job to pin down, the same way the Phase 2 proposal
pinned down BMI band edges and the regression window that its own originating ADR/session left as
mechanism-only.

## Goals / Non-Goals

**Goals:**
- A nutrition target that's computable for any user who has a goal weight, a profile, and either
  step history or a manual activity override — and that fails with a specific, actionable reason
  when it can't be computed, rather than guessing.
- An activity inference that's stable under the two data wrinkles the production query already
  found (multi-day sync gaps; a partial trailing day), without needing per-record timestamps finer
  than what `GET /api/data/steps?bucket=day` already returns.
- A `/settings` route that becomes the natural home for every future per-user preference, not a
  one-off form.

**Non-Goals:**
- Adaptive TDEE (steps-inferred activity is explicitly an interim proxy, called out as its own
  backlog item).
- Storing or caching the computed target — see "Computed on read" below, already decided in the
  originating session.
- Any change to how `weight_goal`, `weight`, or `height` are written — this change only reads them.

## Decisions

### Activity tiers: standard 5-tier multipliers, steps-per-day bands centered on the observed data

| Tier | Trailing 28-day avg steps/day | Multiplier |
|---|---|---|
| Sedentary | < 5,000 | 1.2 |
| Lightly active | 5,000 – 7,499 | 1.375 |
| Moderately active | 7,500 – 9,999 | 1.55 |
| Very active | 10,000 – 12,499 | 1.725 |
| Extra active | ≥ 12,500 | 1.9 |

These are the standard Mifflin-St Jeor/Harris-Benedict multiplier set (1.2/1.375/1.55/1.725/1.9) —
not a novel scale — mapped onto conventional steps-per-day bands. Five tiers, not three: the
production data's own step distribution (96–20,889/day, average 8,552) spans from near-sedentary to
extra-active, and a 3-tier scheme would put most of that range's meaningful variation inside a
single bucket. The same 5 tiers are the manual-override enum, but the enum's literal values do not
share the tier display names — `active` and `very_active` in particular do not positionally match
"Very active"/"Extra active" the way their spelling suggests. The mapping is exact and MUST be
implemented as this table, not inferred from string similarity:

| `activity_override` value | Tier name         | Multiplier |
|---|---|---|
| `sedentary`   | Sedentary          | 1.2   |
| `light`       | Lightly active     | 1.375 |
| `moderate`    | Moderately active  | 1.55  |
| `active`      | Very active        | 1.725 |
| `very_active` | Extra active       | 1.9   |

### Trailing window: 28 calendar days ending yesterday, low-but-nonzero days trimmed from the tail only

Today (the current calendar day) is always excluded from the window: it hasn't ended, so its step
count is structurally partial regardless of sync state, not just occasionally. The window is the 28
calendar days immediately before today. "Today" and every day boundary in the window are computed in
UTC, matching the convention every other time-bounded read/write in this codebase already uses
(`time.Now().UTC()` in `backend/pkg/server/api.go`'s write path; `bucket=day` aggregation uses the
same storage-layer day boundary) — HealthVault has no per-user timezone concept anywhere today, so
this change does not introduce one.

Two separate rules apply within that window, and only the second is edge-only:

1. **Zero-record days are always excluded, anywhere in the window.** A day with no step records at
   all is missing data, not a measured zero, so it never contributes to the average or the valid-day
   count regardless of where it falls — trailing edge or interior. This is what "trailing sync gaps"
   in the production data actually are (two full days with no rows), and it is also what an interior
   sync gap would be, which is exactly as uninformative.
2. **The 500-step floor trims only a contiguous run at the recent edge.** Walking backward from the
   most recent day (yesterday), discard each day with fewer than 500 total steps, **stopping at the
   first day that clears 500** — this never resumes once a clean day is found, so an interior day
   with, say, 300 steps is kept, not discarded. This directly matches what the production query
   already found: the day before the two no-rows days had 96 steps (trimmed by this floor, since 96
   is not a plausible full day even for someone sedentary — it reads as a device capturing a few
   minutes before a sync gap started). 500 is comfortably below the observed sedentary tier's own
   floor and exists only to catch "a fragment of a day," not to reclassify a real low-activity day —
   an interior day with, say, 1,800 real steps (a legitimate rest day) is kept, because this rule
   only ever removes the trailing run.

The remaining days (up to 28, however many survive both rules) are averaged directly — the window is
not backfilled with older days to replace excluded ones, so excluded days simply mean fewer days
feed the average, not a shifted window.

**Minimum data**: fewer than 7 valid days after exclusion, and no `activity_override` set → the
endpoint reports `insufficient_activity_data` (see below) rather than guessing a default tier. 7 is
one calendar week — enough to span a weekday/weekend cycle, which is the shortest period that says
anything about "typical" activity rather than one unusual day.

Rejected: zero-filling missing/trimmed days into the average (silently pulls the average toward
Sedentary for anyone mid-sync-gap on the day they happen to check their target) and blending
override with inference (a user who set an override specifically because steps don't reflect their
activity does not want it diluted by the very steps they said aren't representative).

### Nutrition target computation

```
BMR = 10 * measured_weight_kg + 6.25 * height_m*100 - 5 * age_years + sex_term
      sex_term = +5 (male) / -161 (female)
calories = BMR * activity_multiplier
protein_g = 1.6 * goal_weight_kg
protein_kcal = protein_g * 4
remaining_kcal = calories - protein_kcal
fat_floor_g = 0.8 * goal_weight_kg
carbs_g, fat_g = split remaining_kcal 50/50 by kcal (fat: /9, carbs: /4)
if fat_g < fat_floor_g:
    fat_g = fat_floor_g
    carbs_g = (remaining_kcal - fat_floor_g * 9) / 4
if carbs_g < 0:
    carbs_g = 0
calories, protein_g, carbs_g, fat_g = round each to the nearest gram/kcal
```

- **All four returned values are rounded to the nearest whole unit** (kcal for `calories`, grams for
  the other three) — the formulas above produce non-terminating decimals for most real inputs, and
  an unrounded float in the response is not a meaningful nutrition target for a user to act on.
- **`remaining_kcal` can be zero or negative** when `protein_kcal` (goal-weight-derived) alone meets
  or exceeds `calories` (measured-weight-and-activity-derived) — reachable because `weight_goal` and
  measured `weight` are independent inputs with no enforced relationship (`weight_goal` alone spans
  20-500 kg per `data-api`), most plausibly for a low-BMR/low-activity user with a goal weight set
  well above their current weight. `protein_grams` is still returned as computed (it does not depend
  on `calories`); `fat_grams` still applies its floor; `carbs_grams` is clamped to 0 rather than
  going negative. The returned macros can legitimately sum to more kcal than `calories` in this case
  — that reflects the goal-weight-derived minimums genuinely exceeding the measured-weight-derived
  budget, not a computation bug, and is not one of the four reasons this endpoint returns 422 for
  (this is a valid-input result, not an unmet precondition).

- **Height** feeds the formula in centimetres (Mifflin-St Jeor's canonical form), while the
  `height` metric type stores metres (existing convention, per `data-model`) — the handler converts
  once at read time; no stored-unit change.
- **Age** is calendar age in completed years as of the request time (birthdate month/day already
  passed this year → age increments), computed fresh every call — never cached, matching the
  "computed on read" decision below.
- **Fat floor uses Goal Weight, not measured weight** — the floor exists for the same reason the
  protein target does (a hormonal-health minimum sized to the body being worked toward), so it uses
  the same weight basis as protein rather than introducing a third weight input the calorie side
  doesn't use.
- **Carbs absorb the floor adjustment**, not protein or fat again — protein is fixed by the g/kg
  rate and fat has a hard floor once triggered, so carbs is the only remaining degree of freedom.
- Rejected: applying the fat floor to measured weight — would make the floor tighten as a user
  loses weight even though the target physique (and therefore the physiological reason for the
  floor) hasn't changed.

### Unmet-precondition responses: one reason code per unmet input, no partial target

`GET /api/users/me/nutrition-target` returns `422 Unprocessable Entity` with
`{"error": "<reason>"}` for exactly these reasons, checked in this order (checks are independent —
they are not fallbacks into each other — but reported by whichever is checked first when several are
unmet, so the response always names one concrete unmet input rather than "several things are
wrong"):

1. `missing_profile` — `birthdate` absent, unparsable, implausible (see below), or `sex` absent or
   not exactly `"male"`/`"female"`.
2. `missing_measurements` — no `weight` record or no `height` record exists at all.
3. `missing_goal_weight` — no `weight_goal` record exists.
4. `insufficient_activity_data` — fewer than 7 valid trailing days (see above) and no
   `activity_override` set.

There is no partial-target response (e.g. calories without protein): ADR-003's own math makes
carbs/fat depend on protein being fixed first, so a response missing protein has nothing left to
compute for carbs/fat either — this is the ADR-003 inconsistency this change's corrections remove.

`birthdate`/`sex` are read the same way `display_language` already is — "interpreted rather than
assumed," per the existing `display-language` requirement's own pattern — not validated at the
generic `PUT /api/users/me/settings` write (that endpoint stays schema-agnostic, per
`user-settings`'s existing contract; only "is this valid JSON" is checked there). A malformed
`birthdate` (unparsable, in the future, or implying an age below 5 or above 120 years — 5 and 120
themselves are valid ages) or `sex` outside the two-value enum is treated as absent, producing
`missing_profile` — a value that made it into the blob (however it got there — the form is expected
to prevent this, but nothing stops a direct API call) never produces a wrong number silently.

### Self-only: no `?user=` family-member support

The endpoint description above says it follows "the existing `summaryHandler` pattern," which is
true for auth-via-`ClaimsFromCtx`/no-body/plain-JSON, but `summaryHandler` also calls
`resolveUser(r, storage, claims.UserID, familyID)` to honor an optional `?user=<username>` query
parameter (per `data-api`'s "Family member data access" requirement) — this endpoint deliberately
does **not** adopt that half of the pattern. `birthdate`/`sex`/`activity_override` live in
`UserSettings`, which `user-settings`'s existing "Per-user settings storage" requirement scopes as
"addressable only by the authenticated user's own identity." Since this endpoint's output depends on
those self-only fields, making the endpoint itself family-visible would either require punching a
family-visibility hole through `UserSettings`'s existing self-only invariant, or produce an endpoint
that is family-visible for some of its inputs (`weight`/`height`/`weight_goal`/`steps`) and
self-only for others — an inconsistency with no precedent elsewhere in the API. `GET
/api/users/me/nutrition-target` therefore always computes the caller's own target; there is no
`?user=` parameter, and family members do not see each other's Nutrition Target in this change.

### Computed on read, never stored

Already decided in the originating session, restated here because it constrains the API shape: no
new table, no cache, no invalidation logic. Every input — the goal, the measurement, the profile
fields — already has its own point-in-time history or is itself a stored value, so any past target
is reconstructible without HealthVault storing one. Storing it would create a second copy that a
corrected birthdate or a retroactively-edited weight record wouldn't reach, leaving a stale number
next to a corrected input.

### `/settings` route and the third settings writer

`/settings` is a new top-level authenticated route, structurally parallel to `/data/[type]` and
`/food` — same `Header` chrome, no new layout primitive. Its Profile section is one form with two
required controls (birthdate, sex) and one optional one (activity override), following
`CustomFoodModal.tsx`'s hand-rolled `<input>`/`<select>` idiom (there is no shared form-field
component to reuse) and wrapping every control in `TapTarget`.

The form calls `useLanguage().updateSettings(patch)` — the same function `LanguageContext` already
exposes for exactly this purpose (its doc comment already names "any other screen that writes to
the shared UserSettings blob" as the intended audience) — rather than calling
`api.updateSettings`/`api.putSettings` directly. Both wrap the same read-modify-write, but only the
`LanguageContext` copy is queued behind `useSerialQueue().claim()` alongside the language
switcher's own reads/writes; calling `api.updateSettings` directly from the form would let it race
the switcher's GET/PUT exactly the way the pre-fix dashboard-order/language race did (see
`user-settings`'s new requirement). This is a one-line difference (`useLanguage().updateSettings`
vs. `api.updateSettings`) with a silent-data-loss failure mode if missed, which is why it's called
out explicitly here rather than left implicit in "reuse the existing form idiom."

Display Language's `<select id="display-language">` moves from `Header.tsx` into this same Profile
section, keeping its existing `id` (the E2E race test selects it by that id) and its existing
`setLanguage` wiring unchanged — only its mount location moves. `Header.tsx` replaces the inline
control with a link to `/settings`.

## Risks / Trade-offs

- **The 500-step/7-day trim thresholds are this proposal's own judgment call**, not sourced from
  the grilling session (which settled the mechanism, not the numbers) — flagged the same way the
  Phase 2 proposal flagged its own regression-window and horizon constants, for the same reason: a
  concrete, testable number beats a mechanism description an implementer would otherwise have to
  invent unreviewed.
- **`missing_profile` folds two independent unset fields (birthdate, sex) into one reason code.**
  Rejected a finer-grained `missing_birthdate`/`missing_sex` split as unwarranted complexity: the
  frontend already knows which of its own two required fields is empty before it ever calls this
  endpoint (client-side required-field validation), so the endpoint's reason code only needs to
  cover the case of a non-form caller or a row edited outside the UI.
- **Clamping `carbs_grams` to 0 when protein alone exceeds `calories` is this proposal's own
  judgment call**, not sourced from the grilling session (which didn't anticipate goal weight and
  measured weight diverging this far) — flagged for the same reason as the trim thresholds above: an
  unusual but reachable input combination needs a concrete, reviewed answer rather than an
  implementer inventing one. The alternative considered and rejected was a fifth 422 reason code
  (e.g. `goal_too_aggressive`); clamping was chosen because `protein_grams` and the fat floor are
  still both meaningful, well-defined numbers in this case — only `carbs_grams` has nothing left to
  be, so only it degrades, rather than failing the whole request.

## Migration Plan

Purely additive. `birthdate`/`sex`/`activity_override` are new keys in an already-schemaless JSON
blob — no migration, and existing rows with none of the three keys behave exactly as today's rows
do (the new endpoint reports `missing_profile` for them, which is the correct answer for "this user
has no profile yet"). The new endpoint is a new route; no existing response shape changes.

## Open Questions

None outstanding for this change's own scope. The privacy posture of storing birthdate/sex
unencrypted is flagged in `proposal.md`'s "What's explicitly out of scope" as a decision for the
operator, not a blocker for this change, which is consistent with the app's existing (undocumented)
posture for every other field in `UserSettings`.
