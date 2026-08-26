## Context

`TapTarget` (`frontend/components/ui/TapTarget.tsx`) already exists and enforces the 48×48 minimum
by prepending `min-h-12 min-w-12` to whatever `className` a call site passes, spreading every other
prop through unchanged. The food flows, header/toast and `/settings` were migrated to it by the
archived `mobile-tap-targets` change; `/data/[type]` was outside that change's scope and still uses
bare `<button>` elements.

Two of the three controls sit inside layouts that constrain them, so "make them 48px" is not purely
additive:

- The delete control lives in a table cell, in a row whose height is currently driven by wrapped
  timestamp text (~135 px on mobile, comfortably over 48). Its confirm/cancel siblings appear in
  the same cell when a row enters the pending-delete state.
- The zoom tabs are a compact segmented control shared by every data type, rendered identically on
  desktop where 28 px is a deliberate, unremarkable size.

## Goals / Non-Goals

**Goals:**
- Every control named in the spec delta reaches 48×48 at mobile widths.
- The enforcement comes from `TapTarget`, not from per-call-site sizing classes, so the fix cannot
  silently regress the way the original did.
- An e2e case exists that fails against today's code.

**Non-Goals:**
- The table's layout, column set, or nested horizontal scroll. The columns stay exactly as they are.
- The mobile visual redesign (bottom tab bar, records-as-rows). That is a separate change with its
  own proposal; deliberately not pre-empted here.
- Any change to delete *semantics* — the two-step confirmation stays exactly as it is, only the
  geometry of its controls changes.

## Decisions

**Migrate to `TapTarget` rather than adding utility classes.** The original defect exists precisely
because sizing was left to each call site. Routing these three controls through the shared component
makes the minimum structural. `TapTarget` spreads `aria-label`, `data-testid`, `disabled` and
handlers through unchanged, so e2e locators that address the delete control by `aria-label="Delete
record"` keep working — the archived change already relied on that guarantee.

*Alternative considered:* add `min-h-12 min-w-12` inline at each of the four call sites. Rejected —
identical rendered result, but it re-creates the per-call-site pattern that produced the gap.

**Grow the zoom tabs at every width, not only mobile.** A responsive-only bump (`min-h-12` under a
breakpoint) would leave two geometries to reason about and to test, for a control whose desktop size
carries no meaning worth preserving. A 48 px segmented control is unremarkable on desktop.

*Alternative considered:* `sm:min-h-0` to keep 28 px on desktop. Rejected as complexity with no
benefit — and it would make the e2e assertion viewport-dependent for no reason.

**Let the delete control's 48 px width sit inside the existing Actions cell.** The cell is the last
column and the table already scrolls horizontally, so widening the control from 14 px to 48 px
extends the table's scroll width rather than squeezing the data columns. No data column loses space.

*Alternative considered:* an overflow menu replacing the inline delete. Rejected — that is a
redesign of the interaction, and belongs to the upcoming visual change, not to a geometry fix.

## Risks / Trade-offs

- **The table's horizontal scroll width grows by ~34 px** → Accepted, and bounded: the Actions
  column is last, so the added width lands past the data columns rather than displacing them. The
  container's existing `overflow-auto` already handles content wider than the box.
- **A 48 px-tall segmented control is visibly chunkier on desktop** → Accepted deliberately (see
  the decision above); it is the same trade the food flows already made.
- **Row height could grow on types whose timestamps do not wrap**, where the row is currently
  shorter than 48 px → This is the intended outcome, not a regression: a row shorter than 48 px
  cannot host a compliant control.
- **e2e depends on a data type having records in the target environment** → The new case selects a
  type known to be seeded on `hcw-wip`, and skips rather than fails if no record rows are present,
  so it cannot flake into a false red on an empty database.
