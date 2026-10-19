# Estimate nutrients for food as eaten
Idea: HealthVault / Idea 1165

## Why

A confirmed 150 g portion of buckwheat was recognized as boiled and cooked but
received a dry-grain estimate of 343 kcal per 100 g. The owner asks for calories,
macros and the other tracked nutrients to describe prepared food consistently.
Automatic reference matching remains model-first because past fuzzy matches
selected unrelated foods.

## How

Extend the shared nutrition prompt used by photo recognition, written meal
descriptions and clarification. Request every estimated nutrient per 100 g of
the food in its as-eaten state. Account for water absorbed or lost during
preparation. Treat served buckwheat, rice and pasta as cooked unless an explicit
dry ingredient quantity is supplied. Keep raw ready-to-eat foods and packaged
foods in their actual consumed state. Keep weight, preparation, state and
nutrition consistent. Do not request clarification for ordinary served grains.

Keep reference-selection precedence, response schemas, persistence and existing
meal data unchanged. Do not turn this prompt improvement into an accuracy
guarantee. Use synthetic descriptions for live model checks without meal writes;
the existing deployed synthetic E2E uses fake vision and proves integration only.

## Validation Commands

- `make lint`
- `make test`
- `make test-e2e-disposable DISPOSABLE_HTTP_PORT=<allocated-port> DISPOSABLE_SESSION=<session>`
- Run the actual vision client's Describe and Clarify methods on synthetic food
  descriptions, including buckwheat, rice, pasta, fruit and dairy. Record outputs
  separately from deterministic test results.

### Task 1: Request consistent as-eaten nutrition
- [x] Extend the shared nutrition prompt for recognition, description and clarification.
- [x] Read the complete diff and run static checks and existing tests.
- [x] Run synthetic live-model checks without creating food meals.
- [x] Complete the Review Gate and validate the deployed synthetic stack.
- [x] Publish a draft PR with evidence and the validation limits.
- [x] Mark completed
