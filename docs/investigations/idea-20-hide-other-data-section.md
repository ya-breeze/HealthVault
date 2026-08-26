# Investigation: Hide the "More Data" ("Другие данные") section entirely

Research notes for idea-forge plan `idea-20-hide-investigate.md`.

## Idea, restated

Today's More Data section (`dashboard.moreData` = "Другие данные") already
presence-filters secondary types (idea-6, merged as `578d86c`, archived at
`openspec/changes/archive/...` — see `dashboard-ui` spec's "Presence-based
filtering of the More Data section" requirement): a secondary type with zero
records ever is omitted, and the whole section collapses (no heading, no
`more-data` testid element) when nothing qualifies.

The user's complaint is that a type can have presence (one stray/old record)
without being something they actually track, so the section still shows up
and adds clutter that presence-filtering alone can't remove. The ask is a
single, section-wide "hide all of this" toggle — reachable the same way
per-card hide/show already is (enter Customize/edit mode, click an eye
icon) — as opposed to per-type toggles or a separate settings page.

## Relevant code

- `frontend/app/page.tsx`:
  - `SECONDARY_TYPES` (line 26 area, module scope) — `DATA_TYPES` minus
    `PRIMARY_METRICS`, the fixed candidate set for the section.
  - `presentSecondaryTypes` (~line 163) — `SECONDARY_TYPES` filtered by
    `hasPresence`, gated on `presenceReady` so nothing flashes before the
    presence fetch resolves/fails.
  - More Data render block (~lines 310-329) — `presentSecondaryTypes.length
    > 0 && <div data-testid="more-data">…</div>`, a plain link list with no
    edit-mode state of its own today (no eye icon, no per-item controls).
  - The vitals-grid Customize/Done toggle and `editing` state (~lines
    28, 213-235) are the existing "enter edit mode" mechanism this idea
    would reuse — `editing` is a single page-level boolean, not scoped to
    the vitals grid specifically, so the More Data section can read it too.
  - `handleDone()` (~line 172) — the one place `updateSettings()` is called
    with the vitals `dashboard_order` patch; a section-hidden flag would be
    another key merged into the same PUT, not a second writer (per Phase
    1's queued-write constraint below).
- `frontend/lib/vitals.ts` — `DashboardCardPref`, `reconcileMetricOrder`,
  `hasPresence`. No existing concept of a section-level (as opposed to
  per-card) hidden flag; this would be new, simpler state (one boolean, no
  list, no position-preservation concern since a whole section isn't
  reordered).
- `frontend/lib/api.ts`:
  - `UserSettings` interface (~line 309) — opaque per-user JSON blob,
    already holds `dashboard_order`, `display_language`, `timezone`,
    `usual_meals_per_day`. A new `more_data_hidden?: boolean`-shaped key
    fits the same pattern (no backend schema change, generic
    `GET`/`PUT /api/users/me/settings` from the `user-settings` capability
    is unaffected).
  - `updateSettings()` (~line 477) — GET-merge-PUT with a documented
    lost-update-avoidance contract: it does its own fresh GET rather than
    trusting a caller's cached copy, specifically to avoid clobbering keys
    a concurrent writer just saved. Any new call site (or reuse of the
    existing `handleDone` call site) must go through this same function,
    not a second ad hoc PUT.
- `components/LanguageContext.tsx` — owns a `claim()` queue that
  `page.tsx`'s `updateSettings` calls already queue behind, to avoid a
  race between the reorder screen and the language switcher (both write
  the same blob). A More Data hide toggle piggybacking on the existing
  `handleDone` write inherits this for free; a toggle that saves
  immediately on click (its own PUT, not batched into Done) would need to
  go through the same queued path independently.
- `openspec/specs/dashboard-ui/spec.md` — two requirements this idea
  extends rather than replaces: "Customizable vitals grid order and
  visibility" (Phase 1's per-card show/hide, scoped explicitly to the 8
  vitals cards) and "Presence-based filtering of the More Data section"
  (idea-6, presence-only filtering, no user preference layer today). A new
  requirement (or a modification of the second one) needs to state how a
  user-hidden section composes with presence filtering.
- `openspec/changes/archive/2026-08-21-dashboard-card-visibility/` (Phase
  1) and the idea-6 investigation doc
  (`docs/investigations/idea-6-hide-data-types-with-no-data-at-more-...md`,
  actually `idea-6-hide-data-types-with-no-data-at-all.md`) — the two
  closest precedents; both are read in full for constraints below.
- `e2e/tests/dashboard.spec.ts` — existing More Data coverage: line 151
  (link navigation), 706 (presence-based per-type omission), 717 (section
  absent when nothing qualifies), 727/752 (fail-open on presence-fetch
  failure), 770/776 (various section presence/absence checks). None of
  these exercise an edit-mode control or a user-hidden state — that
  coverage would be new.
- `frontend/lib/i18n/en.ts` / `ru.ts` — `dashboard.moreData` key exists;
  a new toggle needs new label keys in both dictionaries (enforced by the
  `Dictionary`/`DICTIONARIES` typing, a missing `ru` key is a compile
  error — same guarantee Phase 1 relied on).

## Constraints and unknowns

- **Storage shape**: a single boolean (`more_data_hidden` or similar) in
  the existing `UserSettings` blob is the natural fit — no schema
  migration, no read-time reconciliation like Phase 1's `dashboard_order`
  needed (that reconciliation exists because the *shape* of a list changed;
  a single new boolean key has no old shape to be compatible with).
- **Interaction with presence filtering**: needs an explicit decision,
  not an accident of implementation order. Two readings are both
  defensible:
  - User-hidden always wins: if hidden, the section never renders,
    regardless of presence (simplest mental model — "hidden" means hidden).
  - Presence still gates visibility of the *toggle itself*: if presence
    filtering would already collapse the section to nothing, there's
    nothing to hide, so a Customize control with no effect is confusing —
    parallels how Phase 1 excludes zero-presence vitals cards from the
    edit-mode list entirely rather than showing a no-op toggle for them.
  This is the one open design question this investigation leaves for the
  proposal's `design.md`, not something the code inspection alone resolves.
- **Control placement and granularity**: the ask is explicitly one toggle
  for the *whole section*, not per-secondary-type checkboxes (that would be
  a materially bigger feature — 18 individually-toggleable, currently
  presence-filtered types, no existing UI pattern for a checklist of that
  length). Placement is a design choice: most natural is an eye icon next
  to the "Другие данные" heading itself, visible only in edit mode, distinct
  from the vitals grid's per-card row controls (which live inside each
  `VitalCard`, not at a section-header level — no existing component to
  copy verbatim, though the icon asset itself already exists in
  `frontend/components/icons` from Phase 1).
  Confirmed while reading `page.tsx`: the "log food" action row (photo/
  manual/history) sits *between* the vitals grid and the More Data heading,
  and the needs-attention banner sits between the grid and that row —
  neither one is `editing`-aware today, so the new header-level control
  should not accidentally key off a shared "edit mode" region that also
  affects those.
