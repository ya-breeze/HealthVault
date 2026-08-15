## Context

The food entry/edit/review flow (`frontend/app/food/**`, `frontend/components/food/**`) is the primary daily-use surface of HealthVault and is used almost exclusively from a phone. A prior mobile-friendliness pass (PR #15, 2026-08-12) shipped as a single ad-hoc commit outside the OpenSpec workflow: no design doc, no written sizing standard, no test coverage, and (going by what it missed) no verification on a real device. It fixed input font size (iOS zoom-on-focus), header/nav tap targets, and a handful of review-page controls — `MealItemRow`'s delete button and its confirm/cancel pair got `min-h-11`/`min-w-11` (44px). But most of the actual food-picking interaction was never touched: `ItemResolver`/`ManualItemEditor`'s search-result rows and mode-toggle buttons, `ItemResolver`'s search-submit and translation-refresh buttons, and controls in history, custom foods, modals, and elsewhere remain unsized `text-xs`/`py-1` buttons. This surfaced as a real usability problem on the maintainer's Android phone (e.g. the delete "x" on a meal item is hard to hit reliably even at 44px, since 48px is the correct platform minimum — see Decisions).

Because the previous fix was applied element-by-element with no shared primitive, there is nothing today that keeps a newly added button from shipping undersized again.

## Goals / Non-Goals

**Goals:**
- Establish and document a single minimum tap-target size (48×48dp, Android Material Design) for interactive controls in the food flow.
- Make the standard structurally enforced (a shared component), not a convention that has to be remembered.
- Cover every interactive control currently in this flow, including ones the prior pass didn't touch (food search-result pickers, item delete, meal meta edit, modal buttons, history/custom-food list actions, and the header/toast controls left at the old 44px figure).
- Add automated coverage so a future PR can't silently regress a control below the minimum.

**Non-Goals:**
- Fixing horizontal layout overflow (elements/controls pushed off-screen by unwrapped flex rows or un-`overflow-hidden` modal cards). That is a distinct bug class — content escaping its container — tracked as a separate follow-up change.
- Redesigning the interaction pattern (e.g. swipe-to-delete). The chosen fix is sizing, not a new gesture model.
- Any backend, API, or data-model change — this is frontend-only.
- A general design-system overhaul. `TapTarget` is scoped to wrapping existing button/icon-button usages, not introducing a component library.

## Decisions

**48px minimum, not 44px.** Android's Material Design guideline is 48×48dp; Apple's HIG and WCAG 2.1 AA use 44×44px. The user tests exclusively on Android, so 48px is the correct platform-native number. Using the larger of the two also means the standard would satisfy iOS/WCAG if the app is ever used there too.

**A shared `TapTarget` component, not per-element className edits.** PR #15 patched individual elements and the fix didn't hold — untouched components kept the old small sizing, and there's no guardrail against reintroducing it. A wrapper component (`frontend/components/ui/TapTarget.tsx`) that enforces `min-h-12 min-w-12` (48px at Tailwind's default 4px scale) via its own styling, used for every button/icon-button/link-acting-as-button in scope, makes "too small" require actively opting out rather than being the default. Alternative considered: a Tailwind plugin enforcing a `min-touch-target` utility class — rejected as more machinery than a single wrapper component for a scope this size.

**Wrap existing markup rather than restyle in place.** For text buttons that already read fine visually (e.g. "Confirm"/"Cancel"), `TapTarget` adds invisible padding/min-size rather than changing visual appearance, so the fix doesn't turn into a visual-design pass. For icon-only buttons (delete "x", modal close), the click/tap area grows even if the icon glyph itself stays visually small — that's an accepted, standard pattern (icon smaller than its tap area).

**`TapTarget` spreads all native element props through, rather than allowlisting a fixed set.** The existing e2e suite locates several controls by attributes a naive wrapper could drop — e.g. `e2e/tests/food.spec.ts` finds `MealItemRow`'s delete button via `button[title="Delete item"]`, and `ClarifyModal` is located via `data-testid="clarify-modal"`. `TapTarget` is implemented as `<button {...rest} className={merge(minSizeClasses, rest.className)}>{children}</button>` (or the `<a>` equivalent when wrapping a link-as-button), so `title`, `aria-label`, `data-testid`, `disabled`, `onClick`, and any other native prop pass through unchanged. Migrating a control to `TapTarget` must not require touching how that control is located in tests.

**Verification: automated Playwright test + manual real-device check, not either alone.** The Playwright test asserts computed bounding-box size (`getBoundingClientRect()`) at a mobile viewport (e.g. 375×667) for the audited elements, guarding against regression. It cannot fully replace a real-device check (finger size, actual reachability, Android-Chrome-specific rendering), so a manual pass on the maintainer's phone still happens post-deploy, before merge — consistent with the project's dogfood-adjacent verification pattern for this app.

## Risks / Trade-offs

- **[Risk] Migrating every in-scope control to `TapTarget` touches many files → larger-than-usual diff for a "just make buttons bigger" change.** Mitigation: the change is mechanical (swap `<button className="...">` for `<TapTarget>`), low logic risk, and the shared-component approach is what the user explicitly chose over the faster per-element patch specifically to avoid a third round of this bug.
- **[Risk] Increasing icon-button tap areas without changing visual size can visually crowd tightly-packed rows (e.g. a row with name + weight input + delete button).** Mitigation: check each audited row during implementation for visual overlap/crowding at 375px width, not just for tap-area math; adjust row spacing if needed rather than shrinking the tap target back down.
- **[Risk] The out-of-scope layout-overflow bugs (custom foods list, Clarify/Custom Food modal `overflow-hidden`) live in components this change also touches for tap-target sizing.** Mitigation: touch only className/structure needed for tap-target sizing in those files; explicitly do not add `min-w-0`/`truncate`/`overflow-hidden` fixes here, and leave a one-line code comment marking the deferred issue where it's visible during this diff, so the follow-up change has a pointer. In `app/food/custom/page.tsx` specifically, the row's Edit/Delete/Confirm/Cancel controls sit in a `flex` row with no `flex-shrink-0` next to an un-truncated name — enlarging those controls to 48px claims more row width and can make the existing overflow bug surface sooner than it does today. Check this row visually at 375px width after migrating it; if the buttons now visibly crowd or push the name off-screen, that's expected (the underlying bug is unchanged, just more exposed) — do not fix the overflow here, just confirm the controls themselves are still fully visible and tappable.

## Migration Plan

No data migration. This is a frontend-only change:
1. Add `TapTarget` component.
2. Migrate audited components to use it, file by file.
3. Add the Playwright mobile-viewport size-assertion test.
4. Deploy to `hcw-wip`, validate the Playwright test passes, then do a manual pass on the maintainer's Android phone.
5. Rollback is a plain revert — no state to unwind.
