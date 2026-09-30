# Fix Nutrition Chat with Luna high and request retry

## Why
The nutrition advice chat clears the question when sending it. If the request fails, the sheet says to try again but offers no way to resend that question. Manually resubmitting the question adds another displayed turn. Retry should replay the failed request without another turn; different new questions still count as turns.

## How
Keep the failed request in the sheet's existing ephemeral state. Show a translated Retry request button only for retryable errors. Classify upstream HTTP status before decoding its body: 5xx, 408, rate-limit 429, network failures and timeouts are retryable; other 4xx, missing configuration, refusals, quota exhaustion and unknown internal errors are final. Preserve typed error information without returning provider detail or medical text. Return an explicit retryable boolean for unavailable chat answers. Classify browser HTTP errors too; only transport failures and HTTP 5xx/408/429 allow replay. Final errors show different translated text and no Retry button. Missing retry metadata defaults to final. Resend the exact payload, including prior turns and advice evidence, without adding another user turn. Allow repeated retries even at the conversation limit. Disable submission while a request is pending and guard synchronous duplicate clicks. Starting a different question replaces the retry target. Closing the sheet discards it. Add the missing saturated-fat value to an existing typed test fixture so frontend type checking can run. Read-only production logs show gpt-4o-mini rejecting the reasoning_effort argument. The owner requires GPT-6 Luna with high reasoning effort for Nutrition Chat. Use the Responses API for this chat because Luna tool calling with reasoning requires Responses. Preserve the stateless conversation, store:false, strict answer schema, bounded tools, usage accounting, and source provenance. Replay all response output items, including encrypted reasoning, with tool results within the single request. Record the endpoint decision in ADR-018. Keep other vision and advice calls on their existing configured model and endpoint. Do not silently fall back to another model or disable reasoning.

## Validation Commands
- `make test-backend`
- `make test-frontend`
- `make lint`
- `cd frontend && npx tsc --noEmit`
- `make test-e2e E2E_ARGS='tests/logging-gap.spec.ts --grep "Nutrition advice chat" --retries=0'`

### Task 1: Add failure recovery
- [x] Add a retry button in English and Russian with exact payload replay and no duplicate turn.
- [x] Preserve pending protection, conversation limits, and ephemeral lifetime.
- [x] Add regression coverage for unavailable answers, transport failure, repeated retries, and history replay.
- [x] Report the production diagnosis with its evidence and uncertainty.
- [x] Mark completed

### Task 2: Use Luna high through Responses
- [x] Move Nutrition Chat to Responses with GPT-6 Luna and high reasoning, and test complete tool round trips.
- [x] Verify Luna high with real API calls using synthetic tool data only.
- [x] Run static checks, the Review Gate, and deployed WIP browser tests.
- [x] Mark completed

Prior validation (before the owner requested Luna high): backend suite, 260 frontend tests, and 9 chat WIP E2E passed. Revalidate the revised provider implementation before handoff.

Revised validation: backend suite, 260 frontend tests, Go vet, E2E type checking, and frontend TypeScript passed. An actual OpenAI synthetic tool round-trip completed on gpt-6-luna with high reasoning in 3.8 seconds. All 9 Nutrition advice chat browser tests passed without retries against hcw-wip running code commit 02a09ab. Native Codex and Claude peer reviews completed; valid cleanup and spec findings were addressed and reviewed. No production VM mutation occurred. ADR-018 remains Proposed until owner-approved merge.

### Task 3: Distinguish retryable and final errors
- [x] Preserve upstream HTTP status and safe error classification through the server response.
- [x] Offer exact replay only for retryable failures and show final error text otherwise.
- [x] Test provider 4xx/5xx and transport failures, including non-JSON error bodies and quota errors.
- [x] Run tests, static checks, Review Gate, and WIP browser validation.
- [x] Mark completed

Selective-retry validation: backend suite, 264 frontend tests, Go vet, frontend TypeScript and E2E TypeScript passed. Native Codex and Claude peer reviews completed; all valid findings addressed. All 23 Nutrition advice chat browser tests passed without retries against hcw-wip, whose Portainer GitConfig.ConfigHash was 45e7778dab33e0303d46b2eebd6f21d98eaaef44. Coverage includes persistent HTTP 401 after auth refresh, HTML 502, malformed HTTP 200, exact replay and final errors in English and Russian. The final follow-up changes only the 401 test fixture and this validation record. Production VM remains unchanged.
