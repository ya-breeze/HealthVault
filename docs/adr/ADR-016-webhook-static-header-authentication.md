# ADR-016: Authenticate Health Connect webhook with a dedicated static header token

## Status
Accepted

## Context and Problem Statement

`POST /webhook/{username}` accepts a health-data payload with no authentication. The username
in the URL selects the target account but proves nothing about the sender. The Health Connect
Android sender already supports static HTTP headers, but cannot use interactive Google login.
The VM needs an application-level gate before this endpoint could ever be exposed publicly.

## Decision Drivers

- Reject unauthorized requests before user lookup, payload read, persistence, or ingestion.
- Keep the VM independent of the home Cloudflare Access policy and home network.
- Avoid changing Android code or its settings export under idea #810.
- Leave a missing secret closed, including on a fresh deployment.

## Considered Options

- Reuse the HealthVault session cookie: the background sender has no interactive login session.
- Reuse `HCW_MCP_TOKEN`: this couples unrelated clients and widens the effect of one leaked key.
- Use a dedicated `HCW_WEBHOOK_TOKEN` in `X-HCW-Webhook-Token`: the existing sender can set it,
  and the server can check it before invoking the webhook handler. Chosen.

## Decision Outcome

Require one deployment-wide static token in `X-HCW-Webhook-Token` for every webhook POST. Return
503 when `HCW_WEBHOOK_TOKEN` is unset, whitespace-only, or has surrounding whitespace. Return
401 for a missing, wrong, or duplicate header. Compare fixed-size digests in constant time.
Do not include the token in a URL
or log. A valid header reaches the existing username lookup and payload handling unchanged.

This decision does not create a public route or authorize a Cloudflare Access exception. The
home production sender must be configured with its matching header before this change merges
to the branch it deploys. Android settings export and token storage remain outside this idea;
the existing plain JSON export may disclose a configured static header.

### Consequences

- A missing or incorrectly configured token stops webhook ingestion instead of silently
  accepting unauthenticated data.
- All sender accounts for one deployment share the same token. Rotate it as a coordinated
  server-and-sender change.
- The home production webhook will stop ingesting if this code reaches it before its server
  secret and sender header are configured. Keep this branch unmerged until that gate is met.
