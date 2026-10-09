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
- [ ] Run backend tests, static checks, Review Gate, and WIP validation.
- [ ] Mark completed.
