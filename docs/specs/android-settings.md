# Separate Android Settings From Daily Actions

## Why

Weather setup and its explanation occupy most of the Today screen. The owner wants daily actions and settings separated, with details visible only on demand.

## What

- [ ] Keep Today focused on the nutrition summary and Add food; open Settings from its header.
- [ ] Move Weather history and Sign out into a separate Settings screen with header and system Back navigation.
- [ ] Collapse the long weather explanation by default. Show a short status, a compact enable/disable control, and More details.
- [ ] Keep a brief background/location disclosure visible before enabling. Preserve the separate Android permission steps and clearing pending observations when disabling.
- [ ] Preserve settings navigation across rotation and reset it when signing out or starting a new session.
- [ ] Verify the Android build, lint and existing unit tests; review the complete change.

## How

Use the existing Compose activity host with a saveable Settings flag and BackHandler. Keep permission and collection logic in WeatherConsent. Show its detailed explanation through a saveable disclosure. Use existing Material components and English/Russian strings. No backend or weather data changes.
