# Data detail page: read a chart value with a finger
Idea: ya-breeze/idea-forge#267

## Why

The charts on `/data/<type>/` carry five `<Tooltip>` elements — `frontend/app/data/[type]/DataTypeClient.tsx:702` and `:719` (Day-zoom lines), `:736` (blood-pressure band), `:748` (cumulative bars) and `:756` (min-max band, weight trend and projection). None of them passes a `trigger` prop, and none of the charts around them handles touch. The value a user opens the page for is therefore only reliably reachable with a mouse, on an app whose primary device is a phone — the same premise that gave it a bottom navigation bar and a 48x48 `TapTarget` component.

The picture in Recharts 3.9 is narrower than "hover only", and the difference decides the design. Recharts already routes `touchmove` into the same axis-tooltip state the mouse writes to (`frontend/node_modules/recharts/es6/state/touchEventsMiddleware.js` dispatching `setMouseOverAxisIndex`), so dragging a finger across the plot does move the tooltip. Four things stop that from being a feature anyone can use:

- **Nothing activates on first contact.** `RechartsWrapper.js:301` passes `touchstart` to an external handler only; it never reaches tooltip state. A tap, or a press held still, shows nothing. The tooltip a user does see today on a real phone comes from the browser's compatibility mouse events after a tap — unspecified behaviour that is absent under synthetic input and absent as soon as the finger moves at all.
- **The gesture fights the page.** The chart surface sets no `touch-action`, so a drag that drifts a few pixels vertically is claimed by the page's scroll and the touch sequence is cancelled mid-read.
- **The readout sits under the thumb.** Recharts places the tooltip beside the active coordinate, which on a phone is exactly where the finger is.
- **Nothing tests any of it.** `e2e/tests/mobile-tap-targets.spec.ts` is the precedent for asserting phone-viewport behaviour (`test.use({ viewport: MOBILE_VIEWPORT, hasTouch: true })`, `:293`), and it stops at tap-target sizes; no case anywhere asserts that a value is readable without a mouse.

The formatting work is already shared: `formatTooltipValue` (`:113`) handles both plain numbers and the band's `[min, max]` tuple for every one of the five tooltips. This change is only about the interaction.

## How

**Pick the drag-scrub, and seed it on first contact.** A finger placed on the chart shows the value under it immediately; sliding moves the readout along the series; lifting leaves the last value on screen. That is what a native time-series chart does, and it is the gesture Recharts already supports for the moving half.

**`trigger="click"` is rejected, and not only on taste.** The prop is a switch, not an addition: `combineTooltipInteractionState` (`recharts/es6/state/selectors/combiners/combineTooltipInteractionState.js:7-18`) reads `axisInteraction.click` instead of `axisInteraction.hover` once `trigger` is `'click'`, so desktop hover would stop showing the tooltip. It also makes the tooltip sticky with no dismissal path, on a page with nowhere to put one. The scrub keeps hover working by construction, because touch and mouse write to the same hover interaction state and this change adds no `trigger` prop at all.

The change is four small pieces:

- **One touch surface wrapping the chart.** Wrap the `<ResponsiveContainer>` at `DataTypeClient.tsx:688` in a `<div data-testid="chart-surface" className="touch-pan-y">`. `touch-action: pan-y` is the deliberate split: a vertical drag still scrolls the page, a horizontal drag belongs to the chart and its touch events are no longer cancelled. Wrapping the container rather than each chart branch means all five tooltips — including the min-max band and the projection, which share the `<Tooltip>` at `:756` — are covered by one edit, and no future branch can be added without it.
- **A first-contact seed.** A helper in `frontend/lib/chartTouch.ts` re-dispatches a `touchstart`'s own touch points as a bubbling synthetic `touchmove` on the touched element, which is what Recharts listens for. The original `Touch` objects are reused, so only the `TouchEvent` constructor is needed; the helper feature-detects it, catches a constructor failure, and returns `false`. Where it is unavailable the tooltip simply appears on the first movement instead of on contact — a degradation, not a break. The helper never calls `preventDefault`, so scrolling is untouched.
- **A readout the finger does not cover.** On a coarse pointer, pass `position={{ y: 0 }}` to every `<Tooltip>`, which pins the box to the top of the plot area while its x still follows the touch (`recharts/es6/util/tooltip/translate.js:28` applies `position` per axis). Key it off `(pointer: coarse)` through a `useCoarsePointer` hook in `frontend/lib/useCoarsePointer.ts`, not off viewport width — the reasoning in `frontend/components/ui/TapTarget.tsx:12-23` applies unchanged: a phone in landscape is 667 CSS px wide. The hook starts `false` and upgrades in an effect, so server and first client render agree.
- **Sticky after lift, deliberately.** Touch has no `mouseleave`, so the last value stays readable after the thumb comes off, until the next touch, a zoom change, or navigation. That is the desired behaviour for a number someone is reading, and it needs no dismissal control.

