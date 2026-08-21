## Why

Users can already reorder the dashboard's 8 Vital Cards, but cannot hide the ones they don't care about — every user sees all 8, always. This is Phase 1 of a larger dashboard/food-tracking initiative (see `docs/adr/ADR-001-dashboard-card-visibility-inline-with-order.md`); it's a fully independent, immediately valuable change on its own, and later phases' new Food Cards will register into the same show/hide mechanism this change builds.

## What Changes

- Add a per-Vital-Card show/hide toggle (eye icon) to the dashboard's existing Edit/Done reorder mode. No new settings page.
- **BREAKING**: Change the stored shape of the user's dashboard order from `UserSettings.dashboard_order` (`string[]` of metric types) to an ordered list of `{type: string, hidden: bool}` objects. A hidden card keeps its position in the list so re-showing it restores its prior place rather than appending it at the end.
- One-time, transparent migration of existing `dashboard_order` rows (`string[]`) into the new `{type, hidden}[]` shape (all existing entries become `hidden: false`).
- New metric types added to the vitals grid in the future default to visible (`hidden: false`) for users who haven't explicitly hidden them.
- If a user hides every Vital Card, the grid renders a placeholder message instead of being blank — hiding the last card is never blocked.
- Explicitly no "show all" bulk-restore control (rejected as unneeded complexity for 8 cards).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `dashboard-ui`: the existing "Customizable vitals grid order" requirement is replaced by a requirement that covers per-card visibility together with order — the stored/exposed shape changes from a plain ordered list of types to an ordered list of `{type, hidden}` pairs, edit mode gains a show/hide control per card, and the empty-grid (all hidden) case is now specified.

## Impact

- **Backend**: `UserSettings` JSON shape for the dashboard-order key changes; needs a read-time migration (old `string[]` rows still need to load correctly) rather than a DB schema migration, since `user-settings` is a generic JSON blob store (unaffected as a capability — no requirement of `user-settings` itself changes).
- **Frontend**: the existing Edit/Customize mode component (move-up/move-down controls) gains a per-card eye-icon toggle; the vitals-grid render path needs to filter out hidden cards and handle the all-hidden placeholder state.
- No new API endpoints — reuses `GET`/`PUT /api/users/me/settings`.
