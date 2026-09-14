# Make fiber actionable in nutrition guidance

## Why

HealthVault stores dietary fiber on every Food Item and Food Meal, and the review screen now shows
and edits it. The Healthiness Label, generated advice and aggregate nutrition-signal history still
ignore it. A corrected fiber value therefore cannot change the judgment or become an advice basis.

Nutrition Chat can now read Food Item contributors, but it turns every tool result into model-written
prose. The user cannot distinguish a named contributor from the model's wording or open the meal that
contains it. An explanation should lead directly from the advice to the server-owned evidence and to
the existing correction screen.

## How

Add dietary fiber as a sixth deterministic Healthiness Label signal over the same eligible seven-day
pool. Measure the mean grams per eligible day. Use EFSA's adult adequate intake of 25 g/day as the
one evidence-backed boundary. A mean at or above 25 g/day is `ok`; a lower mean is `off`. Do not
invent a `far` boundary, so fiber alone can make the label Fair but never Needs attention. Three or
more independent `off` signals can still produce Needs attention through the existing combination
rule. Append fiber after the existing five signals so their tie-break order is unchanged.

Carry the fiber mean and signal through AdviceInput, NutritionChatInput, request validation, the
complete advice cache hash, both prompts, fakes and tests. Extend `explain_nutrition_signal` to accept
fiber and return its same eligible-day totals and Food Item contributors. Keep the source of truth in
`computeHealthinessLabel`; neither the server nor the model recomputes the verdict.

When `explain_nutrition_signal` reads contributors, its request-scoped server adapter also records at
most five structured source rows. Each row contains the Logged Day, meal identifier, Food Item display
name, signal, contribution, Macro Source and confidence. The identifiers never enter the model tool
result or prompt. The endpoint returns these server-owned rows beside the answer; they are not part of
the model's response schema and the model cannot create or edit them.

Render the rows below the assistant turn that caused the lookup. Each row is a full-width mobile tap
target linking to the existing meal review route, with the date, Food Item, contribution and localized
source badge visible before navigation. Keep sources only in the sheet's React state and strip them
from replayed turns, preserving the existing ephemeral conversation and request limits. A question
that needs no nutrition-contributor lookup returns no source list.

Record the sixth deterministic signal in ADR-004 and CONTEXT.md. This change does not persist chat,
change the Nutrition Target, add a child-specific fiber target, or let model prose control navigation.

Reference: EFSA, “EFSA sets European dietary reference values for nutrient intakes,” 26 March 2010,
https://www.efsa.europa.eu/en/press/news/nda100326.

## Validation Commands

- `make lint`
- `make test`
- `BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS="tests/logging-gap.spec.ts --retries=0"`

### Task 1: Add dietary fiber to the deterministic judgment

- [x] Extend daily inputs, pooled means, signal types and thresholds with dietary fiber
- [x] Preserve the existing five-signal order and evaluate fiber as lower-only against 25 g/day
- [x] Cover the boundary, low-fiber result, combination rule and unchanged existing reason precedence
- [x] Mark completed

### Task 2: Carry fiber through advice and chat

- [x] Add the fiber mean to advice and chat request shapes, normalized inputs, prompts and cache hash
- [x] Let `explain_nutrition_signal` return fiber days and contributors under the existing bounds
- [x] Cover validation, provider request shape, fakes and server-owned history access
- [x] Mark completed

### Task 3: Return server-owned actionable sources

- [x] Collect at most five contributor sources inside the authenticated request-scoped history adapter
- [x] Keep meal identifiers out of model-visible tool results and return sources beside the final answer
- [x] Cover source ordering, bounds, caller isolation, empty-source answers and non-persistence
- [x] Mark completed

### Task 4: Render sources on mobile

- [x] Keep display-only source metadata on its assistant turn and omit it from replayed chat turns
- [x] Render localized full-width source links with date, contribution, Macro Source and confidence
- [x] Navigate each source to its caller-owned meal review screen with a 48px minimum tap target
- [x] Cover narrow-screen layout, source navigation and a response without sources in Playwright
- [x] Mark completed

### Task 5: Record and validate the result

- [x] Update ADR-004 and CONTEXT.md without rewriting any merged spec
- [x] Run every validation command against the final branch and deployed WIP stack
- [x] Run the Review Gate and resolve every valid finding
- [x] Confirm that no task box remains unticked
- [x] Mark completed