**Trade-offs and exclusions.**

- *Accepted: the seed synthesizes a DOM event to reach a third-party component's internal state.* The alternative is driving the tooltip ourselves through `active`/`defaultIndex`, and `defaultIndex` is dead the moment any real interaction happens (`combineTooltipInteractionState.js:40-54`), so a controlled tooltip would mean reimplementing hit-testing for five chart branches. The seed is about fifteen lines, is unit-tested, and fails safe.
- *Excluded: any hint text.* A hint is a new user-visible string in both `frontend/lib/i18n/en.ts` and `ru.ts` for a gesture that also answers to a plain tap once seeded.
- *Excluded: every other chart in the app.* The dashboard's charts are out of this idea's scope. The helper and the hook are written to be reusable, and neither is wired anywhere else here.
- *Excluded: an ADR.* No new dependency, no cross-cutting pattern, no data-model change.
- *Excluded: the unlabelled table headers below the chart,* filed separately.

**Verification.** The gate is the E2E suite: `make lint` covers the backend, and Vitest covers pure functions, so only Playwright can say a value is readable with a finger. The new cases must be shown to fail without the production change — Chromium may emit compatibility mouse events from a CDP-dispatched tap, which would let a vacuous test pass against `main`.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT merge it. Those are the pipeline's own final steps, run once the task list is complete. The operator reviews the pull request and merges it themselves; that is the only gate this work passes through, so leave it in a state worth reading.

### Task 1: Seed the tooltip on first contact
- [x] Add `frontend/lib/chartTouch.ts` exporting `replayTouchAsMove(event: React.TouchEvent): boolean`: return `false` when `event.target` is not an `Element`, when `event.touches.length === 0`, or when `typeof TouchEvent !== 'function'`; otherwise dispatch `new TouchEvent('touchmove', { bubbles: true, cancelable: true, touches, targetTouches, changedTouches })` built from the event's own `Touch` objects on `event.target`, and return `true`.
- [x] Wrap the construction and dispatch in `try`/`catch`, returning `false` on failure — the `TouchEvent` constructor is not available in every browser, and the fallback is Recharts' own `touchmove` handling.
- [x] Comment why the helper exists: `RechartsWrapper.js` dispatches `touchstart` only to an external handler, so first contact never reaches tooltip state, while `touchmove` does via `touchEventsMiddleware`.
- [x] Never call `preventDefault` — the page must keep scrolling.
- [x] Mark completed

### Task 2: Give the data detail chart a touch surface
- [x] In `frontend/app/data/[type]/DataTypeClient.tsx`, wrap the `<ResponsiveContainer>` at `:688` in a `<div data-testid="chart-surface" className="touch-pan-y" onTouchStart={replayTouchAsMove}>`.
- [x] Comment that `touch-pan-y` is what stops a slightly diagonal scrub from being taken by the page's vertical scroll, and that vertical scrolling over the chart is deliberately preserved.
- [x] Add `frontend/lib/useCoarsePointer.ts`: a hook returning `false` on the first render, then the value of `matchMedia('(pointer: coarse)')`, subscribing to its `change` event and unsubscribing on unmount. Guard `matchMedia` for the server render.
- [x] Pass `position={{ y: 0 }}` to all five `<Tooltip>` elements (`:702`, `:719`, `:736`, `:748`, `:756`) when the hook reports a coarse pointer, and leave the tooltip at its default position otherwise. Extract the shared props into one object so a sixth tooltip cannot be added without them.
- [x] Add no `trigger` prop anywhere, and record in a comment that `trigger="click"` would switch the tooltip off hover and regress the mouse.
- [x] Confirm `formatTooltipValue` and `labelFormatter` are unchanged.
- [x] Mark completed

