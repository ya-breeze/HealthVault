# Keep server diagnostics for one year
## Why
The owner wants a year of client telemetry for later analysis. The existing 30-day window and 1000-event cap discard history too early.
## How
Retain events for 365 days from server receipt time. Remove the per-user row cap so frequent syncs do not shorten the window. Keep self-only authentication, bounded upload batches, latest-100 API responses, and the Android local queue unchanged. Reads hide expired events; ingestion prunes expired events for the current user. Inactive expired rows can remain on disk until ingestion. Update the Android README and append a dated retention update to ADR-020. Do not modify the frozen original spec. Production deployment requires owner approval.
## Validation Commands
`make test-backend`
`make lint`
`make test-e2e E2E_ARGS='tests/auth.spec.ts --retries=0' BASE_URL=<wip-url>`
### Task 1: Extend retention
- [x] Apply one shared 365-day retention duration to reads and pruning; remove the row cap.
- [x] Verify reads retain events older than 30 days, pruning removes events older than 365 days, and ingestion retains more than 1000 events.
- [x] Run backend tests, static checks, Review Gate, and WIP validation.
- [x] Mark completed.

Validation: `make test-backend` and `make lint` pass (Android lint visibly skips because no Android code changed and no SDK is present). Three native review angles pass. Claude peer ran direct inspection but did not complete the required review subagent; record peer unavailable, without retry. WIP stack50 runs implementation d9308b070774b2f72879b12f0223e460c03c49bc, backend image8ebe3020d1d796867d843b93509f254ed690f65cd874bfcb76d056900d33a2d4, both services healthy. Deployed diagnostics login/ingest/read checks pass; seven authentication E2E tests pass without retry after installing the required Chromium cache. Production remains unchanged pending approval.
