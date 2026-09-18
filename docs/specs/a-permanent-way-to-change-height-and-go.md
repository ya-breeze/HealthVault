# Add a permanent way to change height and goal weight, on the Settings page
Idea: ya-breeze/idea-forge#627

## Why

Idea #627 asks for "an understandable way to change the user's data — height, goal weight, etc."
("нужно чтобы healthvault имел понятный способ поменять данные о пользователе. Например рост,
целевой вес и т.д."). Investigation found the profile data model already splits two ways: static
fields (`birthdate`/`sex`/`activity_override`) are always editable on the Settings page
(`frontend/app/settings/page.tsx`); height and goal weight are separate time-series metric types
(`height`/`weight_goal`, latest-record-wins) written through `POST /api/data/{type}`, each with
its own always-reachable detail page (`/data/height/`, `/data/weight_goal/`) and Add-record form.
Neither write path is missing or broken.

What is actually missing is a *permanent, expected place to reach them*. The Weight page offers
"Set goal"/"Set height" shortcuts (task 4.2), but they exist for a narrower, already-tested
purpose: helping a user with zero height/goal records discover the write path once. The e2e spec
(`e2e/tests/data-types.spec.ts`, "a user with no height can add one without knowing the URL")
explicitly asserts the height shortcut retires once a height exists — "the shortcut retires once
it has served its purpose" — and a second test ("BMI turns on...") separately pins that a user who
already has a height is never shown it again, guarding against a real prior bug where it flashed
during load and could write a duplicate. Goal weight's own shortcut has no such gate at all and
stays visible unconditionally — an existing inconsistency between the two, and neither one was
designed as an ongoing edit affordance. After the one-time nudge is used (or for goal weight, at
any time a user doesn't happen to be on the Weight page), the only paths back to changing height
are typing `/data/height/` directly or finding it in the dashboard's collapsible "More Data" row
— an unlabeled chip among every other secondary metric type, present only once the value already
exists, and hideable by the user via `more_data_hidden`. None of that is "an understandable way
to change" the value.

## How

Add a "Body measurements" section to `frontend/app/settings/page.tsx`, alongside the existing
"Profile" section, with two always-visible buttons — reusing the existing `dataDetail.setHeight`/
`dataDetail.setGoal` i18n strings ("Set height"/"Set goal", "Указать рост"/"Задать цель") so no
new translation is needed — each toggling the same `AddRecordForm` component
(`type="height"`/`type="weight_goal"`) already used on the Weight page and each type's own detail
page. `AddRecordForm` needs no change: it already posts to the write-allowlisted
`/api/data/{type}` endpoint and already supports being mounted standalone with just
`type`/`onSuccess`/`onCancel`. No backend change at all: `POST /api/data/height`/`weight_goal`
already accepts a new record at any time — latest-record-wins is exactly how "change your height"
already works everywhere it is reachable today.

This does not touch the Weight page's own "Set goal"/"Set height" shortcuts, their existing
behavior, or the e2e coverage pinning it: goal's shortcut stays permanently visible there too;
height's still retires after its first use on that page specifically. That onboarding path solves
a different problem (discovering BMI without ever leaving the Weight page) and is left exactly as
designed and tested. Settings becomes a second, independent, always-available path to the same
write endpoints, matching how `birthdate`/`sex`/`activity_override` already behave on that same
page — one place a user can expect to find and change every piece of their own profile data.

The Settings page's existing labels ("Profile", "Birthdate", "Sex", "Activity level", "Save", ...)
are plain hardcoded English text, not run through `t()` — only its language `<select>` is. The
two new buttons *do* call `t('dataDetail.setHeight')`/`t('dataDetail.setGoal')` rather than
hardcoding English, since both keys already exist and are already translated in both languages —
reusing them costs nothing and gives a Russian-speaking user the same Russian label they already
see on the Weight page, instead of deliberately shipping new English-only text next to a section
this Idea's own body was written in Russian. This leaves the rest of the page's existing labels
exactly as English as they already were; fully localizing Settings is a separate, pre-existing gap
this change does not take on.

**Cut from this pass**: showing the current height/goal-weight value next to each button (e.g.
"Height: 178 cm · change"). Doing that correctly needs the latest-record fetch, the
`latestByTime`/per-type field-name knowledge (height records expose `meters`, weight/weight_goal
records expose `kilograms`), and the display-formatting helpers that `DataTypeClient.tsx`
currently keeps entirely to itself — extracting those into a shared module is real, separable
refactoring work, not needed to give the user a working, discoverable way to change either value.
A bare "Set height"/"Set goal" button, opening the same form used everywhere else already, fully
answers what the Idea asked for.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Settings page gets a permanent, always-visible way to change height and goal weight

- [x] Add a "Body measurements" section to `frontend/app/settings/page.tsx`: two buttons ("Set
      height", "Set goal"), each toggling its own `AddRecordForm` (`type="height"` /
      `type="weight_goal"`) inline, mirroring the toggle pattern the Weight page already uses for
      the same two forms (`showHeightForm`/`showGoalForm` state, `onCancel` closes it back).
- [x] Neither button is gated on whether a value already exists — unlike the Weight page's
      height shortcut, and matching its goal shortcut, since this section's whole purpose is to
      remain available after the value is already set.
- [x] Do not modify `frontend/app/data/[type]/DataTypeClient.tsx`'s existing shortcuts, their
      gating, or `AddRecordForm.tsx` itself.
- [x] Mark completed

### Task 2: Prove it, on the deployed stack

- [x] Add an e2e spec (`e2e/tests/settings.spec.ts`, "Body measurements (Settings)") covering: a
      user with an existing height record still sees and can use the Settings "Set height" button
      to write a new height record, verified as the latest row on `/data/height/`; the same for
      "Set goal", additionally verified against the goal ReferenceLine it feeds on the Weight page
      (height's own downstream BMI readout needs a weight record too, which is out of this test's
      scope to also set up — the direct record check already proves the write path). Confirms the
      fix from the read path a real user would actually notice, not just that a button renders.
- [ ] Run every command in `## Validation Commands` and resolve all failures.
- [ ] Mark completed
