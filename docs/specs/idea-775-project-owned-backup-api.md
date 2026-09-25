# HealthVault-owned backup jobs API

Idea: [idea #775](https://ideaforge.ikoro.in/idea/775)

## Why

The private VM pilot has a writable SQLite database and uploads, but no verified off-host backup. A successful local SQLite copy alone cannot protect it from VM loss or prove that a deployment is safe. The deployment orchestrator must call HealthVault's own private API, never read its database or storage credentials.

## How

Implement the merged [backup API v1 contract](https://github.com/ya-breeze/idea-forge/blob/9a3f4ea019eeece86bb5d12870b38bc9c1ce2326/docs/backup-api-v1.md) inside HealthVault. Keep the HTTP adapter and its per-project service credential separate from user auth. Do not publish `/internal/backups/` through nginx or a public hostname. Empty or incomplete configuration fails closed.

Create durable, idempotent jobs with the contract's exact closed schemas and state transitions. Snapshot SQLite with its online backup mechanism. A process-wide request barrier keeps database rows and upload files stable through snapshot, archive creation, and local verification. This assumes one backend process per data set and no out-of-band database or upload writer. Package a USTAR archive with `manifest.json`, `database.sqlite`, and `uploads/<relative path>` members. The version 1 manifest records UTC creation time plus each file's relative path, byte size, and SHA-256. Require local SQLite integrity and archive-member checksums before age encryption. Use a project-owned age public recipient and generic HTTPS S3-compatible configuration from `HCW_BACKUP_AGE_RECIPIENT`, `HCW_BACKUP_ENCRYPTION_KEY_ID`, `HCW_BACKUP_S3_ENDPOINT`, `HCW_BACKUP_S3_BUCKET`, `HCW_BACKUP_S3_ACCESS_KEY`, `HCW_BACKUP_S3_SECRET_KEY`, and optional `HCW_BACKUP_S3_REGION`. Upload ciphertext only, then stream the remote object back and verify its byte count and SHA-256 before reporting `succeeded`. Keep private keys and storage credentials outside the app response; return opaque evidence only. The local temporary workspace peaks at about two backup-set sizes while snapshotting/encrypting, so provision equivalent free scratch space. Provide an isolated restore command/drill that cannot target a running or production data directory.

Use disposable storage and a synthetic database for development, conformance, and restore tests. Record the new dependency and data-protection boundary in [ADR-016](../adr/ADR-016-project-owned-encrypted-off-host-backups.md). Real off-host provider, bucket, credentials, encryption identity, retention policy, and VM pilot activation require separate configuration and owner approval. No home storage, home network path, real HealthVault data, production deployment, or public ingress is in this change. Deployer integration is also separate.

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
- [x] Encrypt, upload to a configurable off-host destination, verify the remote ciphertext, and publish complete evidence only afterward.
- [x] Test missing uploads, interrupted snapshot, failed encryption/upload/remote verification, and no plaintext or credential leakage.
- [x] Mark completed.

### Task 3: Restore and integration proof

- [x] Add an isolated restore drill that verifies SQLite integrity and uploaded bytes without touching a live target.
- [x] Run black-box v1 conformance against a disposable instance, including a forced failed job.
- [x] Verify the private routing boundary and document the configuration/recovery procedure. Do not activate against real data.
- [x] Mark completed.
