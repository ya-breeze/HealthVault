'use client';
import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api } from '@/lib/api';
import { LinkIcon, ImportIcon, LogoutIcon } from '@/components/icons';

/**
 * Shared header/nav rendered on every authenticated page (dashboard, data
 * pages, food logging, import) — see dashboard-ui's "Shared instrument-panel
 * header" requirement. Redirects to /login if the session is invalid, same
 * as each page used to do individually.
 */
export default function Header() {
  const router = useRouter();
  const [me, setMe] = useState<{ id: string; username: string; family_id: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [showWebhook, setShowWebhook] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

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
          {me && (
            <span className="font-[family-name:var(--font-data)] text-xs uppercase tracking-wide text-text-muted border border-border rounded-md px-2.5 py-1.5 bg-bg">
              {me.username}
            </span>
          )}
          {webhookUrl && (
            <div className="relative" ref={popoverRef}>
              <button
                onClick={() => setShowWebhook(v => !v)}
                className="flex items-center gap-1.5 text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 py-1.5 transition-colors"
                title="Webhook URL"
              >
                <LinkIcon className="w-4 h-4" />
                Webhook
              </button>
              {showWebhook && (
                <div className="absolute right-0 top-full mt-2 w-96 bg-bg-elevated border border-border rounded-xl shadow-lg p-4 z-50">
                  <p className="text-sm font-medium text-text mb-3">Webhook URL</p>
                  <code className="block w-full bg-bg border border-border rounded-lg px-3 py-2 text-sm font-[family-name:var(--font-data)] text-text break-all mb-3 select-all">
                    {webhookUrl}
                  </code>
                  <button
                    onClick={handleCopy}
                    className={`w-full py-2 rounded-lg text-sm font-medium transition-all ${
                      copied ? 'bg-accent/20 text-accent' : 'bg-accent text-bg-elevated hover:opacity-90'
                    }`}
                  >
                    {copied ? 'Copied!' : 'Copy to clipboard'}
                  </button>
                </div>
              )}
            </div>
          )}
          <Link
            href="/food/custom/"
            className="text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 py-1.5 transition-colors"
          >
            Custom Foods
          </Link>
          <Link
            href="/import"
            className="flex items-center gap-1.5 text-sm text-text-muted hover:text-text font-medium border border-border rounded-md px-2.5 py-1.5 transition-colors"
          >
            <ImportIcon className="w-4 h-4" />
            Import
          </Link>
          <button
            onClick={handleLogout}
            className="flex items-center gap-1.5 text-sm text-text-muted hover:text-red-500 font-medium transition-colors"
            title="Logout"
          >
            <LogoutIcon className="w-4 h-4" />
          </button>
        </div>
      </div>
    </header>
  );
}
