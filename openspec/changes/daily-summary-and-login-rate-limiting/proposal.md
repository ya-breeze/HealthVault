## Why

Idea #12's Android Widget needs one cheap, self-contained read to draw a home-screen calorie count
— not 3-4 separate authenticated calls per refresh. Nothing today aggregates "how am I doing
today"; `GET /api/users/me/nutrition-target` computes the target alone, and consumed totals aren't
exposed anywhere.

Separately, the widget's auth design (see the idea-forge decision this implements) requires a
Cloudflare Access **Bypass** policy on `/api/*` so a background refresh can log in with
username/password instead of an interactive Google flow. That policy is Cloudflare zone config,
tracked outside this repo — but it can't land until login can survive being placed on the open
internet. Today it can't: there is no rate limiting, lockout, or backoff anywhere in the backend or
in `kin-core`. This proposal is that prerequisite, not a request to add the Bypass policy here.

## What Changes

- **New capability `daily-summary`**: `GET /api/summary/today` — self-only (no `?user=` override,
  matching `nutrition-target`'s precedent), authenticated, returning in one response: today's
  consumed calories/protein/carbs/fat (summed from confirmed meals only), a raw count of today's
  logged meals (any status), the timestamp of the most recent one, the caller's
  `display_language`, an embedded nutrition target (or a structured reason it's unavailable — the
  same four reasons `nutrition-target` already defines, never a fabricated denominator), and an
  always-null `recommendation` field reserved for Phase 4. "Today" reuses
  `food-day-completeness`'s existing Local Day boundary (the caller's stored `timezone`, UTC
  fallback) rather than inventing a second definition of "day."
- **Login attempt limiting**, added to the `authentication` capability: an in-process, per-username
  sliding-window counter with exponential-backoff lockout on `POST /api/auth/login`, returning 429
  once tripped. This is the in-app half of the plan's "protection travels with the software for any
  self-hoster" requirement. The companion zone-level Cloudflare rate-limit rule is infra work
  tracked outside this repo, same as the Bypass policy itself.

## What's explicitly out of scope

- The `healthvault-android` app itself, the Cloudflare Access path-scoped Bypass policy, and the
  zone rate-limiting rule — all infra/other-repo work per the idea-forge decision.
- Phase 4's actual recommendation computation — `recommendation` is modeled as nullable and always
  null here; nothing in this change computes it.
- A dedicated "issue me a bearer token in the JSON body" login variant, and any change to
  `RequireAuth` itself. **Correction to an earlier assumption in this proposal:** `RequireAuth`
  today validates the `kin_access` cookie only — the pinned `kin-core` v0.1.0's
  `middleware.ValidateRequest` calls `cookies.GetAccessToken(r)` exclusively and has no
  `Authorization: Bearer` header path, even though the pre-existing (unrelated to this change)
  `openspec/specs/authentication/spec.md` "RequireAuth middleware" requirement already claims
  header support — that claim is stale against the pinned dependency version. The Android app is
  still expected to read the token out of the existing `Set-Cookie` response headers on
  login/refresh and replay it as a cookie on subsequent requests (an HTTP client that keeps a
  cookie jar handles this without new backend work). Adding real `Authorization: Bearer` support to
  `RequireAuth` — a `kin-core` version bump or a HealthVault-local shim — is a prerequisite only if
  the Android app's HTTP layer turns out unable to replay cookies on background requests, and is
  left as a follow-up decision for whoever starts `healthvault-android`, not this change.
- Any change to `POST /api/auth/refresh` or `POST /api/auth/logout` — the lockout applies to
  `/api/auth/login` only; a locked-out username's existing valid access/refresh tokens keep working
  through `RequireAuth` and `Refresh` unaffected (see design.md for why).

## Capabilities

### New Capabilities

- `daily-summary`: `GET /api/summary/today`, aggregating today's consumed macros, meal count,
  last-updated timestamp, display language, and embedded nutrition target for the authenticated
  caller only.

### Modified Capabilities

- `authentication`: adds a `Login attempt limiting` requirement (sliding-window failure counter,
  exponential backoff, 429 response) covering `POST /api/auth/login`, and corrects the
  `RequireAuth middleware` requirement's stale claim that it accepts an `Authorization: Bearer`
  header — the pinned `kin-core` v0.1.0 only ever validates the `access_token` cookie.

## Impact

- Backend (Go): new `backend/pkg/server/summary_today.go` (handler + response types, reusing
  `nutrition_target.go`'s precondition/computation helpers and `database.ResolveTimezone` /
  `database.LocalDate` from `food_completeness.go`'s package), a new `database.TodaySummary`-style
  query helper alongside `food_completeness.go`, route registration in
  `backend/pkg/server/server.go`. New `backend/pkg/server/login_limiter.go` (in-memory limiter) and
  a call site in `backend/pkg/server/auth.go`'s `Login`.
- No schema migration: the limiter's state is in-memory and process-local (see design.md for why
  that's acceptable at this project's single-instance, few-user scale), and consumed totals are
  computed fresh from existing `FoodMeal` rows on every read, never stored.
- Docs: `openspec/specs/authentication/spec.md` gains the new requirement; a new
  `openspec/specs/daily-summary/spec.md` is created on archive.
