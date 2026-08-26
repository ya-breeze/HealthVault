## Why

A mobile audit of the deployed app measured two structural defects on the dashboard, both
navigation problems rather than styling ones:

- The shared header consumes **177px on every screen — 21% of the fold** at a 390px viewport,
  before any content renders. It carries seven controls (app name, user badge, webhook, custom
  foods, import, settings, logout) laid out with `flex-wrap`, so on a narrow viewport it wraps to
  three rows and grows rather than adapting.
- **"Log a meal by photo" — the app's core mobile action — sits 1.04 folds down.** It is below the
  vitals grid and the needs-attention banner in the dashboard body, reachable only by scrolling,
  and it exists on no other screen at all.

The whole frontend contains **three responsive utilities** (`sm:w-auto`, `sm:grid-cols-4`,
`sm:flex-row`), which is why the desktop layout is simply narrowed rather than adapted on a phone.

Fixing these by trimming pixels off the header does not reach the second problem: the logging
action needs to be reachable from every screen, not just higher on one of them. A persistent
bottom navigation bar addresses both, and is the standard pattern for thumb reach on a handheld.

## What Changes

- Add a **mobile bottom tab bar** with five destinations — Home, Photo, Manual, History, More —
  rendered on every authenticated page below the mobile breakpoint. Photo and Manual are the two
  food-logging entry points, each promoted to one tap from anywhere.
- **"More" opens a sheet** carrying the controls the mobile header sheds: webhook URL, custom
  foods, import, settings, and logout.
- **Reduce the mobile header to the app title plus the active-user badge.** Its other five
  controls move into the More sheet; the header stops wrapping and stops consuming a fifth of the
  fold.
- **Desktop is unchanged.** At and above the breakpoint the existing header renders exactly as it
  does today and the tab bar is absent. This change introduces a viewport-conditional navigation
  shell, so the header requirement stops being unconditional.
- **Reconcile the app's existing bottom-anchored elements with the bar.** Two pages render their own
  `fixed bottom-0` submit bar (one of them `/food/manual/`, a promoted destination) and toasts sit
  at `fixed bottom-4` app-wide. All three offset by a single shared clearance token so nothing
  overlaps, and the app opts into `viewport-fit=cover` so safe-area insets stop resolving to zero.
- Extend the tap-target spec's enumerated scope to cover the new bar — its requirement lists
  covered surfaces exhaustively, so a new navigation surface would otherwise sit outside it. The
  same edit corrects a stale part of that enumeration and of `dashboard-ui`'s: the header's settings
  link, which ships today but is named in neither.
- Extend `e2e/tests/` with cases that would have caught both measured defects: header height as a
  fraction of the viewport, and the reachability of the logging action without scrolling.

Explicitly **not** in this change:

- **No new visual language.** Palette, type scale and card density stay on the current Instrument
  Panel tokens. The audit's third decision — a fresh visual direction — is deferred to its own
  change so this one stays reviewable and its e2e signal stays clean.
- **No dashboard body restructuring.** The "Log food" row, vitals grid, and More Data section keep
  their current markup and position. Whether the in-body row is redundant once the tab bar exists
  is a follow-up question, not this change's call.

## Capabilities

### New Capabilities

- `mobile-navigation`: the viewport-conditional navigation shell — the bottom tab bar, its five
  destinations, active-state behaviour, the More sheet, and the breakpoint at which it replaces
  the header's control set.

### Modified Capabilities

- `dashboard-ui`: the "Shared instrument-panel header" requirement currently mandates one header
  carrying seven named controls on every authenticated page, unconditionally. It becomes
  viewport-conditional: the full control set is a desktop rendering, and below the breakpoint the
  header retains only title and user badge while the remaining controls are reachable via the
  navigation shell.
- `mobile-touch-targets`: the "Minimum Tap Target Size" requirement enumerates its covered
  surfaces and states that list is exhaustive. Add the bottom tab bar's five destinations and the
  More sheet's controls to that enumeration, so the new surface is inside the 48×48 guarantee
  rather than outside it by omission.

## Impact

- **New**: `frontend/components/BottomNav.tsx`, `frontend/components/MoreSheet.tsx`, and
  `frontend/components/AuthenticatedShell.tsx` wrapping authenticated pages.
- **Modified**: `frontend/components/Header.tsx` (control set becomes viewport-conditional; the
  session fetch moves up into the shell, which passes it to both header and sheet); the **eleven**
  `<Header />` call sites across nine authenticated pages, which take the shell instead — note
  `app/food/review/ReviewClient.tsx` has three, in its loading, error and main branches.
- **Modified — bottom-anchored elements that would otherwise collide with the bar**:
  `frontend/app/food/manual/page.tsx` and `frontend/app/food/review/ReviewClient.tsx` each render a
  `fixed bottom-0` submit bar, and `/food/manual/` is one of the five promoted destinations;
  `frontend/components/Toast.tsx` renders the app-wide toast stack at `fixed bottom-4`. All three
  offset by the shared clearance token instead of sitting at the viewport edge. The token is `0px`
  above the breakpoint, so desktop is unchanged at all three sites.
- **Modified**: `frontend/app/globals.css` defines the clearance token once;
  `frontend/app/layout.tsx` gains a `viewport` export with `viewportFit: 'cover'` — **not**
  bottom-bar clearance, which belongs on the shell because `layout.tsx` also wraps `/login`.
  Without `viewport-fit=cover` every `env(safe-area-inset-bottom)` in the app resolves to zero on
  real devices, including the two already in the submit bars above.
- **Icons**: `frontend/components/icons.tsx` gains home/more glyphs; camera, pencil and history
  icons already exist and are reused.
- **i18n**: five tab labels plus sheet labels in both `en` and `ru`. Russian labels are the
  constraint on tab width — at a 320px viewport five tabs get 64px each.
- **Tests**: `e2e/tests/` gains navigation cases; `e2e/tests/mobile-tap-targets.spec.ts` gains the
  tab bar and sheet.
- **No backend change.** No API, data-model or migration impact.
