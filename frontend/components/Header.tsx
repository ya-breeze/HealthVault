'use client';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api, type Me } from '@/lib/api';
import { LinkIcon, ImportIcon, LogoutIcon, SettingsIcon } from '@/components/icons';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { useCopyToClipboard } from '@/lib/useCopyToClipboard';
import { SHED_CONTROL_IDS, type ShedControlId } from '@/components/nav';

/**
 * Shared header/nav rendered on every authenticated page (dashboard, data
 * pages, food logging, import) — see dashboard-ui's "Shared instrument-panel
 * header" requirement.
 *
 * Its control set is viewport-conditional. At and above the `sm` breakpoint
 * it carries all of them, as it always has. Below it, it keeps only the app
 * name and the user badge: the seven-control row is what made the header
 * wrap to 177px — a fifth of a 390px fold — and the controls it drops are
 * carried by the More sheet the bottom navigation bar opens.
 *
 * The session is no longer fetched here. `AuthenticatedShell` owns it and
 * passes it down, so this header and the More sheet always render from the
 * same response. See that component for why.
 */
export default function Header({ me }: { me: Me }) {
  const router = useRouter();
  const { t } = useLanguage();
  const { copied, copy } = useCopyToClipboard();
  const [showWebhook, setShowWebhook] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!showWebhook) return;
    const handler = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setShowWebhook(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [showWebhook]);

  const handleLogout = async () => {
    await api.logout();
    router.push('/login');
  };

  const webhookUrl = `${typeof window !== 'undefined' ? window.location.origin : ''}/webhook/${me.username}`;

  // Keyed by ShedControlId so this set and the More sheet's cannot diverge
  // without a type error — mobile-navigation's "No control is stranded on
  // mobile" scenario is what that guards.
  const shedControls: Record<ShedControlId, ReactNode> = {
    webhook: (
      <div key="webhook" className="relative hidden sm:block" ref={popoverRef}>
        <TapTarget
          onClick={() => setShowWebhook(v => !v)}
          data-nav-control="webhook"
          className="flex items-center gap-1.5 text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 transition-colors"
          title={t('header.webhookUrl')}
        >
          <LinkIcon className="w-4 h-4" />
          {t('header.webhook')}
        </TapTarget>
        {showWebhook && (
          <div className="absolute right-0 top-full mt-2 w-72 max-w-[calc(100vw-3rem)] bg-bg-elevated border border-border rounded-xl shadow-lg p-4 z-50">
            <p className="text-sm font-medium text-text mb-3">{t('header.webhookUrl')}</p>
            <code className="block w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm font-[family-name:var(--font-data)] text-text break-all mb-3 select-all">
              {webhookUrl}
            </code>
            <TapTarget
              onClick={() => copy(webhookUrl)}
              className={`w-full rounded-lg text-sm font-medium transition-all ${
                copied ? 'bg-accent/20 text-accent' : 'bg-accent text-bg-elevated hover:opacity-90'
              }`}
            >
              {copied ? t('header.copied') : t('header.copyToClipboard')}
            </TapTarget>
          </div>
        )}
      </div>
    ),
    'custom-foods': (
      <TapTarget
        key="custom-foods"
        as={Link}
        href="/food/custom/"
        data-nav-control="custom-foods"
        className="hidden sm:flex items-center text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 transition-colors"
      >
        {t('header.customFoods')}
      </TapTarget>
    ),
    import: (
      <TapTarget
        key="import"
        as={Link}
        href="/import"
        data-nav-control="import"
        className="hidden sm:flex items-center gap-1.5 text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 transition-colors"
      >
        <ImportIcon className="w-4 h-4" />
        {t('header.import')}
      </TapTarget>
    ),
    settings: (
      <TapTarget
        key="settings"
        as={Link}
        href="/settings"
        data-nav-control="settings"
        className="hidden sm:flex items-center justify-center gap-1.5 text-sm text-text-muted hover:text-text font-medium px-2 transition-colors"
        title={t('header.settings')}
      >
        <SettingsIcon className="w-4 h-4" />
      </TapTarget>
    ),
    logout: (
      <TapTarget
        key="logout"
        onClick={handleLogout}
        data-nav-control="logout"
        className="hidden sm:flex items-center justify-center gap-1.5 text-sm text-text-muted hover:text-red-500 font-medium px-2 transition-colors"
        title={t('header.logout')}
      >
        <LogoutIcon className="w-4 h-4" />
      </TapTarget>
    ),
  };

  return (
    <header className="bg-bg-elevated border-b border-border px-6 py-4">
      {/* `flex-nowrap sm:flex-wrap`, not a removed `flex-wrap`: wrapping is
          what made the mobile header grow instead of shrink, but this is one
          class on a container shared with the desktop rendering, and desktop
          keeps its current behaviour exactly — including that the full
          control set can still wrap between 640px and ~768px, which predates
          this change and is out of its scope. */}
      <div className="max-w-4xl mx-auto flex justify-between items-center flex-nowrap sm:flex-wrap gap-3">
        {/* A TapTarget now: with the nav controls gone below the breakpoint
            this and the badge are all the header renders, so this link is
            covered by mobile-touch-targets' "every interactive control the
            header still renders at that width" rather than sitting outside
            the enumeration the way it did when it was one of seven. The row
            is already 48px tall on the badge's account, so nothing moves. */}
        <TapTarget
          as={Link}
          href="/"
          className="flex items-center text-xl font-extrabold tracking-tight text-text"
        >
          HealthVault
        </TapTarget>
        <div className="flex items-center gap-2 flex-nowrap sm:flex-wrap min-w-0">
          {/* The badge is the widest variable-length element the mobile
              header still carries, so it is the one that gives way rather
              than pushing the row to a second line at 320px. The inner span
              is what carries `truncate`: text-overflow has no effect on the
              flex container that supplies the 48px height. */}
          <span className="font-[family-name:var(--font-data)] text-xs uppercase tracking-wide text-text-muted border border-border rounded-md px-2.5 py-2.5 min-h-12 flex items-center bg-bg min-w-0">
            <span className="truncate min-w-0">{me.username}</span>
          </span>
          {SHED_CONTROL_IDS.map(id => shedControls[id])}
        </div>
      </div>
    </header>
  );
}
