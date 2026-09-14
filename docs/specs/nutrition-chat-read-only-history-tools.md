# Let nutrition advice and chat use relevant health history

## Why

Nutrition Chat can currently explain only the aggregate figures posted by the nutrition card.
It receives the Healthiness Label, its signal values, pooled means and Nutrition Target, but no
Food Meals, Food Items, steps, sleep or weight history. When the user asks where a sodium finding
came from, the model can only repeat the mean. It cannot name the foods that contributed to it or
say which values were estimated. The same limitation prevents it from answering useful follow-up
questions about activity, sleep or weight trends.

The user decided on 2026-09-14 that a useful Nutrition Chat must be able to inspect relevant
history, and that the advice lines should take the same broader health context into account when
it materially changes a useful recommendation. This supersedes `docs/specs/nutrition-chat.md`'s
deliberate exclusion of steps, sleep and weight. The conversation remains ephemeral; only the
read-only evidence available during one request becomes broader.

## How

Give the model three purpose-shaped tools instead of database access. `explain_nutrition_signal`
returns the exact seven-day Healthiness Label window, each day's eligibility, and the Food Items
that contributed to a requested protein, carbohydrate, fat, sugar or sodium signal.
`get_health_trend` returns daily steps, sleep or weight values over a bounded 7-, 28- or 90-day
window. `get_day_details` returns the caller's Food Meals and Food Items for one recent Logged Day.

The tool executor is a deep server-owned module behind one interface. It captures the authenticated
user, their timezone and the request time. Tool arguments never accept a user or family identifier.
It performs only scoped reads and returns normalized JSON. It exposes no photo path, raw model
response, clarification history, Meal Description or database identifier. Food Items report their
Display Name, nutrition contribution, Macro Source and confidence so the model can distinguish an
AI estimate from reference or manual data.

The model chooses whether to call a tool. The OpenAI adapter implements the documented Chat
Completions function-tool loop: send the available tool definitions, execute validated calls on the
server, append the assistant tool call and matching tool result, then ask for the final structured
answer. Set `reasoning_effort:none` on this tool-enabled Chat Completions path because the configured
reasoning model rejects function tools while reasoning effort is active. Bound each user question
to three tool calls. Keep `store:false` on every provider request.
Reject unknown tools and invalid arguments without exposing database or provider errors.

The current advice basis remains in the initial prompt. History is additional context, not a new
input to the deterministic Healthiness Label. The prompt must distinguish correlation from
causation, must identify estimated Food Item values as estimates, and must not claim that steps,
sleep or weight caused a nutrition finding.

Give the generated advice lines a compact 28-day health context ending yesterday. Reuse the
existing overlap-collapsed and incomplete-edge-trimmed step average. Include average sleep only
with at least seven recorded daily buckets. Include the first and latest daily weight averages
only with at least three recorded daily buckets. Also tell the model which Activity Level and
source already fed the Nutrition Target. Missing optional metrics do not block advice. A genuine
history read failure does, so HealthVault never caches a degraded result as if it were complete.

The advice prompt may use this context only to tailor an otherwise supported recommendation or
acknowledge a measured trend. It may not change the Healthiness Label, recalculate calorie needs,
claim causation, or invent a recommendation from a sparse metric. The supplied Nutrition Target
already incorporates Activity Level. Add this normalized context to `AdviceInput`, so any material
change invalidates the existing one-row-per-user advice cache through its current complete-input
hash.

No chat content or retrieved history is persisted by HealthVault. Provider error logs retain the
existing question redaction and must not add tool arguments or tool results.

## Validation Commands

- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Define the read-only history seam

- [x] Add a bounded Nutrition Chat tool executor interface to the vision module
- [x] Implement the authenticated server adapter for nutrition-signal evidence, health trends and
      Logged Day details
- [x] Preserve the Healthiness Label's exact eligibility rules without calling a write-capable read
      path
- [x] Expose Macro Source and confidence while excluding private implementation fields
- [x] Cover caller isolation, date/range limits, incomplete days and estimated Food Item evidence
- [x] Mark completed

### Task 2: Let the model request history

- [x] Add strict function-tool definitions for the three history tools
- [x] Implement a bounded Chat Completions tool loop with matching tool-call IDs and `store:false`
      on every request
- [x] Require history questions to use the tools and constrain cross-signal claims in the prompt
- [x] Cover tool selection, result replay, multiple calls, invalid calls, call limits and final
      structured-answer parsing
- [x] Mark completed

### Task 3: Wire and validate the complete chat path

- [x] Bind the tool executor to the authenticated caller in `POST /api/food/advice/chat`
- [x] Keep the public endpoint and ephemeral frontend conversation contract unchanged
- [x] Prove through the handler seam that the model can inspect only the caller's history
- [x] Run the repository validation commands and validate the deployed WIP chat path
- [x] Update the domain context and the architectural decision record
- [x] Mark completed

### Task 4: Contextualize the generated advice lines

- [x] Build a bounded 28-day steps, sleep and weight summary ending yesterday
- [x] Reuse the established step-data eligibility rules and require minimum sleep and weight
      coverage
- [x] Add Activity Level provenance and optional health context to the complete advice input hash
- [x] Constrain the prompt to relevance without causal or calorie-target claims
- [x] Cover complete, sparse, changed and caller-isolated context
- [x] Mark completed
