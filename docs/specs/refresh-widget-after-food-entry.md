# Refresh the Android widget after food entry

Idea: ya-breeze/idea-forge#722

## Why

The loaded HealthVault widget uses its whole surface as the primary **Log food** action. That action
opens `/food/upload/` directly in a Chrome Custom Tab from a Glance callback. When the user finishes
logging and closes the tab, no HealthVault activity resumes, so the widget keeps showing the cached
pre-entry calories and macros until periodic work runs or the user presses the separate refresh
button.

The value shown immediately after logging is the value the user is most likely to check. Refresh the
summary as soon as the food-entry tab returns control to HealthVault.

## How

Route both the home-screen and FlexWindow **Log food** callbacks through a small, non-exported,
translucent Android activity. The activity derives the Custom Tab URL from the stored server URL,
launches that intent through the Activity Result API, and enqueues the existing unique one-off
`RefreshWorker` when the Custom Tab closes. It then finishes immediately, returning the user to the
launcher instead of showing the native Today screen.

Keep the existing widget hierarchy and actions: the loaded/stale card still logs food, signed-out
and error cards still open the native app, and the separate refresh affordance still refreshes
without opening food entry. Reuse `RefreshScheduler.enqueueOneOff` so refresh-token coordination,
rate-limit holdoff, failure state, snapshot persistence, and updates to both widget providers remain
in their existing path.

The activity must not accept a URL from an intent. It reads the configured server URL from
`SecureStore`, preserving the current protection against another app redirecting the flow. If no
server URL exists, it closes without launching or refreshing. FlexWindow still starts the bridge on
the main display before the bridge opens the Custom Tab.

This change does not alter widget layout, periodic refresh cadence, the backend API, or food-entry
web behavior. Device acceptance still requires sideloading the debug APK because JVM and lint tests
cannot render or drive a real launcher, Samsung FlexWindow, or browser Custom Tab.

## Validation Commands

- `make test`
- `make lint`
- `make test-e2e`

### Task 1: Add the refresh-on-return bridge

- [x] Add a non-exported translucent activity that opens the stored `/food/upload/` URL through a Custom Tab and enqueues the existing one-off widget refresh only after the tab returns.
- [x] Route both home-screen and FlexWindow Log food actions through the activity while preserving the main-display rule for FlexWindow.
- [x] Add JVM coverage for URL derivation and structural coverage for launch, return, refresh, and finish wiring.
- [x] Mark completed

### Task 2: Validate the Android client and repository

- [x] Run Android JVM tests, lint, and debug APK assembly with the shared Android build environment.
- [ ] Run the repository's static and test gates, and record any environment-limited validation honestly.
- [ ] Mark completed
