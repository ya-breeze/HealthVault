# HealthVault local backup configuration and restore drill

The restore drill decrypts and verifies one locally published backup in a private temporary directory. It never installs files, changes SQLite, or accepts a restore destination.

## Configure the private backup service

Supply these runtime variables through the deployment's secret manager. `docker-compose.yml` passes them to the backend and leaves the backup runner unconfigured when any required value is empty.

| Variable | Purpose |
| --- | --- |
| `HCW_BACKUP_API_TOKEN` | Per-project bearer credential for the private v1 API. |
| `HCW_BACKUP_SPOOL_DIR` | Persistent private directory for ciphertext and ready sidecars. Defaults to `/data/backups`, on the host-backed `/data` volume. |
| `HCW_BACKUP_AGE_RECIPIENT` | Age public recipient used to encrypt backup sets. |
| `HCW_BACKUP_ENCRYPTION_KEY_ID` | Opaque identifier recorded in successful evidence. |

The backend writes a uniquely named `<uuid>.age` ciphertext and a `<uuid>.ready.json` sidecar. The sidecar contains only `artifact_file`, `ciphertext_size_bytes`, and `ciphertext_sha256`. HealthVault fsyncs and atomically renames the ciphertext first, then publishes and fsyncs the sidecar last. Collectors must enumerate only ready sidecars, validate the artifact basename, then verify the copied ciphertext size and SHA-256. An orphan `.age` file without a matching sidecar is incomplete and must be ignored. HealthVault does not automatically delete published artifacts because the TrueNAS pull sends no acknowledgement; choose and operate retention separately.

The spool must stay below `/data`, must not overlap the uploads directory, and must be writable only by the HealthVault service. The application resolves symlinks before accepting the path. Backup archives contain the SQLite snapshot and uploads, not the spool or its prior artifacts. The directory is required to have mode `0700`; `.age` files and sidecars are created with mode `0600`. No TrueNAS reader has access today. Those modes intentionally prevent a separate SSH account or group from reading the spool. A future collector needs a separately reviewed restricted export or ACL design that preserves these private-service boundaries; do not grant it access to the application database or API credential or loosen the modes as part of this feature.

Keep the age private identity outside the HealthVault service. The application uses only the public recipient. The backup API remains private backend-to-backend traffic: nginx returns a JSON 404 for `/internal/backups/` and does not proxy it. Do not add that prefix to a public hostname or ingress rule.

Before enabling real backups, provision private writable scratch storage with free space for roughly two full backup-set sizes. The backup runner uses Go's `os.TempDir()` (normally `/tmp`, or the directory selected by `TMPDIR`) for its temporary snapshot, archive, and ciphertext; confirm the configured container path has enough capacity. The in-process request barrier supports one backend process per data set; multiple replicas or out-of-process database/upload writers are not supported. The API accepts both `pre_deploy` and `scheduled` jobs. This feature does not install a VM timer or configure the separate TrueNAS pull schedule.

## Run a restore drill

Use a completed job's `ready_file_id`, manifest SHA-256, ciphertext SHA-256, and ciphertext byte count from its successful API evidence. Run the command where the configured private spool is mounted. Feed the private age identity through standard input from an approved secret manager; do not put it in command arguments or save it under the application data directory.

```sh
age-identity-provider | \
hcw backup-restore-drill \
  --ready-file-id '<ready_file_id>' \
  --manifest-sha256 '<manifest_sha256>' \
  --ciphertext-sha256 '<ciphertext_sha256>' \
  --ciphertext-size-bytes '<ciphertext_size_bytes>'
```

Replace `age-identity-provider` with the approved secret manager's command that writes the identity to standard output. Do not substitute a path or put the identity in a flag.

The command reads one age identity from standard input, holds it in memory, verifies the ready sidecar and ciphertext, and decrypts into a new mode-0700 temporary directory under `/tmp` (ignoring `TMPDIR`). It removes that exact directory when it exits. The decrypted USTAR archive is materialized only there with mode-0600 files. The command rejects unsafe paths and non-regular members, verifies the manifest, every member checksum, SQLite integrity, referenced photo paths, and uploaded bytes, then prints a credential-free JSON report. A failed check exits nonzero. A successful report verifies recoverability of the selected artifact; it does not install that data into a running instance.

For an actual recovery, preserve the original artifact and evidence, provision a fresh isolated HealthVault data root, and use a separately reviewed recovery procedure to install the verified database and uploads there. Never point this drill at a live or production directory. Restricted TrueNAS access to the spool, its transport, pull schedule, retention policy, VM activation, and real-data recovery are outside this change and remain blocked on a separate access design.

## Validation in this repository

`go test` runs the restore roundtrip and hostile-archive tests. Set `IDEA_FORGE_ROOT` to an IdeaForge checkout and run `IDEA_FORGE_ROOT=/path/to/idea-forge go test -tags sqlite_fts5 -run TestIdeaForgeBackupConformanceAgainstDisposableHTTPHandler -v ./pkg/backupapi` from `backend/` to invoke its Python 3 black-box runner. Without that variable, the cross-repository test skips so a standalone HealthVault checkout can run its ordinary tests. The runner uses a disposable HTTP handler and a synthetic local spool, including a forced failed job; it does not prove TrueNAS collection or real-data recovery. The server tests assert that nginx's backup location returns 404 without proxying or falling through to the SPA.
