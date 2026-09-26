# HealthVault-owned backup jobs API

Idea: [idea #775](https://ideaforge.ikoro.in/idea/775)

## Why

The private VM pilot has a writable SQLite database and uploads, but no verified backup artifact. HealthVault must create a consistent encrypted backup in its own persistent private spool and report success after local verification. TrueNAS collects ready artifacts on its own schedule; HealthVault does not wait for or require an acknowledgement from that pull. The deployment orchestrator calls HealthVault's private API and never reads its database or backup credentials.

## How

Implement the revised [backup API v1 contract](https://github.com/ya-breeze/idea-forge/blob/7c2bdccd46e18cc2a7a966b77877860964ddb161/docs/backup-api-v1.md) inside HealthVault. Keep the HTTP adapter and its per-project service credential separate from user auth. Do not publish `/internal/backups/` through nginx or a public hostname. Empty or incomplete configuration fails closed.

Create durable, idempotent jobs with the contract's exact closed schemas and state transitions. Accept `pre_deploy` and `scheduled` triggers; the caller supplies `target_revision` for correlation. Snapshot SQLite with its online backup mechanism. A process-wide request barrier keeps database rows and upload files stable through snapshot, archive creation, and local verification. This assumes one backend process per data set and no out-of-band database or upload writer. Package a USTAR archive with `manifest.json`, `database.sqlite`, and `uploads/<relative path>` members; exclude the spool. The version 1 manifest records UTC creation time plus each file's relative path, byte size, and SHA-256. Require local SQLite integrity and archive-member checksums before age encryption. Use a project-owned age public recipient, encryption key ID, and a persistent private spool directory (`HCW_BACKUP_SPOOL_DIR`) below the host-backed `/data` volume, defaulting to `/data/backups`. Resolve and reject symlink escapes and reject spool paths that overlap the uploads directory. Publish each ciphertext as `<uuid>.age` with file fsync, atomic rename, and parent-directory fsync. Publish `<uuid>.ready.json` last, containing only the artifact basename, ciphertext byte size, and SHA-256. Report `succeeded` only after local ciphertext size and SHA-256 are verified and both files are durably published. Return the `.age` basename as the opaque ready-file identifier and integrity evidence; never return a path, private key, or credential. TrueNAS periodically pulls artifacts with a matching ready sidecar and sends no acknowledgement. No TrueNAS reader access is configured here; the future pull needs a separately reviewed restricted export or ACL design. The project does not automatically prune published files; retention remains an operator decision. The API accepts scheduled jobs, but installing a VM timer or other scheduled caller is separate. The local temporary workspace uses Go's `os.TempDir()` (normally `/tmp`, or `TMPDIR`) and needs free space of about two backup-set sizes while snapshotting/encrypting. Provide an isolated restore command/drill that cannot target a running or production data directory.

Use a disposable spool and synthetic database for development, conformance, and restore tests. Record the new dependency and data-protection boundary in [ADR-017](../adr/ADR-017-project-owned-encrypted-local-backups.md). Configure neither a TrueNAS route nor public ingress here; the pull schedule and transport are separate infrastructure work. No home storage, real HealthVault data, production deployment, or public ingress is in this change. Deployer integration is also separate.

## Validation Commands

```sh
make test-backend
make lint
cd backend
IDEA_FORGE_ROOT=/path/to/idea-forge go test -tags sqlite_fts5 -run TestIdeaForgeBackupConformanceAgainstDisposableHTTPHandler -v ./pkg/backupapi
go test -tags sqlite_fts5 -run TestRestoreDrill ./pkg/backupapi
```

### Task 1: Private contract and durable jobs

- [x] Add exact v1 routes, constant-time per-project authentication, bounded request validation, idempotency, durable status, and safe errors.
- [x] Test authentication, malformed requests, same-key replay, key conflict, state transitions, restart recovery, and response/log redaction.
- [x] Mark completed.

### Task 2: Project-owned backup set

- [x] Capture SQLite plus uploads consistently and verify the local set before encryption.
- [x] Encrypt and atomically publish a uniquely named artifact in the configured persistent private spool; verify the local ciphertext and publish complete evidence only afterward.
- [x] Test missing uploads, interrupted snapshot, failed encryption/publication, and no plaintext or credential leakage.
- [x] Mark completed.

### Task 3: Restore and integration proof

- [x] Add an isolated restore drill that verifies SQLite integrity and uploaded bytes from a locally published encrypted artifact without touching a live target.
- [x] Run black-box v1 conformance against a disposable instance, including a forced failed job and no TrueNAS acknowledgement step.
- [x] Verify the private routing boundary and document spool configuration and recovery. Do not activate against real data or configure TrueNAS transport here.
- [x] Mark completed.