- **Empty edit-mode state**: when a user has hidden the section, what does
  edit mode show in its place — the collapsed section with a "show" toggle,
  or something else? Needs to remain discoverable/reversible the same way
  Phase 1 keeps hidden vital cards visible-but-dimmed in edit mode (this
  idea is a single boolean, not a list of hideable rows, but the same
  reversibility expectation from the "Why" — "so hiding is undoable" —
  likely applies).
- **Read-only vs. edit-mode render condition**: today's gate is
  `presentSecondaryTypes.length > 0`. Adding the user flag turns this into
  a two-condition gate (`!moreDataHidden && presentSecondaryTypes.length >
  0` for read-only), which is a small, mechanical change to the existing
  block — not a structural rewrite.
- **Settings-load gating**: Phase 1 added a whole `settingsStatus`
  loading/error state specifically because rendering before the saved
  order arrives would flash hidden cards. A `more_data_hidden` flag reads
  from the exact same `getSettings()` call already gated by
  `dashboardReady`/`settingsLoaded` — so this idea can reuse that existing
  gate rather than inventing a second one, as long as the More Data render
  path is also held behind `dashboardReady` (today it is only gated on
  `presenceReady`, not `settingsLoaded`, since it has never before read
  anything from the settings blob). This is a genuine change to the
  section's current gating, not just an additive flag check.
- **No position/list semantics needed**: unlike vitals cards (`hidden`
  alongside `order`, position preserved on re-show), a whole-section toggle
  has no ordering concept — a plain boolean is sufficient, no
  `DashboardCardPref`-style structure needed for this flag.
- **Scope boundary vs. idea-6**: idea-6's presence filtering and this
  idea's user-preference toggle are orthogonal and composable, same
  relationship Phase 1's per-card hide has to presence-based card
  exclusion (see the "Presence-based visibility" requirement, which already
  documents that composition for the vitals grid). No conflict found; the
  proposal should state the composition rule explicitly for More Data,
  mirroring that existing text.

## Conclusion

This is a small, well-precedented extension: one new boolean key in the
existing generic `UserSettings` blob, one new edit-mode control (placement:
section header, not per-item), and a two-line change to the existing
render-gate condition. Phase 1 (per-card hide/show) and idea-6 (presence
filtering) between them already establish every mechanical pattern needed
— queued settings writes, settings-load gating to avoid flashing hidden
state, fail-open presence composition, i18n key requirements, e2e coverage
style. The two decisions genuinely open for the proposal are (1) whether a
user-hidden section suppresses the section even when presence would already
suppress it (or, conversely, whether the toggle should even appear when
presence has left nothing to hide), and (2) the exact control placement at
the section header. Both are proposal/design-level choices, not blocked by
anything found in the code.

## Suggested next steps

1. Decide and document the presence/user-preference composition rule and
   the toggle's visibility-when-nothing-present behavior in the proposal's
   `design.md`.
2. Add a `more_data_hidden?: boolean` field to `UserSettings`
   (`frontend/lib/api.ts`), documented like its neighboring keys.
3. Add a section-header edit-mode toggle in `page.tsx`, wired through the
   existing `editing` state and the existing `handleDone`/`updateSettings`
   write path (no new writer).
4. Gate the More Data render path on `dashboardReady` (not just
   `presenceReady`) so the new flag can't flash unfiltered content the way
   Phase 1 already prevents for the vitals grid.
5. New i18n keys in `en.ts`/`ru.ts` for the toggle's label/accessible name.
6. New e2e coverage: hide the section and confirm it disappears from the
   read-only page but remains discoverable/reversible in edit mode; re-show
   it; persistence across reload; composition with presence (zero-presence
   *and* user-hidden vs. either alone).
