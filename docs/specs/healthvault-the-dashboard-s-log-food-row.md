# Hide the dashboard's Log food row below the navigation breakpoint
Idea: ya-breeze/idea-forge#268

## Why

On a phone the dashboard renders a `dashboard.logFood` heading and a row of three cards (`frontend/app/page.tsx:337-362`) linking to `/food/upload/`, `/food/manual/` and `/food/history/`. The mobile bottom navigation bar lists those same three routes as its Photo, Manual and History destinations (`frontend/components/nav.ts:20-23`, rendered by `frontend/components/BottomNav.tsx`). The bar is `sm:hidden fixed bottom-0`, so below the `sm` breakpoint it is on screen for the whole time the dashboard is. The same three actions therefore appear twice on one screen, and the in-body copy costs a heading, a `p-4` card row and a `mb-8` gap out of a 390x844 fold that the vitals grid and the More Data section already compete for.

The owner settled the open question on the idea: keep only the bottom bar on the phone, and hide "Записать еду" there. That is the decision this change implements, so no further judgement about discoverability is needed.

Above the `sm` breakpoint the picture is the opposite. `BottomNav` is hidden by `sm:hidden`, and the desktop header carries only the shed controls — webhook, custom foods, import, settings, logout (`frontend/components/nav.ts:38`, `frontend/components/Header.tsx:52-120`). None of them reaches a food route. At desktop widths the in-body row is the only way to get from the dashboard to photo, manual entry or meal history, so deleting it outright would strand desktop. The change has to be width-conditional, not a deletion.

## How

Wrap the heading and the three-card row in `frontend/app/page.tsx` in a single container carrying `hidden sm:block` and a `data-testid` for the tests to address. That is the exact mirror of `BottomNav`'s `sm:hidden`: below the breakpoint the bar is on screen and the row is not; at and above it the row is on screen and the bar is not. Exactly one of the two surfaces offers those three routes at any width.

Use the `sm:` variant rather than a `matchMedia` hook, for the reasons `BottomNav`'s own doc comment gives: the width test then lives in the same stylesheet as every other `sm:` in the app, resize needs no listener and no second render, and the app is `output: 'export'`, so a statically served page shows the right thing before hydration. `frontend/app/globals.css:70-78` already derives `--nav-block` from `theme(--breakpoint-sm)` so the CSS and the Tailwind variants cannot drift apart; this change adds no new breakpoint definition.

Keep the `dashboard.logFood`, `dashboard.photo`, `dashboard.manual` and `dashboard.history` keys in `frontend/lib/i18n/en.ts` and `ru.ts`. They are still rendered at desktop widths, so removing them would break that surface.

The existing `mb-8` stays on the row so desktop spacing is unchanged; the wrapper itself carries no margin, so on a phone the section collapses to nothing and the block that follows moves up by the full height of heading plus row plus gap.

Deliberately excluded:

- **Deleting the row.** Desktop has no other dashboard path to the three food routes, and the owner asked for it hidden on the phone, not removed.
- **Keeping one prominent primary action on mobile.** The idea's body floated keeping the Photo card and dropping the other two. The owner's comment supersedes that: only the bottom row of buttons remains on the phone.
- **Any change to `BottomNav`, `nav.ts`, `Header.tsx` or `MoreSheet.tsx`.** The bar already carries all three destinations; nothing about the navigation surfaces themselves needs to move.
- **Touching the needs-attention link** (`frontend/app/page.tsx:319-335`), which also points at `/food/history/`. It is a conditional alert about specific meals, not a navigation duplicate, and it renders only when there is something to act on.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Hide the Log food block below the `sm` breakpoint

- [x] In `frontend/app/page.tsx`, wrap the `dashboard.logFood` heading (line 337) and the three-card `div` (line 340) in one container element with `className="hidden sm:block"` and `data-testid="log-food-links"`.
- [x] Leave the heading's `mb-3` and the row's `mb-8` as they are, so desktop spacing does not shift.
- [x] Add a short comment above the wrapper explaining that it is the mirror of `BottomNav`'s `sm:hidden`: the bar carries these three destinations below the breakpoint and is hidden above it, so exactly one surface offers them at any width, and the desktop header carries no food route (`frontend/components/nav.ts:38`).
- [x] Confirm no other dashboard element depends on the removed vertical space (the More Data section at `frontend/app/page.tsx:369` follows it directly and needs no change).
- [x] Mark completed

### Task 2: Cover both widths in the e2e suite

- [x] In `e2e/tests/mobile-nav.spec.ts`, add a test to the `Mobile bottom navigation — mobile viewport` describe block asserting that on `/` the `log-food-links` block is hidden while `bottom-nav` and its `photo`, `manual` and `history` destinations are visible.
- [x] Add a test to the `Mobile bottom navigation — desktop viewport` describe block asserting the reverse on `/`: `log-food-links` is visible, `bottom-nav` is hidden, and the block contains anchors to `/food/upload/`, `/food/manual/` and `/food/history/` — so the desktop dashboard is never stranded without a food route.
- [x] Assert the block is CSS-hidden rather than unmounted at the mobile viewport (`toBeHidden`, not `toHaveCount(0)`), matching how the existing desktop test treats the bar and why — the app is statically exported.
- [x] Add a mobile-viewport assertion that the first element after the vitals grid moves up: with the block hidden, the `more-data` section's top edge sits above where the heading used to start. Compare bounding boxes against each other rather than against a pixel constant, in the style of the existing occlusion tests.
- [x] Mark completed

### Task 3: Validate and record the fold measurement

- [ ] Run `make lint` and `make test` and fix anything they report.
- [ ] Deploy the branch to the HealthVault WIP stack and run `make test-e2e` against it; fix failures before reporting the change done.
- [ ] Measure at the 390x844 viewport what the freed space buys — record the `more-data` section's top edge on `main` and on this branch — and state both numbers in the pull request description.
- [ ] Check the Russian dashboard at a desktop width so the retained `Записать еду` heading and its three labels still render, since the keys are kept rather than removed.
- [ ] Mark completed
