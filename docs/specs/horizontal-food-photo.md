# Make the camera capture control reachable in landscape
Idea: ya-breeze/idea-forge#173

## Why

You cannot photograph a meal with the phone held horizontally. The Capture button is not on
screen at all, so the in-app camera is unusable in landscape and the only way out is to rotate
the phone or fall back to "Choose Photo".

`frontend/components/food/CameraCapture.tsx` explains the failure. The overlay is
`fixed inset-0 … flex items-center justify-center p-4`, and the card inside it is
`max-w-md w-full overflow-hidden` with no height bound. The card lays out as one unbroken column:
a header row ("Take a photo" plus Cancel, about 49px), 16px of padding, the `w-full` `<video>`
preview, a 16px gap, the 48px Capture `TapTarget`, and 16px of padding. At the card's 448px
maximum width the preview alone is roughly 200-330px tall depending on the stream's aspect ratio,
so the column's natural height is around 350-450px.

A handheld in landscape has a visible viewport around 320-390px tall. The column is taller than
that, and because the flex parent centers it, the overflow is split between the top and the bottom
of the screen. `overflow-hidden` on the card then clips whatever falls outside — and the Capture
button is the last element in the column, so it is the first thing clipped. The button is in the
DOM, enabled, and completely invisible. This is the same class of silent, functional-only defect
that ADR-008 records for controls that land under the bottom navigation bar: nothing fails to
compile, and nothing in a diff prompts a reviewer to ask about it.

Landscape is not a corner case for this screen. A plate of food is wider than it is tall, and a
landscape stream is what the rear camera produces natively, so holding the phone horizontally to
frame a meal is the obvious thing to do. Nothing guards the layout today either: the frontend has
no linter, `make lint` runs `go vet` over the backend only, and the Vitest suite covers pure
functions rather than rendering. Every existing camera case in `e2e/tests/food.spec.ts` runs at
Playwright's default desktop viewport, where the column fits and the defect is invisible.

## How

Restructure the card into a height-bounded flex column with three regions, and let the preview —
not the controls — absorb the shortfall when the viewport is short.

- **Bound the card by the visible viewport.** The overlay is `fixed inset-0`, whose height is the
  *large* viewport, so on a mobile browser showing its URL bar a card sized to the overlay still
  overflows what you can see. Clamp to the dynamic viewport instead (`dvh`, via `max-h-dvh` on the
  overlay or an equivalent `calc(100dvh - …)` bound on the card), so the bound tracks the space
  actually on screen.
- **Three regions, two of which never shrink.** The header (title and Cancel) and a new footer
  holding the Capture button both get `shrink-0`; the preview region between them gets `flex-1`
  and `min-h-0` so it is the only part that gives way. `min-h-0` is the load-bearing half — a flex
  item's automatic minimum size is its content size, so without it the video refuses to shrink and
  the column overflows exactly as it does today.
- **The preview shrinks without distorting.** The `<video>` takes `object-contain` inside its
  region, so a shrunk preview is letterboxed against the existing black background rather than
  squashed. The frame you see stays the frame you capture: `capture()` draws from `videoWidth` and
  `videoHeight`, which are the stream's own dimensions and are unaffected by any of this.
- **Scrolling is the fallback, not the mechanism.** The preview region gets `overflow-y-auto` for
  the extreme case where even a shrunk preview does not fit. Because the Capture button lives in
  the footer, outside that scroll region, it stays on screen at any viewport height — including
  the case where the preview is scrolled. The error state renders in the same region, so a long
  message stays readable and Cancel in the header stays reachable.
- **Absorb the bottom safe-area inset in the overlay's own padding.** The Capture button now sits
  at the bottom of the screen, which in landscape on a notched device is where the home indicator
  is. ADR-008 deliberately excludes full-screen overlays — `CameraCapture` among them — from the
  `--nav-block` rule, because covering the navigation bar is the point, and that exclusion stands.
  But it also means no element underneath absorbs the inset for this overlay, so the overlay must
  absorb it itself with a literal `env(safe-area-inset-bottom)` term rather than `--edge-inset-b`,
  which is `0px` below the `sm` breakpoint precisely because the bar is normally there to absorb
  it. Use `max(1rem, env(safe-area-inset-bottom))` so the existing 1rem padding is the floor.

**Trade-offs and exclusions.**

- *Rejected: a landscape-specific layout that puts the Capture button beside the preview.* It
  reads more like a native camera app, but it needs an orientation media query to be correct — a
  width breakpoint misfires here for the reason `TapTarget` already records, since a 375x667 phone
  in landscape is 667px wide and reads as "desktop" to any `sm:` test. The shrink-to-fit column
  fixes the reported defect at every viewport and orientation with one rule set and no breakpoint.
- *Excluded: the sibling modals.* `ClarifyModal` and `CustomFoodModal` are unbounded in the same
  way and can overflow a short viewport too, but neither was reported, both are text-only and
  therefore much shorter, and widening the change means touching the confirm and clarify flows
  that carry their own regression tests. If they need the same treatment, that is its own idea.
- *Excluded: an ADR.* This is one component's layout, not a cross-cutting pattern, and it neither
  contradicts nor stales any fact ADR-008 states — the overlay exclusion it records is exactly
  what the safe-area reasoning above relies on.
