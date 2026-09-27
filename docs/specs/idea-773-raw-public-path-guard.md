# Reject ambiguous raw paths before public API routing
Idea: ya-breeze/idea-forge#773

## Why

The planned VM Access exceptions cover `/api/*` and `/webhook/*`, but the current VM Nginx
routes `/api/..%2fmcp` to `/mcp` and `/webhook/%2e%2e/` to the web page. Cloudflare can match
an encoded request path differently from Nginx. Do not publish either exception until the
origin rejects ambiguous paths before selecting a backend or web route.

## How

Use the original Nginx request target, not normalized `$uri`, to reject encoded separators,
encoded dots, encoded percent signs, and literal backslashes in the path portion of every
request. Reject literal dot segments there too. A raw prefix check alone is insufficient: Cloudflare may decode a
prefix before matching an Access application. Do not inspect the query string; API and webhook
queries may legitimately contain encoded characters. Return a non-redirecting 400 before
proxying to `/mcp`, the backend, or the web fallback. Keep normal API, webhook, readiness,
backup-boundary, and web routes unchanged.

Extend the synthetic portless lab smoke with raw-path probes over HTTP and HTTPS. Preserve its
existing login and readiness assertions. Update the lab runner's exact smoke digest and pin it
to this feature branch so the reviewed test command can run. Do not run the smoke against home
production or real user data.

After local checks, build and run a private synthetic lab stack. Update only the private VM
pilot to the reviewed release and verify its raw-path guard through the WARP-only SSH route.
For the temporary public pilot, connect the outbound tunnel to Nginx over VM loopback HTTP.
Terminate public TLS at Cloudflare; do not expose the self-signed Nginx HTTPS listener directly.
Create a temporary VM-only public hostname only after the owner-approved Access application is
in place, following Access app, DNS, then tunnel ingress. Keep the broad Access gate active
while checking the origin. Add path exceptions only to this temporary hostname for the external
probe. Remove the exceptions before tearing down DNS, ingress, and Access. Never change the
current home hostname or add its bypass as part of this change.

## Validation Commands

- `make test`
- `make lint`
- `python3 -m unittest tools/test_idea773_lab_portainer.py`
- `python3 tools/idea773_lab_portainer.py --help`

### Task 1: Reject ambiguous raw request paths
- [x] Add an early Nginx guard for encoded separators, dots, percent signs, literal backslashes, and literal dot segments in every path portion.
- [x] Keep encoded query strings and ordinary application routes working.
- [x] Add synthetic HTTP and HTTPS probes for rejected paths and existing protected routes.
- [x] Update the lab runner's reviewed command digest and exact branch gate.
- [x] Run local tests and static checks.
- [x] Mark completed

### Task 2: Prove the guard without home exposure
- [x] Run the new committed revision in a portless, synthetic lab stack and inspect its outcomes.
- [x] Update only the private synthetic VM pilot and verify raw-path denial over the private route.
- [x] Stage an Access-gated, VM-only temporary hostname in the approved order and verify its broad gate.
- [x] Probe narrow exceptions on that temporary hostname, then remove them and all pilot resources safely.
- [x] Record the evidence and limitations. Do not switch the home hostname or migrate real data.
- [x] Mark completed

## Evidence and limits — 2026-09-27

HealthVault commit `80f9b5b6d5b3e7852342c17a8d4372aef0c779f6` passed `make test`, `make lint`, the 14 lab-tool unit tests, and the isolated Portainer lab smoke. Android test and lint targets were skipped because this agent container has no Android SDK. The lab stack was `idea-773-healthvault-smoke-80f9b5b` (id 103), with no published ports or persistent mounts; its application containers were stopped after the smoke. The smoke passed HTTP and HTTPS raw-path, readiness, login, backup-boundary, and normal-route probes.

The same branch ran on LAN-only `hcw-wip`, not `hcw-prod`. The first full browser run passed 282 tests, skipped one, and failed eight webhook tests because the test process lacked `HCW_E2E_WEBHOOK_TOKEN`. A second run with the WIP token supplied in memory passed 289, skipped one, and failed one unrelated dashboard request-timing assertion. That assertion passed on a focused first-attempt rerun. This is not a full green E2E result. The WIP stack was restored to its previous `feature/idea-810-webhook-auth` branch and returned HTTP 200.

The guarded synthetic VM launcher accepted the pinned release and rebuilt nginx. The backend stayed healthy on its previous release with UID:GID `997:986`, a read-only root, dropped capabilities, no new privileges, and no published port. Nginx became healthy from the new release and kept only `127.0.0.1:18080` and `127.0.0.1:18443`; the synthetic SQLite inode stayed `524936`. Active `nginx -T` contained both raw-path guards. Both loopback listeners returned 400 for the malformed path matrix, 200 for `/`, `/login/`, and an encoded query, 404 for the blocked backup route, and 401 for a webhook POST without its token.

The temporary `healthvault-vm-pilot.ikoro.in` hostname used the existing outbound `healthvault-vm` tunnel, not the home tunnel. Its Access app was installed and read back before proxied DNS and tunnel ingress. With the broad gate, `/`, `/login/`, `/api/summary/today`, and `/webhook/nonexistent-pilot-user` returned Access 302. Temporary public overrides on only `/api/*` and `/webhook/*` let an unauthenticated API request reach the app's own 401. Ambiguous paths either returned the origin's 400 or stayed behind Access with 302. The webhook returned 401 without or with a wrong token, and 404 for a nonexistent user with the valid VM-only token. This proves routing and authentication boundaries with synthetic data; it does not prove Android delivery or a real-user migration.

The overrides were removed first and both paths returned Access 302 again. The pilot tunnel ingress, DNS record, and Access app were then deleted and their absence read back. A cached public DNS lookup returned Cloudflare 530 with no pilot ingress; the home hostname still returned Access 302. The VM pilot remains private and healthy. No home hostname, home Access policy, real-data VM path, or production stack changed.
