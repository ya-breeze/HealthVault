# Native Android app and home-screen widget

Idea: ya-breeze/idea-forge#25

## Why

The backend already carries a feature built for a client that does not exist. `GET /api/summary/today` (`backend/pkg/server/summary_today.go`, registered at `backend/pkg/server/server.go:93`) returns today's consumed macros, meal count, last-logged timestamp, display language and embedded Nutrition Target in one authenticated call. Its doc comment says why: "the endpoint exists so a widget makes one cheap call instead of three". Nothing calls it. The login limiter and the process-wide verification cap (`backend/pkg/server/login_limiter.go`, ADR-009) exist for the same absent client — ADR-009 names "the Android Widget (idea #12)" as the reason `/api/auth/login` had to survive being placed on the open internet.

So the prerequisite change landed and the thing it was a prerequisite for did not. Today the owner answers "how many calories so far" by unlocking the phone, opening a browser, passing Cloudflare Access, waiting for the static Next.js export to load, and reading the dashboard's Food Card. That is a 20-second round trip for a number that fits in a 2x2 home-screen cell, and it is the single most-repeated read in the whole application.

Why now: the blocking work is done on both sides. `daily-summary-and-login-rate-limiting` merged, so the summary endpoint and the login hardening are on `main`. The pipeline defect that stranded this idea (#28) is fixed. What remains is client work.

## How

**Placement.** The app lives in this repository, at `android/`, as a self-contained Gradle project. The earlier spec called it "other-repo work" and named a `healthvault-android` repository, which does not exist. A second repository would split a client from the API contract it consumes, so a breaking change to `summaryTodayResponse` could merge here with nothing failing there. In-repo, the same pull request touches both sides. Recorded as ADR-014.

Package and application ID are `net.ikoro.healthvault`. The literal reverse of `ikoro.in` is unusable — `in` is a Kotlin hard keyword and would need backticks in every file.

**Auth mirrors the browser, because the server only supports that.** `RequireAuth` validates the `kin_access` cookie exclusively; the pinned `kin-core` v0.1.0 has no `Authorization: Bearer` path, and adding one is out of scope here. The client therefore keeps a real cookie jar: `POST /api/auth/login` yields `kin_access` (15 minutes) and `kin_refresh` (365 days, scoped by kin-core to `Path=/api/auth/refresh`), and `POST /api/auth/refresh` rotates both. The jar must honour path scoping, so `kin_refresh` is never sent to `/api/summary/today`, and must persist across process death, since a widget update runs in a freshly started process almost every time.

Two consequences drive the design. First, `authdb.RotateRefreshToken` **consumes** the token it is given, so two concurrent refreshes spend an already-rotated token and sign the device out. The frontend hit exactly this and fixed it in `frontend/lib/api.ts` with `coordinatedRefresh(dispatchedAt)` plus a `lastRefreshAt()` guard; `frontend/lib/api.test.ts` is mostly about that rule. The Android client reproduces the same rule with a mutex and a recorded completion time: a 401 for a request dispatched **before** the last successful refresh means retry, not refresh. Second, a rotated token that is not durably written before the app is killed is gone forever, so the jar commits synchronously inside the refresh call, before the retried request is issued. Synchronously means `SharedPreferences.commit()`, not `apply()`: `apply()` updates the in-memory map at once but writes the file on a background thread, and a process the system kills — the normal end of a widget refresh — never runs that write. Cookies, credentials and sign-out are therefore committed; the summary snapshot, a display cache the next refresh replaces, keeps `apply()` because it is written from the UI thread. Every committing caller runs off the main thread.

When refresh fails outright the client re-logs in once from stored credentials. That is why the password is stored at all: a home-screen widget that silently stops updating until the owner happens to open the app is worse than a widget that recovers itself. **Only a 401 on that re-login means the session actually ended.** A 429, an unreachable server or a Cloudflare Access challenge says nothing about the session, so each is reported as itself rather than collapsed into "signed out": the today screen names the cause — held off by the rate limiter, server unreachable, Access-gated public API, server error — and the widget keeps rendering its snapshot. A real sign-out is the one case that clears the stored session, from the background worker as well as from the app, so the widget falls back to its sign-in prompt instead of a card that can never update again. Credentials, cookies, refresh holdoff/status and the last summary snapshot sit in one Keystore-backed AES-GCM encrypted store. The residual risk — an attacker holding the unlocked, rooted device — is accepted and recorded in ADR-015.

**The widget** is a single Glance `AppWidgetProvider` with `SizeMode.Responsive`, not separate pickable widgets: the owner places one widget and resizes it. Its default is the Android-recommended 2x1 size (about 110x56dp), while launchers that allow it may shrink it to a minimal 1x1 tile with a 48dp touch target. A 1x1 tile keeps the calorie hero, unit, identity, and progress; 2x1 adds target progress and percentage; 2x2 adds the HealthVault label and separates identity from the bottom-anchored calorie group. A separate tall-narrow composition prevents a 1x2 resize from exposing a stretched 1x1 tile. The full 4x2 layout keeps the calorie hero, exposes three full-width macro rows with flexible `LinearProgressIndicator` bars distributed over the available height, and shows **Log food** and refresh as 48dp actions at the upper right. Tapping any loaded or stale widget surface opens **Log food** directly; signed-out and error surfaces still open the native app. The separate actions appear only in the full layout, so the smaller sizes remain glanceable instead of turning into compressed controls. The provider declares fallback dimensions that add as 2x1 on Android 8–11 and resize down to 1x1, plus Android 12 cell targets and a maximum dp range large enough for 4x2 across supported launchers. Android 8–11 ignore maximum resize bounds, so taller legacy shapes cannot be prohibited; weighted layouts keep their content aligned if a launcher exposes one. When the system font scale reaches 1.3, secondary text and macro rows yield to the calorie hero rather than crop. The tighter 1x1 composition starts this reduction at 1.1; at 1.4 it switches to a single calorie line and drops the progress bar so 1.4–2.0 font scaling cannot crop. Its TalkBack description retains HealthVault identity and the kcal unit, while a visible `!` keeps stale state on the tile. TalkBack receives a localized description of the calorie state and whole-card **Log food** action. When no target is available, layouts show the consumed value without a fabricated percentage or progress denominator; progress bars clamp consumption above target while the visible percentage may exceed 100%. Material widget color roles, the system widget radius on Android 12 and a rounded drawable fallback on earlier releases, and flexible widths replace fixed black/green styling and fixed 90dp bars. Glance 1.1 has no generated-preview API, so Android 12 and later receive a hand-written XML preview representative of the 2x1 default plus a concise description; older picker APIs cannot render that scalable layout. The three macro bars correct the original Task 5 wording: the API and implementation expose protein, carbohydrates, and fat, while calories remain the separate hero value. The originally planned recommendation slot was cut during integration: newer work on `main` retired `summaryTodayResponse.recommendation` and moved nutrition advice to `POST /api/food/advice`, whose client-computed input and model latency do not belong in this cheap summary read.

Owner screenshots supersede two assumptions in that first responsive pass. The adaptive launcher foreground collapses into a white dot at 8–14dp, so compact surfaces use text identity instead of scaling the launcher asset below its intended mask. A weighted gap also makes the real Samsung 2x2 surface look almost empty, so 2x2 keeps one compact calorie group near the visual center rather than pinning it to the bottom. The full 4x2 hierarchy remains unchanged.

**The FlexWindow widget** is a second Glance receiver, not another breakpoint of the resizable home-screen provider. Samsung discovers it through `com.samsung.android.appwidget.provider` metadata with `display="sub_screen"`; its standard provider metadata uses the documented full FlexWindow bounds and `keyguard` category. The dedicated near-square composition shows HealthVault identity, calorie progress, three macro rows, refresh, and the same whole-surface **Log food** action. Both receivers share the snapshot, stale/error semantics, WorkManager schedule, and update path. Removing one provider does not stop periodic refresh while the other still has a placed instance. Complex food entry may move to the main display when One UI requires the phone to be opened; the widget itself remains useful without launching an activity.

Refresh is one call per update. WorkManager runs a 30-minute periodic worker (the platform floor is 15 minutes) only while at least one widget is placed, plus one-off updates on every widget placement, on manual refresh, and when the app resumes. A 429 is honoured globally through `Retry-After` — `writeTooManyAttempts` sends both the header and `retry_after_seconds`, and the persisted holdoff prevents periodic, resume and manual triggers from retrying early. The worker has no connectivity constraint so it still wakes while offline to age the widget into its stale state. On any failure the widget keeps rendering the last snapshot with a staleness marker rather than blanking; a snapshot older than 6 hours is also marked stale, and a signed-out session renders a sign-in prompt.

**The app is thin and read-only.** First run asks for a server URL and credentials and validates them by attempting a login. Everything else is one native today screen: calories, four macros against target or the target's structured unavailability reason, meal count, and relative last-logged time. **Log food** opens `<server>/food/upload/` in a Chrome Custom Tab — the trailing slash is required, since `frontend/next.config.ts` sets `trailingSlash: true` on a static export. The Custom Tab URL is always derived from the stored server URL and never from an intent extra, so no other app can drive it to an arbitrary page. Every write still goes through the web UI.

The app ships English and Russian strings and follows the caller's Display Language when `display_language` names a shipped language, matching `shippedUILanguages` in `backend/pkg/server/display_language.go`; anything else falls back to the device locale. Cleartext HTTP is permitted only in the debug build, through a network security config, so a LAN stack like `http://192.168.1.54:8892` stays testable while release builds require HTTPS.

**Gate wiring, and the gap in it.** `make test` and `make lint` gain `test-android` and `lint-android`, which run `./gradlew testDebugUnitTest` and `./gradlew lintDebug`. Both **skip with a printed notice** when no Android SDK is present (`ANDROID_HOME` unset and no `android/local.properties`), because this environment has no Android toolchain. State this plainly: on a machine without the SDK, `make test` is green while proving nothing about `android/`. Two things narrow the gap rather than closing it. The Android unit tests are plain JVM tests over the parts worth testing — cookie jar path and expiry matching, single-flight refresh under concurrent 401s, response parsing, 429 backoff, and a pure `widgetState()` mapping — so they need no emulator and run wherever the SDK exists. And a new Playwright case pins the contract the widget parses, so a change to `summaryTodayResponse` fails `make test-e2e` in this repository even when nothing here can compile Kotlin. The APK itself is built and sideloaded by the owner on a machine with the SDK; there is no automated device coverage, and there will not be one.

**Excluded on purpose.** Native camera capture, any write path, and native Google Sign-In stay deferred exactly as idea #12 settled them. There is no Play Store release and no release signing configuration; the deliverable is a debug APK. Every public `ikoro.in` hostname remains gated by Cloudflare Access, and this cookie-only client has no Access service-token flow, so this version is LAN-only. The client detects an Access challenge specifically and directs the owner to the LAN address, never to weaken Access with a `/api/*` Bypass and never as "invalid credentials". `RequireAuth` still gains no Bearer path.

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

- [x] Add `api/TodaySummary.kt`: kotlinx.serialization models mirroring the summary fields the client consumes, with `last_logged_at` nullable and the four target numbers non-optional
- [x] Add `store/SecureStore.kt`: a Keystore-backed AES-GCM encrypted store holding server URL, username, password, serialized cookies, refresh holdoff/status, and the last summary snapshot with its fetch time
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

- [x] Add `ui/TodayScreen.kt` rendering consumed calories against target calories, the four macros, meal count, and a relative last-logged time
- [x] Render the target's unavailability reason as its own message when `target.available` is false, and show consumed values alone rather than a fabricated denominator
- [x] Add pull-to-refresh and render the cached snapshot with a staleness marker whenever the refresh fails
- [x] Add a **Log food** action opening `<server>/food/upload/` in a Chrome Custom Tab, building the URL from the stored server URL only
- [x] Add `values/strings.xml` and `values-ru/strings.xml`, and apply the per-app locale from the response's `display_language` when it names a language in `shippedUILanguages`, falling back to the device locale
- [x] Mark completed

### Task 5: Home-screen widget

- [x] Add `widget/SummaryWidget.kt` as a Glance widget using `SizeMode.Responsive` with a compact (about 110x110dp) and a wide (about 250x110dp) layout, plus `widget/SummaryWidgetReceiver.kt` and the widget provider XML
- [x] Render calories consumed against target in the compact layout, and calories plus three macro bars plus a **Log food** button in the wide layout
- [x] Add a pure `widgetState(snapshot, fetchedAt, now, session, refreshFailed)` mapping to loaded, stale (failed refresh or snapshot older than 6 hours), signed-out, or error states, kept free of Android types so it is JVM-testable
- [x] Wire widget taps: the body opens the today screen, **Log food** opens the Custom Tab, and a refresh affordance enqueues an immediate update
- [x] Set `updatePeriodMillis` to 0 in the provider XML so all updates come from WorkManager
- [x] Mark completed

### Task 6: Background refresh and update triggers

- [x] Add `work/RefreshWorker.kt` performing exactly one `GET /api/summary/today`, persisting the snapshot, and updating every placed widget
- [x] Add `work/RefreshScheduler.kt` enqueuing a 30-minute periodic worker on first widget placement and cancelling it when the last widget is removed
- [x] Honour a 429 across periodic, resume and manual triggers by persisting the `Retry-After` deadline rather than retrying immediately
- [x] Back off on network failure without clearing the cached snapshot, and never treat a failed refresh as a sign-out
- [x] Enqueue a one-off update on widget placement, on manual refresh, and when the app resumes
- [x] Mark completed

### Task 7: Android unit tests

- [x] Cover the cookie jar: path scoping keeps `kin_refresh` off `/api/summary/today`, expiry is honoured, and cookies survive a store round trip
- [x] Cover single-flight refresh with MockWebServer: concurrent 401s produce exactly one `/api/auth/refresh` call, and a 401 for a request dispatched before the last refresh retries without refreshing again
- [x] Cover that a rotated refresh token is committed to the store before the retried request is issued
- [x] Cover response parsing: an available target, each unavailability reason, a zero-valued target field, and a null `last_logged_at`
- [x] Cover 429 handling: `Retry-After` sets the next attempt time, and the failure is not reported as a sign-out
- [x] Cover `widgetState` for loaded, stale, signed-out and error inputs
- [x] Cover the Access-challenge classification: an HTML body or a redirect to `cloudflareaccess.com` maps to its own result rather than to invalid credentials
- [x] Mark completed

### Task 8: Summary contract test in the existing e2e suite

- [x] Add `e2e/tests/summary-today.spec.ts` signing in through the UI the way `e2e/tests/auth.spec.ts` does, then reading `/api/summary/today` through the authenticated request context
- [x] Assert every field the Android client parses is present with the expected type, including `target.available` and the four target numbers
- [x] Assert `last_logged_at` is either null or a parseable timestamp
- [x] Assert the endpoint stays self-only by passing `?user=` and getting the caller's own data
- [x] Mark completed

### Task 9: Documentation, ADRs, and verification

- [x] Add `docs/adr/ADR-014-android-client-in-repo.md`: the client lives here rather than in a separate repository, and its Gradle build is skipped when no SDK is present — with the gap that leaves stated outright
- [x] Add `docs/adr/ADR-015-android-cookie-session-auth.md`: cookie-session auth with a persistent encrypted jar instead of a Bearer token, the single-flight rotation hazard, and why credentials are stored for unattended recovery
- [x] Create both ADRs as `Proposed` and flip them to `Accepted` as the last commit of the change
- [x] Add `android/README.md` covering SDK prerequisites, building the debug APK, and pointing the app at a LAN stack
- [x] Note in the pull request that public hostnames remain Access-gated and this cookie-only app is LAN-only
- [x] Run `make lint` and `make test`, and record in the pull request that the Android targets skipped for want of an SDK if they did
- [x] Run `make test-e2e` against the deployed stack and fix any failure before finishing
- [x] Mark completed

### Task 10: Responsive widget visual hierarchy

- [x] Make the compact layout fill its surface with the HealthVault label, a large consumed-calorie value, target and percentage, and a full-width `LinearProgressIndicator`
- [x] Make the wide layout use a horizontal strip of three macro items with flexible `LinearProgressIndicator` bars, plus a horizontal pair of 48dp primary **Log food** and secondary refresh actions
- [x] Preserve unavailable-target semantics, clamp progress above target, and keep the visible percentage truthful
- [x] Use Material widget color roles, the system radius on Android 12 with a rounded drawable fallback on API 26–30, and localized accessible action descriptions in both light and dark themes
- [x] Add a representative hand-written XML picker preview and description, pure progress-model tests, and source/XML regression guards for compact and wide content
- [x] Build, test, lint, and assemble the Android app
- [x] Mark completed

### Task 11: Compact sizes and height-aware layout

- [x] Make 2x1 the default size, support useful 1x1 and 1x2 minima, resize on both axes through 4x2, and keep the maximum dp range large enough for two rows across launchers
- [x] Open **Log food** from the loaded or stale card surface, keep signed-out and error taps routed to the native app, and show separate add and refresh controls only in the full 4x2 layout
- [x] Anchor the calorie group to the bottom of 2x2, and replace the compressed 4x2 macro strip with three full-width rows distributed over the available height
- [x] Keep missing-target, over-target, stale-state, localization, Material color, touch-target, and picker-preview behavior correct at every breakpoint
- [x] Add regression coverage for every declared breakpoint, tap routing, provider bounds, and height-aware composition
- [x] Build, test, lint, and assemble the Android app
- [x] Mark completed

### Task 12: Repair compact composition and add Samsung FlexWindow

- [x] Replace the undersized adaptive launcher icon in 1x1, 2x1, 1x2, and reduced layouts with legible text identity, including stale-state treatment
- [x] Recompose 2x2 around a centered calorie group without a full-height weighted gap, while preserving missing-target, over-target, large-font, and whole-card tap behavior
- [x] Add a dedicated FlexWindow Glance widget, receiver, picker preview, `keyguard` provider metadata, and Samsung `sub_screen` metadata using the documented full-screen bounds
- [x] Keep update-all, placement counting, periodic refresh, sign-out, and failure states correct when either or both widget providers are placed
- [x] Add red-capable regression tests for the white-dot source, 2x2 empty-gap source, FlexWindow registration/layout, and dual-provider refresh lifecycle
- [x] Build, test, lint, and assemble the Android app; publish a new owner-acceptance APK without merging or deploying production
- [x] Mark completed

Owner acceptance follows through the prerelease APK after this automated task. It includes checking both sizes on the owner's launcher and is deliberately not an automated completion checkbox, because this environment has no Android device or emulator.

### Task 13: Use the owner-selected web mark in widgets

- [x] Copy the owner-supplied transparent web icon into an Android `drawable-nodpi` resource without redrawing or routing it through the adaptive launcher mask
- [x] Replace compact `HV` identity with the web mark, add it beside full HealthVault headers on home and FlexWindow layouts, and preserve visible stale treatment
- [x] Keep the extreme large-text 1x1 composition calorie-only so its 48dp cell remains uncropped, with app identity retained in semantics
- [x] Update both picker previews to show the same mark and add regression coverage that rejects the old text/adaptive-icon identity paths
- [x] Build, test, lint, and assemble a new owner-acceptance APK without merging or deploying production
- [x] Mark completed

### Task 14: Repair launcher identity and one-row widget rhythm

- [x] Replace the generic plus-in-circle adaptive launcher foreground with the owner-selected web mark, sized inside Android's adaptive-icon safe zone and contrasted against a neutral background
- [x] Recompose the 1x1, 2x1, and 4x1 loaded states as vertically centered content groups, keeping each progress bar close to its calorie row instead of stretching the group across the full height
- [x] Preserve the calorie-only extreme large-text 1x1 state and leave the deliberate 1x2, 2x2, and 4x2 compositions unchanged
- [x] Update the 2x1 picker preview to match the tighter vertical rhythm and add red-capable regression coverage for launcher identity and weighted gaps in one-row layouts
- [x] Build, test, lint, and assemble a new owner-acceptance APK without merging or deploying production
- [x] Mark completed

### Task 15: Implement the owner-selected meal-paced color rails

The owner accepted variant A from `https://artifacts.ikoro.in/healthvault/widget-pacing-v2/`. The expected share of each daily target is `min(eating occasions today / usual meals per day, 1)`. A metric is green within 10 percentage points of that share, amber within 20, and bright red beyond 20; an arrow or check must preserve the meaning without color. This is a pacing signal, not a judgment of food quality.

- [ ] Extend `GET /api/summary/today` with `eating_occasions_today`, collapsed from today's logged meal timestamps through the existing ten-minute occasion rule, and `usual_meals_per_day`, resolved from the caller's existing setting
- [ ] Add a pure Android pacing model with boundary coverage for green, amber, red, below/above direction, over-target progress, and unavailable targets
- [ ] Implement the accepted colored-rail composition at every home-screen breakpoint: no logo on 1x1, explicit `ккал` beside every calorie hero, compact nutrient rails on 1x1/2x1/2x2, inset macro labels on 4x2, and no redundant add button on 4x2
- [ ] Apply the same calorie and macro pacing colors to the FlexWindow surface while preserving its dedicated Samsung provider behavior and non-color status semantics
- [ ] Update summary contract/parsing tests, picker previews, localized accessibility strings, and source guards for the accepted layout
- [ ] Build, test, lint, and assemble a new owner-acceptance APK without merging or deploying production
- [ ] Mark completed
