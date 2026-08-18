// Shared structure for the mode-selector tabs in ItemResolver.tsx and
// ManualItemEditor.tsx: an underline indicator when active, nothing when
// not — never a solid fill, which is reserved for the actual search-submit
// button (see design.md under fix-ambiguous-search-button for why). Callers
// pass their own literal color classes (not interpolated here) so Tailwind's
// static build-time class scan still finds them.
export function tabClass(active: boolean, activeClasses: string, inactiveClasses: string): string {
  return `px-2 pb-1 border-b-2 ${active ? activeClasses : inactiveClasses}`;
}
