# ADR-013: Nutrition advice uses bounded health history

**Status: Proposed**

## Context and Problem Statement

Nutrition Chat receives the aggregate evidence behind the current nutrition advice but no
underlying history. It cannot explain which Food Items contributed to a signal or answer follow-up
questions about steps, sleep or weight. The generated advice lines also see only food means and
the finished Nutrition Target, so they cannot tailor an action to relevant measured context.
Passing all available history in every prompt would be large, disclose unrelated records and make
the model responsible for deciding which data is relevant before it has seen the user's question.

## Decision Drivers

- The chat must explain a nutrition finding from the Food Items that produced it.
- The model needs broader history only when the user's question calls for it.
- Every history read must stay scoped to the authenticated user.
- Estimated nutrition values must remain distinguishable from reference and manual values.
- HealthVault must not expose arbitrary SQL, database identifiers or raw food-analysis artifacts.
- The existing ephemeral-conversation and `store:false` decisions remain in force.
- Generated advice should receive compact, sufficiently covered health summaries rather than raw
  history.

## Considered Options

- **Send all history with every question** — simple, but wasteful and over-disclosing.
- **Let the model construct database queries** — flexible, but creates an unnecessarily broad and
  unsafe interface.
- **Expose purpose-shaped read-only tools** — lets the model retrieve only relevant normalized
  evidence while the server owns authorization, validation and database access.

## Decision Outcome

Chosen: expose three purpose-shaped function tools through one server-owned execution interface:
nutrition-signal explanation, bounded health trends, and recent Logged Day details. The OpenAI
adapter runs a bounded Chat Completions tool-calling loop with reasoning effort disabled, as the
configured reasoning model does not support function tools on that endpoint while reasoning effort
is active. Tool arguments contain no user identity; the server adapter captures it from the
authenticated request.

History augments the explanation but never changes the deterministic Healthiness Label. The model
must distinguish estimates from measurements and must not present cross-metric correlation as
causation.

Generated advice receives a server-computed 28-day summary ending yesterday: the established
eligible step average, an average sleep duration with at least seven recorded days, and first and
latest daily weight averages with at least three recorded days. It also receives the Activity
Level and whether it came from a profile override or inferred steps. These values join the complete
`AdviceInput` cache hash. Missing optional data is omitted; read failures stop generation. The
prompt may use this context only when relevant and may not recalculate a calorie target or infer
causation.

### Consequences

- A chat question can require more than one provider request and database read.
- Tool outputs become transient third-party disclosures to the configured model provider, still
  under `store:false` and never persisted by HealthVault.
- Adding another history domain requires extending the bounded tool interface and its tests rather
  than widening access to the database.
- Advice can regenerate within a Logged Day when newly synced health context changes its complete
  input hash.
