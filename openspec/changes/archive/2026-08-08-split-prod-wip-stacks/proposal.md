## Why

HealthVault currently has exactly one deployed instance (Portainer stack "hcw-wip", classified `dogfood`) holding real, irreplaceable family health data. There is no separate WIP environment: every feature branch that needs live validation is deployed straight onto the instance holding real data, and there is no isolated place to run the E2E suite or try risky changes without touching that data. Splitting into a `prod` instance (tracks `main` only, real data) and a `wip` instance (any branch, its own data, seeded from a copy of prod) removes that risk and gives the project a normal two-tier deployment topology like its sibling projects.

## What Changes

- Consolidate `docker-compose.yml` and `docker-compose.wip.yml` into a single parameterized `docker-compose.yml`: host bind-mount data path via `HCW_DATA_PATH`, ports via `HCW_HTTP_PORT`/`HCW_HTTPS_PORT`. All other backend env vars (`HCW_SEED_USERS`, `HCW_JWT_SECRET`, `HCW_COOKIE_SECURE`, `HCW_MCP_TOKEN`, `HCW_OPENAI_API_KEY`, `HCW_OPENAI_MODEL`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH`) remain env-driven as they already are.
- Delete `docker-compose.wip.yml`. **BREAKING** for anyone deploying this repo from the old hardcoded compose file — the new file requires `HCW_DATA_PATH`, `HCW_HTTP_PORT`, `HCW_HTTPS_PORT` to be supplied by the deployer (Portainer stack env, per `data.json`).
- (Outside this repo, after merge) Split the single deployed instance into two Portainer stacks — `hcw-prod` (real data, current ports 8888/9888, tracks `main` only) and `hcw-wip` (fresh stack, ports 8892/9892, seeded from a one-time copy of prod's data). This infra migration is tracked in `tasks.md` but has no in-repo artifact of its own beyond the compose file this proposal changes.

## Capabilities

### New Capabilities
- `deployment-config`: the backend/nginx stack's data directory and exposed ports must be fully configurable via environment variables (`HCW_DATA_PATH`, `HCW_HTTP_PORT`, `HCW_HTTPS_PORT`), so the same compose file can run multiple independent instances (prod, wip) side by side on one host.

### Modified Capabilities
(none — no application-level request/response behavior changes)

## Impact

- `docker-compose.yml` (rewritten), `docker-compose.wip.yml` (deleted).
- No backend or frontend code changes — `HCW_DATA_PATH`/`HCW_HTTP_PORT`/`HCW_HTTPS_PORT` are consumed at the compose/nginx-port-mapping level, not read by the Go backend itself (which already reads `HCW_DBPATH`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH` directly).
- `/data/data.json` (outside this repo): replace the single `hcw-wip` deployment entry with `hcw-prod` and `hcw-wip` entries.
- Two Portainer stacks instead of one (infra change, executed after this PR merges — see `tasks.md`).
