# ADR-016: HealthVault Owns Encrypted Off-Host Backup Sets

## Status

Proposed

## Context and Problem Statement

The deployment orchestrator must block a change when HealthVault cannot prove that its current SQLite data and meal-photo uploads have a recoverable off-host copy. The orchestrator must not acquire database access, object-store credentials, or an encryption identity. HealthVault uses SQLite WAL and stores referenced photos as separate files, so copying either one without coordinating writes can produce an unusable set.

## Decision Drivers

- Keep data operations and credentials inside the project boundary.
- Fail closed before any deployer mutation if remote verification fails.
- Include both SQLite and referenced uploads in one recoverable set.
- Keep the VM unable to decrypt an uploaded backup on its own.
- Support an isolated restore drill before using real data.

## Considered Options

- **A central backup coordinator with database and storage access.** Rejected: it broadens privileges across projects and repeats the earlier mistaken design.
- **A local SQLite copy or object-store upload acknowledged by PUT alone.** Rejected: it does not prove off-host recoverability.
- **A project-owned private job API with an encrypted archive and remote read-back.** Chosen.

## Decision Outcome

HealthVault implements the private backup API v1 contract. One process-wide request barrier excludes application writes while SQLite's online backup and upload archive are captured; the contract assumes one backend process and no out-of-band writer. A single active runner bounds temporary disk use. The archive contains a versioned manifest, SQLite snapshot, and uploads. HealthVault checks local integrity, encrypts the archive with an age public recipient, uploads ciphertext through the MinIO S3-compatible client, and streams the remote object back to verify size and SHA-256 before reporting success.

The age private identity stays outside the application and VM runtime configuration. The CLI restore drill accepts it through standard input, verifies the object in a new private `/tmp` directory, and never accepts a destination data path. The application never exposes the API through nginx. Unconfigured storage and all verification failures block deployment.

### Consequences

- `filippo.io/age` and `github.com/minio/minio-go/v7` become backend dependencies.
- A full backup needs about two backup-set sizes of free temporary space and may pause application writes during capture.
- Multiple backend replicas or direct writers require a new cross-process coordination decision before use.
- Provider retention/immutability, credentials, key recovery, and actual VM activation remain separate operator decisions and tests.
- The synthetic HTTP conformance and restore drill do not prove a real S3 provider or real-data recovery; those need separate validation before activation.
