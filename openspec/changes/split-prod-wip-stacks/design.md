## Context

Today there is one Portainer stack (id 38, `data.json` key `hcw-wip`, `class: dogfood`) running `docker-compose.wip.yml`: a hardcoded host bind mount to `/mnt/eight-2/eight-2/data/data/hcw-wip` and hardcoded ports 8888/9888. `docker-compose.yml` exists but is unused by any deployed stack — it uses an anonymous named Docker volume instead of a host path, which doesn't match this environment's convention of keeping per-project data under `/mnt/eight-2/eight-2/data/data/<project>/` (see `/data/CLAUDE.md`, "Per-Project Persistent Data"). `hcw.db` is a live SQLite database in WAL mode; as of 2026-08-08 it is ~310MB with a ~3.6GB `-wal` file.

Stakeholder: the single family using the app today, whose data must not be lost or corrupted during the split.

## Goals / Non-Goals

**Goals:**
- One compose file that can run two independent, simultaneous instances (different data dir, different ports) on the same host, matching the pattern already used by `email-parser-prod`/`email-parser-wip`.
- Zero data loss and minimal downtime for the real family data during the cutover.
- `hcw-prod` ends up indistinguishable in behavior from today's stack (same ports, same data, same env defaults) — only its `class` and deployment discipline change.
- `hcw-wip` starts from a faithful copy of prod's data so it's immediately useful for feature validation.

**Non-Goals:**
- Not changing any application-level behavior, API, or data model.
- Not building a repeatable prod→wip resync mechanism (user explicitly chose a one-time copy; a future resync is out of scope).
- Not hardening `HCW_COOKIE_SECURE`/removing the seed test user on prod — carried forward unchanged from today, since the site is served over plain HTTP and introducing TLS is a separate concern.
- Not giving `hcw-wip` a separate OpenAI API key (user explicitly chose to share prod's key).

## Decisions

**Single parameterized `docker-compose.yml`, `docker-compose.wip.yml` deleted.**
Alternative considered: keep two files (add a third `docker-compose.prod.yml` alongside the existing two). Rejected — it leaves the unused, convention-violating `docker-compose.yml` in place and triples the maintenance surface for what is otherwise identical service definitions. A single file with `HCW_DATA_PATH`/`HCW_HTTP_PORT`/`HCW_HTTPS_PORT` env vars, matching the `email-parser` repo's existing prod/wip split, needs one definition and lets `data.json` carry the only per-stack differences.

**Physical rename: move the data directory and recreate the Portainer stack as `hcw-prod`, rather than leaving it at the old `hcw-wip` path/name.**
Alternative considered: leave the running stack and directory untouched, only relabel `data.json`. Rejected per explicit user preference — despite the small maintenance window, a stack literally named/pathed `hcw-wip` while holding production data is exactly the misleading-name trap `/data/CLAUDE.md` warns about, and the user asked for it fixed at the storage layer, not just in bookkeeping.

**One-time live copy via `sqlite3 .backup`, no ongoing resync tooling.**
`sqlite3 <db> ".backup <dest>"` is safe to run against a live WAL-mode database without stopping writers (it's the documented safe-copy method, unlike `cp`, which can copy a torn/inconsistent file mid-checkpoint). This gives `hcw-wip` a valid starting copy without any downtime for the copy step itself. Per user's choice, no `sync.healthvault` entry or resync skill is being built — HealthVault has no existing backup-archive process the `sync-prod-to-wip` skill's glob-based pattern could reuse anyway, so building one is deferred until actually needed.

**`hcw-wip` gets ports 8892/9892.**
Next free pair after `email-parser-prod` (8891/9891) per `data.json`'s current allocations.

## Risks / Trade-offs

- [Directory move (`mv`) or stack delete/recreate fails partway, leaving prod stopped] → Do the live data copy for `hcw-wip` *before* touching the prod stack at all, so a failure during the prod cutover doesn't also cost the wip seed data. Verify the moved directory's contents (row counts, file sizes) before deleting the old Portainer stack id 38, so there's no window where both the old stack and its data are gone at once without verification.
- [`sqlite3 .backup` on a 310MB DB with a 3.6GB WAL takes longer than expected, or the WAL is unusually large because checkpointing isn't happening] → Note the large WAL size as an existing operational observation (worth a follow-up outside this change), but it doesn't block the backup — `.backup` reads the DB through the SQLite API, which itself reconciles the WAL, so it doesn't matter how large the WAL file is on disk.
- [Recreating the Portainer stack changes its internal Docker network/container names, breaking anything that hardcoded the old container name] → Nothing in this repo or `data.json` references the container by name directly (only by stack/port), so this is low risk; confirmed by inspecting `docker-compose.wip.yml`, which has no `container_name` (per repo convention).
- [`docker-compose.yml`'s env var defaults silently differ from `docker-compose.wip.yml`'s hardcoded values in some field, causing `hcw-prod`'s first auto-redeploy after merge to pick up an unintended default] → `data.json`'s `hcw-prod` entry will pass every env var explicitly (not rely on compose-file defaults) so the deployed values are pinned regardless of file defaults.

## Migration Plan

See `tasks.md` for the ordered, confirmable steps. Summary: copy data for wip first (zero downtime) → merge compose PR → stop old stack → move directory → recreate as `hcw-prod` → deploy new `hcw-wip` stack with the pre-copied data → verify both.

Rollback: until Portainer stack id 38 is deleted, the original stack/data is untouched and redeployable as-is. After the directory move, rollback means moving the directory back and recreating the original stack — this is why verification happens before deleting stack id 38, not after.

## Open Questions

None outstanding — resolved via brainstorming with the user before this proposal was written (rename approach, one-time copy, shared OpenAI key).
