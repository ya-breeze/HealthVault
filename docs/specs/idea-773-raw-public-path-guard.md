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
- [ ] Add an early Nginx guard for encoded separators, dots, percent signs, literal backslashes, and literal dot segments in every path portion.
- [ ] Keep encoded query strings and ordinary application routes working.
- [ ] Add synthetic HTTP and HTTPS probes for rejected paths and existing protected routes.
- [ ] Update the lab runner's reviewed command digest and exact branch gate.
- [ ] Run local tests and static checks.
- [ ] Mark completed

### Task 2: Prove the guard without home exposure
- [ ] Run the new committed revision in a portless, synthetic lab stack and inspect its outcomes.
- [ ] Update only the private synthetic VM pilot and verify raw-path denial over the private route.
- [ ] Stage an Access-gated, VM-only temporary hostname in the approved order and verify its broad gate.
- [ ] Probe narrow exceptions on that temporary hostname, then remove them and all pilot resources safely.
- [ ] Record the evidence and limitations. Do not switch the home hostname or migrate real data.
- [ ] Mark completed
