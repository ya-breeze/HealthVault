# Nutrition advice: show the basis and let the user discuss it

Idea: https://ideaforge.ikoro.in/idea/504

## Why

The dashboard's nutrition card states a Healthiness Label and one or two advice lines
underneath it. When one of those lines looks wrong, the user has nowhere to go. The owner put
it plainly on 2026-09-11: "хочу чат прямо сейчас - иначе я вижу странный совет и не могу его
обсудить."

The advice is generated from a deterministic judgment the user never sees. `computeHealthinessLabel`
pools seven days of complete food days, derives five signals, and flags at most two of them. The
model is then told the label, the reason codes and the pooled means, and is forbidden from
disputing them. So an advice line like "reduce salt" rests on a specific measured sodium mean,
a specific threshold and a specific number of days — none of which reach the screen. A user who
believes they already cut salt has no way to find out that the window is seven days, that three
of them were incomplete, or that the threshold is 2.3 g/day of elemental sodium rather than salt.

Two things follow, and this change does both. Show the measured basis of every flagged signal.
Then let the user ask about it in their own words.

This supersedes the evidence gate recorded in `todo.md` and in the merged
`docs/specs/a-nutrition-chat-affordance-on-healthvau.md`, which said to build the chat only after
observing qualified views and refresh requests in production. The owner lifted that gate
deliberately, before any aggregates existed. The engagement measurement shipped in
[PR #65](https://github.com/ya-breeze/HealthVault/pull/65) stays in place and keeps counting; it
is simply no longer a precondition.

## How

**One new surface, opened from the card.** The card gains a single control beside the advice
lines. It opens a sheet, modelled on `MoreSheet`: Escape closes it, Tab is confined to the panel,
and a dismissing click must both press and release on the backdrop. The card itself stays exactly
as compact as it is now, which is the arrangement the dashboard/food initiative settled on when it
merged the food rows into one card. A long conversation must not push the rest of the dashboard
down the screen.

**The sheet shows the basis first, then the chat.** The basis is not prose and not generated: it
is rendered from the same computation that produced the label. Each flagged signal contributes one
row naming what was measured, the threshold it crossed, and the verdict. A footer states how many
of the seven days were complete enough to count. The user can read the whole basis without
spending a model call.

**`computeHealthinessLabel` returns its own workings.** It already evaluates five signals and
discards everything except the label and the two winning reason codes. Extend `HealthinessResult`
with `eligibleDays` and a `signals` array carrying, per signal, its code when flagged, the measured
value, the unit that value is in, the verdict, and the boundary values it was judged against. No
threshold moves and no verdict changes: this is the same arithmetic, reported instead of dropped.
Recomputing any of it in the component would be a second source of truth for a number the user is
being shown as evidence.

**The chat is a new text-only `vision.Client` call.** `NutritionChat` takes the label, the reason
codes, the pooled means, the signal evaluations, the eligible-day count, the Nutrition Target, the
Display Language, and the conversation so far. It returns one answer string. It replays the turns
on every call the way `Clarify` does, because the client keeps no thread. Every implementation is
extended: the OpenAI one, `Fake`, and `Unconfigured`. `store:false` holds, as it does on every
other call in this package.

The prompt constrains the answer to the measurements it was given. The model must name an estimate
as an estimate, say when something was not measured, never diagnose, never reassure beyond the
evidence, and never dispute the label — the same standing rule `Advise` already carries, for the
same reason: the label is a deterministic judgment the model is not entitled to overrule.

**The history lives in the tab and nowhere else.** Turns are React state inside the sheet.
Closing the sheet, navigating, reloading or logging out discards them. No chat table, no
`localStorage`, no `sessionStorage`, no raw prompt log, no cross-day thread. This was the owner's
choice when the chat was first scoped and it is unchanged: a medical conversation that is never
written down cannot leak from a place nobody remembered to clear.

**The endpoint is authenticated and self-only.** `POST /api/food/advice/chat` enforces the
repository's same-origin rule, requires claims, and accepts no `user` field — the caller is
whoever the claims say. It validates the posted label, reason codes and window exactly as
`PostFoodAdvice` does, resolves the Nutrition Target server-side from the caller's own profile, and
rejects a request whose target cannot be computed. Limits: 16 KiB body, a question of at most 500
characters after trimming, and at most 8 prior turns. Anything past a limit is a 400, not a
silent truncation, so the client can never quietly lose the user's words.

**Deliberately excluded: steps, sleep and weight.** The Healthiness Label is computed from logged
food alone. Handing the model activity or sleep data would invite it to assert a link between
them and a label that never measured them, and the user would have no way to tell that assertion
from the arithmetic. The chat explains the evidence the advice actually used. If the owner later
wants a broader health conversation, that is a different surface with a different prompt, and it
should be specified as one.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Report the Healthiness Label's workings
- [ ] Extend `HealthinessResult` in `frontend/lib/healthiness.ts` with `eligibleDays` and a
      `signals` array whose entries carry the signal's measured value, its unit, its verdict, the
      reason code when flagged, and the boundaries it was judged against
- [ ] Populate `signals` from the existing five evaluations without changing any threshold,
      verdict, label, or reason-selection rule
- [ ] Keep the evaluation order the spec's fixed tie-break order, so the reported signals and the
      chosen reasons can never disagree about precedence
- [ ] Cover the new fields in `frontend/lib/healthiness.test.ts`, including a case where a signal
      is `ok` and therefore contributes no reason code, and assert the existing label and reason
      expectations still hold unchanged
- [ ] Mark completed

### Task 2: Add the nutrition-chat model call
- [ ] Add `NutritionChatInput`, `NutritionChatTurn` and `NutritionChatResult` to
      `backend/pkg/vision/vision.go`, carrying label, reason codes, means, signal evaluations,
      eligible days, Nutrition Target, Display Language and the prior turns
- [ ] Add `NutritionChat` to the `vision.Client` interface, documented as text-only, bounded, and
      replaying the turns because the client keeps no thread
- [ ] Implement it in `backend/pkg/vision/openai.go` with `store:false`, a bounded output, a
      response schema carrying a single answer string, and a system prompt that forbids disputing
      the label, forbids diagnosis, requires estimates and missing data to be named, and requires
      the answer in the Display Language
- [ ] Implement it in `backend/pkg/vision/fake.go` deterministically and in
      `backend/pkg/vision/unconfigured.go` as the established unconfigured error
- [ ] Cover request shape, `store:false`, and response parsing in `backend/pkg/vision/openai_test.go`
- [ ] Mark completed

### Task 3: Serve the chat behind an authenticated self-only endpoint
- [ ] Add `backend/pkg/server/food_advice_chat.go` with `foodHandlers.PostFoodAdviceChat`,
      accepting the label, reason codes, window means, signal evaluations, the question, and the
      prior turns, and accepting no caller-supplied user identity
- [ ] Enforce `isSameOriginRequest`, require `ClaimsFromCtx`, reject unknown fields and trailing
      JSON, and reuse the existing advice validation for label, reason codes and window
- [ ] Resolve the Nutrition Target and Display Language server-side from the caller's own profile,
      and return the established unavailable response when the target cannot be computed
- [ ] Enforce a 16 KiB body, a question of at most 500 characters after trimming, and at most 8
      prior turns, rejecting anything past a limit rather than truncating it
- [ ] Register `POST /api/food/advice/chat` beside the protected food routes in
      `backend/pkg/server/server.go`
- [ ] Mark completed

### Task 4: Cover the endpoint's contract
- [ ] Cover authentication, same-origin enforcement, unknown fields, trailing JSON, a malformed
      window, an invalid reason code, an over-long question, too many turns, and an over-sized body
      in `backend/pkg/server/food_advice_chat_test.go`
- [ ] Prove a caller cannot name another user and cannot reach another user's data through the
      endpoint
- [ ] Prove a model failure returns the unavailable response rather than a 500 leaking the error,
      and that an unconfigured vision client is reported as unavailable
- [ ] Prove a successful call returns the fake client's answer and that the prior turns reached
      the client in order
- [ ] Mark completed

### Task 5: Build the sheet
- [ ] Add `frontend/components/NutritionChatSheet.tsx` modelled on `MoreSheet`'s Escape handling,
      focus confinement, and press-and-release backdrop dismissal
- [ ] Render the basis rows from the `signals` the card already holds: per flagged signal the
      measured value, its threshold and its verdict, plus the eligible-day count
- [ ] Render the conversation, an input, a send control, a pending state, and a failure line that
      leaves the conversation usable
- [ ] Keep every turn in component state only, with no `localStorage`, no `sessionStorage`, and no
      persistence call, so closing the sheet discards the conversation
- [ ] Add `NutritionChatRequest`, `NutritionChatResponse` and `api.postNutritionChat` to
      `frontend/lib/api.ts` using the existing authenticated JSON helpers
- [ ] Mark completed

### Task 6: Open the sheet from the card
- [ ] Add a discuss control beside the advice lines in `frontend/components/LoggingGapCard.tsx`,
      shown only when advice is visible, meeting the repository's 48px mobile tap-target minimum
- [ ] Mount the sheet only while open, the way `MoreSheet` is mounted, and return focus to the
      control on close
- [ ] Leave the card's existing label, advice, refresh, engagement measurement and precedence
      rendering unchanged
- [ ] Add the English and Russian strings for the control, the basis rows, the units, the
      verdicts, the input, the send control and the failure line
- [ ] Mark completed

### Task 7: Cover the browser behaviour
- [ ] Extend `e2e/tests/logging-gap.spec.ts` to open the sheet, assert the basis rows match the
      seeded measurements, ask a question, and assert the answer renders
- [ ] Prove the conversation is discarded when the sheet closes and after a reload
- [ ] Prove a failing chat request leaves the sheet, the advice and the refresh control usable
- [ ] Prove the discuss control is absent when no advice is visible
- [ ] Mark completed

### Task 8: Record the decision and verify the result
- [ ] Update `CONTEXT.md` with the chat's ephemeral boundary and the basis rows' meaning
- [ ] Update `todo.md` to record that the owner lifted the evidence gate on 2026-09-11 and why
- [ ] Run every command in `## Validation Commands` against the deployed `hcw-wip` stack on the
      final head
- [ ] Run the Review Gate and resolve every valid finding before handoff
- [ ] Mark completed
