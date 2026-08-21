## Context

The vitals grid's order today lives entirely in the frontend, keyed off `PRIMARY_METRICS` (`frontend/lib/vitals.ts`) and a plain `dashboard_order: string[]` inside the generic `UserSettings` JSON blob (`frontend/lib/api.ts`, backed by the already-generic `GET`/`PUT /api/users/me/settings` — see `user-settings` spec, unchanged by this proposal). `frontend/app/page.tsx`:
- loads `s.dashboard_order` and reconciles it against `PRIMARY_METRICS` via `reconcileMetricOrder()`,
- renders `order` as the vitals grid, with move-up/move-down controls in edit mode,
- writes `{ dashboard_order: order.map(m => m.type) }` back through `updateSettings()` on Done.

`updateSettings()` queues behind `LanguageContext`'s own settings write to avoid a lost-update race between the reorder screen and the language switcher (both write into the same JSON blob) — this change must keep using that same queued path, not introduce a second writer.

## Goals / Non-Goals

**Goals:**
- Add a per-card hidden flag alongside order, in the same persisted list.
- Preserve a hidden card's position so re-showing restores it in place.
- Migrate existing `string[]` rows losslessly and silently (all entries start visible).
- Keep this scoped to the 8 vitals-grid cards; no settings-page UI.

**Non-Goals:**
- Phase 2-4 Food/Goal cards — this change only needs to leave the registry extensible for them (default-visible), not build them.
- A "show all" bulk-restore control (deliberately rejected — see proposal).
- Any backend schema migration — `UserSettings` stays a generic JSON blob; only the frontend's interpretation of the `dashboard_order` key changes.

## Decisions

**Stored shape**: `dashboard_order` becomes `{ type: string; hidden: boolean }[]` (was `string[]`). Chosen over a separate `hidden_cards: string[]` list (the ADR-001-rejected alternative) because a single ordered list can't drift out of sync with a second list, and "hide" is naturally just a per-entry flag on the thing that already tracks position.

**Migration**: purely a read-time reconciliation, no backend migration needed. `reconcileMetricOrder()` in `frontend/lib/vitals.ts` is extended to accept the old shape too:
- If an entry is a `string` (old shape), treat it as `{ type: entry, hidden: false }`.
- If an entry is `{ type, hidden }` (new shape), use as-is.
- Behavior for unknown/removed types and metrics missing from the saved order is unchanged from today (dropped / appended visible, respectively) — this is exactly today's `reconcileMetricOrder` behavior, just extended to carry `hidden` through.

This makes the change **forward**-read-compatible (new build reads old data losslessly). It is *not* symmetric: the reverse direction degrades. Today's `reconcileMetricOrder` does `typeof t === 'string' ? byType.get(t) : undefined`, so an older build handed the new object-shaped entries resolves every entry to `undefined`, ends up with an empty `ordered`, and falls through to returning `PRIMARY_METRICS` in **default** order — the user's custom order is silently ignored (not crashed). Worse, if that user then opens edit mode and clicks Done on the old build, it persists `string[]` of the default order, permanently discarding their saved arrangement. See "Migration Plan" for what this means for rollback.

**UI**: add an eye/eye-off icon button next to each card's existing move-up/move-down controls in edit mode (`frontend/app/page.tsx`'s edit-mode card row). Toggling it flips that entry's `hidden` in local `order` state, mirroring how `moveCard()` already mutates local `order` before Done persists it. Hidden cards still render in edit mode (dimmed, so the user can find and re-show them) but are filtered out of the read-only (non-editing) grid.

**All-hidden placeholder**: when every entry in `order` has `hidden: true`, the non-editing render path shows a placeholder message in the grid area instead of nothing. No new component needed — a conditional string in the existing grid container.

**New/future card types default visible**: unchanged from today's "missing from saved order" handling — `reconcileMetricOrder` already appends any current metric absent from the saved list; this change only needs that appended entry's `hidden` to default to `false`.

## Risks / Trade-offs

- **Silent old-shape entries never explicitly migrated server-side** → mitigated by making the frontend's reconciliation permanently tolerant of both shapes (see above), so there is no cutover moment that could leave a user's settings unreadable.
- **Edit-mode showing hidden cards (dimmed) adds a bit of visual complexity to that mode** → accepted; the alternative (hiding hidden cards from edit mode too) would make them unreachable to re-show, which the proposal explicitly rules out.
- **Existing race-avoidance queue (`updateSettings` behind `LanguageContext`'s claim)** must keep being used for the Done-time write; a hidden/order change is just a different payload shape through the same queued call, so this is a reminder for implementation, not a new risk.

## Migration Plan

No deploy-order dependency: the frontend change is self-contained (new interpretation of an already-generic JSON key), and the backend `user-settings` capability needs no change at all. Ship as a single frontend deploy.

**Rollback is lossy, not free.** Reverting the frontend after any user has saved the new shape leaves that user on the default grid order (see the asymmetry noted under "Migration" above), and a Done click on the reverted build overwrites their saved arrangement for good. That is an acceptable cost at this scale — one user, eight cards, a customization that takes seconds to redo — but it should be a deliberate choice, not a surprise. If a revert is ever needed after real use, the honest options are: accept the loss and re-customize, or restore the `dashboard_order` key from a settings backup.
