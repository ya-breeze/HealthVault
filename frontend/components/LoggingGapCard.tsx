'use client';
import { useEffect, useState } from 'react';
import { api, NutritionTargetUnmetError, NutritionTargetUnmetReason, DayCompletenessState } from '@/lib/api';
import { emaSeries, linearRegression, toDayOffset } from '@/lib/dataTypeMeta';
import { loggedDayKey } from '@/lib/loggedDay';
import {
  resolveLoggingGapWindow,
  rejectOutliers,
  checkHardFloor,
  computeLoggingGap,
  slopeStandardError,
  excludedOutlierCount,
  DayWindowData,
} from '@/lib/loggingGap';
import { useLanguage } from './LanguageContext';
import { interpolate } from '@/lib/i18n';
import TapTarget from './ui/TapTarget';
import { EyeIcon, EyeOffIcon } from './icons';

type ContentState =
  | { kind: 'loading' }
  | { kind: 'gap'; value: number; interval: number }
  | { kind: 'not_enough_data' }
  | { kind: 'nutrition_target_unmet'; reason: NutritionTargetUnmetReason }
  | { kind: 'retrieval_error' };

interface LoggingGapCardProps {
  /**
   * The caller's IANA timezone, from the settings blob `app/page.tsx` has
   * already loaded (it does not render this card until `settingsStatus ===
   * 'loaded'`). Passed down rather than re-fetched here: an own
   * `api.getSettings()` would duplicate a request the parent already made,
   * and — since the window boundary must be known before the four content
   * fetches can be issued — would put a serial round trip in front of them on
   * every dashboard load. `undefined` resolves the window in UTC, matching
   * ResolveTimezone's own fail-open default (food_completeness.go).
   */
  timezone?: string;
  editing?: boolean;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  moveUpDisabled?: boolean;
  moveDownDisabled?: boolean;
  hidden?: boolean;
  onToggleHidden?: () => void;
  controlsDisabled?: boolean;
}

// Day-buckets outlier-surviving raw weigh-ins by averaging same-day values —
// design.md decision 4's "outlier-filtered, day-bucketed weight series", run
// once the rejection walk (task 3.1) has finished, never partway through it.
function bucketByDay(records: { day: number; value: number }[]): { day: number; value: number }[] {
  const buckets = new Map<number, number[]>();
  for (const r of records) {
    const vals = buckets.get(r.day);
    if (vals) vals.push(r.value);
    else buckets.set(r.day, [r.value]);
  }
  return [...buckets.entries()]
    .sort(([a], [b]) => a - b)
    .map(([day, vals]) => ({ day, value: vals.reduce((a, v) => a + v, 0) / vals.length }));
}

// The profile/goal-weight settings location a "complete your profile" link
// should point at, per NutritionTargetUnmetReason (design.md decision 6).
// Goal weight is set from the weight detail page's Add Record form (see
// DataTypeClient.tsx), not the profile settings screen, so only that one
// reason routes there — every other reason is a profile-form field.
function unmetReasonHref(reason: NutritionTargetUnmetReason): string {
  return reason === 'missing_goal_weight' ? '/data/weight/' : '/settings/';
}

/**
 * A Food Card (dashboard-ui, design.md decision 8): computes and renders the
 * Logging Gap — see docs/specs/logging-gap.md. Unlike VitalCard, this card
 * has no `/api/data/{type}` presence signal and owns its own fetch lifecycle
 * (weight, Nutrition Target, Day Completeness, Daily Totals), so its
 * loading/error states are entirely local. The window boundary's timezone is
 * not a fifth request — it comes in as a prop from the parent's settings
 * load; see `LoggingGapCardProps.timezone`.
 */
