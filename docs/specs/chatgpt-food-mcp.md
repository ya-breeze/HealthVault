# Record meals from ChatGPT through a private MCP tunnel

## Why

The owner verified the OpenAI Secure MCP Tunnel demo from ChatGPT on the web and phone. HealthVault already accepts meal descriptions, but its administrative MCP only reads data. A phone connection needs a fixed user identity, the existing meal review workflow, and safe recovery after an interrupted tool call.

## How

Add a separate stateless `/phone-mcp` endpoint and an exact nginx proxy route for WIP access. Require a dedicated bearer token and configured user UUID; disable it when either is absent. Resolve identity on every call. Expose only describe, get, clarify, confirm, and retry meal tools. Reuse the existing HTTP food handlers and their ownership checks. Return meal status, items, clarification questions, and nutrition totals without raw model output or other users' metadata. Return preview nutrition for drafts without changing daily totals. Treat only `confirmed` as saved in totals. An explicit request to record a clearly specified meal authorizes confirmation; ask when portions are uncertain. Acquire the backup capture barrier per tool invocation rather than holding it for an MCP stream.

Persist an optional description request ID and normalized input fingerprint with the initial FoodMeal row. A unique user/request index prevents duplicate creation across retries and restarts. Conflicting reuse fails. Clarification tools require the expected round and question version; submitted-round retries cannot answer a newer question. Confirmation retries return an already-confirmed owned meal without recomputing it.

Use the private tunnel with a server-side bearer header and a fixed synthetic WIP user first. This does not expose an unauthenticated production endpoint. Production account selection, owner-approved merge, permanent VM deployment, OAuth for sharing, and other projects are outside this change. Keep the existing read-only MCP unchanged.

## Validation Commands

- `make test-backend`
- `make lint`
- `BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS=--retries=0`
- Protocol smoke on deployed WIP: describe, replay, clarify if needed, confirm, replay confirm, cross-user rejection, and read-back.

### Task 1: Add recoverable meal creation
- [x] Persist optional bounded request ID and fingerprint atomically with the described meal.
- [x] Return the original meal for matching replays; reject conflicting inputs.
- [x] Cover concurrent replays, restart/reload, missing request IDs, and conflicts.
- [x] Mark completed.

### Task 2: Add fixed-user phone MCP tools
- [x] Add disabled-by-default dedicated token/user configuration and stateless endpoint.
- [x] Reuse food handlers under per-call backup locking and fixed user claims.
- [x] Return bounded structured meal results with accurate status and clarification rounds.
- [x] Protect clarification and confirmation retries; expose existing retry recovery.
- [x] Cover auth, identity isolation, workflow, recovery, and backup behavior through protocol tests.
- [x] Document private-tunnel header configuration and WIP operation.
- [x] Run Review Gate, deploy and validate WIP, and record the actual validation level.
- [x] Mark completed.


## Validation results

Implementation commit: `1a1ea6ea66f477100feb9bdb3cd553add7262ca0`. HealthVault WIP stack 50 runs the feature branch. The running backend binary matches the independently built pinned-source image: SHA-256 `4fb229477e5bbd2dbcb91e888dd5db606f44c6fd78e10bb479c328e8bfe4f06e`. The running nginx configuration matches the committed file. Production and the VM are unchanged.

`make test-backend` and `make lint` pass. Android lint is skipped because no SDK is available; this change touches no Android code. Native correctness, standards, and spec/test reviews are clean. Claude's system peer review completed. Its valid replay, draft-nutrition, and coverage findings were addressed and verified by the final native review; no unaddressed finding remains.

The full WIP browser run exercised 326 tests: 315 passed, one skipped, and ten failed. Eight failures were caused by an omitted test webhook credential; two timed out. The ten failed tests were rerun serially with the credential supplied through the environment, `--retries=0`, and traces: all ten passed in 19.8 seconds. Thus 325 distinct tests passed across the full run and targeted rerun. Secret-bearing trace archives were removed after validation.

Deployed protocol validation passed initialization, exactly five tools, a description draft, persistent request replay, confirmation, confirmation replay, read-back, conflicting request rejection, caller-selected-user rejection, missing-meal rejection, and missing-bearer rejection. The synthetic 100 g banana meal returned 89 kcal after confirmation. The validation record was deleted through the owned data API, and its absence was verified. Clarification generations and returned-version transitions are covered by HTTP protocol tests.

The temporary tunnel runtime now targets WIP `/phone-mcp` with its dedicated server-side bearer. Control-plane polling works. ChatGPT discovery and a phone meal command still require the owner to refresh the existing personal plugin and test it. The runtime's readiness diagnostic reports malformed optional OAuth discovery because WIP nginx returns the SPA for metadata paths; this does not gate the client's Noauth JSON-RPC dispatcher. Permanent deployment should target the private backend directly, where absent metadata returns 404. No production account is configured.
