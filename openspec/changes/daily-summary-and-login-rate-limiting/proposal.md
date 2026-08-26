## Why

Idea #12's Android Widget needs one cheap, self-contained read to draw a home-screen calorie count
from — a widget refresh should not have to make 3-4 separate authenticated calls (nutrition target,
today's meals, display language) just to render one number. Nothing today aggregates "how am I
doing today" into one response; `GET /api/users/me/nutrition-target` computes the target alone,
and today's consumed totals aren't exposed anywhere.

Separately, the widget's own architecture (see the idea-forge decision this proposal implements)
requires putting `/api/*` behind a Cloudflare Access **Bypass** policy so a background refresh can
authenticate with a username/password call instead of an interactive Google login. That change is
explicitly **not** part of this repo (it's Cloudflare zone config, tracked outside `openspec/`
here) — but it cannot land anywhere until this repo's login endpoint can survive being placed on
the open internet. Today it can't: there is no rate limiting, lockout, or backoff anywhere in the
backend or in `kin-core`, the shared library HealthVault's auth is built on. `POST
/api/auth/login` currently has unlimited attempts as its only gate, hidden solely by Access sitting
in front of it. This proposal is the prerequisite that unblocks the Bypass policy, not a request to
add it here.

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
- A dedicated "issue me a bearer token in the JSON body" login variant. `RequireAuth` already
  accepts `Authorization: Bearer <token>` as an alternative to the cookie (no backend change
  needed); the Android app is expected to read the token out of the existing `Set-Cookie` response
  headers on login/refresh. Changing the login response body shape is a separate concern this
  change does not touch.
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
  exponential backoff, 429 response) covering `POST /api/auth/login`.

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
