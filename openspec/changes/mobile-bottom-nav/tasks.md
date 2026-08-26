# Tasks

## 0. Groundwork

- [x] 0.1 Read the routing/layout guide under `frontend/node_modules/next/dist/docs/` before
  writing component code — `frontend/AGENTS.md` states this Next.js version has breaking changes
  from what is in training data. Note anything that affects a client-component shell wrapping a
  statically-exported page.
- [x] 0.2 Confirm 640px (`sm:`) is the only breakpoint in use. Match the variant only where it
  prefixes a utility, and count occurrences rather than eyeballing:
  `grep -rhoE '(^|[^a-zA-Z0-9-])(sm|md|lg|xl|2xl):' frontend/app frontend/components | grep -oE '[a-z0-9]+:' | sort | uniq -c`.
  (The earlier form of this command piped `grep -rn` output through `grep -o '\b[a-z]*:'`, so it
  matched its own `path:line:` prefix and reported `tsx:` and a bare `:` as breakpoints.) If a
  variant other than `sm:` appears, resolve which is canonical before building on the assumption.
- [x] 0.3 Measure the baseline so the change has a before/after number: header height and the
  y-offset of the photo action on the dashboard at 390×844 against `hcw-wip`. Record both.
- [x] 0.4 Confirm the two known bottom-anchored submit bars and the toast stack are still the
  complete set before relying on the enumeration:
  `grep -rn 'fixed bottom-\|fixed inset-x-0' frontend/app frontend/components`. Expected today:
  `app/food/manual/page.tsx:89`, `app/food/review/ReviewClient.tsx:303`, `components/Toast.tsx:47`.
  Anything else found is a fourth site that also needs the clearance token in task 3.2.

## 1. Icons and labels

- [x] 1.1 Add `HomeIcon` and `MoreIcon` (horizontal ellipsis or stacked bars) to
  `frontend/components/icons.tsx`, matching the existing icons' `className`-driven sizing
  convention. `CameraIcon`, `PencilIcon` and `HistoryIcon` already exist and are reused as-is.
- [x] 1.2 Add filled variants for the five destination icons, or an `active` prop on each, for the
  active-state indication that design.md requires not be carried by color alone.
