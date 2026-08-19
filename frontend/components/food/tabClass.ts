// Shared structure for the mode-selector tabs in ItemResolver.tsx and
// ManualItemEditor.tsx: a muted, neutral label that gains weight, accent
// color, and an underline sitting flush on the tab row's shared baseline
// when active — the classic "tab bar" pattern (GitHub, browser tabs, etc.),
// chosen specifically so these read as navigation, not as buttons, next to
// the actual search-submit button (see design.md under
// fix-ambiguous-search-button). Both tabs use the same neutral gray when
// inactive regardless of the panel's accent color, so the inactive tab
// doesn't visually compete with the accent-colored submit button below it.
// Pair with a container that has `border-b` so the active tab's `-mb-px
// border-b-2` overlaps it exactly. `activeClasses` is a literal string (not
// interpolated here) so Tailwind's static build-time class scan still finds
// it.
export function tabClass(active: boolean, activeClasses: string): string {
  const base = 'pb-2 -mb-px border-b-2';
  return active ? `${base} font-semibold ${activeClasses}` : `${base} border-transparent font-normal text-gray-400 dark:text-gray-500`;
}
