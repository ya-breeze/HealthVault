'use client';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { MoreIcon } from '@/components/icons';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { NAV_DESTINATIONS, isSameRoute } from '@/components/nav';

/**
 * Mobile bottom navigation bar — five destinations, fixed to the bottom of
 * the viewport below the `sm` breakpoint. See mobile-navigation's "Mobile
 * bottom navigation bar".
 *
 * Hidden with `sm:hidden` rather than mounted on a `matchMedia` result. The
 * app is statically exported, so a JS-evaluated media test would bake one
 * viewport's navigation into the served HTML and correct itself only after
 * hydration — a visible flash on every load, and the wrong branch entirely
 * for a client that never hydrates. The cost is that the markup ships at
 * every width; for five links that is negligible, and it is what makes the
 * transition on resize instant.
 *
 * `z-40` puts the bar above the two pages' submit bars (`z-30`) and below
 * the full-screen overlays and toasts (`z-50`), which are meant to cover it.
 * Stacking is not how the bar avoids occluding those submit bars, though —
 * that is `--nav-block` in globals.css, since a control underneath the bar
 * is unusable no matter which of the two paints on top.
 */
export default function BottomNav({
  onMoreClick,
  moreOpen,
  moreButtonRef,
}: {
  onMoreClick: () => void;
  moreOpen: boolean;
  moreButtonRef?: React.Ref<HTMLButtonElement>;
}) {
  const pathname = usePathname();
  const { t } = useLanguage();

  return (
    <nav
      aria-label={t('nav.label')}
      data-testid="bottom-nav"
      className="sm:hidden fixed bottom-0 inset-x-0 z-40 bg-bg-elevated border-t border-border pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="flex h-[var(--nav-bar-h)] items-stretch">
        {NAV_DESTINATIONS.map(({ id, href, labelKey, Icon }) => {
          const active = isSameRoute(pathname, href);
          return (
            <li key={id} className="flex-1 flex">
              <TapTarget
                as={Link}
                href={href}
                data-nav-destination={id}
                aria-current={active ? 'page' : undefined}
                className={`flex-1 flex flex-col items-center justify-center gap-1 px-1 transition-colors ${
                  active ? 'text-accent' : 'text-text-muted'
                }`}
              >
                <Icon active={active} className="w-6 h-6 shrink-0" />
                {/* `truncate` and not a wrap: the tap target keeps its width
                    and the label is what gives way, per mobile-touch-targets'
                    "Bottom navigation destinations meet the minimum". */}
                <span className={`w-full text-center text-[10px] leading-tight truncate ${active ? 'font-semibold' : 'font-medium'}`}>
                  {t(labelKey)}
                </span>
              </TapTarget>
            </li>
          );
        })}
        <li className="flex-1 flex">
          <TapTarget
            ref={moreButtonRef}
            onClick={onMoreClick}
            data-nav-destination="more"
            aria-haspopup="dialog"
            aria-expanded={moreOpen}
            className={`flex-1 flex flex-col items-center justify-center gap-1 px-1 transition-colors ${
              moreOpen ? 'text-accent' : 'text-text-muted'
            }`}
          >
            <MoreIcon active={moreOpen} className="w-6 h-6 shrink-0" />
            <span className={`w-full text-center text-[10px] leading-tight truncate ${moreOpen ? 'font-semibold' : 'font-medium'}`}>
              {t('nav.more')}
            </span>
          </TapTarget>
        </li>
      </ul>
    </nav>
  );
}
