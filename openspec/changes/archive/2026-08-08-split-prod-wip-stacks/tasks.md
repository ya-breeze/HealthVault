## 1. Repo: consolidate compose files

- [ ] 1.1 Rewrite `docker-compose.yml`: replace the `backend_data` named volume with a host bind mount driven by `HCW_DATA_PATH` (fail fast / no silent default if unset — this must always be supplied per-stack), and make nginx's port mappings use `HCW_HTTP_PORT`/`HCW_HTTPS_PORT` (default to `80`/`443` inside the container as today, but host-side ports must come from the env vars with no hardcoded fallback that could cause two stacks to collide).
- [ ] 1.2 Carry over all existing backend env vars (`HCW_SEED_USERS`, `HCW_JWT_SECRET`, `HCW_COOKIE_SECURE`, `HCW_MCP_TOKEN`, `HCW_OPENAI_API_KEY`, `HCW_OPENAI_MODEL`, `HCW_UPLOADS_DIR`, `HCW_USDA_DB_PATH`) unchanged from `docker-compose.wip.yml`'s definitions.
- [ ] 1.3 Delete `docker-compose.wip.yml`.
- [ ] 1.4 Update any Makefile targets, README, or docs referencing `docker-compose.wip.yml` or the old default ports/paths.
- [ ] 1.5 Commit, push feature branch, open PR containing only this repo change (spec + compose consolidation) for user review.

## 2. Infra: seed WIP data (before touching the live prod stack)

- [ ] 2.1 On the host, `sqlite3 /mnt/eight-2/eight-2/data/data/hcw-wip/hcw.db ".backup /tmp/hcw-wip-seed/hcw.db"` (live copy, zero downtime) — create the destination dir first.
- [ ] 2.2 Copy `usda.db` and the `uploads/` directory into the same staging dir (`cp`/`rsync`, these are static/append-only so a plain copy is fine).
- [ ] 2.3 Verify the staged copy: compare row counts / file sizes against the live source, confirm the sqlite file opens cleanly (`sqlite3 ... "PRAGMA integrity_check;"`).

## 3. Infra: cut prod over (brief downtime, confirm with user before this step)

- [ ] 3.1 Stop the current Portainer stack (id 38).
- [ ] 3.2 `mv /mnt/eight-2/eight-2/data/data/hcw-wip /mnt/eight-2/eight-2/data/data/hcw-prod`.
- [ ] 3.3 Delete Portainer stack id 38 (`/data/portainer.py delete hcw-wip` equivalent) — only after 3.2 is confirmed complete.
- [ ] 3.4 Add a `hcw-prod` entry to `data.json`: `class: prod`, `branch: main`, `compose_file: docker-compose.yml`, ports 8888/9888, `data_path`/`container_path` pointing at the moved directory, full explicit `env` array (all values pinned, not relying on compose defaults), existing `credentials`.
- [ ] 3.5 `/data/portainer.py git-deploy hcw-prod` to create the new stack; `/data/portainer.py wait hcw-prod`.
- [ ] 3.6 Verify: log into `hcw-prod` at its URL with existing credentials, confirm existing data (meals, records) is visible.

## 4. Infra: stand up WIP stack

- [ ] 4.1 Move the staged copy from Task 2 into the (now-vacated) `/mnt/eight-2/eight-2/data/data/hcw-wip` path.
- [ ] 4.2 Add an `hcw-wip` entry to `data.json`: `class: wip`, `branch: main`, `compose_file: docker-compose.yml`, ports 8892/9892, `data_path`/`container_path` pointing at the new dir, own `HCW_JWT_SECRET`/`HCW_MCP_TOKEN`, same `HCW_OPENAI_API_KEY`/`HCW_OPENAI_MODEL` as prod (per approved design), `credentials` matching the seeded data's existing test user.
- [ ] 4.3 `/data/portainer.py git-deploy hcw-wip` to create the new stack; `/data/portainer.py wait hcw-wip`.
- [ ] 4.4 Verify: log into `hcw-wip` at its new URL with the same credentials, confirm the copied data is visible and independent (writes here must not appear in `hcw-prod`).

## 5. Validation

- [ ] 5.1 Run the existing Playwright E2E suite (`e2e/`) against `hcw-wip` only (`BASE_URL=http://192.168.1.54:8892`).
- [ ] 5.2 Update port-assignment docs (`/data/CLAUDE.md` table if applicable) to reflect 8892/9892.
- [ ] 5.3 Update `docs/superpowers/` or other in-repo deployment docs that reference the old single-stack topology, if any exist.

## 6. Close out

- [ ] 6.1 Get `/code-review` on the branch (tier 2); address findings.
- [ ] 6.2 Get user approval to finalize.
- [ ] 6.3 `openspec archive split-prod-wip-stacks --yes`; commit and push.
- [ ] 6.4 Get `/code-review` again on the post-archive HEAD.
- [ ] 6.5 Merge PR to `main` with squashing, per user's go-ahead.
