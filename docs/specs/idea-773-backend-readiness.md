# HealthVault backend readiness stays internal
Idea: ya-breeze/idea-forge#773

## Why

The current root page can return HTTP 200 while the backend or its SQLite database is unavailable. Compose only orders Nginx after the backend container starts; it does not wait for database readiness. HealthVault needs a probe that reports backend and database readiness without exposing health data or creating a public API for deployment state.

## How

Add an unauthenticated `GET /internal/ready` route to the backend. It runs `SELECT COUNT(*) FROM users` through `db.WithContext(ctx)` with a 2-second context deadline and returns only HTTP 200 or a generic HTTP 503. A missing database handle or any query error returns 503. The route returns no database details or user data.

Keep the route off `/api`. Nginx proxies the exact readiness path only from `127.0.0.1` and denies other clients on both listeners. Do not add real-IP rewriting. The backend and Nginx runtime images include curl so their healthchecks can make bounded GET requests; curl must fail on every non-2xx response. The backend check uses `http://127.0.0.1:8080/internal/ready`, matching Nginx's existing backend port. Nginx's healthcheck uses `http://127.0.0.1:80/internal/ready`, not its self-signed HTTPS listener. Both healthcheck commands use `--max-time 3` and Compose gives them a 5-second timeout. The backend check runs every 10 seconds, has 3 retries, and has a 2-minute start period for migrations and backfill. The Nginx check runs every 30 seconds, has 3 retries and a 10-second start period. Compose makes Nginx depend on backend `service_healthy`; failure to become healthy blocks the stack startup.

The lab file belongs here because parent Idea 768 needs VM-independent evidence for the readiness boundary and login path, without touching any shared stack or real data. Add a standalone `docker-compose.lab.yml` named `healthvault-lab` for a synthetic smoke run. It uses the same build contexts, a project-scoped named data volume, and an internal-only network. It publishes no host ports and mounts no host or home data. Set `HCW_PORT` to `8080`, use a fixed lab-only JWT secret, set `HCW_COOKIE_SECURE=false` for the internal HTTP smoke, and seed only the throwaway `lab` user. These committed test credentials are not secrets and must never be reused outside the disposable lab volume. A one-shot `lab-smoke` service uses the backend image and waits for Nginx `service_healthy`. It checks that `/login/` loads, that other containers get HTTP 403 for `/internal/ready` on ports 80 and 443, then logs in through Nginx and verifies the authenticated `/api/users/me` route. This verifies the Nginx peer-IP boundary from another container; because the lab publishes no ports, it does not exercise the host-published-port route. Document these manual commands for a machine with Docker Compose:

```sh
docker compose -f docker-compose.lab.yml --profile smoke up --build --abort-on-container-exit --exit-code-from lab-smoke
docker compose -f docker-compose.lab.yml --profile smoke down --volumes --remove-orphans
```

No public hostname, Access rule, or production Compose ingress changes in this work. This subtask does not deploy to WIP or production. The Docker CLI is unavailable in the implementation environment, so local Go/TypeScript checks and source review can run here; image builds, Compose rendering, and the smoke run remain unverified until a Docker host is available.

## Validation Commands

- `make test`
- `make lint`

### Task 1: Add private SQLite-backed readiness and container health semantics
- [ ] Add the backend readiness handler and unit tests for a successful schema query, a query error, a missing database handle, a bounded timeout, and unauthenticated access.
- [ ] Add an exact-match Nginx readiness location restricted to loopback clients.
- [ ] Add backend and Nginx healthchecks with explicit curl GET behavior, container-local addresses, bounded timeout/retry/start-period values, and Nginx depending on backend `service_healthy`.
- [ ] Add the portless, host-data-free synthetic lab Compose file and its one-shot readiness/login smoke service, using only lab credentials and a named volume.
- [ ] Run `make test` and `make lint`; inspect the final Compose and Nginx configuration for public-route leakage. Record the Docker-render/build/smoke gap if Docker remains unavailable.
- [ ] Mark completed
