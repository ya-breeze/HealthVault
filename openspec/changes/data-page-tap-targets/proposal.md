## Why

The `mobile-touch-targets` capability requires a 48×48 CSS pixel minimum for interactive controls,
but its requirement is scoped by an explicit list of routes: the food flows, the app header/toast,
and `/settings`. The data detail route (`/data/[type]`) was never in that list, so its controls were
never brought up to the minimum and nothing guards them.

Measured on `hcw-wip` at a 390×844 viewport, that gap is live:

- the per-record delete control renders **14×20 px** with `padding: 0` — roughly a twelfth of the
  required area, in a dense table where the adjacent row's delete control is ~52 px away
- the Day / Week / Month / Year zoom tabs render **28 px** tall — the most-used control on the page

A mis-tap on the delete control destroys a health record, so this is the highest-consequence
instance of the gap rather than a cosmetic one.

## What Changes

- Extend the existing `Minimum Tap Target Size` requirement's route scope to include the data
  detail route (`/data/[type]`).
- Bring that route's controls to the 48×48 minimum by migrating them to the existing `TapTarget`
  component rather than hand-sizing each call site:
  - the per-record delete control (`aria-label="Delete record"`)
  - the confirm and cancel controls of that delete's inline confirmation step
  - the Day / Week / Month / Year zoom range tabs
- Extend `e2e/tests/mobile-tap-targets.spec.ts` with a data-detail-route case that fails against
  today's code, using the spec file's existing `assertMinTapTarget` helper.

Explicitly **out of scope**: the table's layout, its column set, the nested horizontal scroll, and
the wider mobile visual redesign. Those belong to a separate upcoming change; this one moves only
tap-target geometry so it can ship independently and be reviewed on its own terms.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `mobile-touch-targets`: the `Minimum Tap Target Size` requirement's enumerated route scope gains
  the data detail route (`/data/[type]`), with scenarios covering its record delete control, that
  control's confirmation step, and the zoom range tabs.

## Impact

- `frontend/app/data/[type]/DataTypeClient.tsx` — delete, confirm/cancel and zoom-tab controls
  migrate to `TapTarget`; layout of the surrounding table is untouched.
- `e2e/tests/mobile-tap-targets.spec.ts` — one new data-detail-route case.
- `openspec/specs/mobile-touch-targets/spec.md` — updated on archive.
- No backend, API, schema, or dependency changes. No visual redesign.