- [x] 1.3 Add tab labels to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts` under a `nav.*`
  key group: home, photo, manual, history, more.
- [x] 1.4 Add More-sheet labels to both locale files, reusing the existing `header.*` strings where
  the sheet's control is the same control (webhook, custom foods, import, settings, logout) rather
  than duplicating them under new keys.

## 2. Navigation components

- [x] 2.1 Create `frontend/components/BottomNav.tsx`: five destinations, `fixed bottom-0 inset-x-0`,
  hidden at and above the breakpoint via `sm:hidden`. Each destination is a `TapTarget` — do not
  restate `min-h-12`/`min-w-12` at the call site.
- [x] 2.2 Derive the active destination from the current pathname. Exactly one active at a time;
  none active on a route matching no destination (`/data/[type]`, `/settings`, `/import`,
  `/food/custom`, `/food/review`). Match on the route, not on a prefix that would make Home active
  everywhere.
- [x] 2.3 Create `frontend/components/MoreSheet.tsx` carrying webhook URL + copy action, custom
  foods, import, settings, logout. Lift the webhook copy logic (clipboard with `execCommand`
  fallback) out of `Header.tsx` into a shared place rather than duplicating it — it is ~20 lines of
  branching that must not diverge between the two surfaces.
- [x] 2.3a The webhook URL is `${origin}/webhook/${me.username}`, so the sheet needs the session.
  Take `me` as a prop rather than calling `api.me()` a second time — task 3.1a moves that fetch into
  the shell, which passes the same object to both `Header` and `MoreSheet`. Do not introduce a
  session context for this; one prop through one shell is enough.
- [x] 2.4 Sheet dismissal: backdrop click and Escape both close without navigating. Return focus to
  the More destination on close.
- [x] 2.5 Sheet is a modal surface — `role="dialog"`, `aria-modal`, focus trapped while open,
  background scroll locked.

## 3. Shell and header

- [x] 3.0 Add the `viewport` export to `frontend/app/layout.tsx`:
  `export const viewport: Viewport = { viewportFit: 'cover' }` (type imported from `next`). Without
  it `env(safe-area-inset-bottom)` is `0` on every device — real phones included, not just headless
  Chromium — and every inset rule in this change is dead CSS. Verify by loading a page and checking
  the rendered `<meta name="viewport">` contains `viewport-fit=cover`. Note this also activates the
  two existing `pb-[calc(0.75rem+env(safe-area-inset-bottom))]` rules in the submit bars, which are
  inert today; that visual change on notched devices is intended.
- [x] 3.1 Create `frontend/components/AuthenticatedShell.tsx` wrapping `<Header />`, `{children}`,
  and `<BottomNav />`, and applying the bottom clearance from the token defined in 3.2.
- [x] 3.1a Move the session fetch out of `Header.tsx` and into the shell: the `api.me()` call and its
  `.catch(() => router.push('/login'))` redirect. Pass `me` down to both `Header` and `MoreSheet`.
  `Header` stops fetching and takes `me` as a prop. This is what stops the sheet needing a second
  request per page load, and stops the two surfaces rendering from two different responses.
- [x] 3.1b Render the shell's chrome (header and bar) only once the session has resolved; until then
  render the page area alone. The app is statically exported and the auth check is client-side, so
  an unauthenticated visitor deep-linking to `/` or `/food/history/` otherwise sees header and bar
  painted before the redirect fires.
- [x] 3.2 Settle the bar height against 48px of tap target plus one label line (design.md's open
  question) and define the tokens once in `frontend/app/globals.css`. Use Tailwind v4's `theme()`
  in the media query so the breakpoint is not restated — `--breakpoint-sm` stays the single
  definition. Every element that must clear the bar reads these and nothing hardcodes the height:
  ```css
  :root {
    --nav-bar-h: <settled height>;
    /* space the bar occupies, including the inset it absorbs */
    --nav-block: calc(var(--nav-bar-h) + env(safe-area-inset-bottom));
    /* inset an element anchored at bottom:var(--nav-block) must still absorb itself.
       Below the breakpoint the bar is underneath it and has already absorbed it. */
    --edge-inset-b: 0px;
  }
  @media (width >= theme(--breakpoint-sm)) {
    :root { --nav-block: 0px; --edge-inset-b: env(safe-area-inset-bottom); }
  }
  ```
  Two tokens, not one, because the inset is absorbed by whichever element is bottom-most, and that
  is the bar below the breakpoint but the page's own bar above it.
- [x] 3.2a Offset the app's existing bottom-anchored fixed elements, since padding on the shell does
  not move a `position: fixed` descendant:
  - `app/food/manual/page.tsx:89` — `bottom: var(--nav-block)` instead of `bottom-0`, and its
    padding becomes `calc(0.75rem + var(--edge-inset-b))` in place of
    `pb-[calc(0.75rem+env(safe-area-inset-bottom))]`. This is the important one: `/food/manual/` is
    a promoted destination, so without the offset the bar lands on the page's submit button.
  - `app/food/review/ReviewClient.tsx:303` — same markup, same fix.
  - `components/Toast.tsx:47` — `bottom: calc(1rem + var(--nav-block) + var(--edge-inset-b))`
    instead of `bottom-4`.
  Do **not** simply delete the local `env()` padding on the submit bars: below the breakpoint
  `--nav-block` absorbs the inset for them, but above it `--nav-block` is `0px` and the bar is flush
  to the screen edge again, so it needs the inset itself — a 667px landscape phone is both above the
  breakpoint and a device with a home indicator. `--edge-inset-b` is what keeps both regimes right
  with one expression, which is why the padding is rewritten rather than removed.
  All three sites resolve to their current values above the breakpoint, so none needs an `sm:`
  variant.
- [x] 3.2b Give the bar `z-40` and the two submit bars `z-30` (neither carries a `z-` class today).
  The `inset-0` modals and the toast stack keep `z-50` and are expected to cover the bar. Do not use
  z-index as a substitute for 3.2a — stacking decides which element paints on top, not whether the
  one underneath is usable.
- [x] 3.3 Make `Header.tsx`'s control set viewport-conditional: webhook, custom foods, import,
  settings and logout get `hidden sm:flex`; app name and user badge render at all widths.
- [x] 3.3a Stop the *mobile* header wrapping: the `flex-wrap` at `Header.tsx:76` and `:79` is one
  class on a shared container, not a mobile branch, so it becomes `flex-nowrap sm:flex-wrap` rather
  than being removed. Desktop keeps its current wrapping behaviour exactly — including the fact
  that the full control set can still wrap between 640px and ~768px, which is pre-existing and out
  of scope for this change (recorded in `dashboard-ui`'s spec text).
- [x] 3.4 Verify the reduced mobile header is a single row at 320px, the narrowest supported width,
  with the user badge present — the badge is the widest variable-length element left in it.
- [x] 3.5 Replace `<Header />` with `<AuthenticatedShell>` at all **eleven** call sites across nine
  authenticated pages: `app/page.tsx`, `app/food/custom/page.tsx`, `app/food/upload/page.tsx`,
  `app/food/manual/page.tsx`, `app/food/history/page.tsx`, `app/import/page.tsx`,
  `app/settings/page.tsx`, `app/data/[type]/DataTypeClient.tsx`, and
  `app/food/review/ReviewClient.tsx` — **which renders `<Header />` three times, at lines 162, 170
  and 192 (loading, error and main branches). All three convert.** Converting only the main branch
  leaves the review page's loading and error states with no bar and no bottom clearance.
  `/login` must NOT get the shell.
- [x] 3.5a Completeness check: `grep -rn "<Header" frontend/app frontend/components` returns hits
  only inside `AuthenticatedShell.tsx`.
- [x] 3.6 Confirm the three `fixed inset-0 z-50` full-screen surfaces — `CameraCapture.tsx`,
  `ClarifyModal.tsx`, `CustomFoodModal.tsx` — still cover the bar rather than sitting under it.
  These are the benign case and need no offset; the check is that `z-40` on the bar has not
  inverted the relationship. The submit bars and toasts are handled in 3.2a, not here.

## 4. Verification

> **Measured, against `hcw-wip` at 390×844.** Task 0.3's baseline was taken with `main` deployed
> to that same stack, so the two numbers are comparable rather than being read off different data.
>
> | | before (`main`) | after |
> |---|---|---|
> | Shared header height | 177px (21% of the fold) | **81px** (9.6%) |
> | Photo action, dashboard | y=880.5 — 1.04 folds down | y=780 in the bar; the in-body row also rises to y=784.5 |
> | Photo action, `/data/steps` | **absent from the page entirely** | y=780 |
>
> Desktop diff (4.6, 1280px, before/after pixel comparison): `/` and `/data/steps` are
> **pixel-identical**. `/food/manual/` differs in 256 pixels confined to a 34×11 box at (716,189)
> — the minutes/AM-PM digits of the "When" field, which shows the current wall-clock time and so
> differed between the two capture runs. Nothing differs near the submit bar.
>
> Labels at 320px (4.5): no destination label truncates, in English or in Russian — "Вручную" and
> "История", the longest, fit the 64px column at the 10px label size. Every destination measures
> 64×64 at 320px, clearing the 48×48 minimum.

- [x] 4.1 `cd frontend && npx tsc --noEmit` clean.
- [x] 4.2 `make lint` clean. (It vets the Go backend only — this change is frontend-only, and the
  frontend has no linter configured, so `tsc --noEmit` above plus `next build` are its static
  checks.)
- [x] 4.3 Unit tests pass.
- [x] 4.4 Re-measure 0.3's baseline: header height and photo-action y-offset at 390×844. Both must
  improve, and the photo action must be within the viewport without scrolling.
- [x] 4.5 Sweep tap targets across the bar and sheet at 320, 360, 390 and 430px widths; confirm
  every destination clears 48×48 and record whether any label truncates.
- [x] 4.6 Confirm the desktop rendering is unchanged: screenshot `/` and `/data/steps` at 1280px
  before and after, and diff. Include `/food/manual/` — it is the page whose submit bar changes
  anchoring, so it is where a mistake in `--nav-block` shows up on desktop.
- [x] 4.7 Verify the tokens resolve as intended on `/food/manual/`: at 390px the shell's computed
  `padding-bottom` and the submit bar's computed `bottom` are both non-zero and equal, and the
  submit bar's own bottom padding is `0.75rem` (the bar beneath it absorbs the inset); at 1280px
  the first two are `0px` and the submit bar's padding is back to `0.75rem` plus the inset. With a
  zero inset in headless Chromium the two regimes differ only in the offset, which is the part this
  check can actually settle — the inset half is 6.2a.

## 5. E2E

- [x] 5.1 Add `e2e/tests/mobile-nav.spec.ts`: bar visible below the breakpoint; each destination
  navigates; active state correct on a destination route and absent on `/data/steps`. Match the
  assertion to what the spec means in each case — at desktop widths the bar is in the DOM but
  hidden, so assert `not.toBeVisible()` and that no destination is keyboard-focusable, **not**
  `toHaveCount(0)`; on `/login` the shell is never applied, so there `toHaveCount(0)` is correct.
- [x] 5.1a Cover the unauthenticated deep-link flash: load `/food/history/` with no session and
  assert neither the bar nor the header is ever visible before the redirect to `/login` lands.
- [x] 5.2 Cover the two audit defects directly, so a regression fails a test rather than needing
  another audit: photo and manual actions inside the viewport without scrolling at 390×844 from
  both the dashboard and `/data/steps`; header height below a threshold derived from 4.4.
- [x] 5.3 Cover the More sheet: opens, exposes all five shed controls, logout ends the session and
  lands on `/login`, dismissal closes without navigating.
- [x] 5.4 Add a test that fails when the header's desktop control set and the sheet's control set
  diverge — the "no control stranded on mobile" scenario. Assert against a shared list rather than
  two hand-maintained literals, or the test drifts with the code it guards.
- [x] 5.4a Cover the occlusion cases, which are the ones a screenshot review misses: at 390px on
  `/food/manual/`, assert the submit bar's bounding box does not intersect the bar's and the submit
  control is clickable; same on `/food/review/`; and assert a toast's box does not intersect the
  bar's. Compare bounding boxes rather than asserting a specific pixel offset, so the test survives
  the bar height being settled in 3.2.
- [x] 5.4b Assert the rendered `<meta name="viewport">` contains `viewport-fit=cover`, and that the
  bar's computed `padding-bottom` carries an `env(` term. This is the testable half of the
  safe-area scenario; the non-zero-inset half is manual (see 6.2).
- [x] 5.5 Extend `e2e/tests/mobile-tap-targets.spec.ts` with the bar's five destinations and the
  sheet's controls, including the 320px case.
- [x] 5.6 **`e2e/tests/mobile-tap-targets.spec.ts:120-134` will fail** — its "header nav controls
  meet the 48px minimum" test runs under `test.use({ viewport: MOBILE_VIEWPORT })` and asserts on
  the header's Custom Foods link, Import link, Logout button and Settings link, all four of which
  this change hides below the breakpoint. Fix it rather than weaken it: those four controls now live
  in the More sheet, so the mobile assertions move there (5.5 covers the sheet), and the header's
  full-control-set assertions move to a desktop-viewport block, matching the two scenarios the
  `mobile-touch-targets` spec now splits this into. Its in-file comment about enumerating header
  controls by name needs updating with it.
  (`auth.spec.ts` and `settings.spec.ts` were previously guessed here and are *not* affected —
  `grep -n logout` returns nothing in either. Re-run `grep -rln 'Custom Foods\|Logout\|getByTitle('\''Settings'\'')' e2e/tests`
  to confirm the affected set before editing.)
