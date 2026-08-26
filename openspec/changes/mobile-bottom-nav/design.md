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
- Desktop rendering byte-for-byte unchanged.
- Every control the mobile header sheds stays reachable.

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

### One shell component, not nine edited pages

Introduce `AuthenticatedShell` wrapping header + `{children}` + bottom bar + clearance, and replace
each page's `<Header />` with it. Nine call sites change once, and the clearance requirement is
enforced structurally in one place rather than depending on nine pages each remembering bottom
padding — the same reasoning that put tap-target sizing inside `TapTarget`.

Alternative considered: put the bar in `app/layout.tsx`. Rejected — `layout.tsx` also wraps
`/login`, which must not show the bar, and the shell needs the authenticated session that `Header`
already fetches.

### Clearance via padding on the shell, not margin on the last child

The shell reserves `padding-bottom: calc(<bar height> + env(safe-area-inset-bottom))` below the
breakpoint. Padding on the scroll container is robust against whatever the page renders last;
a margin on the final child breaks the moment a page's last element is conditionally rendered —
and several of these pages end in a conditional block (the dashboard's More Data section is one).

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
- **`env(safe-area-inset-bottom)` is untestable in headless Chromium**, which reports 0. → The
  inset scenario can only be verified by construction (the CSS is present and well-formed) plus a
  manual check on a real device. This will be disclosed rather than claimed as e2e-covered.
- **The in-body "Log food" row becomes arguably redundant** once Photo and Manual are permanent
  tabs. → Deliberately left in place. Removing it is a dashboard-body decision, and bundling it
  here would put a second, separately-arguable change inside this PR.
- **Bar occludes content on pages with their own fixed elements.** → The camera capture flow is the
  realistic case; its full-screen state needs checking against the bar during implementation.
- **Nine pages change to adopt the shell**, so a mistake touches every authenticated screen at
  once. → The shell change is mechanical and identical per page; e2e covers all nine via the
  header-presence scenarios that already exist.

## Migration Plan

No data migration. Deploy is a frontend rebuild. Rollback is redeploying the prior image — nothing
persists that would outlive a revert.

## Open Questions

- **Does the dashboard's in-body "Log food" row survive?** Deferred to the follow-up change that
  restructures the dashboard body; noted here so the question is not lost.
- **Bar height** is stated as ~56px above but should be settled against the 48px tap-target
  minimum plus label line during implementation — 48px of target plus label may want 60–64px.
