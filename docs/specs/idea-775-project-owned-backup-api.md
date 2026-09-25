# HealthVault-owned backup jobs API

Idea: [idea #775](https://ideaforge.ikoro.in/idea/775)

## Why

The private VM pilot has a writable SQLite database and uploads, but no verified off-host backup. A successful local SQLite copy alone cannot protect it from VM loss or prove that a deployment is safe. The deployment orchestrator must call HealthVault's own private API, never read its database or storage credentials.

## How

Implement the merged [backup API v1 contract](https://github.com/ya-breeze/idea-forge/blob/9a3f4ea019eeece86bb5d12870b38bc9c1ce2326/docs/backup-api-v1.md) inside HealthVault. Keep the HTTP adapter and its per-project service credential separate from user auth. Do not publish `/internal/backups/` through nginx or a public hostname. Empty or incomplete configuration fails closed.

Create durable, idempotent jobs with the contract's exact closed schemas and state transitions. Snapshot SQLite consistently using its online backup mechanism. Include uploads in the same backup set and verify references from the captured database; fail a job if a referenced upload cannot be captured consistently. Package a manifest and data, encrypt before leaving the project boundary, upload to a configurable off-host object store, and verify the remote ciphertext checksum before reporting `succeeded`. Keep encryption and storage credentials project-owned; return opaque evidence only. Provide an isolated restore command/drill that cannot target a running or production data directory.

Use disposable storage and a synthetic database for development, conformance, and restore tests. Real off-host provider, bucket, credentials, encryption identity, retention policy, and VM pilot activation require separate configuration and owner approval. No home storage, home network path, real HealthVault data, production deployment, or public ingress is in this change. Deployer integration is also separate.

## Validation Commands

```sh
make test-backend
make lint
# Run IdeaForge's backup_conformance against a disposable HealthVault instance and store.
# Run the isolated restore drill and verify database contents plus uploaded bytes.
```

### Task 1: Private contract and durable jobs

- [ ] Add exact v1 routes, constant-time per-project authentication, bounded request validation, idempotency, durable status, and safe errors.
- [ ] Test authentication, malformed requests, same-key replay, key conflict, state transitions, restart recovery, and response/log redaction.
- [ ] Mark completed.

### Task 2: Project-owned backup set

- [ ] Capture SQLite plus uploads consistently and verify the local set before encryption.
- [ ] Encrypt, upload to a configurable off-host destination, verify the remote ciphertext, and publish complete evidence only afterward.
- [ ] Test missing uploads, interrupted snapshot, failed encryption/upload/remote verification, and no plaintext or credential leakage.
- [ ] Mark completed.

### Task 3: Restore and integration proof

- [ ] Add an isolated restore drill that verifies SQLite integrity and uploaded bytes without touching a live target.
- [ ] Run black-box v1 conformance against a disposable instance, including a forced failed job.
- [ ] Verify the private routing boundary and document the configuration/recovery procedure. Do not activate against real data.
- [ ] Mark completed.
