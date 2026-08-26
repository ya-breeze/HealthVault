## Context

`TapTarget` (`frontend/components/ui/TapTarget.tsx`) already exists and enforces the 48×48 minimum
by prepending `min-h-12 min-w-12` to whatever `className` a call site passes, spreading every other
prop through unchanged. The food flows, header/toast and `/settings` were migrated to it by the
archived `mobile-tap-targets` change.

`/data/[type]` was outside that change's scope and is only *partly* migrated: `DataTypeClient.tsx`
already imports `TapTarget` and uses it for the "Set goal" (line 583) and "Set height" (line 607)
controls, and the `AddRecordForm` rendered on the same route uses it for its own buttons. What was
never covered is the table's per-record controls, the zoom range tabs, and the nutrition macro tabs
— all still bare `<button>` elements.

Measured on `hcw-wip` at 375×667, the uncovered controls are: delete **14×20**, zoom tabs **28**
tall, macro tabs **27** tall.

Two of these sit inside layouts that constrain them, so "make them 48px" is not purely additive:

- The delete control lives in a table cell whose row height is currently driven by the timestamp
  text (65–85 px depending on data type).
- The zoom tabs are a compact segmented control shared by every data type, rendered identically on
  desktop where 28 px is a deliberate, unremarkable size.

## Goals / Non-Goals

**Goals:**
- Every control named in the spec delta reaches 48×48 at mobile widths.
- The enforcement comes from `TapTarget`, not from per-call-site sizing classes, so the fix cannot
  silently regress the way the original did.
- An e2e case exists that fails against today's code and is deterministic.

**Non-Goals:**
- The table's layout, column set, or nested horizontal scroll. The columns stay exactly as they are.
- The record-entry form's inputs on this route (`AddRecordForm`, ~34 px tall). Form-field sizing is
  a separate concern from button geometry and would pull in a wider pass; the spec delta names this
  exclusion explicitly rather than leaving it ambiguous.
- The mobile visual redesign (bottom tab bar, records-as-rows). That is a separate change with its
  own proposal; deliberately not pre-empted here.
- Any change to delete *semantics* — the two-step confirmation stays exactly as it is, only the
  geometry of its controls changes.

## Decisions

**Migrate to `TapTarget` rather than adding utility classes.** The original defect exists precisely
because sizing was left to each call site. Routing these controls through the shared component makes
the minimum structural. `TapTarget` spreads `aria-label`, `data-testid`, `disabled` and handlers
through unchanged, so e2e locators that address the delete control by `aria-label="Delete record"`
keep working — the archived change already relied on that guarantee.

*Alternative considered:* add `min-h-12 min-w-12` inline at each call site. Rejected — identical
rendered result, but it re-creates the per-call-site pattern that produced the gap.

**Release the compact segmented controls for mouse users, keyed off pointer type.** The zoom tabs
and the nutrition macro tabs are deliberately compact where the pointer is precise (28 and 27 px),
and the tap-target problem they would solve does not exist there. Growing them to 48 px for a mouse
makes the UI chunkier for no benefit, so they take the minimum only off a fine pointer, via
`pointer-fine:min-h-auto pointer-fine:min-w-auto`.

**Release to `auto`, not `0`.** `auto` is what `min-height`/`min-width` computed to on these
controls before `TapTarget` was applied, so the release restores the original rendering exactly.
`min-width: 0` looks equivalent and is not: for a flex item it *also* switches off the automatic
minimum size, and both call sites are flex children. That would let a cramped row shrink a label
below its own content width instead of leaving it intact — a behaviour change this fix never
intended to make, on the one pointer type where it is meant to change nothing at all.

**Pointer type, not viewport width.** A width test is the obvious implementation and it is wrong:
a 375×667 phone rotated to landscape is 667 CSS px wide, so a `sm:`-based release (≥640 px) hands
the fix straight back on the exact device this change was filed for — and a tablet in portrait
(768 px) never gets it at all. `(pointer: fine)` asks the question actually being asked, "is this a
mouse?", and it fails safe: a device reporting no fine pointer keeps the minimum. An earlier
iteration of this change did use `sm:`; it was caught in review.

