# Show Saved Weather on the Dashboard and by Day

## Why

Approximate-location weather history is collected, but the owner cannot inspect it. Show a compact summary and open day details on demand, preserving visible gaps and the time of each observation.

## How

Use the authenticated weather-history API, without changing collection or retention. Query the last seven days for the latest saved hour, label its date/time explicitly, and never call it current weather. Web adds a compact dashboard entry and a /weather/day/ page with a date picker, temperature and pressure charts, remaining values and coverage gaps. Charts must not connect across missing hours. Keep the saved-hour list collapsed by default; expand individual source details inside it. Use the account timezone where available; accept a clearly labelled timezone passed by Android so its latest-hour day link opens the matching day. Clamp today's history query to the current time; handle daylight-saving day lengths. Android adds a compact Today row and opens the day page in Custom Tabs using the phone timezone. Reading stored history remains available with collection off. Weather failures do not hide nutrition or other health data. Health correlation overlays and causal conclusions are deferred.

## Validation Commands

`make test-frontend`

`make lint`

`python3 /data/android-build.py HealthVault-worktrees/weather-display/android "testDebugUnitTest lintDebug assembleDebug"`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS="tests/weather-display.spec.ts --retries=0"`

### Task 1: Display saved weather and preserve missing evidence

- [x] Add typed weather history reads and latest-hour helpers with valid units, date/time and no invented current values.
- [x] Add the compact web dashboard entry and day view with temperature/pressure charts, humidity, wind, precipitation and honest empty/error/gap states.
- [x] Keep details behind a click, use English/Russian strings and preserve dashboard customization behavior.
- [x] Add an Android Today weather row, session-safe reads and a timezone-aware link to web day details; collection settings stay separate.
- [x] Test latest-hour selection, timezone/DST bounds, gaps, error states and account/auth behavior at existing API/helper/UI seams.
- [ ] Validate mobile layouts, native builds and a deployed WIP interaction; complete the Review Gate and restore WIP before handoff.
