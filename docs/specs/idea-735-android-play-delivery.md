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
manual debug APK path documented as an available development path.

## Validation Commands

- `make lint`
- `make test`
- `python3 -m json.tool android-delivery.json`
- `python3 /data/Useful/ai/truenas/android-delivery/android_delivery.py inspect-config android-delivery.json`
- `python3 /data/android-build.py HealthVault-worktrees/idea-735-android-play-delivery/android testDebugUnitTest`
- Build `:app:bundleRelease` through the shared builder with a disposable keystore and delivery
  version properties, then inspect the resulting AAB signature.

### Task 1: Add the application delivery contract

- [ ] Add and validate the public delivery contract.
- [ ] Wire delivery versioning and release signing into the Android Gradle build.
- [ ] Mark completed.

### Task 2: Update operator guidance and verify the delivery build

- [ ] Update the Android README and Makefile comment for local Internal Testing delivery.
- [ ] Prove existing Android unit tests and lint still pass.
- [ ] Prove invalid delivery configuration fails closed.
- [ ] Prove a signed release AAB builds without storing the key in the checkout.
- [ ] Mark completed.
