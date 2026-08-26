# Tasks

## 0. Groundwork

- [ ] 0.1 Read the routing/layout guide under `frontend/node_modules/next/dist/docs/` before
  writing component code — `frontend/AGENTS.md` states this Next.js version has breaking changes
  from what is in training data. Note anything that affects a client-component shell wrapping a
  statically-exported page.
- [ ] 0.2 Confirm 640px (`sm:`) is the only breakpoint in use:
  `grep -rn 'sm:\|md:\|lg:' frontend/app frontend/components | grep -o '\b[a-z]*:' | sort -u`.
  If another appears, resolve which is canonical before building on the assumption.
- [ ] 0.3 Measure the baseline so the change has a before/after number: header height and the
  y-offset of the photo action on the dashboard at 390×844 against `hcw-wip`. Record both.

## 1. Icons and labels

- [ ] 1.1 Add `HomeIcon` and `MoreIcon` (horizontal ellipsis or stacked bars) to
  `frontend/components/icons.tsx`, matching the existing icons' `className`-driven sizing
  convention. `CameraIcon`, `PencilIcon` and `HistoryIcon` already exist and are reused as-is.
- [ ] 1.2 Add filled variants for the five destination icons, or an `active` prop on each, for the
  active-state indication that design.md requires not be carried by color alone.
- [ ] 1.3 Add tab labels to `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts` under a `nav.*`
  key group: home, photo, manual, history, more.
- [ ] 1.4 Add More-sheet labels to both locale files, reusing the existing `header.*` strings where
  the sheet's control is the same control (webhook, custom foods, import, settings, logout) rather
  than duplicating them under new keys.

## 2. Navigation components

- [ ] 2.1 Create `frontend/components/BottomNav.tsx`: five destinations, `fixed bottom-0 inset-x-0`,
  hidden at and above the breakpoint via `sm:hidden`. Each destination is a `TapTarget` — do not
  restate `min-h-12`/`min-w-12` at the call site.
- [ ] 2.2 Derive the active destination from the current pathname. Exactly one active at a time;
  none active on a route matching no destination (`/data/[type]`, `/settings`, `/import`,
  `/food/custom`, `/food/review`). Match on the route, not on a prefix that would make Home active
  everywhere.
- [ ] 2.3 Create `frontend/components/MoreSheet.tsx` carrying webhook URL + copy action, custom
  foods, import, settings, logout. Lift the webhook copy logic (clipboard with `execCommand`
  fallback) out of `Header.tsx` into a shared place rather than duplicating it — it is ~20 lines of
  branching that must not diverge between the two surfaces.
- [ ] 2.4 Sheet dismissal: backdrop click and Escape both close without navigating. Return focus to
  the More destination on close.
- [ ] 2.5 Sheet is a modal surface — `role="dialog"`, `aria-modal`, focus trapped while open,
  background scroll locked.

## 3. Shell and header

- [ ] 3.1 Create `frontend/components/AuthenticatedShell.tsx` wrapping `<Header />`, `{children}`,
  and `<BottomNav />`, and applying the bottom clearance
  `calc(<bar height> + env(safe-area-inset-bottom))` below the breakpoint only.
- [ ] 3.2 Settle the bar height against 48px of tap target plus one label line (design.md's open
  question), and define it once as a CSS custom property the shell's clearance also reads, so the
  two cannot drift.
- [ ] 3.3 Make `Header.tsx`'s control set viewport-conditional: webhook, custom foods, import,
  settings and logout get `hidden sm:flex`; app name and user badge render at all widths. Remove
  `flex-wrap` from the mobile branch so it cannot wrap to a second row.
- [ ] 3.4 Verify the reduced mobile header is a single row at 320px, the narrowest supported width,
  with the user badge present — the badge is the widest variable-length element left in it.
- [ ] 3.5 Replace `<Header />` with `<AuthenticatedShell>` in all nine authenticated pages:
  `app/page.tsx`, `app/food/custom/page.tsx`, `app/food/review/ReviewClient.tsx`,
  `app/food/upload/page.tsx`, `app/food/manual/page.tsx`, `app/food/history/page.tsx`,
  `app/import/page.tsx`, `app/settings/page.tsx`, `app/data/[type]/DataTypeClient.tsx`.
  `/login` must NOT get the shell.
- [ ] 3.6 Check the camera capture flow (`components/food/CameraCapture.tsx`) against the bar — if
  it renders a full-screen fixed surface, confirm the bar does not sit on top of its controls.

## 4. Verification

- [ ] 4.1 `cd frontend && npx tsc --noEmit` clean.
- [ ] 4.2 `make lint` clean.
- [ ] 4.3 Unit tests pass.
- [ ] 4.4 Re-measure 0.3's baseline: header height and photo-action y-offset at 390×844. Both must
  improve, and the photo action must be within the viewport without scrolling.
- [ ] 4.5 Sweep tap targets across the bar and sheet at 320, 360, 390 and 430px widths; confirm
  every destination clears 48×48 and record whether any label truncates.
- [ ] 4.6 Confirm the desktop rendering is unchanged: screenshot `/` and `/data/steps` at 1280px
  before and after, and diff.

## 5. E2E

- [ ] 5.1 Add `e2e/tests/mobile-nav.spec.ts`: bar present below the breakpoint, absent at/above,
  absent on `/login`; each destination navigates; active state correct on a destination route and
  absent on `/data/steps`.
- [ ] 5.2 Cover the two audit defects directly, so a regression fails a test rather than needing
  another audit: photo and manual actions inside the viewport without scrolling at 390×844 from
  both the dashboard and `/data/steps`; header height below a threshold derived from 4.4.
- [ ] 5.3 Cover the More sheet: opens, exposes all five shed controls, logout ends the session and
  lands on `/login`, dismissal closes without navigating.
- [ ] 5.4 Add a test that fails when the header's desktop control set and the sheet's control set
  diverge — the "no control stranded on mobile" scenario. Assert against a shared list rather than
  two hand-maintained literals, or the test drifts with the code it guards.
- [ ] 5.5 Extend `e2e/tests/mobile-tap-targets.spec.ts` with the bar's five destinations and the
  sheet's controls, including the 320px case.
- [ ] 5.6 Check whether any existing spec asserts on header controls that are now mobile-hidden —
  `e2e/tests/auth.spec.ts` and `e2e/tests/settings.spec.ts` are the likely ones — and fix rather
  than weaken them.
- [ ] 5.7 Run the full suite against the deployed `hcw-wip` stack. All green before the change is
  reported done.

## 6. Finalize

- [ ] 6.1 Self-review every modified file per CLAUDE.md's mandatory review step.
- [ ] 6.2 Disclose explicitly what is NOT e2e-covered: `env(safe-area-inset-bottom)` reports 0 in
  headless Chromium, so the safe-area scenario is verified by construction plus a manual device
  check, not by a passing test.
- [ ] 6.3 Ask the user to run `/code-review` on the branch; fix findings.
- [ ] 6.4 On the user's approval, archive the change on the feature branch
  (`openspec archive mobile-bottom-nav --yes`), regenerate projected specs as a separate commit,
  and validate `openspec validate --specs --strict`.