### Task 3: Unit-test the seed helper
- [x] Add `frontend/lib/chartTouch.test.ts` alongside the existing Vitest suites in `frontend/lib/`.
- [x] Assert the helper dispatches exactly one bubbling `touchmove` on the touch target, carrying the same touch points as the source event.
- [x] Assert it returns `false` and dispatches nothing when the `TouchEvent` constructor is missing, when there are no touches, and when the target is not an `Element`.
- [x] Assert it never calls `preventDefault` on the source event.
- [x] Mark completed

### Task 4: Prove a value is readable without a mouse
- [x] Add `e2e/tests/chart-touch-readout.spec.ts` using `test.use({ viewport: { width: 390, height: 844 }, hasTouch: true })`, following `e2e/tests/mobile-tap-targets.spec.ts:286-293`.
- [x] Mock `**/api/data/**` following the `mockDataRecords` pattern at `mobile-tap-targets.spec.ts:268`, but return bucketed rows as well as raw ones — this account holds no guaranteed blood-pressure or step history, so every branch must be fed deterministic data. Supply `bucket_start`, `avg`, `min`, `max`, `sum`, and the `systolic_*`/`diastolic_*` columns as each branch needs.
- [x] Cover all five tooltips, one case per chart branch: `weight` at Day zoom (`:719`), `blood_pressure` at Day zoom (`:702`), `blood_pressure` at Week zoom (`:736`), a cumulative type at Week zoom (`:748`), and `weight` at Week zoom for the min-max band, `Avg` and `Trend` series (`:756`).
- [x] For the projection series on the same `:756` tooltip, mock a `weight_goal` record plus a 60-day daily-bucketed downward weight series that clears `hasEnoughDataForProjection` (`frontend/lib/dataTypeMeta.ts:278` — at least 5 records, a 14-day lifetime span, 5 window points and a 14-day window span), select Month zoom, and assert the tooltip shows the `Projection` entry on a synthetic future bucket.
- [x] Assert a plain `page.touchscreen.tap` inside the chart surface makes `.recharts-tooltip-wrapper` visible and show the value from the mocked data, formatted the way the stats row formats it.
- [x] Add a scrub case: dispatch `touchstart`, several `touchmove`s across the plot and `touchend` from `page.evaluate` using the `Touch` and `TouchEvent` constructors, and assert the tooltip's label changes between two x positions and that the readout survives `touchend`.
- [x] Assert the chart surface computes `touch-action: pan-y`, and that on a coarse pointer the tooltip box sits in the upper part of the plot area rather than at the touch point.
- [x] Add one case at `{ width: 1280, height: 900 }` with `hasTouch: false` asserting `page.mouse.move` over the chart still shows the tooltip — the no-mouse-regression criterion.
- [x] Run the new file against the deployed stack **before** the production change is deployed (or with it reverted) and record that it fails; a tap that passes through Chromium's compatibility mouse events would otherwise make the whole file vacuous.
- [x] Mark completed

### Task 5: Review and validate against the deployed stack
- [x] Re-read every modified file for leftover props, hydration-unsafe hook initialization, and consistency with the surrounding code.
- [x] Run `make lint` and `make test`, and state in the summary that they cover the backend and the pure-function suite only.
- [x] Deploy the branch to `hcw-wip` and run `make test-e2e E2E_ARGS=--retries=0` against it; the suite must be green on the first attempt, not on retry.
- [x] Confirm the existing `e2e/tests/mobile-tap-targets.spec.ts` data-detail cases and `e2e/tests/chart-day-boundary.spec.ts` still pass — both drive the page this change touches.
- [x] Summarize which files changed and what each change does.
- [x] Mark completed
