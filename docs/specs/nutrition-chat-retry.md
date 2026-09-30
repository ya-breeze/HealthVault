# Retry a failed nutrition chat request

## Why
The nutrition advice chat clears the question when sending it. If the request fails, the sheet says to try again but offers no way to resend that question. Repeated failures also consume the displayed conversation limit.

## How
Keep the failed request in the sheet's existing ephemeral state. Show a translated Retry request button beside the error. Resend the exact payload, including prior turns and advice evidence, without adding another user turn. Allow repeated retries even at the conversation limit. Disable submission while a request is pending and guard synchronous duplicate clicks. Starting a different question replaces the retry target. Closing the sheet discards it. Investigate the production failure through read-only logs; change server behavior only if the evidence establishes an in-scope defect.

## Validation Commands
- `make test-frontend`
- `make lint`
- `cd frontend && npx tsc --noEmit`
- `make test-e2e E2E_ARGS='tests/logging-gap.spec.ts --grep "Nutrition advice chat" --retries=0'`

### Task 1: Add failure recovery
- [ ] Add a retry button in English and Russian with exact payload replay and no duplicate turn.
- [ ] Preserve pending protection, conversation limits, and ephemeral lifetime.
- [ ] Add regression coverage for unavailable answers, transport failure, repeated retries, and history replay.
- [ ] Run static checks, the Review Gate, and deployed WIP browser tests.
- [ ] Report the production diagnosis with its evidence and uncertainty.
- [ ] Mark completed
