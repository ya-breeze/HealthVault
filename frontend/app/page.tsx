'use client';
import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, DATA_TYPES } from '@/lib/api';

const DASHBOARD_TYPES = ['steps', 'heart_rate', 'sleep', 'heart_rate_variability', 'distance',
  'weight', 'blood_pressure', 'oxygen_saturation'];

const TYPE_ACCENT: Record<string, string> = {
  steps: 'bg-blue-500',
  heart_rate: 'bg-red-500',
  sleep: 'bg-indigo-500',
  heart_rate_variability: 'bg-purple-500',
  distance: 'bg-green-500',
  weight: 'bg-orange-500',
  blood_pressure: 'bg-rose-500',
  oxygen_saturation: 'bg-cyan-500',
};

export default function Dashboard() {
  const router = useRouter();
  const [summary, setSummary] = useState<{ steps: number; avg_heart_rate: number; sleep_seconds: number } | null>(null);
  const [me, setMe] = useState<{ id: string; username: string; family_id: string } | null>(null);
  const [selectedUser, setSelectedUser] = useState<string>('');
  const [summaryError, setSummaryError] = useState(false);
  const [copied, setCopied] = useState(false);
  const [showWebhook, setShowWebhook] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  const from = (() => {
    const d = new Date();
    d.setDate(d.getDate() - 7);
    return d.toISOString().slice(0, 10) + 'T00:00:00Z';
  })();
  const to = new Date().toISOString().slice(0, 10) + 'T23:59:59Z';

  useEffect(() => {
    api.me()
      .then(user => {
        setMe(user);
        setSelectedUser(user.username);
      })
      .catch(() => router.push('/login'));
  }, [router]);

  useEffect(() => {
    if (!selectedUser) return;
    setSummaryError(false);
    api.summary(from, to, selectedUser)
      .then(setSummary)
      .catch(() => setSummaryError(true));
  }, [selectedUser, from, to]);

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

  const webhookUrl = selectedUser
    ? `${typeof window !== 'undefined' ? window.location.origin : ''}/webhook/${selectedUser}`
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
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="max-w-4xl mx-auto flex justify-between items-center">
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">HealthVault</h1>
          <div className="flex items-center gap-4">
            {me && (
              <select
                value={selectedUser}
                onChange={e => setSelectedUser(e.target.value)}
                className="border border-gray-300 dark:border-gray-600 rounded-md px-3 py-1.5 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value={me.username}>{me.username}</option>
              </select>
            )}
            {webhookUrl && (
              <div className="relative" ref={popoverRef}>
                <button
                  onClick={() => setShowWebhook(v => !v)}
                  className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 font-medium border border-gray-200 dark:border-gray-600 rounded-md px-2.5 py-1.5 transition-colors"
                  title="Webhook URL"
                >
                  Webhook
                </button>
                {showWebhook && (
                  <div className="absolute right-0 top-full mt-2 w-96 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-xl shadow-lg p-4 z-50">
                    <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Webhook URL</p>
                    <code className="block w-full bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg px-3 py-2 text-sm font-mono text-gray-800 dark:text-gray-200 break-all mb-3 select-all">
                      {webhookUrl}
                    </code>
                    <button
                      onClick={handleCopy}
                      className={`w-full py-2 rounded-lg text-sm font-medium transition-all ${
                        copied
                          ? 'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300'
                          : 'bg-blue-600 hover:bg-blue-700 text-white'
                      }`}
                    >
                      {copied ? 'Copied!' : 'Copy to clipboard'}
                    </button>
                  </div>
                )}
              </div>
            )}
            <button
              onClick={handleLogout}
              className="text-sm text-red-500 hover:text-red-700 dark:hover:text-red-400 font-medium"
            >
              Logout
            </button>
          </div>
        </div>
      </header>

      <main className="max-w-4xl mx-auto px-6 py-8">
        {summary && !summaryError && (
          <div className="grid grid-cols-3 gap-4 mb-8">
            <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm overflow-hidden flex">
              <div className="w-1.5 bg-blue-500 flex-shrink-0" />
              <div className="p-5 flex-1">
                <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Steps (last 7 days)</p>
                <p className="text-3xl font-bold text-gray-900 dark:text-white mt-1">{summary.steps.toLocaleString()}</p>
              </div>
            </div>
            <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm overflow-hidden flex">
              <div className="w-1.5 bg-red-500 flex-shrink-0" />
              <div className="p-5 flex-1">
                <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Avg Heart Rate</p>
                <p className="text-3xl font-bold text-gray-900 dark:text-white mt-1">
                  {summary.avg_heart_rate.toFixed(0)}{' '}
                  <span className="text-lg font-normal text-gray-500 dark:text-gray-400">bpm</span>
                </p>
              </div>
            </div>
            <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm overflow-hidden flex">
              <div className="w-1.5 bg-indigo-500 flex-shrink-0" />
              <div className="p-5 flex-1">
                <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Sleep (last night)</p>
                <p className="text-3xl font-bold text-gray-900 dark:text-white mt-1">
                  {(summary.sleep_seconds / 3600).toFixed(1)}{' '}
                  <span className="text-lg font-normal text-gray-500 dark:text-gray-400">h</span>
                </p>
              </div>
            </div>
          </div>
        )}

        {summaryError && (
          <p className="text-gray-500 dark:text-gray-400 text-sm mb-8">Could not load summary data.</p>
        )}

        <h2 className="text-base font-semibold text-gray-900 dark:text-white mb-3">Browse Data</h2>
        <div className="grid grid-cols-4 gap-3 mb-8">
          {DASHBOARD_TYPES.map(t => (
            <a
              key={t}
              href={`/data/${t}/${selectedUser ? `?user=${selectedUser}` : ''}`}
              className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-4 flex flex-col items-center gap-2 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-md transition-all text-sm font-medium text-gray-700 dark:text-gray-200 capitalize"
            >
              <div className={`w-2.5 h-2.5 rounded-full ${TYPE_ACCENT[t] ?? 'bg-gray-400'}`} />
              {t.replace(/_/g, ' ')}
            </a>
          ))}
        </div>

        <h2 className="text-base font-semibold text-gray-900 dark:text-white mb-3">All Data Types</h2>
        <div className="grid grid-cols-4 gap-2">
          {DATA_TYPES.map(t => (
            <a
              key={t}
              href={`/data/${t}/${selectedUser ? `?user=${selectedUser}` : ''}`}
              className="bg-white dark:bg-gray-800 rounded-lg border border-gray-100 dark:border-gray-700 px-3 py-2 text-center hover:border-blue-400 dark:hover:border-blue-500 transition-colors text-xs font-medium text-gray-600 dark:text-gray-300 capitalize"
            >
              {t.replace(/_/g, ' ')}
            </a>
          ))}
        </div>
      </main>
    </div>
  );
}
