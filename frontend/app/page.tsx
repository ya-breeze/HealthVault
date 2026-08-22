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

// Shared by the vitals and secondary-types presence fetches below so the two
// stay in lockstep if the lookback window ever changes.
function last7DaysRange() {
  const from = new Date();
  from.setDate(from.getDate() - 7);
  from.setUTCHours(0, 0, 0, 0);
  return { from: from.toISOString(), to: new Date().toISOString() };
}

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
  // False until the primary-metrics fetch below resolves. Gates the
  // no-data filter in the read-only grid: filtering on `vitals` before it's
  // populated would hide every card for one render, then pop them back in —
  // a visible flash on every load. See the render below.
  const [vitalsLoaded, setVitalsLoaded] = useState(false);
  // Types whose presence fetch failed (as opposed to succeeding with zero
  // rows). Kept separate from `vitals` so a transient fetch error can't be
  // mistaken for "no data" by the filter below.
  const [vitalsFailed, setVitalsFailed] = useState<Set<string>>(new Set());
  // Presence-only (no rendered value), so a plain type -> has-data map is
  // enough — unlike PRIMARY_METRICS, SECONDARY_TYPES only ever render as a
  // link pill, never a value/sparkline.
  const [secondaryHasData, setSecondaryHasData] = useState<Record<string, boolean>>({});
  const [secondaryLoaded, setSecondaryLoaded] = useState(false);
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
    const { from, to } = last7DaysRange();

    Promise.all(
      PRIMARY_METRICS.map(m =>
        api.data(m.type, from, to, undefined, 'day').then(
          rows => ({ ok: true as const, rows }),
          () => ({ ok: false as const, rows: [] as Record<string, unknown>[] }),
        ),
      )
    ).then(results => {
      const next: Record<string, VitalResult | null> = {};
      const failed = new Set<string>();
      PRIMARY_METRICS.forEach((m, i) => {
        const r = results[i];
        // A failed fetch is not evidence of "no data" — fail open (keep the
        // card in the grid, same as before this change) rather than mixing
        // errors into the no-data filter below and letting a transient
        // failure hide a card, or the whole grid, behind a false "No vitals
        // recorded yet" message. `vitals[type]` stays null either way, so
        // VitalCard renders its normal inline no-data placeholder for it.
        if (!r.ok) failed.add(m.type);
        next[m.type] = extractVital(m.type, r.rows);
      });
      setVitals(next);
      setVitalsFailed(failed);
      setVitalsLoaded(true);
    });
  }, [ready]);

  // Presence check for the "More Data" pills, mirroring the vitals fetch
  // above but unbucketed: these types are never rendered with an aggregated
  // value, only linked to, so raw rows are enough and — unlike the bucketed
  // endpoint — the same call works for food_meal too (bucket=day 400s for
  // that type; see backend/pkg/server/api.go's queryBucketed). Reuses the
  // vitals grid's 7-day window for consistency, even though that means a
  // type logged only further back still shows as "no data" here — see the
  // idea-5 investigation notes for the tradeoff.
  useEffect(() => {
    if (!ready) return;
    const { from, to } = last7DaysRange();

    Promise.all(
      SECONDARY_TYPES.map(type =>
        api.data(type, from, to).then(
          rows => ({ ok: true as const, rows }),
          () => ({ ok: false as const, rows: [] as Record<string, unknown>[] }),
        ),
      )
    ).then(results => {
      const next: Record<string, boolean> = {};
      SECONDARY_TYPES.forEach((type, i) => {
        const r = results[i];
        // Fail open on a failed fetch, same as the vitals grid above: a
        // request error is not evidence of "no data," so it must not cause a
        // pill to vanish for a type that actually has data, with no retry
        // available in this section.
        next[type] = !r.ok || r.rows.length > 0;
      });
      setSecondaryHasData(next);
      setSecondaryLoaded(true);
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
  // Until the fetch resolves, treat every card as having data so the no-data
  // filter below doesn't hide the whole grid for one render and then pop
  // cards back in once `vitals` populates.
  const hasVital = (type: string) => !vitalsLoaded || vitalsFailed.has(type) || vitals[type] != null;
  const visibleCards = order.filter(m => !m.hidden && hasVital(m.type));
  // Distinct from allHidden: this is "the user left cards visible, but none
  // of them have data yet" — Customize can't fix that, so it gets its own
  // message (dashboard.noVitalsData) rather than allCardsHidden's "tap
  // Customize" copy.
  const noVitalsData = !allHidden && vitalsLoaded && visibleCards.length === 0;
  const visibleSecondaryTypes = SECONDARY_TYPES.filter(type => !secondaryLoaded || secondaryHasData[type]);

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
        ) : noVitalsData && !editing ? (
          <p className="mb-8 text-sm text-text-muted bg-bg-elevated border border-border rounded-[10px] px-4 py-3" data-testid="vitals-grid-empty-no-data">
            {t('dashboard.noVitalsData')}
          </p>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-8" data-testid="vitals-grid">
            {order.map((m, i) => (editing || (!m.hidden && hasVital(m.type))) && (
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
                controlsDisabled={saving}
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

        {/* Hidden outright once loaded and empty, rather than shown with an
            empty-state message: unlike the vitals grid this section has no
            Customize escape hatch, so there's nothing actionable to tell the
            user — the pills simply reappear once a type has data. Renders
            unfiltered while secondaryLoaded is false, matching the vitals
            grid's same load-then-filter pattern above. */}
        {visibleSecondaryTypes.length > 0 && (
          <>
            <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
              {t('dashboard.moreData')}
            </p>
            <div className="flex flex-wrap gap-2" data-testid="more-data">
              {visibleSecondaryTypes.map(type => (
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
          </>
        )}
      </main>
    </div>
  );
}
