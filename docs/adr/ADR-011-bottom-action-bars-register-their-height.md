# ADR-011: A Page's Bottom Action Bar Registers Its Height for Anything Anchored Above It

## Status
Proposed

## Context and Problem Statement

ADR-008 gave every bottom-anchored element a shared clearance against the mobile navigation
bar: `--nav-block` and `--edge-inset-b`, read by the app-wide toast stack and by the two pages'
own submit/confirm bars alike. It left one relationship unaddressed on purpose — the tokens
describe the space the *navigation bar* occupies, and nothing describes the space a *page's own*
bottom bar occupies.

That gap is real. `/food/review/`'s confirm bar is `position: fixed`, anchored above the
navigation bar, roughly 4.5rem tall. The toast stack is anchored 1rem above the navigation bar —
inside the confirm bar. Every weight edit on that page raises a success toast (`MealItemRow`'s
`commitWeight`, on blur), so the toast both covers the Confirm Meal button and, being `z-50` with
`pointer-events-auto` on its card, swallows taps aimed at it for the full 3s auto-dismiss window.
The same collision exists on `/food/manual/`'s Save Meal bar, at every viewport width.

A CSS token cannot carry the fix the way ADR-008's did. The bar exists on two pages, is
conditional (`showConfirmBar`) on one of them, and its height depends on the rendered button and
the safe-area inset — so the value is only known at runtime, and the toast stack lives in a
different React subtree from the page that renders the bar.

## Decision Drivers

- The failure mode is the same one ADR-008 named: a control that's present, visible in the DOM,
  and unreachable by a thumb. Stacking order alone (a `z-index` fix) would not resolve it — the
  button would still be invisible under the toast.
- The value needed is a runtime measurement, not a constant, and has to reach a sibling subtree.
- A future third bottom bar should get the same clearance by construction, the way ADR-008's
  navigation-bar clearance already does — not by each new page remembering a manual offset.

## Considered Options

- **A CSS token, sized to the tallest bar in the app.** Rejected: the review page's bar is
  conditional, so a fixed token either overshoots when it's absent (wasted toast offset on every
  other page) or undershoots when a future bar is taller.
- **Each toast call site passes its own offset.** Rejected: the offset depends on which bar is
  currently mounted, not on which call site raised the toast — `MealItemRow`'s `commitWeight` and
  `ReviewClient`'s `handleConfirm` both call `showToast` through the same `useToast()`, and both
  need the same answer.
- **A registration context: a bar reports its measured border-box height while mounted; the toast
  stack reads the maximum of what's currently registered.** Chosen.

## Decision Outcome

Chosen: **`frontend/components/ui/BottomActionBar.tsx`, a small registration context.**

- `BottomActionBarProvider` holds a map from a `useId`-keyed registration to a measured height,
  and exposes the maximum of the registered heights (or `0`) through `useBottomActionBarHeight()`.
  Mounted in `frontend/app/layout.tsx`, wrapping `ToastProvider` — the same place `ToastProvider`
  already sits.
- The default-exported `BottomActionBar` component carries the fixed-bar markup `/food/manual/`
  and `/food/review/` used to duplicate: `fixed bottom-[var(--nav-block)] … z-30 …`. It measures
  its own border-box height with a `ResizeObserver` and registers it while mounted, unregistering
  on unmount (which is what confirming a meal does, since `showConfirmBar` goes `false`).
- `useBottomActionBarHeight()` throws outside the provider, matching `useToast`'s existing
  behavior in `frontend/components/Toast.tsx`, for the same reason: a silent fallback would hide a
  missing provider instead of failing where it's introduced.
- The toast stack's offset branches on whether a bar is registered: `calc(1rem + var(--nav-block)
  + <height>px)` when one is, and exactly today's `calc(1rem + var(--nav-block) +
  var(--edge-inset-b))` when none is. The two aren't interchangeable — the bar already carries
  `--edge-inset-b` inside its own bottom padding, so that inset is inside the measured height, and
  adding `--edge-inset-b` again in the first branch would double-count the safe-area inset on a
  notched device in landscape.

**The obligation this places on future code.** A page's bottom-anchored action bar renders through
`BottomActionBar`, not a hand-rolled `fixed bottom-[var(--nav-block)]` element. Anything anchored
above such a bar — the toast stack today, and any future bottom-anchored element — reads the
registered height through `useBottomActionBarHeight()` rather than a literal.

### Consequences

- A third bottom bar added later gets the toast clearance for free by using the component, closing
  (for the toast, specifically) the hole ADR-008 named: "cannot catch a *fourth* site added later."
- The rule is still not mechanically enforced. Nothing fails to compile when a new page hand-rolls
  its own fixed bar instead of using `BottomActionBar`. The guard remains an e2e case — see
  `e2e/tests/mobile-nav.spec.ts`'s "a toast does not occlude a page's own bottom action bar" —
  which catches a regression on the two known sites but not a new one written to ignore this ADR.
- The safe-area half of the offset arithmetic is only partly testable, for the same reason ADR-008
  recorded: headless Chromium reports `env(safe-area-inset-bottom)` as `0` and offers no way to set
  it. The exactly-once reasoning above is verified by inspection and by the zero-inset case; the
  non-zero case is a manual check on a notched device.
