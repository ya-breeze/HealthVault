# Repeatable HealthVault synthetic Portainer smoke

Use this runner only for the disposable readiness/login lab. It does not deploy
to `hcw-wip` or production, and it does not create a public route. It obtains
Portainer credentials from the existing local registry helper without printing
them. The runner refuses dirty worktrees and existing stacks with the same
revision-derived name. It never updates or deletes stacks or images.

## Commands

Run from the committed Idea 773 worktree on a host with Python 3, PyYAML, the
shared `/data/Useful/ai/truenas/portainer.py` helper, and Portainer access:

```sh
cd /data/HealthVault-worktrees/idea-773-backend-readiness
python3 tools/idea773_lab_portainer.py check
python3 -m unittest tools.test_idea773_lab_portainer -v
```

`check` is local and read-only. It prints the source revision, the two tracked
file allowlist sizes, and the safety properties of the in-memory Compose
transform. Review that output and send the normal Idea/project coordination
intent before creating a stack. `run` is restricted to the clean
`feature/idea-773-backend-readiness` branch. Then run:

```sh
python3 tools/idea773_lab_portainer.py run
```

`run` builds `healthvault-lab-backend:<short-sha>` and
`healthvault-lab-nginx:<short-sha>` through the shared Portainer build helper,
then creates `idea-773-healthvault-smoke-<short-sha>` with the standalone
string-stack API. The endpoint is fixed to the reviewed `ENDPOINT_ID` from the
shared Portainer helper; the runner rejects a different value. It always uses
the repository containing the runner, not a caller-supplied path. The backend
build context contains only tracked `backend/`
files, rooted at the context directory. The Nginx context contains only tracked
`frontend/` files needed by `COPY frontend .`, plus `nginx/Dockerfile` and
`nginx/nginx.conf`. Both contexts reject `.env*`, credential-like names,
private-key/certificate files, and database files. The runner tars these files
in memory; it never sends the repository root or an unrestricted tar archive.

Before stack creation, the runner verifies that no stack with that exact name
or image with either exact revision tag exists. It refuses collisions instead
of overwriting/reusing resources. Before stack creation, it transforms
`docker-compose.lab.yml` in memory:

- It replaces image tags with the revision-specific built tags and removes all
  build directives.
- It removes the template's named `lab-data` volume and all service mounts.
- It uses a 512 MiB `/data` tmpfs for the backend.
- It removes the smoke service's opt-in profile so the one-shot runs on startup.
- It preserves the internal-only lab network and publishes no ports.

The runner validates the effective containers after smoke exit: no published
ports, no host or persistent mounts, and the exact backend `/data` tmpfs. It
waits up to six minutes for `lab-smoke`, prints its exit status and demultiplexed
logs, and stops only the backend and Nginx containers whose Compose project and
service labels match the stack it just created, after successful validation.
It retains the stopped stack and both images for inspection. If smoke fails or
times out, it leaves the stack intact for diagnosis. It never removes
containers, stacks, volumes, or images. It deliberately has no general-purpose
stop or deployment-target option; that prevents a typo from stopping or
mutating an unrelated stack.

Use the explicit retained stack name to inspect an earlier revision:

```sh
python3 tools/idea773_lab_portainer.py status --stack-name idea-773-healthvault-smoke-15cbbd5
python3 tools/idea773_lab_portainer.py logs --stack-name idea-773-healthvault-smoke-15cbbd5
```

Replace `15cbbd5` with the seven-character commit SHA when inspecting a newer
retained revision. `status` and `logs` accept only revision-scoped Idea 773 lab
stack names and only read them. A successful `run` stops its newly created
backend and Nginx containers after verification; no runner action stops an
earlier stack. A failed run may leave its exact new stack running; coordinate
with the owner, inspect labels and IDs, then use a one-off reviewed Portainer
stop request for only those exact backend/Nginx container IDs. There is no
cleanup command. Request explicit owner approval before deleting retained
resources.

## Evidence and limits

The 2026-09-24 run from commit `15cbbd5` created stack
`idea-773-healthvault-smoke-15cbbd5` (Portainer stack ID 99). Its synthetic
login/session smoke exited 0. Nginx and backend became healthy; no service
published a host port or mounted a host path. Docker inspect reported backend
`HostConfig.Tmpfs={"/data":"size=536870912"}` and `Mounts=[]`. After
verification, the backend and Nginx containers were stopped; stack 99 and its
images remain retained. The first client-side assertion incorrectly expected
tmpfs to appear in `Mounts`; the correct Docker inspect field is
`HostConfig.Tmpfs`. The initial frontend/Nginx image build failed at `npm run
build`; available diagnostics did not establish a cause. The unchanged
allowlisted-context retry succeeded. Do not treat the first failure as resolved:
if a future VM build repeats it, inspect build logs and resource status before
retrying.

This exercises the internal readiness denial and synthetic login path through
Nginx. It does not prove host-published-port behavior, real-user migration,
food-photo placement, backup/restore, VM isolation, firewall rules, or any
home-to-VM route. No home or production data was included.

The current shared `nginx/Dockerfile` also embeds `192.168.1.54` as a
self-signed certificate SAN. The synthetic smoke uses `curl --insecure` and
does not validate that certificate for a real hostname. Keep this image and
certificate lab-only; do not treat the Nginx image as portable VM ingress or
create a public hostname from it. A separate paused child Idea tracks the
project-owned ingress/TLS configuration needed to remove this home-LAN
assumption without changing existing home WIP behavior.
