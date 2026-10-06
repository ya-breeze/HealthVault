# Show Saved Weather on the Dashboard and by Day

## Why

Approximate-location weather history is collected, but the owner cannot inspect it. Show a compact summary and open day details on demand, preserving visible gaps and the time of each observation.

## How

Use the authenticated weather-history API, without changing collection or retention. Query the last seven days for the latest saved hour, label its date/time explicitly, and never call it current weather. Web adds a compact dashboard entry and a /weather/day/ page with a date picker, temperature and pressure charts, remaining values and coverage gaps. Charts must not connect across missing hours. Keep the saved-hour list collapsed by default; expand individual source details inside it. Use the account timezone where available; accept a clearly labelled timezone from the page URL for explicit day links. Clamp today's history query to the current time; handle daylight-saving day lengths. Android weather display and Internal Testing publication are deferred at the owner's request. Reading stored history remains available with collection off. Weather failures do not hide nutrition or other health data. Health correlation overlays and causal conclusions are deferred.

## Validation Commands

`make test-frontend`

`make lint`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS="tests/weather-display.spec.ts --retries=0"`

### Task 1: Display saved weather and preserve missing evidence

- [x] Add typed weather history reads and latest-hour helpers with valid units, date/time and no invented current values.
- [x] Add the compact web dashboard entry and day view with temperature/pressure charts, humidity, wind, precipitation and honest empty/error/gap states.
- [x] Keep details behind a click, use English/Russian strings and preserve dashboard customization behavior.
- [x] Test latest-hour selection, timezone/DST bounds, gaps, error states and authenticated history reads at existing web API/helper/UI seams.
- [x] Validate mobile web layouts and a deployed WIP interaction; complete the Review Gate and restore WIP before handoff.
