# Ignore Test Webhook Payloads

## Why
The Health Connect Webhook app's Test action sends a clearly marked synthetic payload. HealthVault currently persists its sample measurements as ordinary health records, so a webhook connectivity check can add false readings such as 75.5 kg to the dashboard.

## How
Keep authenticated test payloads in the raw webhook audit log, but do not ingest their measurements. Parse `test` as a JSON boolean: `true` means audit-only, `false` or an absent field preserves normal ingestion, and other JSON types are invalid. Remove existing owner health records only when their source payload is verified as test-marked, and preserve the original raw payload audit rows.

Existing synthetic measurements were removed in a one-time, owner-authorized production cleanup. The matching raw webhook audit rows remain intact. No recurring startup cleanup is needed; the ingestion guard prevents new test payloads from creating records.

## Validation Commands
- `make test-backend`
- `make lint`

### Task 1: Ignore synthetic webhook measurements
- [x] Add regression coverage for test, ordinary, and malformed test flags.
- [x] Delete existing health records linked to verified test-marked payloads while retaining their raw webhook audit records.
- [ ] Verify the deployed WIP endpoint accepts an authenticated test payload without creating health records and continues ingesting a normal payload.
- [ ] Mark completed
