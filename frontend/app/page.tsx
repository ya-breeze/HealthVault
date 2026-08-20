'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, DATA_TYPES, UserSettings } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { PRIMARY_METRICS, extractVital, reconcileMetricOrder, VitalResult } from '@/lib/vitals';
import { useToast } from '@/components/Toast';
import Header from '@/components/Header';
import VitalCard from '@/components/VitalCard';
import TapTarget from '@/components/ui/TapTarget';
import { CameraIcon, PencilIcon, HistoryIcon } from '@/components/icons';

const SECONDARY_TYPES = DATA_TYPES.filter(t => !PRIMARY_METRICS.some(m => m.type === t));

export default function Dashboard() {
  const router = useRouter();
  const { showToast } = useToast();
  const [ready, setReady] = useState(false);
  const [vitals, setVitals] = useState<Record<string, VitalResult | null>>({});
  const [needsAttentionCount, setNeedsAttentionCount] = useState(0);
  const [settings, setSettings] = useState<UserSettings>({});
  const [settingsLoaded, setSettingsLoaded] = useState(false);
  const [order, setOrder] = useState(PRIMARY_METRICS);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.me()
      .then(() => setReady(true))
      .catch(() => router.push('/login'));
  }, [router]);

  useEffect(() => {
    if (!ready) return;
    api.getSettings()
      .then(s => {
        setSettings(s);
        setOrder(reconcileMetricOrder(s.dashboard_order));
        setSettingsLoaded(true);
      })
      .catch(() => {
        // Leave settingsLoaded false: editing/saving stays disabled rather
        // than risking a Done that overwrites real stored settings with a
        // PUT built from the (unknown) default order.
        showToast('Could not load your saved dashboard order. Reordering is unavailable right now.', 'error');
      });
  }, [ready, showToast]);

  useEffect(() => {
    if (!ready) return;
    const from = (() => {
      const d = new Date();
      d.setDate(d.getDate() - 7);
      d.setUTCHours(0, 0, 0, 0);
      return d.toISOString();
    })();
    const to = new Date().toISOString();

    Promise.all(
      PRIMARY_METRICS.map(m => api.data(m.type, from, to, undefined, 'day').catch(() => []))
    ).then(results => {
      const next: Record<string, VitalResult | null> = {};
      PRIMARY_METRICS.forEach((m, i) => {
        next[m.type] = extractVital(m.type, results[i]);
      });
      setVitals(next);
    });
  }, [ready]);

  useEffect(() => {
    if (!ready) return;
    api.needsAttentionCount()
      .then(res => setNeedsAttentionCount(res.count))
      .catch(() => setNeedsAttentionCount(0));
  }, [ready]);

  function moveCard(index: number, direction: -1 | 1) {
    setOrder(prev => {
      const target = index + direction;
      if (target < 0 || target >= prev.length) return prev;
      const next = [...prev];
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
  }

  async function handleDone() {
    setSaving(true);
    try {
      // Reads a fresh copy of settings immediately before writing, rather
      // than merging onto the possibly-stale cached `settings` state: this
      // component and LanguageContext each keep their own independent
      // cached UserSettings and PUT a read-modify-write built from it, with
      // no shared store between them — see LanguageContext.tsx's setLanguage
      // for the lost-update this closes (reordering here, then switching
      // language without navigating, used to silently revert this save).
      const current = await api.getSettings().catch(() => settings);
      const next: UserSettings = { ...current, dashboard_order: order.map(m => m.type) };
      await api.putSettings(next);
      setSettings(next);
      setEditing(false);
    } catch {
      showToast('Could not save the new card order. Try again.', 'error');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="min-h-screen bg-bg">
      <Header />

      <main className="max-w-4xl mx-auto px-6 py-8">
        <div className="flex items-center justify-between mb-3">
          <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent">
            Vitals · last 7 days
          </p>
          {editing ? (
            <TapTarget
              onClick={handleDone}
              disabled={saving}
              className="px-3 rounded-md border border-accent text-accent text-[11px] font-bold uppercase tracking-wide disabled:opacity-50"
            >
              {saving ? 'Saving…' : 'Done'}
            </TapTarget>
          ) : (
            <TapTarget
              onClick={() => setEditing(true)}
              disabled={!settingsLoaded}
              title={settingsLoaded ? undefined : 'Loading your saved order…'}
              className="px-3 rounded-md border border-border text-text-muted hover:border-accent hover:text-accent transition-colors text-[11px] font-bold uppercase tracking-wide disabled:opacity-50"
            >
              Edit order
            </TapTarget>
          )}
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-8" data-testid="vitals-grid">
          {order.map((m, i) => (
            <VitalCard
              key={m.type}
              type={m.type}
              label={m.label}
              result={ready ? vitals[m.type] ?? null : null}
              editing={editing}
              onMoveUp={() => moveCard(i, -1)}
              onMoveDown={() => moveCard(i, 1)}
              moveUpDisabled={i === 0}
              moveDownDisabled={i === order.length - 1}
            />
          ))}
        </div>

        {needsAttentionCount > 0 && (
          <a
            href="/food/history/"
            className="mb-8 flex items-center gap-2 bg-bg-elevated border border-border rounded-[10px] px-4 py-3 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <span className="w-1.5 h-1.5 rounded-full bg-accent flex-shrink-0" />
            {needsAttentionCount} meal{needsAttentionCount === 1 ? '' : 's'} need{needsAttentionCount === 1 ? 's' : ''} attention
          </a>
        )}

        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          Log food
        </p>
        <div className="flex gap-2.5 mb-8">
          <a
            href="/food/upload/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <CameraIcon className="w-4 h-4 text-accent" />
            Photo
          </a>
          <a
            href="/food/manual/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <PencilIcon className="w-4 h-4 text-accent" />
            Manual
          </a>
          <a
            href="/food/history/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <HistoryIcon className="w-4 h-4 text-accent" />
            History
          </a>
        </div>

        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          More data
        </p>
        <div className="flex flex-wrap gap-2">
          {SECONDARY_TYPES.map(t => (
            <a
              key={t}
              href={`/data/${t}/`}
              className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide px-2.5 py-1.5 rounded-lg border border-border bg-bg-elevated hover:border-accent transition-colors flex items-center gap-1.5"
              style={{ color: metricColorVar(t) }}
            >
              <span className="w-1.5 h-1.5 rounded-full" style={{ background: metricColorVar(t) }} />
              {t.replace(/_/g, ' ')}
            </a>
          ))}
        </div>
      </main>
    </div>
  );
}