- [x] 5.7 Run the full suite against the deployed `hcw-wip` stack. All green before the change is
  reported done. **159 passed, 1 skipped, 1 failed** — the failure is
  `completeness.spec.ts:268` ("the settings panel saves both fields without clobbering unrelated
  settings, and updates the page live"), and it is **pre-existing**: it fails identically with
  `main` deployed to the same stack, checked by redeploying `main` and re-running that file. Not
  caused by, and not touched by, this change; it is reported to the user rather than fixed here.

## 6. Finalize

- [x] 6.1 Self-review every modified file per CLAUDE.md's mandatory review step.
- [x] 6.2 Disclose explicitly what is NOT e2e-covered: headless Chromium reports a zero
  `env(safe-area-inset-bottom)` and offers no way to set a non-zero one, so 5.4b asserts the
  `viewport-fit=cover` meta tag and the presence of the `env()` term, and the *effect* of a non-zero
  inset is verified manually on a notched device. Say which half is which rather than reporting the
  scenario as covered.

  **Disclosure, stated plainly.** Of the "Safe-area inset is respected" scenario:
  - *Covered by e2e:* the document emits `viewport-fit=cover`, and the bar's `padding-bottom` is
    declared with an `env()` term (asserted against the matching CSS rule, since a computed style
    resolves the term away).
  - *Not covered by e2e, verified manually only (6.2a):* that a **non-zero** inset actually pushes
    the bar's controls above the home indicator, that the `/food/manual/` submit button stays
    reachable with a real inset, and that a toast still clears the bar with one. Headless Chromium
    reports the inset as 0 and offers no way to set it, so no assertion here can distinguish
    "handled correctly" from "handled not at all".
- [ ] 6.2a Manual device pass on a notched phone: bar sits above the home indicator, the
  `/food/manual/` submit button is reachable with the bar present, and a toast does not land on the
  bar. These are the three things the headless suite cannot fully settle.
- [ ] 6.3 Ask the user to run `/code-review` on the branch; fix findings.
- [ ] 6.4 On the user's approval, archive the change on the feature branch
  (`openspec archive mobile-bottom-nav --yes`), regenerate projected specs as a separate commit,
  and validate `openspec validate --specs --strict`.
