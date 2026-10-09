# Record meals from ChatGPT through a private MCP tunnel

## Why

The owner verified the OpenAI Secure MCP Tunnel demo from ChatGPT on the web and phone. HealthVault already accepts meal descriptions, but its administrative MCP only reads data. A phone connection needs a fixed user identity, the existing meal review workflow, and safe recovery after an interrupted tool call.

## How

Add a separate stateless `/phone-mcp` endpoint. Require a dedicated bearer token and configured user UUID; disable it when either is absent. Resolve identity on every call. Expose only describe, get, clarify, confirm, and retry meal tools. Reuse the existing HTTP food handlers and their ownership checks. Return meal status, items, clarification questions, and nutrition totals without raw model output or other users' metadata. Treat only `confirmed` as saved in totals. Acquire the backup capture barrier per tool invocation rather than holding it for an MCP stream.

Persist an optional description request ID and normalized input fingerprint with the initial FoodMeal row. A unique user/request index prevents duplicate creation across retries and restarts. Conflicting reuse fails. Clarification tools require the expected round; submitted-round retries cannot answer a newer question. Confirmation retries return an already-confirmed owned meal without recomputing it.

Use the private tunnel with a server-side bearer header and a fixed synthetic WIP user first. This does not expose an unauthenticated production endpoint. Production account selection, owner-approved merge, permanent VM deployment, OAuth for sharing, and other projects are outside this change. Keep the existing read-only MCP unchanged.

## Validation Commands

- `make test-backend`
- `make lint`
- `BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS=--retries=0`
- Protocol smoke on deployed WIP: describe, replay, clarify if needed, confirm, replay confirm, cross-user rejection, and read-back.

### Task 1: Add recoverable meal creation
- [ ] Persist optional bounded request ID and fingerprint atomically with the described meal.
- [ ] Return the original meal for matching replays; reject conflicting inputs.
- [ ] Cover concurrent replays, restart/reload, missing request IDs, and conflicts.
- [ ] Mark completed.

### Task 2: Add fixed-user phone MCP tools
- [ ] Add disabled-by-default dedicated token/user configuration and stateless endpoint.
- [ ] Reuse food handlers under per-call backup locking and fixed user claims.
- [ ] Return bounded structured meal results with accurate status and clarification rounds.
- [ ] Protect clarification and confirmation retries; expose existing retry recovery.
- [ ] Cover auth, identity isolation, workflow, recovery, and backup behavior through protocol tests.
- [ ] Document private-tunnel header configuration and WIP operation.
- [ ] Run Review Gate, deploy and validate WIP, and record the actual validation level.
- [ ] Mark completed.