This is expressed as a named `compactOnMouse` prop on `TapTarget`, not as opt-out utility classes at
the call site. Sizing still lives in exactly one place — a call site asks for the behaviour by name
and never spells out its own dimensions — so the property this change is built on ("the minimum is
structural, not re-created per call site") survives intact. The prop is named for the condition it
actually tests: an earlier `touchOnly` claimed the minimum applied to touch, while the
implementation keys off the *negation* of `pointer: fine`, so a device reporting no pointer at all
also keeps it.

Because this is the first sanctioned way for a `TapTarget` call site to render below the minimum,
the capability's *Shared Tap Target Enforcement Component* requirement is modified in the same
delta. Left alone it would keep asserting the minimum unconditionally, and the two requirements
would contradict each other the moment this change archived.

Both compact groups get the same treatment, because they render together on `/data/nutrition`:
releasing one and not the other would put a 28 px control next to a 48 px one in the same view.

The record delete control and its confirm/cancel siblings are deliberately **not** `compactOnMouse`.
They are not compact-by-design — the delete target was 14×20, which is too small for a mouse as
well as a thumb, and it is the control whose mis-tap destroys a record.

*Superseded alternative:* an earlier draft of this design grew the zoom tabs at every width, on the
grounds that one geometry is simpler to reason about and to test. The operator reversed that call on
review; the trade it bought (uniform geometry) was not worth a visibly worse mouse rendering.

**Mock the data API in the new e2e case** rather than depending on seeded records. Every other case
in `mobile-tap-targets.spec.ts` mocks its API (`page.route('**/api/food/…')`), and the page defaults
to a `week` zoom that fetches only the last 7 days — so a case relying on real data would silently
start skipping the moment that type stopped receiving recent records, turning a regression guard into
a permanent green. Mocking removes both the flake and the need for a skip guard.

**Let the delete control's 48 px width sit inside the existing Actions cell.** Verified by
measurement rather than assumed: with the 48×48 minimum applied at 375 and 390 px on both
`/data/steps` and `/data/weight`, every column width is byte-identical before and after
(e.g. weight: 110 / 102 / 104 / 110 / Actions 86) and the table's `scrollWidth` does not move at all
(steps 590, weight 513). The reason is that the Actions column is already ~86 px wide — its width is
set by the "Actions" header label plus `px-4` cell padding, comfortably more than 48 — so the larger
control drops into space the cell already has. The data columns are unaffected because the table
already overflows its container, which means they are already at their min-content widths and cannot
be squeezed further.

*Alternative considered:* an overflow menu replacing the inline delete. Rejected — that is a
redesign of the interaction, and belongs to the upcoming visual change, not to a geometry fix.

## Risks / Trade-offs

- **Row height grows on types whose rows are currently under 48 px** → Measured: `/data/weight` rows
  go 65 → 73 px; `/data/steps` rows are unchanged at 85 px because their timestamps already wrap.
  This is the intended outcome, not a regression: a row shorter than 48 px cannot host a compliant
  control.
- **Two geometries now exist for the compact controls, so their size is pointer-dependent** →
  Accepted; this is the cost of the decision above. The suite's project is
  `devices['Desktop Chrome']`, which reports a *fine* pointer at every viewport size, so the
  data-detail cases set `hasTouch: true` explicitly — without it they would measure the compact
  mouse rendering and assert the wrong thing.
- **A regression could add `compactOnMouse` to the delete control** — an easy consistency mistake, since
  its two tab neighbours in the same file have it — returning the highest-consequence control on the
  page to 14×20 for mouse users → Guarded by a dedicated fine-pointer case that asserts the delete
  control still measures 48×48 while the zoom tabs go compact. Every other assertion runs on a
  coarse pointer, where `compactOnMouse` and plain `TapTarget` are indistinguishable.
- **Seven macro tabs at 48 px tall will wrap onto more rows on `/data/nutrition`** → Their container
  is already `flex-wrap`, so this changes how many rows they occupy, not whether they fit. Below
  `sm` only. Verified in task 3.6 rather than assumed.
- **The table's horizontal scroll is unpleasant on mobile** → Real, but out of scope and untouched
  by this change; it belongs to the visual redesign. Noted here so it is not mistaken for something
  this change introduces.
