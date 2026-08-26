## Context

The More Data section (`frontend/app/page.tsx`, ~lines 310-329) today renders whenever `presentSecondaryTypes.length > 0`, where `presentSecondaryTypes` is `SECONDARY_TYPES` filtered by `hasPresence` and gated on `presenceReady`. It has no edit-mode state of its own: no eye icon, no per-item controls, and it does not currently read anything from the settings blob. The dashboard's `editing` boolean (added by Phase 1 for the vitals grid's Customize/Done mode) is a single page-level flag, not scoped to the vitals grid specifically, so it is available to gate a More Data control too. `handleDone()` is the one place that calls `updateSettings()`, itself a GET-merge-PUT that does its own fresh GET to avoid clobbering concurrent writes (notably `LanguageContext`'s own settings write) and queues behind that same claim.

## Goals / Non-Goals

**Goals:**
- One whole-section hide/show preference for More Data, persisted per user.
- Reuse the existing Customize/Done edit mode, the existing `updateSettings()` write path, and the existing settings-load gating pattern — no new mechanism class.
- Keep the toggle undoable/discoverable the same way Phase 1's per-card hide is.

**Non-Goals:**
- Per-secondary-type toggles (19 currently presence-filtered types — a materially bigger feature, not what was asked for).
- A separate settings page or control location outside the existing Customize/Done mode.
- Any change to presence filtering itself (idea-6) — this preference composes with it, not replaces it.

## Decisions

**Storage shape**: a single `more_data_hidden?: boolean` key in `UserSettings`, alongside `dashboard_order`. Unlike Phase 1's `dashboard_order`, this key has no prior shape to migrate — an absent key simply means "not hidden" (default `false`), so no read-time reconciliation is needed.

**Composition with presence filtering (the open question from the investigation)**: user-hidden always wins. If `more_data_hidden` is `true`, the section does not render in the read-only view, full stop — including when the presence fetch fails and would otherwise fail-open to showing every secondary type. This is the simpler mental model ("hidden" means hidden, not "hidden unless something else would have shown it anyway") and matches how Phase 1's per-card `hidden` flag is not overridden by anything else once set. The read-only render condition becomes `!moreDataHidden && presentSecondaryTypes.length > 0`.

**When the toggle is offered (the other open question)**: only when the section would otherwise render, i.e. `presentSecondaryTypes.length > 0`. This mirrors Phase 1 excluding zero-presence vitals metrics from the edit-mode list entirely rather than showing a no-op toggle — a Customize control for a section with nothing in it is confusing, not useful. Since presence is monotonic (a type that has ever had a record stays present), a toggle that becomes available cannot later become unavailable except via the separate `record-deletion` capability deleting every record of every secondary type, an edge case not worth special-casing (it degrades to "the control disappears," which is harmless, not a lost preference — the stored flag itself is untouched).

**Control placement**: an eye icon on the More Data section's own heading, shown only in edit mode — distinct from the vitals grid's per-card row controls (which live inside each card, not at a section-header level). No existing component to reuse verbatim, but the eye/eye-off icon asset already exists in `frontend/components/icons` from Phase 1.

**Edit-mode visibility**: when hidden and `presentSecondaryTypes.length > 0`, edit mode still renders the section (heading, toggle, and link list), visually distinguished as hidden (e.g. dimmed), exactly mirroring Phase 1's dimmed-hidden-card treatment — so a user can find and re-show it without needing to remember what was in it.

**Gating**: the More Data render path currently only waits on `presenceReady`. This change adds a wait on the settings load (`dashboardReady`/`settingsLoaded`, the same flag Phase 1 gates the vitals grid on) so the section can't flash its unhidden content before the saved `more_data_hidden` value is known — a genuine change to today's gating, not just an additive check, since the section has never before depended on settings.

**Write path**: no new writer. `handleDone()`'s existing `updateSettings()` call gains `more_data_hidden` in its payload, alongside the existing `dashboard_order` key — one PUT, same queued path behind `LanguageContext`'s claim.

## Risks / Trade-offs

- **Settings-load gating is a new dependency for a section that never had one** — mitigated by reusing the exact same `dashboardReady` flag and loading/error placeholder pattern Phase 1 already built and tested for the vitals grid, not inventing a second one.
- **User-hidden overriding the presence-fetch-failure fail-open** means a user who hid the section sees it stay hidden even during a presence outage that would otherwise show everything. Accepted: the alternative (presence failure force-unhides a section the user explicitly hid) is more surprising, not less.
- **Toggle disappearing if presence regresses to zero via record deletion** — accepted as a harmless edge case (see Decisions); the stored preference is not lost, just temporarily inert.

## Migration Plan

No deploy-order dependency and no backend change. The new key defaults to absent/`false` for every existing user, so this ships as a single frontend deploy with no transition state to reason about — unlike Phase 1, there is no old shape to remain compatible with.
