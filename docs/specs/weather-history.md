# Background weather history

## Why

The owner wants weather history at the user's location to compare with wellbeing and health measurements. A timezone does not identify a location. Historical health uploads do not establish where the phone was when those measurements occurred. Preserve uncertainty rather than assigning home weather to travel or missing-location periods.

## How

Add voluntary background approximate location collection to the native Android app, independent of widgets and health-data uploads. Default it off. Explain its purpose before requesting coarse foreground location, then request background access separately. Use an hourly WorkManager job; Android can delay work. Never promise exact sampling or detection of trips between observations. Do not request precise location. Round coordinates to 0.1 degrees on the phone before durable storage or transmission. Retain observation time, accuracy and a stable UUID. Accept only fresh fixes (15 minutes), including elapsed-realtime validation. Collect without requiring network; persist a bounded encrypted account-scoped queue, retry unchanged observations, and clear consent/queue on sign-out or account replacement. Disabling cancels work and clears pending uploads. Recheck consent, permissions and session identity around collection/upload.

Authenticated POST /api/weather/locations accepts one observation: id (UUID), observed_at (RFC3339 UTC), latitude, longitude (rounded to 0.1 degrees), accuracy_m. It derives user/family from authentication and accepts observations up to 14 days old, at most five minutes ahead. Reject malformed/non-finite values. Retry of identical identity/content is idempotent; conflicting UUID reuse fails. Never log exact coordinates. GET /api/weather/history?from=RFC3339&to=RFC3339 returns the current user's weather and explicit coverage gaps, with a bounded range. Keep weather separate from medical measurements.

Use adjacent observations from the same user to establish conservative intervals: at most six hours apart, accuracy at most 10 km, and distance plus both uncertainty radii at most 25 km. Save complete UTC hourly intervals contained between compatible observations. A first observation alone establishes no weather interval. Moving, poor-accuracy and long-gap pairs create gaps rather than weather. Resume after a compatible pair at the new location. Late observations invalidate and recompute affected coverage; uniqueness prevents duplicate hourly records. Never infer an unobserved trip did not occur.

Fetch model-based historical weather from Open-Meteo for confirmed intervals: temperature, apparent temperature, relative humidity, surface pressure, mean-sea-level pressure, precipitation and wind speed, with explicit units, source/model, coordinates used, fetched time, and observation provenance. Do not store forecasts as observed health facts. A provider outage retains pending work for automatic retry. Limit HTTP duration, response size, processing batches and history range. Reject incomplete/non-finite or misaligned provider output rather than writing zero values. Server background persistence must participate in the existing backup capture barrier. The Android job must not wait for the external provider. No historical weather backfill without historical location evidence.

This change stores and exposes weather for later analysis. It does not add medical advice, correlation claims, a weather dashboard, air quality, or production deployment. Enabling the feature is optional and unrelated to health ingestion. Testing permissions and actual background execution on a real phone remains an explicit owner acceptance check; a build cannot prove it.

## Validation Commands

`make test-backend`, `make lint`, Android `testDebugUnitTest lintDebug assembleDebug` through `/data/android-build.py`, and `make test-e2e` against reserved hcw-wip. Add a deployed authenticated API test for storage, isolation, validation, idempotence, travel gaps and weather retrieval. Validate the real Open-Meteo transport separately. Run the native Review Gate and attempt Claude system code-review once per gate. Publish a branch PR; do not merge or deploy production without owner approval.

### Task 1: Persist location evidence and weather coverage
- [ ] Add user/family-scoped location, hourly weather and coverage storage, with automatic migration.
- [ ] Add authenticated ingestion/history endpoints and conservative coverage decisions including late observations.
- [ ] Add bounded Open-Meteo enrichment and restart-safe retries, compatible with backup capture.
- [ ] Test validation, isolation, repeated/conflicting uploads, UTC hours, travel/uncertainty/gaps, late data and provider failures.
- [ ] Mark completed.

### Task 2: Collect approximate location in the background
- [ ] Add voluntary consent and separate foreground/background permission flow.
- [ ] Schedule hourly collection independently of widgets; bound acquisition and reject stale fixes.
- [ ] Round before persistence, queue offline observations encrypted, and upload with existing session recovery.
- [ ] Stop and clear pending data on disable/sign-out/account replacement; test session and queue boundaries.
- [ ] Build, lint and test Android using the shared build container.
- [ ] Mark completed.

### Task 3: Review and validate the integrated change
- [ ] Document the data model and provider choice in a proposed ADR and describe phone acceptance checks.
- [ ] Pass backend/static checks and the Review Gate.
- [ ] Deploy the reviewed feature branch only to hcw-wip and pass deployed E2E/API validation.
- [ ] Publish the PR with exact validation results and unverified phone acceptance; restore/release WIP.
- [ ] Mark completed.
