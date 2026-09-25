# HealthVault backup configuration and restore drill

The restore drill proves that one completed backup can be downloaded, decrypted, and verified without writing into HealthVault's data directory. It never installs files, changes SQLite, or accepts a restore destination.

## Configure the private backup service

Supply these runtime variables through the deployment's secret manager. `docker-compose.yml` passes them to the backend and leaves the backup runner unconfigured when required values are empty.

| Variable | Purpose |
| --- | --- |
| `HCW_BACKUP_API_TOKEN` | Per-project bearer credential for the private v1 API. |
| `HCW_BACKUP_AGE_RECIPIENT` | Age public recipient used to encrypt backup sets. |
| `HCW_BACKUP_ENCRYPTION_KEY_ID` | Opaque identifier recorded in successful evidence. |
| `HCW_BACKUP_S3_ENDPOINT` | HTTPS S3-compatible origin, without a path or query. |
| `HCW_BACKUP_S3_BUCKET` | Project-owned backup bucket. |
| `HCW_BACKUP_S3_ACCESS_KEY` | Project-scoped object-store access key. |
| `HCW_BACKUP_S3_SECRET_KEY` | Project-scoped object-store secret. |
| `HCW_BACKUP_S3_REGION` | Optional S3-compatible region. |

Keep the age private identity outside the HealthVault service. The application uses only the public recipient. The backup API remains private backend-to-backend traffic: nginx returns a JSON 404 for `/internal/backups/` and does not proxy it. Do not add that prefix to a public hostname or ingress rule.

Before enabling real backups, provision writable temporary storage with space for roughly two full backup-set sizes. The in-process request barrier supports one backend process per data set; multiple replicas or out-of-process database/upload writers are not supported.

## Run a restore drill

Use a completed job's opaque `remote_object_id`, manifest SHA-256, ciphertext SHA-256, and ciphertext byte count from its successful API evidence. Run the command in an isolated environment with the same S3 settings and encryption key ID. Feed the private age identity through standard input from an approved secret manager; do not put it in command arguments or save it under the application data directory.

```sh
age-identity-provider | \
hcw backup-restore-drill \
  --object-id '<remote_object_id>' \
  --manifest-sha256 '<manifest_sha256>' \
  --ciphertext-sha256 '<ciphertext_sha256>' \
  --ciphertext-size-bytes '<ciphertext_size_bytes>'
```

Replace `age-identity-provider` with the approved secret manager's command that writes the identity to standard output. Do not substitute a path or put the identity in a flag.

The command reads one age identity from standard input, holds it in memory, downloads the ciphertext into a new mode-0700 temporary directory under `/tmp` (ignoring `TMPDIR`), and removes that exact directory when it exits. The decrypted USTAR archive is materialized only there with mode-0600 files. The command rejects unsafe paths and non-regular members, verifies the ciphertext, manifest, every member checksum, SQLite integrity, referenced photo paths, and uploaded bytes, then prints a credential-free JSON report. A failed check exits nonzero. A successful report verifies recoverability of the selected object; it does not install that data into a running instance.

For an actual recovery, preserve the original object and evidence, provision a fresh isolated HealthVault data root, and use a separately reviewed recovery procedure to install the verified database and uploads there. Never point this drill at a live or production directory. Real provider credentials, retention policy, VM activation, and real-data recovery are outside this change.

## Validation in this repository

`go test` runs the restore roundtrip and hostile-object tests. Set `IDEA_FORGE_ROOT` to an IdeaForge checkout and run `IDEA_FORGE_ROOT=/path/to/idea-forge go test -tags sqlite_fts5 -run TestIdeaForgeBackupConformanceAgainstDisposableHTTPHandler -v ./pkg/backupapi` from `backend/` to invoke its Python 3 black-box runner. Without that variable, the cross-repository test skips so a standalone HealthVault checkout can run its ordinary tests. The runner uses a disposable HTTP handler and in-memory store, including a forced upload failure; it does not prove the VM's S3 credentials or actual provider. The server tests assert that nginx's backup location returns 404 without proxying or falling through to the SPA.
