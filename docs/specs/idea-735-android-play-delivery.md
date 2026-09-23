# Local Google Play Internal delivery

Idea: ya-breeze/idea-forge#735

## Why

HealthVault already has a native Android app, but its release build has a fixed version code and no
release signing configuration. Installing downloaded APK files by hand is awkward and does not
scale to the owner's planned additional apps. The shared local Android delivery service can build
a signed App Bundle and publish it to Google Play Internal Testing, but each app must expose a
small public contract and consume the standard signing inputs.

## How

Add `android-delivery.json` at the repository root for package `net.ikoro.healthvault`. The contract
selects the existing `android/` project, `:app:bundleRelease`, Java 17, and the generated release
bundle. Allocate delivery version codes from a high app-local base so repeated source revisions
remain stable and do not collide with the current development version code.

Update `android/app/build.gradle.kts` so a delivery build reads
`androidDeliveryVersionCode` and `androidDeliveryVersionName` from Gradle properties. Require the
properties as a pair. When they are present, require the four standard delivery signing environment
variables, configure the release variant with that keystore, and never log their values. Ordinary
debug builds and existing unsigned local release builds keep their current behaviour.

The upload key, passwords, and Google service-account JSON stay outside this repository. The
owner-only local registration and the typed Infisical record are platform state, not application
source. Publishing stays restricted to Google Play Internal Testing by the shared delivery service.

Update the Android README and the `android-apk` Makefile comment so they no longer claim that
HealthVault has no release signing or that this control environment cannot build Android. Keep the
manual debug APK path documented as an available development path. Name the existing trusted-LAN
HTTPS hostname as the supported release target, while keeping the public Cloudflare Access route
unsupported. Add a dated update to ADR-014 because its statement that this environment cannot
build Android is now stale; the original accepted decision stays unchanged.

## Validation Commands

- `make lint`
- `make test`
- `python3 -m json.tool android-delivery.json`
- `python3 /data/Useful/ai/truenas/android-delivery/android_delivery.py inspect-config android-delivery.json`
- `python3 /data/android-build.py HealthVault-worktrees/idea-735-android-play-delivery/android testDebugUnitTest`
- `python3 /data/android-build.py HealthVault-worktrees/idea-735-android-play-delivery/android lintDebug`
- Build `:app:bundleRelease` through the shared builder with a disposable keystore and delivery
  version properties, then inspect the resulting AAB signature.
- Deploy the branch to `hcw-wip`, wait for readiness, then run `make test-e2e` against that stack.

### Task 1: Add the application delivery contract

- [x] Add and validate the public delivery contract.
- [x] Wire delivery versioning and release signing into the Android Gradle build.
- [x] Mark completed.

### Task 2: Update operator guidance and verify the delivery build

- [x] Update the Android README and Makefile comment for local Internal Testing delivery.
- [x] Prove existing Android unit tests and lint still pass.
- [x] Prove invalid delivery configuration fails closed.
- [x] Prove a signed release AAB builds without storing the key in the checkout.
- [x] Document the supported release HTTPS target and update the stale ADR-014 facts.
- [x] Mark completed.

### Task 3: Validate the deployed branch

- [x] Deploy the branch to `hcw-wip` without displacing another active Idea.
- [x] Wait for stack readiness and run the repository E2E suite against it.
- [x] Mark completed.

### Task 4: Meet the current Google Play target API requirement

The first live readiness upload on 2026-09-23 proved the signing, service-account access, and
bundle build path, then Google Play rejected the bundle because it targeted API 34. Update the
app to compile and target API 36, with mutually supported Gradle, Android Gradle plugin, and
Kotlin plugin versions. Preserve `minSdk = 26`. Apply safe-drawing insets to both activity screens
because targeting API 35 or newer enforces edge-to-edge system bars. Resolve build-script
deprecations and override-parameter warnings exposed by the Kotlin upgrade without changing
runtime behavior. Re-run Android unit tests, lint, the Review Gate, and the non-publishing Play
readiness check.

- [x] Upgrade the Android SDK and mutually supported build plugins.
- [x] Keep setup and today content outside enforced system-bar insets.
- [x] Re-run Android validation and the non-publishing Google Play readiness check.
- [x] Mark completed.
