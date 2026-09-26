# Authenticate the Health Connect webhook on the VM

Idea: ya-breeze/idea-forge#810

## Why

`POST /webhook/{username}` currently accepts a payload based only on a username in the URL.
A public caller could submit health data as that user. The Android sender already supports a
static HTTP header, so HealthVault can authenticate the request without changing Android code.

## How

Read a dedicated `HCW_WEBHOOK_TOKEN`. Require it in `X-HCW-Webhook-Token` before user lookup,
body parsing, persistence, or ingestion. Return a non-success response when the server token is
unset, the header is missing, or the value is wrong. Compare token bytes in constant time and
never put the token in a URL or log. Pass the variable through Compose with an empty default;
the empty default disables the webhook rather than leaving it open.
Treat a token with surrounding whitespace as invalid server configuration instead of letting
every sender receive a misleading unauthorized response.

Keep Android settings export/import, secret storage, sender changes, and frontend setup hints
out of scope. Keep
Cloudflare DNS, tunnel ingress, public exposure, and home production out of scope. This branch
must not merge into the home production deployment until its webhook sender and server secret
are coordinated; merging without that setup would stop ingestion there.
Record this new security boundary in a Proposed ADR. Add a dated update to the earlier
Cloudflare sign-in ADR, which described the webhook as unauthenticated when it was written.

## Validation Commands

- `make test-backend`
- `make lint`
- `make test-e2e E2E_ARGS='--retries=0 tests/data-types.spec.ts tests/dashboard.spec.ts'`

### Task 1: Enforce backend webhook authentication

- [x] Add the dedicated server setting and fail-closed header check before handler side effects.
- [x] Add unit coverage for unset, missing, wrong, and correct tokens and for no handler call on rejection.
- [x] Record the security decision in a Proposed ADR and qualify the historical ADR.
- [x] Mark completed

### Task 2: Validate against a private WIP deployment

- [x] Pass the token through Compose and make webhook E2E requests send the WIP-only token.
- [x] Test rejection and successful synthetic ingestion on the private WIP stack without public ingress.
- [x] Run backend tests, static checks, and the Review Gate before WIP deployment.
- [x] Mark completed
