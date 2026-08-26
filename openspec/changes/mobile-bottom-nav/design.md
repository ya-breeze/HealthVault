## Context

A mobile audit of `hcw-wip` measured the dashboard at a 390px viewport: the shared header takes
177px (21% of the fold) and the photo-logging action sits 1.04 folds down. The frontend contains
three responsive utilities in total, so its layout is a narrowed desktop rather than an adapted
mobile one.

`Header.tsx` renders seven controls in a `flex-wrap` row. Wrapping is why the header *grows* on a
narrow viewport instead of shrinking — it is the mechanism behind the 177px, not an incidental
detail. Nine authenticated pages render `<Header />` themselves; there is no shared authenticated
shell today.

Constraints carried in from the surrounding codebase:

- `frontend/AGENTS.md`: this Next.js version has breaking changes from what is in training data.
  Read the relevant guide under `node_modules/next/dist/docs/` before writing component code.
- The app is statically exported. Nothing here may depend on a server request per navigation.
- `TapTarget` (`components/ui/TapTarget.tsx`) is the single place tap-target dimensions are
  expressed. New controls use it rather than restating `min-h-12`.
- Russian is a first-class UI language, and `LanguageContext` supplies `t()`. Russian labels are
  systematically longer than English ones, which is the binding constraint on tab width.

## Goals / Non-Goals

**Goals:**

- Both logging entry points reachable without scrolling, from every authenticated page, on mobile.
- Mobile header stops wrapping and stops consuming a fifth of the fold.
- Desktop **rendered output** unchanged. Not the DOM: the bar and sheet ship in the markup at every
  width and are hidden above the breakpoint (see "The bar is CSS-hidden"), so the desktop DOM does
  gain nodes. What must not change is what a desktop user sees and can interact with.
- Every control the mobile header sheds stays reachable.
- No bottom-anchored element the app already renders is occluded by the bar, or occludes it.

**Non-Goals:**

- No new visual language. Palette, type scale and card density stay on current tokens; the audit's
  third decision is deferred to its own change.
- No dashboard body restructuring. The in-body "Log food" row stays where it is.
- No change to the other six authenticated screens beyond receiving the shell.
- No backend, API, or data-model change.

## Decisions

### Viewport width, not pointer type, gates the bar

The sibling `mobile-touch-targets` work concluded the opposite for tap targets — that pointer type
is the right axis and viewport width is the wrong one, because a phone in landscape is wider than a
mobile breakpoint but still needs a thumb-sized target. A reviewer who knows that change will
expect the same conclusion here. It does not follow, and the reason is that the two conditions
solve different problems.

The header's defect is *horizontal*: it wraps only when the viewport is too narrow to hold seven
controls in one row. The bar's cost is *vertical*: roughly 56px plus safe-area inset, permanently.
A phone in landscape at 667×375 is the case where vertical space is scarcest and horizontal space
is sufficient — exactly where the bar is least justified and the header already fits on one row.
Gating on pointer type would put the bar there and take 15% of a 375px-tall viewport to solve a
problem that viewport does not have.

Alternative considered: `(pointer: coarse) and (max-width: …)`. Rejected as strictly more complex
with no case that distinguishes it — every coarse-pointer device below the width breakpoint is
already covered, and a fine-pointer device below it (a narrow desktop window) is better served by
the bar than by a three-row wrapped header.

**The cost of this gate, stated plainly:** a 667×375 phone in landscape gets no bar and the full
desktop header. The argument above is only about the bar's vertical cost there; the consequence is
that on that device *this change fixes nothing*. "Log a meal by photo" stays 1+ folds down on the
dashboard and stays absent from every other screen — the exact defect the change exists to fix, on
the orientation where vertical space is scarcest. `TapTarget.tsx:18` already flags 667px-landscape
as the case a width gate mishandles, and this is the same trade made in the other direction.

That is accepted, not overlooked, on two grounds: sustained data entry on a phone in landscape is
not a flow this app has, and the alternative gate costs 15% of a 375px viewport on every screen to
serve it. If landscape use turns out to matter, the fix is a third state (bar shown, labels dropped
to icons only) rather than flipping the gate — noted here so the option is not lost.

### Breakpoint at 640px, defined once