export default function LoggingGapCard({
  timezone, editing, onMoveUp, onMoveDown, moveUpDisabled, moveDownDisabled,
  hidden, onToggleHidden, controlsDisabled,
}: LoggingGapCardProps) {
  const { t } = useLanguage();
  const [state, setState] = useState<ContentState>({ kind: 'loading' });
  const [outlierExcluded, setOutlierExcluded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setState({ kind: 'loading' });
    setOutlierExcluded(false);

    (async () => {
      // The timezone prop drives resolveLoggingGapWindow's "yesterday"
      // boundary the same way food-day-completeness resolves the caller's
      // Logged Day. It arrives with the parent's own settings load, so there
      // is no settings failure state to handle here — a settings error keeps
      // the dashboard grid (and therefore this card) unrendered entirely.
      const gapWindow = resolveLoggingGapWindow(new Date(), timezone);
      const now = new Date();

      let weightRaw: Record<string, unknown>[];
      let nutritionTargetCalories: number;
      let completeness: { date: string; state: DayCompletenessState }[];
      let dailyTotals: { date: string; calories: number; unconfirmed_meals: number }[];
      try {
        const [w, nt, c, d] = await Promise.all([
          api.data('weight', gapWindow.leadInFetchFromUTC, now.toISOString()),
          api.getNutritionTarget(),
          api.getCompleteness(gapWindow.windowStart, gapWindow.windowEnd),
          api.getFoodDailyTotals(gapWindow.windowStart, gapWindow.windowEnd),
        ]);
        weightRaw = w;
        nutritionTargetCalories = nt.calories;
        completeness = c;
        dailyTotals = d;
      } catch (err) {
        if (cancelled) return;
        if (err instanceof NutritionTargetUnmetError) {
          setState({ kind: 'nutrition_target_unmet', reason: err.reason });
        } else {
          setState({ kind: 'retrieval_error' });
        }
        return;
      }
      if (cancelled) return;

      // Raw, un-bucketed weigh-ins keyed by Logged Day offset (task 3.1
      // operates on raw records, never bucketed ones) — the in-progress
      // "today" and any later reading is excluded here rather than at the
      // fetch boundary, since a `to` of "now" is the only fetch cutoff that
      // is safe for every timezone (see resolveLoggingGapWindow's own doc
      // comment on why the window itself ends at "yesterday"). The lower
      // bound is filtered client-side too, since `leadInFetchFromUTC`
      // deliberately over-fetches by a day to stay safe across timezones.
      const rawRecords = weightRaw
        .map(r => ({
          day: toDayOffset(loggedDayKey(new Date(String(r.time)), timezone)),
          value: Number(r.kilograms),
        }))
        .filter(r => r.day >= gapWindow.leadInStartDayOffset && r.day <= gapWindow.windowLastDayOffset);

      const { kept, rejected, bootstrapSiblingAmbiguous } = rejectOutliers(rawRecords);

      const perDayWindowData: Record<number, DayWindowData> = {};
      const completenessByDate = new Map(completeness.map(c => [c.date, c.state]));
      for (const total of dailyTotals) {
        perDayWindowData[toDayOffset(total.date)] = {
          state: completenessByDate.get(total.date) ?? 'incomplete',
          calories: total.calories,
          unconfirmedMeals: total.unconfirmed_meals,
        };
      }

      const keptInWindow = kept.filter(r => r.day >= gapWindow.windowStartDayOffset);
      const rejectedInWindow = rejected.filter(r => r.day >= gapWindow.windowStartDayOffset);
      const mostRecentKeptDayOffset = kept.length > 0 ? Math.max(...kept.map(r => r.day)) : -Infinity;

      const hardFloor = checkHardFloor(
        keptInWindow,
        rejectedInWindow,
        bootstrapSiblingAmbiguous,
        gapWindow.windowStartDayOffset,
        perDayWindowData,
        mostRecentKeptDayOffset,
        gapWindow.windowLastDayOffset,
      );

      let result: ContentState;
      if (hardFloor) {
        result = { kind: 'not_enough_data' };
      } else {
        const bucketed = bucketByDay(kept);
        const ema = emaSeries(bucketed.map(b => b.value), 0.25);
        const points = bucketed
          .map((b, i) => ({ x: b.day, y: ema[i] }))
          .filter(p => p.x >= gapWindow.windowStartDayOffset && p.x <= gapWindow.windowLastDayOffset);
        const { slope, intercept } = linearRegression(points);
        const se = slopeStandardError(points, slope, intercept);
        const gap = computeLoggingGap(
          { slope, intercept },
          se,
          nutritionTargetCalories,
          perDayWindowData,
          gapWindow.windowStartDayOffset,
          gapWindow.windowLastDayOffset,
        );
        result = gap.kind === 'gap' ? { kind: 'gap', value: gap.value, interval: gap.interval } : { kind: 'not_enough_data' };
      }

      if (cancelled) return;
      setState(result);
      setOutlierExcluded(excludedOutlierCount(rejected, gapWindow.windowStartDayOffset, gapWindow.windowLastDayOffset) > 0);
    })();

    return () => {
      cancelled = true;
    };
  }, [timezone]);

  const dim = editing && hidden ? ' opacity-40' : '';

  function renderContent() {
    switch (state.kind) {
      case 'loading':
        return <p className={`text-sm text-text-muted py-2${dim}`} data-testid="logging-gap-loading">{t('loggingGap.loading')}</p>;
      case 'not_enough_data':
        return <p className={`text-sm text-text-muted py-2${dim}`} data-testid="logging-gap-not-enough-data">{t('loggingGap.notEnoughData')}</p>;
      case 'retrieval_error':
        return <p className={`text-sm text-text-muted py-2${dim}`} data-testid="logging-gap-error">{t('loggingGap.retrievalError')}</p>;
      case 'nutrition_target_unmet':
        return (
          <p className={`text-sm text-text-muted py-2${dim}`} data-testid="logging-gap-target-unmet">
            {t('loggingGap.targetUnmet')}{' '}
            <a href={unmetReasonHref(state.reason)} className="text-accent underline">
              {t('loggingGap.targetUnmetLink')}
            </a>
          </p>
        );
      case 'gap': {
        // Never a negative-to-negative range (spec's "Logging Gap Card
        // content and placement" requirement): a negative value means Mean
        // Logged Intake exceeds Implied Intake, rendered with the absolute
        // range and direction-aware copy instead.
        const unlogged = state.value >= 0;
        const lower = Math.round(Math.abs(state.value) - state.interval);
        const upper = Math.round(Math.abs(state.value) + state.interval);
        const range = `${lower}–${upper}`;
        return (
          <div className={`py-2${dim}`} data-testid="logging-gap-value">
            <div className="font-[family-name:var(--font-data)] text-xl font-bold tabular-nums">
              {interpolate(t(unlogged ? 'loggingGap.unlogged' : 'loggingGap.loggedMore'), { range })}
            </div>
          </div>
        );
      }
    }
  }

  const inner = (
    <>
      <div className={`flex items-center justify-between gap-1 mb-2${dim}`}>
        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide flex items-center gap-1.5 text-accent">
          <span className="w-1.5 h-1.5 rounded-full bg-accent" />
          {t('loggingGap.title')}
        </p>
      </div>
      {editing && (
        <div className="flex flex-wrap items-center justify-end gap-1.5 mb-2">
          <TapTarget
            onClick={onMoveUp}
            disabled={moveUpDisabled || controlsDisabled}
            aria-label={interpolate(t('vitals.moveUp'), { metric: t('loggingGap.title') })}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↑
          </TapTarget>
          <TapTarget
            onClick={onMoveDown}
            disabled={moveDownDisabled || controlsDisabled}
            aria-label={interpolate(t('vitals.moveDown'), { metric: t('loggingGap.title') })}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↓
          </TapTarget>
          <TapTarget
            onClick={onToggleHidden}
            disabled={controlsDisabled}
            aria-label={interpolate(t(hidden ? 'dashboard.showCard' : 'dashboard.hideCard'), { metric: t('loggingGap.title') })}
            data-testid="logging-gap-card-visibility"
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            {hidden ? <EyeOffIcon className="w-4 h-4" /> : <EyeIcon className="w-4 h-4" />}
          </TapTarget>
        </div>
      )}
      {renderContent()}
      {outlierExcluded && (
        <p className={`text-xs text-text-muted mt-1${dim}`} data-testid="logging-gap-outlier-note">
          {t('loggingGap.outlierNote')}
        </p>
      )}
      <div className={`text-xs text-text-muted mt-2 space-y-1${dim}`}>
        <p>{t('loggingGap.caveatPhoto')}</p>
        <p>{t('loggingGap.caveatActivity')}</p>
      </div>
    </>
  );

  return (
    <div
      className="col-span-2 sm:col-span-4 block bg-bg-elevated border border-border rounded-[10px] px-3.5 py-3"
      data-testid="logging-gap-card"
      data-hidden={editing ? (hidden ? 'true' : 'false') : undefined}
    >
      {inner}
    </div>
  );
}