- *No new user-visible text*, so nothing to add to `frontend/lib/i18n/en.ts`.

**Verification.** The gate for this change is the E2E suite, because the defect is a rendering
outcome: `make lint` covers the backend only, and the Vitest suite has no component-rendering
setup. Add coverage to the `In-app camera capture` describe block in `e2e/tests/food.spec.ts`,
using the `addInitScript` `navigator.mediaDevices` mock the existing `camera capture uses the same
hinted upload path` test already establishes — the deployed stack is a plain-HTTP origin, so the
real `getUserMedia` path is unreachable there and the component shows its secure-context error
instead. Assert against bounding boxes rather than pixel offsets, the way
`e2e/tests/mobile-nav.spec.ts` does, so the cases survive any later restyling. The safe-area half
is not fully testable: headless Chromium reports `env(safe-area-inset-bottom)` as 0 and offers no
way to set it, so E2E can assert the padding is declared with an `env()` term but not that a
non-zero inset behaves correctly — the same residue ADR-008 records, and a manual check on a
notched device.

## Validation Commands
- make lint
- make test
- make test-e2e

### Task 1: Bound the capture modal to the visible viewport
- [ ] In `frontend/components/food/CameraCapture.tsx`, clamp the overlay to the dynamic viewport
      (`dvh`) so the card is bounded by the space actually on screen, not by the large viewport a
      `fixed inset-0` element resolves to.
- [ ] Restructure the card into a flex column with three regions: the existing header, a preview
      region, and a new footer holding the Capture `TapTarget`.
- [ ] Give the header and footer `shrink-0`, and the preview region `flex-1` plus `min-h-0`, so
      the preview is the only region that gives way when the viewport is short. Note in a comment
      that `min-h-0` is what actually allows the shrink, since a flex item's automatic minimum
      size otherwise pins it to its content height.
- [ ] Give the `<video>` `object-contain` so a shrunk preview is letterboxed rather than
      distorted, and confirm `capture()` still draws at the stream's own `videoWidth`/
      `videoHeight` — the captured image must not change with the preview's rendered size.
- [ ] Give the preview region `overflow-y-auto` as the last-resort fallback, and keep the Capture
      button outside that scroll region so it cannot be scrolled off screen.
- [ ] Render the error state inside the same preview region, so a long message scrolls and the
      header's Cancel control stays reachable at any viewport height.
- [ ] Mark completed

### Task 2: Absorb the bottom safe-area inset in the overlay
- [ ] Give the overlay a bottom padding of `max(1rem, env(safe-area-inset-bottom))`, so the
      Capture button clears a home indicator in landscape without shrinking the existing 1rem
      padding on devices with no inset.
- [ ] Add a comment stating why the literal `env()` term is used rather than `--edge-inset-b`:
      ADR-008 leaves full-screen overlays outside the `--nav-block` rule, so no element underneath
      absorbs the inset for this overlay, and `--edge-inset-b` is `0px` below the `sm` breakpoint
      exactly because the navigation bar normally does.
- [ ] Confirm no `--nav-block` offset is added — covering the navigation bar is deliberate for
      this overlay and ADR-008 says so.
- [ ] Mark completed

### Task 3: Cover the landscape case in E2E
- [ ] In the `In-app camera capture` describe block in `e2e/tests/food.spec.ts`, add a landscape
      case at a short viewport (for example 740x320, a phone held horizontally with browser
      chrome showing) that opens the camera through the existing `addInitScript`
      `navigator.mediaDevices` mock and asserts the Capture button is visible, enabled, and has a
      bounding box fully inside the viewport (`y >= 0` and `y + height <= viewport height`).
- [ ] Assert the button is genuinely usable, not merely painted, with `click({ trial: true })` —
      the assertion a stacking-order-only change would not satisfy.
- [ ] Assert the preview keeps a usable height at that viewport (a non-trivial floor, not just a
      non-zero box), so a "fix" that collapses the preview to nothing fails here.
- [ ] Assert the card's own bounding box fits within the viewport height, which is the invariant
      the clipping violated.
- [ ] Add the same assertions at a portrait mobile viewport (390x844) and at the desktop viewport
      the existing camera tests run at, proving the change fixes landscape without regressing the
      orientations that already worked.
- [ ] Assert the overlay's `padding-bottom` is declared with an `env()` term, and state in a
      comment that the non-zero-inset half is a manual check on a notched device because headless
      Chromium reports the inset as 0 — the same limitation ADR-008 records.
- [ ] Confirm the pre-existing camera cases (`camera capture uses the same hinted upload path` and
      the secure-context error case) still pass unchanged.
- [ ] Mark completed

### Task 4: Validate the change against the deployed stack
- [ ] Re-read every modified file and check the layout classes for consistency with the
      surrounding code, unused leftovers from the old structure, and any class that reintroduces a
      fixed height.
- [ ] Run `make lint` and `make test`, and note in the summary that both cover the backend and
      the pure-function Vitest suite only — this change's real gate is the E2E run below.
- [ ] Deploy the branch to the `hcw-wip` stack and run `make test-e2e` against it, which is what
      the new landscape cases need in order to mean anything.
- [ ] Summarize which files changed and what each change does, and disclose the one part not
      covered by any automated check: the non-zero safe-area inset.
- [ ] Mark completed
