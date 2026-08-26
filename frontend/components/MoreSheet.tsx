'use client';
import { useEffect, useRef, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api, type Me } from '@/lib/api';
import { ImportIcon, LinkIcon, LogoutIcon, SettingsIcon } from '@/components/icons';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { useCopyToClipboard } from '@/lib/useCopyToClipboard';
import { SHED_CONTROL_IDS, type ShedControlId } from '@/components/nav';

const ROW_CLASSES =
  'w-full flex items-center gap-3 px-4 rounded-lg text-sm font-medium text-text hover:bg-bg transition-colors';

/**
 * The sheet the bottom bar's More destination opens. It carries exactly the
 * controls the header sheds below the breakpoint — see mobile-navigation's
 * "More sheet carries the header's remaining controls".
 *
 * Mounted only while open, rather than shipped hidden the way BottomNav is.
 * The reason BottomNav is CSS-hidden — that the served static HTML must
 * already show the right navigation before hydration — does not apply to a
 * surface that only ever appears in response to a tap. Keeping it unmounted
 * also keeps its controls out of the accessibility tree and out of
 * text/title-based test locators at desktop widths, where the header renders
 * the same five controls.
 */
export default function MoreSheet({
  me,
  onClose,
}: {
  me: Me;
  onClose: () => void;
}) {
  const router = useRouter();
  const { t } = useLanguage();
  const { copied, copy } = useCopyToClipboard();
  const panelRef = useRef<HTMLDivElement>(null);

  const webhookUrl = `${typeof window !== 'undefined' ? window.location.origin : ''}/webhook/${me.username}`;

  const handleLogout = async () => {
    await api.logout();
    router.push('/login');
  };

  // Escape closes; Tab is confined to the panel. Both are here rather than
  // on the panel element itself because focus can legitimately be on the
  // document body at the moment the sheet mounts.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
        return;
      }
      if (e.key !== 'Tab' || !panelRef.current) return;
      const focusable = panelRef.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input, [tabindex]:not([tabindex="-1"])'
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (e.shiftKey && (active === first || !panelRef.current.contains(active))) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  // Background scroll lock. Restores whatever `overflow` was set before,
  // rather than assuming it was the empty string.
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = previous; };
  }, []);

  // Moves focus into the sheet on open; AuthenticatedShell puts it back on
  // the More destination when the sheet unmounts.
  useEffect(() => {
    panelRef.current?.querySelector<HTMLElement>('a[href], button:not([disabled])')?.focus();
  }, []);

  // A Record keyed by ShedControlId, so a control added to the header
  // without a counterpart here is a compile error rather than a control that
  // is simply unreachable on mobile.
  const controls: Record<ShedControlId, ReactNode> = {
    webhook: (
      <div key="webhook" data-nav-control="webhook" className="px-4 py-3 border border-border rounded-lg">
        <p className="flex items-center gap-2 text-sm font-medium text-text mb-2">
          <LinkIcon className="w-4 h-4" />
          {t('header.webhookUrl')}
        </p>
        <code className="block w-full bg-bg border border-border rounded-lg px-3 py-2 text-xs font-[family-name:var(--font-data)] text-text break-all mb-2 select-all">
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
    ),
    'custom-foods': (
      <TapTarget
        key="custom-foods"
        as={Link}
        href="/food/custom/"
        data-nav-control="custom-foods"
        onClick={onClose}
        className={ROW_CLASSES}
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
        onClick={onClose}
        className={ROW_CLASSES}
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
        onClick={onClose}
        className={ROW_CLASSES}
      >
        <SettingsIcon className="w-4 h-4" />
        {t('header.settings')}
      </TapTarget>
    ),
    logout: (
      <TapTarget
        key="logout"
        onClick={handleLogout}
        data-nav-control="logout"
        className={`${ROW_CLASSES} hover:text-red-500`}
      >
        <LogoutIcon className="w-4 h-4" />
        {t('header.logout')}
      </TapTarget>
    ),
  };

  return (
    <div
      data-testid="more-sheet-backdrop"
      className="fixed inset-0 z-50 bg-black/50 flex items-end"
      onClick={onClose}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={t('nav.moreTitle')}
        data-testid="more-sheet"
        // Stops a click inside the panel from reaching the backdrop's
        // dismiss handler above.
        onClick={e => e.stopPropagation()}
        className="w-full bg-bg-elevated border-t border-border rounded-t-2xl px-4 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] max-h-[85vh] overflow-y-auto flex flex-col gap-2"
      >
        <div className="flex items-center justify-between mb-1">
          <p className="text-sm font-semibold text-text">{t('nav.moreTitle')}</p>
          <TapTarget
            onClick={onClose}
            aria-label={t('nav.close')}
            className="flex items-center justify-center text-text-muted hover:text-text text-xl leading-none"
          >
            &times;
          </TapTarget>
        </div>
        {SHED_CONTROL_IDS.map(id => controls[id])}
      </div>
    </div>
  );
}
