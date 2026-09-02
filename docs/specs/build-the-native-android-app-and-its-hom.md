# Native Android app and home-screen widget

Idea: ya-breeze/idea-forge#25

## Why

The backend already carries a feature built for a client that does not exist. `GET /api/summary/today` (`backend/pkg/server/summary_today.go`, registered at `backend/pkg/server/server.go:93`) returns today's consumed macros, meal count, last-logged timestamp, display language and embedded Nutrition Target in one authenticated call. Its doc comment says why: "the endpoint exists so a widget makes one cheap call instead of three". Nothing calls it. The login limiter and the process-wide verification cap (`backend/pkg/server/login_limiter.go`, ADR-009) exist for the same absent client — ADR-009 names "the Android Widget (idea #12)" as the reason `/api/auth/login` had to survive being placed on the open internet.

So the prerequisite change landed and the thing it was a prerequisite for did not. Today the owner answers "how many calories so far" by unlocking the phone, opening a browser, passing Cloudflare Access, waiting for the static Next.js export to load, and reading the dashboard's Food Card. That is a 20-second round trip for a number that fits in a 2x2 home-screen cell, and it is the single most-repeated read in the whole application.

Why now: the blocking work is done on both sides. `daily-summary-and-login-rate-limiting` merged, so the summary endpoint and the login hardening are on `main`. The pipeline defect that stranded this idea (#28) is fixed. What remains is client work.

## How

**Placement.** The app lives in this repository, at `android/`, as a self-contained Gradle project. The earlier spec called it "other-repo work" and named a `healthvault-android` repository, which does not exist. A second repository would split a client from the API contract it consumes, so a breaking change to `summaryTodayResponse` could merge here with nothing failing there. In-repo, the same pull request touches both sides. Recorded as ADR-012.

Package and application ID are `net.ikoro.healthvault`. The literal reverse of `ikoro.in` is unusable — `in` is a Kotlin hard keyword and would need backticks in every file.

**Auth mirrors the browser, because the server only supports that.** `RequireAuth` validates the `kin_access` cookie exclusively; the pinned `kin-core` v0.1.0 has no `Authorization: Bearer` path, and adding one is out of scope here. The client therefore keeps a real cookie jar: `POST /api/auth/login` yields `kin_access` (15 minutes) and `kin_refresh` (365 days, scoped by kin-core to `Path=/api/auth/refresh`), and `POST /api/auth/refresh` rotates both. The jar must honour path scoping, so `kin_refresh` is never sent to `/api/summary/today`, and must persist across process death, since a widget update runs in a freshly started process almost every time.

Two consequences drive the design. First, `authdb.RotateRefreshToken` **consumes** the token it is given, so two concurrent refreshes spend an already-rotated token and sign the device out. The frontend hit exactly this and fixed it in `frontend/lib/api.ts` with `coordinatedRefresh(dispatchedAt)` plus a `lastRefreshAt()` guard; `frontend/lib/api.test.ts` is mostly about that rule. The Android client reproduces the same rule with a mutex and a recorded completion time: a 401 for a request dispatched **before** the last successful refresh means retry, not refresh. Second, a rotated token that is not durably written before the app is killed is gone forever, so the jar commits synchronously inside the refresh call, before the retried request is issued.

When refresh fails outright the client re-logs in once from stored credentials, then reports a signed-out state. That is why the password is stored at all: a home-screen widget that silently stops updating until the owner happens to open the app is worse than a widget that recovers itself. Credentials, cookies and the last summary snapshot sit in one Keystore-backed AES-GCM encrypted store. The residual risk — an attacker holding the unlocked, rooted device — is accepted and recorded in ADR-013.

**The widget** is a single Glance `AppWidgetProvider` with `SizeMode.Responsive`, not two separate widgets: the owner places one widget and resizes it, rather than choosing a size in the picker and having to delete and re-add to change it. The compact layout (about 110x110dp, a 2x2 cell) shows consumed calories against target calories and nothing else. The wide layout (about 250x110dp, a 4x2 cell) adds four macro bars and a **Log food** button. The wide layout reserves a single-line slot below the bars for the recommendation `GET /api/summary/today` already models and always returns as null; the slot renders only when the field is non-null, so shipping the recommendation later needs no re-layout and today's widget wastes no space on it.

Refresh is one call per update. WorkManager runs a 30-minute periodic worker (the platform floor is 15 minutes) only while at least one widget is placed, plus one-off updates on widget placement, on manual refresh, and when the app resumes. A 429 is honoured through `Retry-After` — `writeTooManyAttempts` sends both the header and `retry_after_seconds`, and the header is the one an unattended client should read. On any failure the widget keeps rendering the last snapshot with a staleness marker rather than blanking; a snapshot older than 6 hours is marked stale, and a signed-out session renders a sign-in prompt.

**The app is thin and read-only.** First run asks for a server URL and credentials and validates them by attempting a login. Everything else is one native today screen: calories, four macros against target or the target's structured unavailability reason, meal count, and relative last-logged time. **Log food** opens `<server>/food/upload/` in a Chrome Custom Tab — the trailing slash is required, since `frontend/next.config.ts` sets `trailingSlash: true` on a static export. The Custom Tab URL is always derived from the stored server URL and never from an intent extra, so no other app can drive it to an arbitrary page. Every write still goes through the web UI.

The app ships English and Russian strings and follows the caller's Display Language when `display_language` names a shipped language, matching `shippedUILanguages` in `backend/pkg/server/display_language.go`; anything else falls back to the device locale. Cleartext HTTP is permitted only in the debug build, through a network security config, so a LAN stack like `http://192.168.1.54:8892` stays testable while release builds require HTTPS.

**Gate wiring, and the gap in it.** `make test` and `make lint` gain `test-android` and `lint-android`, which run `./gradlew testDebugUnitTest` and `./gradlew lintDebug`. Both **skip with a printed notice** when no Android SDK is present (`ANDROID_HOME` unset and no `android/local.properties`), because this environment has no Android toolchain. State this plainly: on a machine without the SDK, `make test` is green while proving nothing about `android/`. Two things narrow the gap rather than closing it. The Android unit tests are plain JVM tests over the parts worth testing — cookie jar path and expiry matching, single-flight refresh under concurrent 401s, response parsing, 429 backoff, and a pure `widgetState()` mapping — so they need no emulator and run wherever the SDK exists. And a new Playwright case pins the contract the widget parses, so a change to `summaryTodayResponse` fails `make test-e2e` in this repository even when nothing here can compile Kotlin. The APK itself is built and sideloaded by the owner on a machine with the SDK; there is no automated device coverage, and there will not be one.

**Excluded on purpose.** Native camera capture, any write path, and native Google Sign-In stay deferred exactly as idea #12 settled them. There is no Play Store release and no release signing configuration; the deliverable is a debug APK. The path-scoped Cloudflare Access **Bypass** policy on `/api/*` is zone configuration outside this repository and remains the owner's step — until it exists, the app reaches the API only on the LAN. The client detects that case specifically: a redirect to `*.cloudflareaccess.com` or an HTML body where JSON was expected surfaces as "this server needs an Access bypass on /api/*", never as "invalid credentials". `RequireAuth` still gains no Bearer path.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Android module and build wiring

- [x] Add the Gradle project under `android/`: `settings.gradle.kts`, root `build.gradle.kts`, `gradle.properties`, `gradle/libs.versions.toml`, the wrapper, and `app/build.gradle.kts` with application ID `net.ikoro.healthvault`, Kotlin, Compose, and Glance
- [x] Declare dependencies in the version catalogue: Compose, Glance for app widgets, WorkManager, OkHttp, kotlinx.serialization, and AndroidX Browser for Custom Tabs
- [x] Add `app/src/main/AndroidManifest.xml` with the internet permission, `MainActivity`, the widget receiver, and a debug-only network security config permitting cleartext
- [x] Add `test-android` and `lint-android` Makefile targets that run `./gradlew testDebugUnitTest` and `./gradlew lintDebug`, and print a visible skip notice when `ANDROID_HOME` is unset and `android/local.properties` is absent
- [x] Wire those targets into `test` and `lint`, and add an `android-apk` target producing the debug APK
- [x] Add Gradle build output, `local.properties` and `.gradle/` to `.gitignore`, and `android/` to `.dockerignore` so the backend image build context is unchanged
- [x] Mark completed

### Task 2: API client, session storage, and single-flight refresh

- [x] Add `api/TodaySummary.kt`: kotlinx.serialization models mirroring `summaryTodayResponse` and `summaryTargetPayload` field for field, with `last_logged_at` and `recommendation` nullable and the four target numbers non-optional
- [x] Add `store/SecureStore.kt`: a Keystore-backed AES-GCM encrypted store holding server URL, username, password, serialized cookies, and the last summary snapshot with its fetch time
- [x] Add `api/SessionCookieJar.kt`: a persistent `CookieJar` that honours domain, path and expiry, so `kin_refresh` is sent only to `/api/auth/refresh`, and that writes through to `SecureStore` synchronously on every change
- [x] Add `api/RefreshInterceptor.kt`: record each request's dispatch time, and on a 401 for a non-exempt path run a mutex-guarded refresh that returns early when a refresh completed at or after that dispatch time, then retry the request once
- [x] Exempt `/api/auth/login` and `/api/auth/refresh` from the interceptor, mirroring `isAuthExemptPath` in `frontend/lib/api.ts`
- [x] Add `api/HealthVaultApi.kt` with `login`, `refresh`, `summaryToday`, and a re-login-from-stored-credentials fallback used once when refresh fails
- [x] Add `api/ApiResult.kt` classifying outcomes as success, unauthenticated, rate limited with a `Retry-After` duration, Cloudflare Access challenge, network failure, or server error
- [x] Mark completed

### Task 3: First-run setup and sign-in

- [x] Add `ui/SetupScreen.kt`: server URL, username and password fields, with the URL normalized to an origin and a warning shown when the scheme is not `https`
- [x] Validate the entered server by attempting a login, and map each `ApiResult` case to its own message — wrong credentials, locked out with a retry time, Access challenge, unreachable server
- [x] Persist server URL and credentials in `SecureStore` only after a successful login
- [x] Add `MainActivity.kt` routing to setup when no session exists and to the today screen when one does
- [x] Add a sign-out action that clears the cookie jar, the stored credentials and the cached snapshot, and refreshes any placed widget into its signed-out state
- [x] Mark completed

### Task 4: Native today screen

- [ ] Add `ui/TodayScreen.kt` rendering consumed calories against target calories, the four macros, meal count, and a relative last-logged time
- [ ] Render the target's unavailability reason as its own message when `target.available` is false, and show consumed values alone rather than a fabricated denominator
- [ ] Add pull-to-refresh and render the cached snapshot with a staleness marker whenever the refresh fails
- [ ] Add a **Log food** action opening `<server>/food/upload/` in a Chrome Custom Tab, building the URL from the stored server URL only
- [ ] Add `values/strings.xml` and `values-ru/strings.xml`, and apply the per-app locale from the response's `display_language` when it names a language in `shippedUILanguages`, falling back to the device locale
- [ ] Mark completed

### Task 5: Home-screen widget

- [ ] Add `widget/SummaryWidget.kt` as a Glance widget using `SizeMode.Responsive` with a compact (about 110x110dp) and a wide (about 250x110dp) layout, plus `widget/SummaryWidgetReceiver.kt` and the widget provider XML
- [ ] Render calories consumed against target in the compact layout, and calories plus four macro bars plus a **Log food** button in the wide layout
- [ ] Reserve a single-line recommendation slot in the wide layout that renders only when `recommendation` is non-null
- [ ] Add a pure `widgetState(snapshot, fetchedAt, now, session)` mapping to loaded, stale (snapshot older than 6 hours), signed-out, or error states, kept free of Android types so it is JVM-testable
- [ ] Wire widget taps: the body opens the today screen, **Log food** opens the Custom Tab, and a refresh affordance enqueues an immediate update
- [ ] Set `updatePeriodMillis` to 0 in the provider XML so all updates come from WorkManager
- [ ] Mark completed

### Task 6: Background refresh and update triggers

- [ ] Add `work/RefreshWorker.kt` performing exactly one `GET /api/summary/today`, persisting the snapshot, and updating every placed widget
- [ ] Add `work/RefreshScheduler.kt` enqueuing a 30-minute periodic worker on first widget placement and cancelling it when the last widget is removed
- [ ] Honour a 429 by scheduling the next attempt from the `Retry-After` header rather than retrying immediately
- [ ] Back off on network failure without clearing the cached snapshot, and never treat a failed refresh as a sign-out
- [ ] Enqueue a one-off update on widget placement, on manual refresh, and when the app resumes
- [ ] Mark completed

### Task 7: Android unit tests

- [ ] Cover the cookie jar: path scoping keeps `kin_refresh` off `/api/summary/today`, expiry is honoured, and cookies survive a store round trip
- [ ] Cover single-flight refresh with MockWebServer: concurrent 401s produce exactly one `/api/auth/refresh` call, and a 401 for a request dispatched before the last refresh retries without refreshing again
- [ ] Cover that a rotated refresh token is committed to the store before the retried request is issued
- [ ] Cover response parsing: an available target, each unavailability reason, a zero-valued target field, and a null `last_logged_at`
- [ ] Cover 429 handling: `Retry-After` sets the next attempt time, and the failure is not reported as a sign-out
- [ ] Cover `widgetState` for loaded, stale, signed-out and error inputs
- [ ] Cover the Access-challenge classification: an HTML body or a redirect to `cloudflareaccess.com` maps to its own result rather than to invalid credentials
- [ ] Mark completed

### Task 8: Summary contract test in the existing e2e suite

- [ ] Add `e2e/tests/summary-today.spec.ts` signing in through the UI the way `e2e/tests/auth.spec.ts` does, then reading `/api/summary/today` through the authenticated request context
- [ ] Assert every field the Android client parses is present with the expected type, including `target.available`, the four target numbers, and a null `recommendation`
- [ ] Assert `last_logged_at` is either null or a parseable timestamp
- [ ] Assert the endpoint stays self-only by passing `?user=` and getting the caller's own data
- [ ] Mark completed

### Task 9: Documentation, ADRs, and verification

- [ ] Add `docs/adr/ADR-012-android-client-in-repo.md`: the client lives here rather than in a separate repository, and its Gradle build is skipped when no SDK is present — with the gap that leaves stated outright
- [ ] Add `docs/adr/ADR-013-android-cookie-session-auth.md`: cookie-session auth with a persistent encrypted jar instead of a Bearer token, the single-flight rotation hazard, and why credentials are stored for unattended recovery
- [ ] Create both ADRs as `Proposed` and flip them to `Accepted` as the last commit of the change
- [ ] Add `android/README.md` covering SDK prerequisites, building the debug APK, and pointing the app at a LAN stack
- [ ] Note in the pull request that the `/api/*` Cloudflare Access bypass is an operator step outside this repository, and that the app is LAN-only until it exists
- [ ] Run `make lint` and `make test`, and record in the pull request that the Android targets skipped for want of an SDK if they did
- [ ] Run `make test-e2e` against the deployed stack and fix any failure before finishing
- [ ] Mark completed
