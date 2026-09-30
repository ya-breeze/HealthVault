# Retry a failed nutrition chat request

## Why
The nutrition advice chat clears the question when sending it. If the request fails, the sheet says to try again but offers no way to resend that question. Repeated failures also consume the displayed conversation limit.

## How
Keep the failed request in the sheet's existing ephemeral state. Show a translated Retry request button beside the error. Resend the exact payload, including prior turns and advice evidence, without adding another user turn. Allow repeated retries even at the conversation limit. Disable submission while a request is pending and guard synchronous duplicate clicks. Starting a different question replaces the retry target. Closing the sheet discards it. Add the missing saturated-fat value to an existing typed test fixture so frontend type checking can run. Read-only production logs show gpt-4o-mini rejecting the reasoning_effort argument. Send the tool compatibility setting only for GPT-5 models; omit it for the configured GPT-4o model. Keep tool calls, structured answers, and store:false unchanged.

## Validation Commands
- `make test-frontend`
- `make lint`
- `cd frontend && npx tsc --noEmit`
- `make test-e2e E2E_ARGS='tests/logging-gap.spec.ts --grep "Nutrition advice chat" --retries=0'`

### Task 1: Add failure recovery
- [x] Fix the unsupported reasoning_effort parameter and test GPT-4o request bodies.
- [x] Add a retry button in English and Russian with exact payload replay and no duplicate turn.
- [x] Preserve pending protection, conversation limits, and ephemeral lifetime.
- [x] Add regression coverage for unavailable answers, transport failure, repeated retries, and history replay.
- [x] Run static checks, the Review Gate, and deployed WIP browser tests.
- [x] Report the production diagnosis with its evidence and uncertainty.
- [x] Mark completed

Validation evidence: backend suite, 260 frontend tests, Go vet, E2E type checking, and frontend TypeScript passed. All 9 Nutrition advice chat tests passed without retries against hcw-wip running the reviewed feature branch. The GPT-4o regression reproduced the provider error before the fix. Native Codex Review Gate passed after test fixes. Claude completed a direct read-only review, but did not run the required system review subagent; the formal peer gate is unavailable. Production VM logs at 2026-09-30 04:41:31 UTC identify the unsupported reasoning_effort argument on gpt-4o-mini. Production remains unchanged.
