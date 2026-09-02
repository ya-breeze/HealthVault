# HealthVault Android client

A thin, read-only native client (`net.ikoro.healthvault`) plus a home-screen widget, both reading
`GET /api/summary/today`. See `docs/specs/build-the-native-android-app-and-its-hom.md` for the
full design, and `docs/adr/ADR-012-android-client-in-repo.md` /
`docs/adr/ADR-013-android-cookie-session-auth.md` for why it's built the way it is.

There is no Play Store release and no release signing configuration — the only deliverable is a
debug APK, built and sideloaded by hand.

## Prerequisites

- **Android Studio** (any recent version) or a standalone Android SDK + JDK 17.
- **Android SDK Platform 34**, plus build-tools matching `compileSdk = 34` in
  `app/build.gradle.kts`.
- No emulator is required to build the debug APK or run the unit tests — this project has no
  instrumented (device/emulator) tests, and none are planned (see ADR-012).

This repository's own build environment (`/data/CLAUDE.md`) has none of the above. `make
test`/`make lint` detect that and print a skip notice for the Android targets rather than failing —
see the Makefile's `test-android`/`lint-android` targets. Building the app for real requires a
machine that does have the SDK, i.e. not this one.

## First-time setup

1. Open `android/` as a project root in Android Studio, **or** point the Gradle wrapper at your SDK
   by creating `android/local.properties`:
   ```
   sdk.dir=/path/to/Android/sdk
   ```
2. From `android/`: `./gradlew tasks` should succeed once the SDK is found.

## Building the debug APK

```
cd android
./gradlew assembleDebug
# APK lands at app/build/outputs/apk/debug/app-debug.apk
```

Equivalently, from the repository root: `make android-apk`.

Install it over ADB (device or emulator, developer mode + USB debugging enabled):

```
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

## Running the unit tests / lint locally

```
cd android
./gradlew testDebugUnitTest   # or: make test-android, from the repository root
./gradlew lintDebug           # or: make lint-android
```

These are plain JVM tests (no emulator) over the parts worth testing in isolation — cookie jar
path/expiry matching, single-flight refresh under concurrent 401s, response parsing, 429 handling,
and the widget's pure state mapping. See ADR-012 for what they do and don't cover.

## Pointing the app at a stack

On first run, the app asks for a server URL, username, and password, and validates them with a
real login call.

- **A LAN stack, e.g. `http://192.168.1.54:8892`:** works out of the box in a **debug** build only
  — `app/src/debug/res/xml/network_security_config.xml` permits cleartext HTTP there. A release
  build enforces HTTPS everywhere (`app/src/main/res/xml/network_security_config.xml`).
- **A public hostname behind Cloudflare Access (e.g. `https://healthvault.ikoro.in`):** requires a
  path-scoped Cloudflare Access **Bypass** policy on `/api/*`. That policy is zone configuration
  outside this repository and is the owner's own step to create (`cloudflare-access` skill, in the
  environment this app was built from) — until it exists, every API call gets an Access login
  challenge instead of JSON, which the app reports distinctly ("this server needs an Access bypass
  on `/api/*`") rather than as invalid credentials. **Until that policy exists, this app only works
  on the LAN.**

## What this app deliberately does not do

Native camera capture, any write path, and native Google Sign-In are all deferred, matching how
idea #12 originally scoped them. Every write still goes through the web UI — **Log food** opens
`<server>/food/upload/` in a Chrome Custom Tab rather than reimplementing the upload flow natively.
