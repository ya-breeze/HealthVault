'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, DATA_TYPES } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { PRIMARY_METRICS, extractVital, reconcileMetricOrder, VitalResult } from '@/lib/vitals';
import { useToast } from '@/components/Toast';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate, metricLabel, pluralForm } from '@/lib/i18n';
import Header from '@/components/Header';
import VitalCard from '@/components/VitalCard';
import TapTarget from '@/components/ui/TapTarget';
import { CameraIcon, PencilIcon, HistoryIcon } from '@/components/icons';

const SECONDARY_TYPES = DATA_TYPES.filter(t => !PRIMARY_METRICS.some(m => m.type === t));

export default function Dashboard() {
  const router = useRouter();
  const { showToast } = useToast();
  // updateSettings queues this screen's dashboard_order PUT behind
  // LanguageContext's own claim() — see that provider's doc comment. Both
  // this screen and the language switcher write to the same UserSettings
  // blob; without a shared queue, saving a reorder and then switching
  // language (or vice versa) before the first PUT lands can silently
  // clobber whichever save loses the race — found in code review.
  const { updateSettings, t, language } = useLanguage();
  const [ready, setReady] = useState(false);
  const [vitals, setVitals] = useState<Record<string, VitalResult | null>>({});
  const [needsAttentionCount, setNeedsAttentionCount] = useState(0);
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
        setOrder(reconcileMetricOrder(s.dashboard_order));
        setSettingsLoaded(true);
      })
      .catch(() => {
        // Leave settingsLoaded false: editing/saving stays disabled rather
        // than risking a Done that overwrites real stored settings with a
        // PUT built from the (unknown) default order.
        showToast(t('dashboard.orderLoadFailed'), 'error');
      });
  }, [ready, showToast, t]);

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
      await updateSettings({ dashboard_order: order.map(m => m.type) });
      setEditing(false);
    } catch {
      showToast(t('dashboard.orderSaveFailed'), 'error');
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
            {t('dashboard.vitalsHeading')}
          </p>
          {editing ? (
            <TapTarget
              onClick={handleDone}
              disabled={saving}
              className="px-3 rounded-md border border-accent text-accent text-[11px] font-bold uppercase tracking-wide disabled:opacity-50"
            >
              {saving ? t('dashboard.saving') : t('dashboard.done')}
            </TapTarget>
          ) : (
            <TapTarget
              onClick={() => setEditing(true)}
              disabled={!settingsLoaded}
              title={settingsLoaded ? undefined : t('dashboard.loadingOrder')}
              className="px-3 rounded-md border border-border text-text-muted hover:border-accent hover:text-accent transition-colors text-[11px] font-bold uppercase tracking-wide disabled:opacity-50"
            >
              {t('dashboard.editOrder')}
            </TapTarget>
          )}
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-8" data-testid="vitals-grid">
          {order.map((m, i) => (
            <VitalCard
              key={m.type}
              type={m.type}
              label={metricLabel(t, m.type)}
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
            {interpolate(
              pluralForm(language, needsAttentionCount, {
                one: t('dashboard.needsAttention.one'),
                few: t('dashboard.needsAttention.few'),
                many: t('dashboard.needsAttention.many'),
                other: t('dashboard.needsAttention.other'),
              }),
              { count: needsAttentionCount },
            )}
          </a>
        )}

        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          {t('dashboard.logFood')}
        </p>
        <div className="flex gap-2.5 mb-8">
          <a
            href="/food/upload/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <CameraIcon className="w-4 h-4 text-accent" />
            {t('dashboard.photo')}
          </a>
          <a
            href="/food/manual/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <PencilIcon className="w-4 h-4 text-accent" />
            {t('dashboard.manual')}
          </a>
          <a
            href="/food/history/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <HistoryIcon className="w-4 h-4 text-accent" />
            {t('dashboard.history')}
          </a>
        </div>

        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          {t('dashboard.moreData')}
        </p>
        <div className="flex flex-wrap gap-2">
          {SECONDARY_TYPES.map(type => (
            <a
              key={type}
              href={`/data/${type}/`}
              className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide px-2.5 py-1.5 rounded-lg border border-border bg-bg-elevated hover:border-accent transition-colors flex items-center gap-1.5"
              style={{ color: metricColorVar(type) }}
            >
              <span className="w-1.5 h-1.5 rounded-full" style={{ background: metricColorVar(type) }} />
              {metricLabel(t, type)}
            </a>
          ))}
        </div>
      </main>
    </div>
  );
}
