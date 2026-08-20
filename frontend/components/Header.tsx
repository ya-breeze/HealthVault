'use client';
import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api } from '@/lib/api';
import { LinkIcon, ImportIcon, LogoutIcon } from '@/components/icons';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { SUPPORTED_LANGUAGES, LanguageCode } from '@/lib/i18n';
import { useToast } from '@/components/Toast';

/**
 * Shared header/nav rendered on every authenticated page (dashboard, data
 * pages, food logging, import) — see dashboard-ui's "Shared instrument-panel
 * header" requirement. Redirects to /login if the session is invalid, same
 * as each page used to do individually.
 */
export default function Header() {
  const router = useRouter();
  const { t, language, setLanguage } = useLanguage();
  const { showToast } = useToast();
  const [me, setMe] = useState<{ id: string; username: string; family_id: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [showWebhook, setShowWebhook] = useState(false);
  const [changingLanguage, setChangingLanguage] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  const handleLanguageChange = async (code: LanguageCode) => {
    if (code === language) return;
    setChangingLanguage(true);
    try {
      await setLanguage(code);
    } catch {
      showToast(t('header.languageChangeFailed'), 'error');
    } finally {
      setChangingLanguage(false);
    }
  };

  useEffect(() => {
    api.me()
      .then(setMe)
      .catch(() => router.push('/login'));
  }, [router]);

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

  const webhookUrl = me
    ? `${typeof window !== 'undefined' ? window.location.origin : ''}/webhook/${me.username}`
    : '';

  const execCommandCopy = () => {
    const el = document.createElement('input');
    el.value = webhookUrl;
    el.style.cssText = 'position:fixed;opacity:0;top:0;left:0';
    document.body.appendChild(el);
    el.focus();
    el.select();
    document.execCommand('copy');
    document.body.removeChild(el);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleCopy = () => {
    if (navigator.clipboard) {
      navigator.clipboard.writeText(webhookUrl)
        .then(() => { setCopied(true); setTimeout(() => setCopied(false), 2000); })
        .catch(() => execCommandCopy());
    } else {
      execCommandCopy();
    }
  };

  return (
    <header className="bg-bg-elevated border-b border-border px-6 py-4">
      <div className="max-w-4xl mx-auto flex justify-between items-center flex-wrap gap-3">
        <Link href="/" className="text-xl font-extrabold tracking-tight text-text">
          HealthVault
        </Link>
        <div className="flex items-center gap-2 flex-wrap">
          <label className="sr-only" htmlFor="display-language">{t('header.language')}</label>
          <select
            id="display-language"
            value={language}
            disabled={changingLanguage}
            onChange={e => handleLanguageChange(e.target.value as LanguageCode)}
            // min-h-12 (48px), matching TapTarget's minimum rather than the
            // 44px this started at: the header is named explicitly in
            // openspec/specs/mobile-touch-targets "Minimum Tap Target Size",
            // and this is an interactive control in it. Not TapTarget itself —
            // that renders a single element and a <select> owns its own
            // options, so wrapping would put the tap target on a box around
            // the control rather than on the control. Found in code review.
            className="text-sm font-medium border border-border rounded-md px-2 py-2.5 min-h-12 bg-bg text-text-muted hover:text-text disabled:opacity-60"
          >
            {SUPPORTED_LANGUAGES.map(l => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>
          {me && (
            <span className="font-[family-name:var(--font-data)] text-xs uppercase tracking-wide text-text-muted border border-border rounded-md px-2.5 py-2.5 min-h-12 flex items-center bg-bg">
              {me.username}
            </span>
          )}
          {webhookUrl && (
            <div className="relative" ref={popoverRef}>
              <TapTarget
                onClick={() => setShowWebhook(v => !v)}
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
                    onClick={handleCopy}
                    className={`w-full rounded-lg text-sm font-medium transition-all ${
                      copied ? 'bg-accent/20 text-accent' : 'bg-accent text-bg-elevated hover:opacity-90'
                    }`}
                  >
                    {copied ? t('header.copied') : t('header.copyToClipboard')}
                  </TapTarget>
                </div>
              )}
            </div>
          )}
          <TapTarget
            as={Link}
            href="/food/custom/"
            className="flex items-center text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 transition-colors"
          >
            {t('header.customFoods')}
          </TapTarget>
          <TapTarget
            as={Link}
            href="/import"
            className="flex items-center gap-1.5 text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 transition-colors"
          >
            <ImportIcon className="w-4 h-4" />
            {t('header.import')}
          </TapTarget>
          <TapTarget
            onClick={handleLogout}
            className="flex items-center justify-center gap-1.5 text-sm text-text-muted hover:text-red-500 font-medium px-2 transition-colors"
            title={t('header.logout')}
          >
            <LogoutIcon className="w-4 h-4" />
          </TapTarget>
        </div>
      </div>
    </header>
  );
}