640px is Tailwind's `sm`, already the project's only existing breakpoint (all three of its
responsive utilities use it). Introducing a second, differently-valued breakpoint for navigation
would leave the frontend with two notions of "mobile" that drift.

The spec requires a single definition. In practice that means the `sm:` variant is the definition
and both `BottomNav` and `Header` key off it — not a duplicated `matchMedia('(max-width: 639px)')`
in component logic, which would desynchronize from the CSS at exactly the boundary and, being
JS-evaluated, would also render the wrong branch during static export hydration.

Consequence worth stating: a 768px tablet in portrait gets the desktop header and no bar. That is
deliberate — at 768px the header fits on one row and the 21%-of-fold defect does not occur.

### The bar is CSS-hidden, not conditionally mounted

`BottomNav` renders into the DOM at every width and is hidden above the breakpoint with `sm:hidden`
(the header's mobile-only pieces mirror this with `hidden sm:flex`). Both branches are therefore
present in the exported HTML and the boundary is resolved purely by CSS.

The alternative — mounting on a `matchMedia` result — evaluates to a fixed branch during static
export, so the served HTML would carry one viewport's navigation and correct itself only after
hydration. That is a visible flash on every page load and, worse, a wrong-branch render for any
client that does not hydrate.

The cost is that both branches' markup ships on every page. For five links and a sheet this is
negligible, and it is what makes the transition instant on resize.

**This decides how the desktop scenarios are asserted**, and the spec is worded to match. At a
desktop width the bar is in the DOM but not visible, so the assertion is `toBeHidden()` /
`not.toBeVisible()` — `toHaveCount(0)` would fail. The unauthenticated case is the opposite: the
shell is never applied to `/login`, so there the assertion genuinely is `toHaveCount(0)`. Two
scenarios that read similarly in prose need different assertions, so each states which it means
rather than both saying "SHALL NOT be rendered".

### One shell component, not nine edited pages

Introduce `AuthenticatedShell` wrapping header + `{children}` + bottom bar + clearance, and replace
each page's `<Header />` with it. The clearance requirement is then enforced structurally in one
place rather than depending on each page remembering bottom padding — the same reasoning that put
tap-target sizing inside `TapTarget`.

**Eleven call sites across nine files**, not nine sites: `app/food/review/ReviewClient.tsx` renders
`<Header />` three times, at lines 162, 170 and 192 — its loading, error and main branches. All
three convert. Converting only the main branch would leave the review page's loading and error
states with no bar and, more consequentially, no bottom clearance, which is exactly the kind of
partial adoption a per-page padding approach was rejected to avoid. `grep -rn "<Header" frontend`
is the check, and it must return zero hits outside the shell when the change is done.

Alternative considered: put the bar in `app/layout.tsx`. Rejected — `layout.tsx` also wraps
`/login`, which must not show the bar. (`layout.tsx` is still edited by this change, but only to
add the `viewport` export; see below. It gets no clearance padding.)

**The shell owns the session fetch.** `Header.tsx:26` currently calls `api.me()` in a `useEffect`
and redirects to `/login` on failure; the webhook URL it renders is `${origin}/webhook/${me.username}`.
The More sheet needs that same `me`. Calling `api.me()` a second time from `MoreSheet` would double
the request on every authenticated page load and could render the two surfaces from two different
responses. So the fetch and the redirect-on-failure move up into `AuthenticatedShell`, which passes
`me` down to both `Header` and `MoreSheet` as a prop. `Header` keeps working the same way from the
outside; it just stops being the thing that fetches. No new context is introduced — one prop through
one shell is enough, and a context would be a second way to reach the session.

### `viewport-fit=cover` is a prerequisite, not an optional extra

`app/layout.tsx` exports `metadata` but no `viewport`, so the rendered `<meta name="viewport">` has
no `viewport-fit=cover`. Without that, `env(safe-area-inset-bottom)` resolves to `0` on **every**
device — a real iPhone included, not just headless Chromium. The two `pb-[calc(0.75rem+env(safe-area-inset-bottom))]`
rules already in `ReviewClient.tsx:303` and `food/manual/page.tsx:89` are therefore inert today.

This change adds `export const viewport: Viewport = { viewportFit: 'cover' }` to `app/layout.tsx`.
It is a prerequisite: without it the bar's own safe-area padding is dead CSS and the bar ships
tucked under the home indicator on every notched phone. It is listed as a task, not left implicit.

Two consequences to be aware of when reviewing the diff:

- The two existing submit bars' `env()` padding **becomes live**. That is a real (correct) visual
  change on notched devices to pages this change otherwise only re-parents. It is intended.
- With `viewport-fit=cover` the page paints into the safe areas, so anything already flush to a
  screen edge relies on its own inset padding. The audit found no such element besides the two
  submit bars and the toast stack, all three of which this change handles below.

### One vertical budget token; every bottom-anchored element reads it

Padding on the shell reserves space for **in-flow** content, which is what the scroll-to-bottom case
needs. It does nothing for a `position: fixed` descendant — padding on an ancestor does not move a
fixed element, because a fixed element is out of flow relative to the viewport, not the ancestor.
The app already has three bottom-anchored fixed elements, and a shell-padding-only design occludes
all three:

| Element | Today | Collision |
|---|---|---|
| `app/food/manual/page.tsx:89` submit bar | `fixed bottom-0 … ` , no `z-`| `/food/manual/` **is a promoted tab** — the bar lands on its submit button |
| `app/food/review/ReviewClient.tsx:303` submit bar | `fixed bottom-0 …`, no `z-` | same markup, reached from History |
| `components/Toast.tsx:47` toast stack | `fixed bottom-4 inset-x-0 z-50` | `useToast` is the app-wide error path — every toast lands in the bar's 4rem |

So clearance is expressed once, as a token, and everything bottom-anchored offsets by it:

```css
/* globals.css. --breakpoint-sm is Tailwind v4's theme variable — the single
   breakpoint definition the spec requires; `sm:` variants and this media
   query both derive from it, so they cannot drift. */
:root {
  --nav-block: calc(var(--nav-bar-h) + env(safe-area-inset-bottom));
  --edge-inset-b: 0px;
}
@media (width >= theme(--breakpoint-sm)) {
  :root { --nav-block: 0px; --edge-inset-b: env(safe-area-inset-bottom); }
}
```

- shell clearance: `padding-bottom: var(--nav-block)` (in-flow content)
- the two submit bars: `bottom: var(--nav-block)`, own padding `calc(0.75rem + var(--edge-inset-b))`
- the toast stack: `bottom: calc(1rem + var(--nav-block) + var(--edge-inset-b))`

**Why two tokens and not one.** The safe-area inset is absorbed by whichever element is bottom-most,
and which element that is changes at the breakpoint. Below it the bar is bottom-most and `--nav-block`
carries the inset, so a submit bar resting on the bar needs none of its own. Above it `--nav-block`
collapses to `0px`, the submit bar is flush to the screen edge again, and it needs the inset itself —
a 667px phone in landscape is above the breakpoint *and* has a home indicator, so this is a real
configuration and not a hypothetical. A single token forces a choice between double-padding below the
breakpoint and no padding above it; `--edge-inset-b` is the complement that makes one expression
correct in both regimes. It is also why the submit bars' existing `env()` padding is *rewritten*
rather than deleted when they adopt the offset.

The in-flow half stays padding on the shell rather than a margin on the last child: padding on the
scroll container is robust against whatever the page renders last, whereas a margin on the final
child breaks the moment that element is conditionally rendered — and several of these pages end in
a conditional block (the dashboard's More Data section is one).

Above the breakpoint `--nav-block` is `0px`, so all three resolve to exactly their current values
and desktop is untouched — which is what lets this be a one-line change at each site rather than a
`sm:` variant at each site. It also means the submit bars stop being independently responsible for
their own `env()` padding, removing the duplicated inset arithmetic rather than adding a third copy.

Rejected alternative: give the bar a high `z-index` and let it sit over the submit bars. That keeps
the submit button unreachable on `/food/manual/` — a z-index fight resolves *which* element is on
top, not whether the lower one is usable. The elements must not overlap at all.

**Stacking order**, since none of these carry an explicit `z-` today except the toast stack:
full-screen modals (`CameraCapture`, `ClarifyModal`, `CustomFoodModal`, all `fixed inset-0 z-50`) and
the More sheet stay at `z-50` and deliberately cover the bar; the toast stack stays at `z-50` and
now clears it geometrically; the bar takes `z-40`; the two submit bars take `z-30`. The bar being
*below* the modals is the point — a full-screen capture UI covering it is correct behaviour, and it
is why `CameraCapture` needs no offset.

### Five tabs at 320px: labels are the constraint, not tap targets

320px ÷ 5 = 64px per destination, which clears the 48px minimum on both axes, so the tap-target
guarantee holds at the narrowest supported width. What does not fit is the Russian labels —
"Вручную" (manual) and "История" (history) at the current label size overflow 64px.

Resolution: labels shrink to the smallest size the type scale offers and wrap to no more than one
line, with the icon carrying primary identification. If a label still overflows at 320px, the label
is what truncates — the tap target does not shrink to accommodate it. This is why the spec fixes
the 48×48 guarantee down to 320px explicitly rather than at "a mobile viewport width".

### Active state is not carried by color alone

Per the spec. The active destination gets a filled icon variant and a weight change alongside the
accent color, so the indication survives both a monochrome rendering and the deferred visual
change that will move the palette.

## Risks / Trade-offs

- **Two navigation implementations can drift** — a control added to the desktop header and not to
  the More sheet becomes mobile-unreachable, silently. → The spec's "No control is stranded on
  mobile" scenario is written to be a real test, enumerating the sheet's controls; it fails when
  the sets diverge.
- **`env(safe-area-inset-bottom)` cannot be given a non-zero value in headless Chromium.** Note
  this is now the *only* remaining obstacle — the `viewport-fit=cover` decision above fixes the
  substantive half, which was that the inset was 0 on real devices too. What survives is a test-rig
  limit: e2e can assert the `viewport-fit=cover` meta tag is emitted and that the bar's computed
  `padding-bottom` contains the `env()` term, but it cannot observe a non-zero inset. The residue is
  a manual check on a notched device, disclosed rather than claimed as e2e-covered.
- **The in-body "Log food" row becomes arguably redundant** once Photo and Manual are permanent
  tabs. → Deliberately left in place. Removing it is a dashboard-body decision, and bundling it
  here would put a second, separately-arguable change inside this PR.
- **Bar occludes content on pages with their own fixed elements.** → Enumerated, not left to
  discovery during implementation: two `fixed bottom-0` submit bars (`food/manual`, `food/review`)
  and the app-wide toast stack, all handled by the `--nav-block` token above. `CameraCapture` and
  the two other `inset-0` modals are the benign case — they cover the bar by design. The risk that
  remains is a *future* bottom-anchored element added without reading the token; the e2e case
  asserting no overlap on `/food/manual/` is what catches that.
- **Eleven call sites across nine pages change to adopt the shell**, so a mistake touches every
  authenticated screen at once. → The change is mechanical and identical per site; e2e covers all
  nine pages via the header-presence scenarios that already exist, and `grep -rn "<Header"`
  returning zero hits outside the shell is the completeness check.
- **The bar flashes for unauthenticated visitors on deep links.** The app is `output: 'export'` and
  the auth check is a client-side `api.me().catch(() => router.push('/login'))`. A visitor hitting
  `/` or `/food/history/` without a session gets the static HTML — shell, header and bar — painted
  before the redirect fires. → The shell renders its chrome only once the session has resolved;
  until then it renders the page area alone. This costs nothing on the authenticated path (the
  chrome appears with the content it frames) and is why the unauthenticated scenario is written
  against a deep link rather than only `/login`.

## Migration Plan

No data migration. Deploy is a frontend rebuild. Rollback is redeploying the prior image — nothing
persists that would outlive a revert.

## Open Questions

- **Does the dashboard's in-body "Log food" row survive?** Deferred to the follow-up change that
  restructures the dashboard body; noted here so the question is not lost.
- **Bar height** is stated as ~56px above but should be settled against the 48px tap-target
  minimum plus label line during implementation — 48px of target plus label may want 60–64px.
  Whatever it lands on becomes `--nav-bar-h`, defined once; every consumer reads `--nav-block`, so
  settling it late costs one edit rather than five.
