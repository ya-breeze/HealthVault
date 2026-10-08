# Homemade and low-added-salt context during food entry

## Why

The owner mostly cooks at home with little added salt. Photo recognition cannot see added salt and may assign a normally salted recipe to that food. The owner asks for checkboxes during entry, with homemade and little added salt selected by default, and explicitly marks restaurant or prepared purchased food when entering it.

## How

Add a shared two-checkbox block to photo and description entry. Both start selected for each new meal. Turning off homemade clears and disables low added salt; turning homemade on restores the default. Submit the explicit cooking context separately from the user's hint or description, and persist it on that meal. Existing meals and API callers that omit context retain unknown preparation context. Feed persisted context into photo recognition, described-meal recognition, retry, clarification, and normal photo reanalysis without changing the provider interface. Low added salt describes cooking only: intrinsic sodium, processed ingredients, sauces, and label values remain relevant. Explicit quantities or corrections override the generic context in model instructions. Initial recognition and normal photo reanalysis can update nutrient estimates. Preserve the existing clarification safeguard that carries forward the original nutrient profile for the same item; clarification alone does not revise that profile. Use normal photo reanalysis to apply a nutrient correction. Keep original description and hint length limits. Structured manual nutrient entry keeps its explicit values. Do not recalculate old meals, infer zero sodium, or change healthiness thresholds.

## Validation Commands

- `make test-backend`
- `make test-frontend`
- `make lint`
- `make test-e2e E2E_ARGS='tests/food.spec.ts --retries=0'`
- Bounded synthetic WIP recognition with homemade/low-added-salt context and explicit sodium on a label; inspect persisted flags, estimates, and subsequent retry/clarification behavior separately from mocked tests.

### Task 1: Persist and apply optional meal preparation context
- [x] Add optional typed cooking context to meals and photo/description requests, with validation for inconsistent flags and malformed upload JSON.
- [x] Preserve context across recognition, retry, clarification, and normal reanalysis while leaving omitted context unknown.
- [x] Add meaningful storage and handler tests for forwarding, persistence, validation, and lifecycle reuse.

### Task 2: Show and submit preparation checkboxes
- [x] Add shared localized controls to photo and text entry with the agreed defaults and dependency behavior.
- [x] Extend API types and submissions without altering user text or structured manual values.
- [x] Cover photo/text defaults, non-homemade submissions, and low-salt changes in browser tests.

### Task 3: Validate and review
- [ ] Run tests, static checks, Review Gate, WIP deployment, browser validation, and real-provider probes.
- [ ] Mark completed
