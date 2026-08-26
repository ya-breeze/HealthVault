## Why

The dashboard's More Data ("Другие данные") section already omits a secondary type the user has never recorded (presence filtering, idea-6). That is not enough: a type with a single stray or historical record still qualifies as "present" and clutters the section even though the user does not actually track it, and presence filtering has no way to tell "not tracked" apart from "tracked, but old." The user wants the same escape hatch the vitals grid already has (Phase 1's per-card hide/show, reached via Customize/Done) applied to the More Data section as a whole: one toggle that hides the entire section, reachable from the same edit mode.

## What Changes

- Add a per-user, persisted "hide the More Data section" preference, toggled from a new eye icon on the More Data section heading, reachable only through the dashboard's existing Edit/Customize mode (the same mode Phase 1 added for vitals cards) — no new settings page, no per-secondary-type toggles.
- The toggle is only offered when the section would otherwise render (i.e. at least one secondary type has presence); there is nothing to hide otherwise, so no control is shown.
- Once hidden, the section SHALL NOT render in the read-only view, regardless of presence (including the presence-fetch-failure fail-open case) — a user-hidden section always wins over presence.
- In edit mode, a hidden More Data section (and its link list) still renders, visually distinguished as hidden, so it stays discoverable and reversible — mirroring how Phase 1 keeps hidden vital cards visible-but-dimmed in edit mode.
- The preference is stored as a new `more_data_hidden?: boolean` key in the existing generic `UserSettings` JSON blob and saved through the same queued `updateSettings()` write Phase 1 already uses on Done — no new writer, no backend schema change.
- The More Data render path additionally gates on the saved settings having loaded (today it only waits on the presence fetch), so a hidden section is never flashed unhidden before the saved preference arrives.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `dashboard-ui`: the "Presence-based filtering of the More Data section" requirement gains a composition rule with the new user preference and a settings-load gating note. A new requirement, "User-hidden More Data section", covers the toggle itself: when it is offered, hiding/re-showing, persistence, and the user-hidden-always-wins-over-presence rule.

## Impact

- **Backend**: none. `UserSettings` stays a generic JSON blob (`user-settings` capability unaffected — no requirement of it changes); the new key needs no migration since it has no prior shape to be compatible with (unlike Phase 1's `dashboard_order`, this is a brand-new key that simply defaults to absent/`false`).
- **Frontend**: `frontend/lib/api.ts` (`UserSettings` type), `frontend/app/page.tsx` (More Data render gate, edit-mode heading control, `handleDone`'s settings payload), `frontend/lib/i18n/en.ts` / `ru.ts` (new toggle label keys). Reuses the existing `editing` state, `updateSettings()` queued write, and settings-load gating pattern Phase 1 established — no new component, no new API endpoint.
