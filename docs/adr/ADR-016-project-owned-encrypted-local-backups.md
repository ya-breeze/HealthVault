# ADR-016: HealthVault Owns Encrypted Local Backup Artifacts

## Status

Proposed

## Context and Problem Statement

HealthVault must create a consistent encrypted backup of its current SQLite data and meal-photo uploads in a persistent private spool. The project reports success after locally verifying and durably publishing that artifact. TrueNAS periodically pulls ready files independently; the project does not wait for or require an acknowledgement. HealthVault uses SQLite WAL and stores referenced photos as separate files, so copying either one without coordinating writes can produce an unusable set.

## Decision Drivers

- Keep data operations and encryption configuration inside the project boundary.
- Include both SQLite and referenced uploads in one recoverable set.
- Keep the VM unable to decrypt a published backup on its own.
- Support an isolated restore drill before using real data.

## Considered Options

- **A central backup coordinator with database and storage access.** Rejected: it broadens privileges across projects and repeats the earlier mistaken design.
- **A local SQLite copy without validation or encryption.** Rejected: it does not prove a coherent, protected backup set.
- **A project-owned private job API with an encrypted archive in a persistent private spool.** Chosen.

## Decision Outcome

HealthVault implements the private backup API v1 contract. One process-wide request barrier excludes application writes while SQLite's online backup and upload archive are captured; the contract assumes one backend process and no out-of-band writer. A single active runner bounds temporary disk use. The archive contains a versioned manifest, SQLite snapshot, and uploads; it excludes the backup spool. HealthVault checks local integrity, encrypts the archive with an age public recipient, then writes ciphertext into the configured private spool. The Compose default is `/data/backups`, under the persistent `/data` volume, and the application rejects paths outside that volume or overlapping the uploads directory. It reports success only after verifying the ciphertext size and SHA-256, syncing the file, atomically publishing a unique ready filename, and syncing the spool directory. The spool and files remain mode `0700` and `0600`; no TrueNAS reader access is configured. A future pull requires a separately reviewed restricted export or ACL design. Published artifacts are not automatically pruned by the backup job.

The age private identity stays outside the application and VM runtime configuration. The CLI restore drill accepts it through standard input, verifies a selected file in a new private `/tmp` directory, and never accepts a destination data path. The application never exposes the API through nginx. Unconfigured spool storage and all verification or publication failures report a failed job. A deploy caller may continue to fail closed on that state, but this artifact is not yet a TrueNAS copy.

### Consequences

- `filippo.io/age` becomes a backend dependency.
- A full backup needs about two backup-set sizes of free temporary space and may pause application writes during capture.
- Multiple backend replicas or direct writers require a new cross-process coordination decision before use.
- Restricted TrueNAS spool access and transport, schedule, retention policy, credentials, key recovery, and actual VM activation remain separate operator decisions and tests.
- The synthetic HTTP conformance and restore drill do not prove TrueNAS collection, disaster recovery from another host, or real-data recovery; those need separate validation before activation.
