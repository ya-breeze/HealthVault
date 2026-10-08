# Nutrition Chat answers practical food questions and preserves its evidence

## Why

The owner asks where to get unsaturated fats after the dashboard flags saturated fat. Nutrition Chat redirects the question to the current food log because its instructions allow answers only from supplied records. The owner also reports that the chat calls saturated-fat quantity unknown while saying it is high. The input already carries its estimated daily grams and energy share. A stored estimate is available evidence even when it is not an exact measurement.

## How

Allow established general nutrition knowledge for food examples and practical substitutions. Answer the latest question first. Use history tools for questions about the caller's logged food, not as a prerequisite for general food suggestions. Keep personal figures, thresholds, and claims about actual consumption grounded in the supplied basis or tools. Explain that saturated-fat share is a share of estimated energy and its grams are estimated daily intake; missing unsaturated-fat totals do not erase supplied saturated-fat values. Correct contradictory prior assistant answers instead of repeating them. Preserve the model policy, four-sentence limit, read-only tools, authentication, and ephemeral conversation. Do not change sodium estimates or healthiness calculations.

## Validation Commands

- `make test-backend`
- `make lint`
- `make test-e2e E2E_ARGS='tests/logging-gap.spec.ts -g "Nutrition advice chat" --retries=0'`
- Exercise the deployed WIP chat with synthetic Russian questions about unsaturated-fat foods, supplied saturated-fat estimates, and a contradictory prior answer. Inspect actual configured-model answers separately from mocked tests.

### Task 1: Clarify the answer and evidence instructions
- [x] Permit practical food advice from general nutrition knowledge without inventing personal quantities.
- [x] Require a direct answer to the current question and distinguish supplied estimates from missing measurements.
- [x] Explain saturated-fat grams and energy-share evidence and correction of contradictory prior answers.
- [x] Extend provider-request coverage for the saturated-fat evidence and follow-up question.
- [ ] Run validation, Review Gate, and bounded real-model WIP probes.
- [ ] Mark completed
