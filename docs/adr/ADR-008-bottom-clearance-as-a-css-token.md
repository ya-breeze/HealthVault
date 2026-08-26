# ADR-008: The Mobile Navigation Bar's Clearance Is a CSS Token Every Bottom-Anchored Element Reads

## Status
Accepted

## Context and Problem Statement

The mobile bottom navigation bar (`mobile-bottom-nav`) is `position: fixed` at the bottom of the
viewport below the `sm` breakpoint. Anything else the app anchors to the bottom of the viewport
now shares that space with it, and a control that ends up underneath the bar cannot be tapped at
all.

Two mechanisms are needed, not one, and they are not interchangeable:

- **In-flow content** — the last row of a scrolled list — needs the page to *reserve* space, which
  is padding on a container.
- **`position: fixed` elements the page renders itself** need to be *moved*, because a fixed
  element is out of flow relative to the viewport, not to its ancestor: no amount of padding on a
  parent shifts it. The app has three today — the submit bars on `/food/manual/` and
  `/food/review/`, and the app-wide toast stack.

Layered on top is the safe-area inset. `env(safe-area-inset-bottom)` must be absorbed exactly
once, by whichever element is bottom-most — and which element that is *changes at the
breakpoint*. Below it the bar is bottom-most; at and above it the bar does not render and a page's
own submit bar is flush to the screen edge again. A handheld in landscape is both above the
breakpoint and a device with a home indicator, so both regimes are real configurations rather than
one being hypothetical.

The question this ADR settles: where does "how much room the bar takes" live, and what is every
future bottom-anchored element obliged to do about it?

## Decision Drivers

- A control rendered underneath the bar is unusable. This is a functional defect, not a cosmetic
  one, and it fails silently — the element is present, visible in the DOM, and simply not
  reachable by a thumb.
- The failure has no mechanical guard. TypeScript cannot see it, there is no frontend linter, and
  a reviewer reading a diff that adds `fixed bottom-0` to a new page has nothing prompting them to
  ask about the bar.
- The bar's height was an open question during implementation (48px of tap target plus a label
  line — it landed on 4rem). Whatever it settled on had to be changeable in one place.
- Desktop rendering had to be provably untouched, which means the mechanism has to collapse to
  exactly today's values at and above the breakpoint rather than needing an `sm:` variant at each
  site.

## Considered Options

- **Padding on the shell alone.** Rejected: it handles the in-flow half and silently occludes all
  three existing fixed elements, `/food/manual/`'s submit button among them — and `/food/manual/`
  is one of the bar's own destinations, so the bar would land on the button of a page it promotes.
- **A high `z-index` on the bar, letting it sit over the submit bars.** Rejected: stacking order
  decides which element *paints* on top, not whether the one underneath is usable. The submit
  button stays unreachable. The elements must not overlap at all.
- **Each site hardcodes the bar's height.** Rejected: five copies of a number that was still
  unsettled when the sites were written, and nothing keeps a sixth site's copy correct.
- **One token carrying height plus inset.** Rejected: it forces a choice between double-padding
  below the breakpoint (the bar absorbs the inset *and* the submit bar adds it again) and dropping
  the inset above it (where no bar is beneath to absorb it). Neither is correct in both regimes.

## Decision Outcome

Chosen: **two CSS custom properties in `frontend/app/globals.css`, redefined once at the
breakpoint, which every bottom-anchored element reads.**

```css
:root {
  --nav-bar-h: 4rem;
  --nav-block: calc(var(--nav-bar-h) + env(safe-area-inset-bottom));
  --edge-inset-b: 0px;
}
@media (width >= theme(--breakpoint-sm)) {
  :root { --nav-block: 0px; --edge-inset-b: env(safe-area-inset-bottom); }
}
```

- `--nav-block` is the space the bar occupies, inset included. It is `0px` at and above the
  breakpoint.
- `--edge-inset-b` is the complement: the inset an element anchored at `bottom: var(--nav-block)`
  must still absorb *itself*. It is `0px` below the breakpoint, because the bar underneath has
  already absorbed it, and `env(safe-area-inset-bottom)` above it, where the element is flush to
  the screen edge.

The media query reads Tailwind v4's own `--breakpoint-sm`, so it and every `sm:` variant in the
app derive from one definition and cannot drift apart.

**The obligation this places on future code.** Any element that anchors itself to the bottom of
the viewport MUST offset by `--nav-block` rather than sitting at `bottom-0`, and MUST absorb
`--edge-inset-b` in its own bottom padding rather than `env(safe-area-inset-bottom)` directly. The
bar itself takes `--nav-block` as its own height — not `--nav-bar-h` plus whatever its border
adds — so that what it occupies and what everything else clears are the same value by
construction. (The first implementation got this wrong by one pixel, sizing the content box and
then adding a top border, and both submit bars overlapped the bar by exactly that pixel.)

Deliberately outside the rule: full-screen overlays that cover the whole viewport on purpose —
`CameraCapture`, `ClarifyModal`, `CustomFoodModal`, and the More sheet. They are expected to cover
the bar, and the bar sitting below them in the stacking order is the point.

### Consequences

- Settling the bar's height differently later is one edit to `--nav-bar-h`. No consumer names a
  height.
- Desktop is untouched by construction: at and above the breakpoint all three existing sites
  resolve to exactly the values they had before this change, which is what let each be a one-line
  edit rather than a responsive variant.
- **The rule is not mechanically enforced.** Nothing fails to compile when a new bottom-anchored
  element ignores it. The guard is the e2e case in `e2e/tests/mobile-nav.spec.ts` asserting that
  the submit bar's and the toast's bounding boxes do not intersect the bar's — which catches a
  regression on the three known sites, but cannot catch a *fourth* site added later on a page it
  does not visit. That gap is why this decision is recorded here rather than only in the change's
  `design.md`.
- The inset half is only partly testable. Headless Chromium reports `env(safe-area-inset-bottom)`
  as 0 and offers no way to set it, so e2e can assert the `viewport-fit=cover` meta tag is emitted
  and that the bar's `padding-bottom` is declared with an `env()` term, but not that a non-zero
  inset behaves correctly. That residue is a manual check on a notched device.
- `viewport-fit=cover` is a prerequisite, added to `app/layout.tsx` by the same change. Without it
  `env(safe-area-inset-*)` resolves to 0 on every device, and every rule above is dead CSS. It
  also makes the two submit bars' pre-existing inset padding live for the first time — an intended
  visual change on notched devices to pages that change in no other way.
