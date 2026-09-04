# ADR-013: The Android Client Authenticates With a Persistent Cookie Jar, Not a Bearer Token

## Status
Accepted

## Context and Problem Statement

`RequireAuth` (kin-core, pinned at v0.1.0) validates the `kin_access` cookie exclusively — there is
no `Authorization: Bearer` path, and adding one to a pinned third-party dependency is out of scope
for this change. The Android client has to authenticate the same way the browser does: a real
cookie jar, holding `kin_access` (15-minute access token) and `kin_refresh` (365-day refresh token,
scoped by kin-core's `SetRefreshCookie` to `Path=/api/auth/refresh` —
`backend/pkg/server/auth.go`).

Two properties of the server side make this harder than "store the cookies":

- `authdb.RotateRefreshToken` **consumes** the token it is given. Two concurrent refreshes present
  the same pre-rotation token; the second one gets a dead token and a 401, signing the device out
  from under a session that was actually still valid. The frontend hit exactly this and fixed it —
  `frontend/lib/api.ts`'s `coordinatedRefresh(dispatchedAt)` plus `lastRefreshAt()` guard, and
  `frontend/lib/api.test.ts` is mostly about that one rule.
- A widget update runs in a freshly started process almost every time (Android kills app processes
  aggressively when nothing is foregrounded). A rotated refresh token that is not durably written
  before the process is killed is gone forever — the *next* freshly started process refreshes with
  a token the server has already discarded, and the session dies not from any attacker action but
  from ordinary process lifecycle.

## Decision Drivers

- The refresh-rotation hazard is not hypothetical — it is already a fixed bug on the frontend, with
  its own regression tests. Reproducing the fix's *reasoning*, not just persisting cookies and
  hoping, is the point of this decision.
- A background widget update has no human present to retype a password. If the refresh token dies
  (device long unused, token revoked, refresh itself races and loses), the only two outcomes are
  "recover automatically" or "go dark until the owner happens to open the app" — and the second one
  defeats the reason a widget exists at all (see the spec's "one number that fits in a 2x2 cell").
- `RequireAuth` gaining a Bearer path would remove the need for cookie-jar complexity entirely, but
  is explicitly out of scope here (a pinned third-party library change).

## Considered Options

- **Bearer token, stored in SecureStore, sent as `Authorization: Bearer <token>`.** Would sidestep
  cookie domain/path/expiry matching and refresh-rotation coordination entirely. Rejected: the
  server does not accept it. `RequireAuth` only reads `kin_access`; a client-side Bearer scheme
  with no server-side counterpart authenticates nothing.
- **A persistent, RFC-6265-matching cookie jar, reproducing the frontend's single-flight refresh
  rule with a mutex instead of a Web Lock.** Chosen.

## Decision Outcome

Chosen: **`api/SessionCookieJar.kt`**, an OkHttp `CookieJar` backed by `store/SecureStore.kt`.
Matching is delegated to `okhttp3.Cookie.matches(url)` rather than hand-rolled, since it already
implements the domain/path/expiry semantics this needs — in particular, `kin_refresh`'s
`Path=/api/auth/refresh` scoping is exactly what keeps it off every other request, including `GET
/api/summary/today`, without this client having to special-case that path itself. Every mutation
(`saveFromResponse`, `clear()`) writes through to `SecureStore` **synchronously**, before the call
returns — the "gone forever" failure mode above is specifically a race between a cookie rotation
and a process death, and an async write reopens exactly that window.

`api/RefreshInterceptor.kt` reproduces `frontend/lib/api.ts`'s `coordinatedRefresh` rule: record
each request's dispatch time; on a non-exempt 401, take a `ReentrantLock`-guarded refresh path that
re-checks, both before and after acquiring the lock, whether a refresh already completed at or
after that dispatch time — if so, retry instead of refreshing again. A `ReentrantLock` plays the
role the frontend's Web Lock + in-flight-promise pair plays there; Android has no equivalent to a
per-tab JS event loop needing that split, so one mutex covers it.

**Credentials are stored, not just cookies — this is the part that most needs explaining.**
`SecureStore` holds the username and password alongside the session, and `HealthVaultApi.
summaryToday()` uses them for one re-login attempt when a refresh fails outright (dead refresh
token, revoked session). The alternative — reporting signed-out immediately and waiting for the
owner to retype a password — turns any refresh-token loss into a widget that silently stops
updating until someone happens to open the app and notice. For a background-updating home-screen
widget, that failure mode is worse than the residual risk being accepted below.

Everything (credentials, cookies, the last summary snapshot) sits in one Keystore-backed AES-GCM
store — `androidx.security.crypto.EncryptedSharedPreferences` with a Keystore-backed
`MasterKey`. `SecureStore` takes a `SharedPreferences` instance rather than a `Context`
specifically so unit tests can inject a plain in-memory fake
(`android/app/src/test/kotlin/net/ikoro/healthvault/store/FakeSharedPreferences.kt`):
`android.content.SharedPreferences` is only an interface at compile time and needs no Android
runtime to implement, while `EncryptedSharedPreferences` itself does. That is also this decision's
sharpest limit — the tests exercise `SecureStore`'s logic, not the real Keystore-backed encryption
path, which has no automated coverage at all (see ADR-012's "no automated device coverage").

### Consequences

- **Accepted risk: an attacker holding the unlocked, rooted device reads the stored password.**
  Keystore-backed AES-GCM protects the data at rest against extraction from a locked device or a
  backup, not against a live, unlocked, rooted device with debugging access to this app's own
  process — at which point the stored credentials, the live session cookies, and the data the
  session already grants access to are all equally exposed. Storing the password adds one specific
  thing to that already-compromised scenario: it survives a sign-out-and-back-in that would
  otherwise have rotated it out.
- **The re-login fallback fires at most once per `summaryToday()` call**, not in a retry loop — a
  wrong or since-changed password fails visibly (`ApiResult.Unauthenticated`, which the widget
  renders as its signed-out state) rather than retrying silently forever.
- **`RequireAuth` still gains no Bearer path.** Every future client that needs the API — this one,
  the browser, and any later one — authenticates the same way, which is the property this decision
  preserves rather than works around.
- If `kin-core` ever gains a Bearer path, this ADR is the one to revisit: it would remove the
  cookie-jar and refresh-coordination complexity entirely, not just simplify it.
