'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, DATA_TYPES } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { PRIMARY_METRICS, extractVital, reconcileMetricOrder, DashboardCardPref, VitalResult } from '@/lib/vitals';
import { useToast } from '@/components/Toast';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate, metricLabel, pluralForm } from '@/lib/i18n';
import { useLatest } from '@/lib/useLatest';
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
  const tRef = useLatest(t);
  const [ready, setReady] = useState(false);
  const [vitals, setVitals] = useState<Record<string, VitalResult | null>>({});
  const [needsAttentionCount, setNeedsAttentionCount] = useState(0);
  // 'loading' until the saved order/visibility arrives. The grid is not
  // rendered at all until then — `order` starts as every-card-visible
  // defaults, so rendering it early would flash cards the user has hidden
  // onto the screen on every single load, and leave them there permanently
  // if the GET fails. Found in code review.
  const [settingsStatus, setSettingsStatus] = useState<'loading' | 'loaded' | 'error'>('loading');
  const settingsLoaded = settingsStatus === 'loaded';
  // Bumped by the error placeholder's Try again control to re-run the load
  // effect. Without it a single transient 500 leaves the dashboard showing an
  // error paragraph instead of the vitals grid for the rest of the session,
  // with no in-page way back. Found in code review.
  const [settingsAttempt, setSettingsAttempt] = useState(0);
  const [order, setOrder] = useState<DashboardCardPref[]>(() => reconcileMetricOrder(undefined));
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.me()
      .then(() => setReady(true))
      .catch(() => router.push('/login'));
  }, [router]);

  useEffect(() => {
    if (!ready) return;
    setSettingsStatus('loading');
    api.getSettings()
      .then(s => {
        setOrder(reconcileMetricOrder(s.dashboard_order));
        setSettingsStatus('loaded');
      })
      .catch(() => {
        // Not 'loaded': editing/saving stays disabled rather than risking a
        // Done that overwrites real stored settings with a PUT built from the
        // (unknown) default order. The grid stays unrendered too — we cannot
        // tell which cards the user hid, and showing all of them would expose
        // exactly what they chose to hide.
        setSettingsStatus('error');
        showToast(tRef.current('dashboard.orderLoadFailed'), 'error');
      });
    // tRef, not t: the language selector sits in this page's own Header, and
    // this effect overwrites `order` with the stored order. Depending on `t`
    // meant switching language while reordering cards threw away the
    // in-progress arrangement — and because `editing` stays true, a
    // subsequent Done would persist the reverted order as if the user had
    // chosen it. Found in code review. See lib/useLatest.
  }, [ready, showToast, tRef, settingsAttempt]);

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

  const allHidden = order.every(m => m.hidden);

  function toggleHidden(index: number) {
    setOrder(prev => prev.map((m, i) => (i === index ? { ...m, hidden: !m.hidden } : m)));
  }

  async function handleDone() {
    setSaving(true);
    try {
      await updateSettings({ dashboard_order: order.map(m => ({ type: m.type, hidden: m.hidden })) });
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
              title={settingsLoaded ? undefined : t(settingsStatus === 'error' ? 'dashboard.orderLoadFailed' : 'dashboard.loadingOrder')}
              className="px-3 rounded-md border border-border text-text-muted hover:border-accent hover:text-accent transition-colors text-[11px] font-bold uppercase tracking-wide disabled:opacity-50"
            >
              {t('dashboard.customize')}
            </TapTarget>
          )}
        </div>
        {/* Edit mode renders every card, including hidden ones, so they can be
            found and shown again; the read-only grid renders only the visible
            ones. Move controls are indexed against the full `order` either way,
            so hiding a card never shifts what a neighbour's arrow does. */}
        {!settingsLoaded ? (
          <div
            className="mb-8 flex flex-wrap items-center justify-between gap-2 text-sm text-text-muted bg-bg-elevated border border-border rounded-[10px] px-4 py-3"
            data-testid={settingsStatus === 'error' ? 'vitals-grid-error' : 'vitals-grid-loading'}
          >
            <p>{t(settingsStatus === 'error' ? 'dashboard.orderLoadFailed' : 'dashboard.loadingOrder')}</p>
            {settingsStatus === 'error' && (
              <TapTarget
                onClick={() => setSettingsAttempt(n => n + 1)}
                data-testid="vitals-grid-retry"
                className="px-3 rounded-md border border-accent text-accent text-[11px] font-bold uppercase tracking-wide"
              >
                {t('dashboard.retryLoad')}
              </TapTarget>
            )}
          </div>
        ) : allHidden && !editing ? (
          <p className="mb-8 text-sm text-text-muted bg-bg-elevated border border-border rounded-[10px] px-4 py-3" data-testid="vitals-grid-empty">
            {t('dashboard.allCardsHidden')}
          </p>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-8" data-testid="vitals-grid">
            {order.map((m, i) => (editing || !m.hidden) && (
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
                hidden={m.hidden}
                onToggleHidden={() => toggleHidden(i)}
              />
            ))}
          </div>
        )}

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
