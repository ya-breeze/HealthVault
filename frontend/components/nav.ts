import type { ComponentType, SVGProps } from 'react';
import type { Dictionary } from '@/lib/i18n';
import { CameraIcon, HistoryIcon, HomeIcon, PencilIcon } from '@/components/icons';

type IconComponent = ComponentType<SVGProps<SVGSVGElement> & { active?: boolean }>;

/**
 * The four routed destinations of the mobile bottom navigation bar, in the
 * order they appear. The fifth destination, More, is not routed — it opens a
 * sheet — so it lives in BottomNav itself rather than here.
 *
 * See mobile-navigation's "Mobile bottom navigation bar".
 */
export const NAV_DESTINATIONS: ReadonlyArray<{
  id: string;
  href: string;
  labelKey: keyof Dictionary;
  Icon: IconComponent;
}> = [
  { id: 'home', href: '/', labelKey: 'nav.home', Icon: HomeIcon },
  { id: 'photo', href: '/food/upload/', labelKey: 'nav.photo', Icon: CameraIcon },
  { id: 'manual', href: '/food/manual/', labelKey: 'nav.manual', Icon: PencilIcon },
  { id: 'history', href: '/food/history/', labelKey: 'nav.history', Icon: HistoryIcon },
];

/**
 * The controls the header carries at desktop widths and sheds below the
 * navigation breakpoint. Both `Header` and `MoreSheet` build a
 * `Record<ShedControlId, ReactNode>` and render it through this list, so
 * adding a control to one surface without the other is a type error rather
 * than a control that is silently unreachable on mobile — see
 * mobile-navigation's "No control is stranded on mobile".
 *
 * Each rendered control also carries `data-nav-control="<id>"`, which is
 * what lets the e2e test compare the two surfaces' sets against each other
 * rather than against a literal copied into the test.
 */
export const SHED_CONTROL_IDS = ['webhook', 'custom-foods', 'import', 'settings', 'logout'] as const;

export type ShedControlId = (typeof SHED_CONTROL_IDS)[number];

/**
 * Trailing-slash-insensitive route comparison. `next.config.ts` sets
 * `trailingSlash: true`, so `usePathname()` yields `/food/history/`, but the
 * hrefs the app links to are written both ways (`/import` and
 * `/food/custom/` both appear in the header today). Normalizing both sides
 * means an active destination is decided by the route, never by how its
 * href happened to be spelled.
 */
export function isSameRoute(pathname: string | null, href: string): boolean {
  if (!pathname) return false;
  const strip = (p: string) => (p.length > 1 ? p.replace(/\/+$/, '') : p);
  return strip(pathname) === strip(href);
}
