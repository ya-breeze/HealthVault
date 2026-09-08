# ADR-012: The Android Client Lives In This Repository, With a Build That Skips Without an SDK

## Status
Accepted

## Context and Problem Statement

`GET /api/summary/today` (`backend/pkg/server/summary_today.go`) and the login hardening in
`backend/pkg/server/login_limiter.go` (ADR-009) both exist for a native Android client — idea #12 —
that was never built. ADR-009 names it directly: "the Android Widget (idea #12) needs a Bypass
policy on `/api/*`... Login hardening must land before that Bypass policy is safe to create." That
prerequisite shipped; the client did not, until this change.

The earlier spec for this idea called the client "other-repo work" and named a
`healthvault-android` repository. That repository does not exist, and this environment
(`/data/CLAUDE.md`) has no Android SDK, no `java`, and no `gradle` — this container cannot compile,
lint, or test a Kotlin/Gradle project regardless of which repository it lives in.

## Decision Drivers

- `android/`'s only real API contract is `summaryTodayResponse` — one Go struct, hand-mirrored by
  `android/app/src/main/kotlin/net/ikoro/healthvault/api/TodaySummary.kt`, with no shared schema
  between them. A breaking change to that struct should be visible in the same pull request as the
  client code it breaks.
- This is a personal, single-user deployment (`/data/CLAUDE.md`'s scale guidance) — a second
  repository, its own CI, and its own release process is overhead nothing here asks for.
- No environment available to this pipeline has an Android SDK today, and none is expected to
  gain one. A gate that silently no-ops when the SDK is absent is only honest if that absence is
  stated outright, not designed around.

## Considered Options

- **A separate `healthvault-android` repository**, as the earlier spec assumed. Keeps this
  repository free of Gradle/Kotlin tooling, but splits the client from the one contract it depends
  on: a change to `summaryTodayResponse` can merge here with nothing failing there, and the two
  repositories drift until someone notices at runtime — on a widget that updates unattended, in the
  background, with no one watching.
- **In-repo, under `android/`, as a self-contained Gradle project.** The same pull request touches
  both sides of the contract. Chosen.

## Decision Outcome

Chosen: **the Android client lives at `android/` in this repository**, as a self-contained Gradle
project (`net.ikoro.healthvault`) that does not affect the backend's Docker build context
(`android/` is in `.dockerignore`).

`make test` and `make lint` gain `test-android` and `lint-android`, running `./gradlew
testDebugUnitTest` and `./gradlew lintDebug`. Both print a visible skip notice and exit 0 when
`ANDROID_HOME` is unset and `android/local.properties` is absent — the condition true in every
environment this pipeline runs in today. **This is a real, named gap, not a workaround**: on a
machine without the SDK, `make test` is green while proving nothing about `android/`. Two things
narrow it without closing it:

- The Android unit tests (`android/app/src/test/kotlin/...`) are plain JVM tests — no emulator, no
  Robolectric — over the parts worth testing in isolation: `SessionCookieJar`'s domain/path/expiry
  matching, `RefreshInterceptor`'s single-flight refresh, response parsing, 429 backoff, and
  `widgetState()`. They run correctly wherever a JDK and Gradle exist, which is not this container.
- `e2e/tests/summary-today.spec.ts` pins the `summaryTodayResponse` contract in this repository's
  own Playwright suite, against a real deployed backend. A change that breaks the contract
  `TodaySummary.kt` parses fails `make test-e2e` here, even though nothing here can compile the
  Kotlin that would also break.

Neither substitutes for actually building the app. The debug APK (`make android-apk`, or `./gradlew
assembleDebug` directly) is built and sideloaded by the owner on a machine that has the SDK — there
is no CI runner for it, and none is planned. **There is no automated device coverage, and there
will not be one.**

### Consequences

- A Kotlin compile error, a Gradle/AGP version mismatch, or a Glance/WorkManager API drift in
  `android/` can sit on `main` for an arbitrary length of time before the owner next builds the
  APK by hand and discovers it. Nothing in this pipeline's gate will ever catch it.
- The two narrowing measures above catch a different, narrower class of regression — a broken
  *contract* between the backend and the client's models, and broken *pure logic* the JVM tests
  exercise directly. Neither one exercises Compose UI, the Glance widget's rendering, WorkManager
  scheduling, or the Keystore-backed encrypted store's production path
  (`androidx.security.crypto.EncryptedSharedPreferences`) — see ADR-013 for why the store's tests
  use a fake `SharedPreferences` instead.
- If this environment ever gains an Android SDK, `test-android`/`lint-android` start running for
  real with no further change — the skip condition is the SDK's absence, not a flag someone has to
  remember to flip.
