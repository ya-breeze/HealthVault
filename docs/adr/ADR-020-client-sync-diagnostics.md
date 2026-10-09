# ADR-020: Send bounded client diagnostics to HealthVault

## Status

Accepted

## Decision

Use the existing cookie session and encrypted Android preferences for a bounded technical journal and outbox. Send acknowledged batches to an authenticated self-only HealthVault endpoint after a successful sync or explicit submission. Correlate requests with UUIDs and server route-template logs. Store only closed metadata fields; exclude payloads, raw exceptions, personal identifiers, credentials and medical data. Clear local records on session replacement. Retain server events for at most 30 days on reads, prune on ingestion, and cap each user's stored journal at 1000 records.

## Consequences

The server owns the evidence without a third-party telemetry SDK. Offline evidence survives process restarts but can be lost through retention, queue bounds or sign-out. A failed diagnostic upload never changes the primary sync result. Reporting remains unavailable until a newer server accepts the endpoint. Alerts and crash reporting remain separate work.
