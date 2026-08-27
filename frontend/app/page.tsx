'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, DATA_TYPES, DataType } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { PRIMARY_METRICS, extractVital, reconcileMetricOrder, hasPresence, hasCardPresence, secondaryTypes, DashboardCardPref, VitalResult } from '@/lib/vitals';
import { useToast } from '@/components/Toast';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate, metricLabel, pluralForm } from '@/lib/i18n';
import { useLatest } from '@/lib/useLatest';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import VitalCard from '@/components/VitalCard';
import LoggingGapCard from '@/components/LoggingGapCard';
import TapTarget from '@/components/ui/TapTarget';
import { CameraIcon, PencilIcon, HistoryIcon, EyeIcon, EyeOffIcon } from '@/components/icons';

const SECONDARY_TYPES = secondaryTypes(DATA_TYPES);

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
  // Strict `=== true`, not `?? false`: settings is opaque, unvalidated JSON,
  // and a malformed stored value (e.g. the string "false", or 1) must read as
  // "not hidden" rather than being coerced into truthy-hidden. Mirrors
  // reconcileMetricOrder's `entry.hidden === true` check in lib/vitals.ts.
  const [moreDataHidden, setMoreDataHidden] = useState(false);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  // `null` means "not resolved yet" (initial state) *and* "fetch failed" —
  // both fail open via hasPresence's `!presence` branch, so no separate error
  // state is needed here the way settingsStatus needs one. `presenceReady`
  // is the loading-gate flag: false until the fetch settles (success or
  // failure), so the grid/More Data never flash unfiltered cards before
  // presence resolves.
  const [presence, setPresence] = useState<Record<string, boolean> | null>(null);
  const [presenceReady, setPresenceReady] = useState(false);

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
        setMoreDataHidden(s.more_data_hidden === true);
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

    // 'logging_gap' has no /api/data/{type} backing (design.md decision 8) —
    // it fetches and computes its own state (task 5's LoggingGapCard), so it's
    // excluded here rather than passed to api.data/extractVital, which are
    // DataType-only.
    const dataMetrics = PRIMARY_METRICS.filter((m): m is { type: DataType } => m.type !== 'logging_gap');
    Promise.all(
      dataMetrics.map(m => api.data(m.type, from, to, undefined, 'day').catch(() => []))
    ).then(results => {
      const next: Record<string, VitalResult | null> = {};
      dataMetrics.forEach((m, i) => {
        next[m.type] = extractVital(m.type, results[i]);
      });
      setVitals(next);
    });
  }, [ready]);

  useEffect(() => {
    if (!ready) return;
    api.dataTypesPresence()
      .then(p => setPresence(p))
      .catch(() => setPresence(null))
      .finally(() => setPresenceReady(true));
  }, [ready]);

  useEffect(() => {
    if (!ready) return;
    api.needsAttentionCount()
      .then(res => setNeedsAttentionCount(res.count))
      .catch(() => setNeedsAttentionCount(0));
  }, [ready]);

  // moveCard/toggleHidden take an index into the presence-filtered list
  // (what's actually rendered — see presentOrder below), not into `order`
  // itself, since zero-presence metrics are excluded from the customizable
  // set entirely and must not be reachable by these controls. Each resolves
  // the index to a type first, then updates that type's entry within the
  // full `order`, so absent-presence entries keep their stored position
  // untouched (they may gain presence on a later load).
  function moveCard(index: number, direction: -1 | 1) {
    setOrder(prev => {
      const visibleTypes = prev.filter(m => hasCardPresence(presence, m.type)).map(m => m.type);
      const target = index + direction;
      if (target < 0 || target >= visibleTypes.length) return prev;
      const idxA = prev.findIndex(m => m.type === visibleTypes[index]);
      const idxB = prev.findIndex(m => m.type === visibleTypes[target]);
      const next = [...prev];
      [next[idxA], next[idxB]] = [next[idxB], next[idxA]];
      return next;
    });
  }

  // Gates the vitals grid and Customize control on both the saved order and
  // presence resolving, so neither a flash of unfiltered cards (order without
  // presence-filtering) nor a flash of the pre-presence full set happens
  // before both are known.
  const dashboardReady = settingsLoaded && presenceReady;

  // Only metrics with presence are ever rendered (read-only grid or edit
  // mode) — see design.md, "Presence excludes a type from the customizable
  // set entirely". A zero-presence metric never appears here, regardless of
  // its stored `hidden` flag.
  const presentOrder = order.filter(m => hasCardPresence(presence, m.type));
  // "No data at all" (nothing to show, Customize can't help) is distinct from
  // "user hid everything that does have data" (allHidden, below) — see the
  // two-empty-states decision in design.md. allHidden must not fire when
  // there's nothing present to hide.
  const noPrimaryData = presentOrder.length === 0;
  const allHidden = presentOrder.length > 0 && presentOrder.every(m => m.hidden);
  // Gated on dashboardReady (not just presenceReady) so a section the user
  // hid can't flash unhidden before the saved more_data_hidden preference has
  // loaded — the same reasoning as the vitals grid's own dashboardReady gate.
  // See design.md, "More Data collapses, not renders empty".
  const presentSecondaryTypes = dashboardReady ? SECONDARY_TYPES.filter(type => hasPresence(presence, type)) : [];

  function toggleHidden(index: number) {
    setOrder(prev => {
      const visibleTypes = prev.filter(m => hasCardPresence(presence, m.type)).map(m => m.type);
      const type = visibleTypes[index];
      return prev.map(m => (m.type === type ? { ...m, hidden: !m.hidden } : m));
    });
  }

  function toggleMoreDataHidden() {
    setMoreDataHidden(prev => !prev);
  }

  async function handleDone() {
    setSaving(true);
    try {
      await updateSettings({
        dashboard_order: order.map(m => ({ type: m.type, hidden: m.hidden })),
        more_data_hidden: moreDataHidden,
      });
      setEditing(false);
    } catch {
      showToast(t('dashboard.orderSaveFailed'), 'error');
    } finally {
      setSaving(false);
    }
  }

  return (
    <AuthenticatedShell className="min-h-screen bg-bg">
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
              disabled={!dashboardReady}
              title={dashboardReady ? undefined : t(settingsStatus === 'error' ? 'dashboard.orderLoadFailed' : 'dashboard.loadingOrder')}
              className="px-3 rounded-md border border-border text-text-muted hover:border-accent hover:text-accent transition-colors text-[11px] font-bold uppercase tracking-wide disabled:opacity-50"
            >
              {t('dashboard.customize')}
            </TapTarget>
          )}
        </div>
        {/* Edit mode renders every present card, including hidden ones, so they
            can be found and shown again; the read-only grid renders only the
            visible ones. Move controls are indexed against the presence-filtered
            list — see the moveCard/toggleHidden comment above. */}
        {!dashboardReady ? (
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
        ) : noPrimaryData ? (
          <p className="mb-8 text-sm text-text-muted bg-bg-elevated border border-border rounded-[10px] px-4 py-3" data-testid="vitals-grid-empty-no-data">
            {t('dashboard.noPrimaryData')}
          </p>
        ) : allHidden && !editing ? (
          <p className="mb-8 text-sm text-text-muted bg-bg-elevated border border-border rounded-[10px] px-4 py-3" data-testid="vitals-grid-empty">
            {t('dashboard.allCardsHidden')}
          </p>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-8" data-testid="vitals-grid">
            {presentOrder.map((m, i) => (editing || !m.hidden) && (
              // 'logging_gap' has no /api/data/{type} presence or VitalCard
              // rendering — LoggingGapCard (task 5) owns its own fetch
              // lifecycle and content states instead of reading `vitals`.
              m.type === 'logging_gap' ? (
                <LoggingGapCard
                  key={m.type}
                  editing={editing}
                  onMoveUp={() => moveCard(i, -1)}
                  onMoveDown={() => moveCard(i, 1)}
                  moveUpDisabled={i === 0}
                  moveDownDisabled={i === presentOrder.length - 1}
                  hidden={m.hidden}
                  onToggleHidden={() => toggleHidden(i)}
                  controlsDisabled={saving}
                />
              ) : (
              <VitalCard
                key={m.type}
                type={m.type}
                label={metricLabel(t, m.type)}
                result={ready ? vitals[m.type] ?? null : null}
                editing={editing}
                onMoveUp={() => moveCard(i, -1)}
                onMoveDown={() => moveCard(i, 1)}
                moveUpDisabled={i === 0}
                moveDownDisabled={i === presentOrder.length - 1}
                hidden={m.hidden}
                onToggleHidden={() => toggleHidden(i)}
                controlsDisabled={saving}
              />
              )
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

        {/* Read-only: hidden when the user chose to hide it. Edit mode: always
            rendered (dimmed when hidden) so the section can be found and
            re-shown, mirroring the vitals grid's hidden-card treatment —
            except when there's nothing present to show at all, in which case
            neither mode renders the section or its toggle. */}
        {presentSecondaryTypes.length > 0 && (editing || !moreDataHidden) && (
          <div data-testid="more-data" data-hidden={moreDataHidden ? 'true' : 'false'}>
            <div className="flex items-center justify-between mb-3">
              <p className={`font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent${editing && moreDataHidden ? ' opacity-40' : ''}`}>
                {t('dashboard.moreData')}
              </p>
              {editing && (
                <TapTarget
                  onClick={toggleMoreDataHidden}
                  disabled={saving}
                  aria-label={t(moreDataHidden ? 'dashboard.showMoreData' : 'dashboard.hideMoreData')}
                  data-testid="more-data-visibility"
                  className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  {moreDataHidden ? <EyeOffIcon className="w-4 h-4" /> : <EyeIcon className="w-4 h-4" />}
                </TapTarget>
              )}
            </div>
            <div className={`flex flex-wrap gap-2${editing && moreDataHidden ? ' opacity-40' : ''}`}>
              {presentSecondaryTypes.map(type => (
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
          </div>
        )}
      </main>
    </AuthenticatedShell>
  );
}
