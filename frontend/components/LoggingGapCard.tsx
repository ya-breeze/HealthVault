'use client';
import { useEffect, useState } from 'react';
import { api, NutritionTargetUnmetReason, DayCompletenessState, TodaySummary } from '@/lib/api';
import { emaSeries, linearRegression, toDayOffset } from '@/lib/dataTypeMeta';
import { loggedDayKey } from '@/lib/loggedDay';
import {
  resolveLoggingGapWindow,
  rejectOutliers,
  bucketByDay,
  checkHardFloor,
  computeLoggingGap,
  slopeStandardError,
  excludedOutlierCount,
  DayWindowData,
  LoggingGapResult,
} from '@/lib/loggingGap';
import { useLanguage } from './LanguageContext';
import { interpolate } from '@/lib/i18n';
import TapTarget from './ui/TapTarget';
import { EyeIcon, EyeOffIcon } from './icons';

// Today's intake beside the target it is measured against — the card's top
// row. Both halves are carried together because neither is meaningful alone:
// a consumed figure with no target is a number with no scale.
interface TodayRow {
  calories: number;
  proteinGrams: number;
  carbsGrams: number;
  fatGrams: number;
  targetCalories: number;
  targetProteinGrams: number;
  targetCarbsGrams: number;
  targetFatGrams: number;
}

// What the bottom row can say: the three outcomes of the gap computation,
// plus a failure of the three requests that feed it — which is a gap-line
// state rather than a card-level one, since the top row is still perfectly
// renderable when only those three failed.
type GapLine = LoggingGapResult | { kind: 'retrieval_error' };

// `loading`, `retrieval_error` and `nutrition_target_unmet` are whole-card
// states: each means there is no top row *and* no gap line to draw. Only
// `ready` splits, because once the fetches land the two rows have genuinely
// independent outcomes — a card can show today's intake perfectly well while
// the 28-day gap stays unresolvable, which is the normal state of a user in
// their first weeks.
type ContentState =
  | { kind: 'loading' }
  | { kind: 'ready'; today: TodayRow; gap: GapLine }
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

    // Everything below runs inside one try/catch, not just the fetches: the
    // date arithmetic and the whole computation chain can throw too, and an
    // uncaught throw in an async IIFE is an unhandled rejection that leaves
    // the card stuck on "Calculating…" forever with no way out. The concrete
    // case is a bad IANA timezone (a corrupted settings blob, say) or an
    // unparseable record timestamp: `Intl.DateTimeFormat.format` raises
    // RangeError on an Invalid Date, and `loggedDayKey` raises it from inside
    // *both* its try and its own catch, so nothing downstream of it can
    // recover. "Temporarily unavailable" is the honest state for all of it —
    // we have no gap to show and no reason to claim the data is missing.
    (async () => {
      try {
        // The timezone prop drives resolveLoggingGapWindow's "yesterday"
        // boundary the same way food-day-completeness resolves the caller's
        // Logged Day. It arrives with the parent's own settings load, so there
        // is no settings failure state to handle here — a settings error keeps
        // the dashboard grid (and therefore this card) unrendered entirely.
        const gapWindow = resolveLoggingGapWindow(new Date(), timezone);
        const now = new Date();

        // `getTodaySummary`, not `getNutritionTarget`: it carries today's
        // consumed macros alongside the same target, so the top row costs this
        // card no additional request. Still four fetches, not five.
        //
        // `allSettled`, not `all`: the four requests feed two rows with
        // different appetites for failure. The summary feeds both — without a
        // target there is neither a top row nor an Implied Intake — so its
        // failure is the card's. The other three feed only the 28-day gap, and
        // losing today's calories because a 58-day weight history 500'd would
        // throw away the row this card leads with. They still go out in
        // parallel; only how their failures are read differs.
        const [weightR, summaryR, completenessR, dailyTotalsR] = await Promise.allSettled([
          api.data('weight', gapWindow.leadInFetchFromUTC, now.toISOString()),
          api.getTodaySummary(),
          api.getCompleteness(gapWindow.windowStart, gapWindow.windowEnd),
          api.getFoodDailyTotals(gapWindow.windowStart, gapWindow.windowEnd),
        ]);
        if (cancelled) return;

        if (summaryR.status === 'rejected') {
          setState({ kind: 'retrieval_error' });
          return;
        }
        const summary: TodaySummary = summaryR.value;

        // An unavailable target ends the card rather than degrading it: the
        // top row has nothing to measure today's intake against, and the gap
        // has no Implied Intake to derive. Same "complete your profile"
        // state as before, now read from a field instead of caught — there is
        // no 422 on /summary/today, which reports this as ordinary data.
        const target = summary.target;
        if (!target.available) {
          setState({ kind: 'nutrition_target_unmet', reason: target.reason });
          return;
        }
        const nutritionTargetCalories = target.calories;

        const todayRow: TodayRow = {
          calories: summary.calories_consumed,
          proteinGrams: summary.protein_grams_consumed,
          carbsGrams: summary.carbs_grams_consumed,
          fatGrams: summary.fat_grams_consumed,
          targetCalories: target.calories,
          targetProteinGrams: target.protein_grams,
          targetCarbsGrams: target.carbs_grams,
          targetFatGrams: target.fat_grams,
        };

        // All three or none: the gap is a single computation over the three
        // together, so a partial set cannot produce a weaker answer, only a
        // wrong one — a missing daily-totals response would read as "no valid
        // days" and a missing weight response as "no weigh-ins", both of which
        // render as "not enough data yet" and blame the user's logging for an
        // outage.
        if (
          weightR.status === 'rejected' ||
          completenessR.status === 'rejected' ||
          dailyTotalsR.status === 'rejected'
        ) {
          setState({ kind: 'ready', today: todayRow, gap: { kind: 'retrieval_error' } });
          return;
        }
        const weightRaw = weightR.value;
        const completeness = completenessR.value;
        const dailyTotals = dailyTotalsR.value;

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

        let gapResult: LoggingGapResult;
        if (hardFloor) {
          gapResult = { kind: 'not_enough_data' };
        } else {
          const bucketed = bucketByDay(kept);
          const ema = emaSeries(bucketed.map(b => b.value), 0.25);
          const points = bucketed
            .map((b, i) => ({ x: b.day, y: ema[i] }))
            .filter(p => p.x >= gapWindow.windowStartDayOffset && p.x <= gapWindow.windowLastDayOffset);
          const { slope, intercept } = linearRegression(points);
          const se = slopeStandardError(points, slope, intercept);
          gapResult = computeLoggingGap(
            { slope, intercept },
            se,
            nutritionTargetCalories,
            perDayWindowData,
            gapWindow.windowStartDayOffset,
            gapWindow.windowLastDayOffset,
          );
        }

        if (cancelled) return;
        setState({ kind: 'ready', today: todayRow, gap: gapResult });
        setOutlierExcluded(excludedOutlierCount(rejected, gapWindow.windowStartDayOffset, gapWindow.windowLastDayOffset) > 0);
      } catch {
        if (cancelled) return;
        setState({ kind: 'retrieval_error' });
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [timezone]);

  const dim = editing && hidden ? ' opacity-40' : '';

  function renderToday(today: TodayRow) {
    // Clamped so an over-target day fills the bar rather than overflowing its
    // container; the numbers beside it keep counting past 100%, which is where
    // that information belongs. Going over target is not an error and is not
    // coloured as one. The guard against a non-positive target is defensive —
    // the backend never emits one — but a zero would otherwise divide to
    // Infinity and produce a `width: Infinity%` style.
    const pct =
      today.targetCalories > 0
        ? Math.min(100, Math.max(0, (today.calories / today.targetCalories) * 100))
        : 0;
    // Cast-free, the way metricLabel builds its keys: the template literal
    // expands to the union of the three `loggingGap.` macro keys, so dropping
    // one from the dictionary is a type error here rather than a raw key
    // rendered on the dashboard.
    const macro = (labelKey: 'protein' | 'carbs' | 'fat', consumed: number, target: number) =>
      interpolate(t(`loggingGap.${labelKey}`), {
        consumed: String(Math.round(consumed)),
        target: String(Math.round(target)),
      });
    return (
      <div className={`py-1${dim}`} data-testid="nutrition-today">
        <div
          className="font-[family-name:var(--font-data)] text-xl font-bold tabular-nums"
          data-testid="nutrition-today-calories"
        >
          {interpolate(t('loggingGap.todayCalories'), {
            consumed: String(Math.round(today.calories)),
            target: String(Math.round(today.targetCalories)),
          })}
        </div>
        {/* Presentational: the calorie line above is the accessible content,
            and a bar that repeated it would only add a second thing to read. */}
        <div className="mt-1.5 h-1.5 w-full rounded-full bg-border overflow-hidden" aria-hidden="true">
          <div className="h-full rounded-full bg-accent" style={{ width: `${pct}%` }} />
        </div>
        <div
          className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-text-muted tabular-nums"
          data-testid="nutrition-today-macros"
        >
          <span>{macro('protein', today.proteinGrams, today.targetProteinGrams)}</span>
          <span>{macro('carbs', today.carbsGrams, today.targetCarbsGrams)}</span>
          <span>{macro('fat', today.fatGrams, today.targetFatGrams)}</span>
        </div>
      </div>
    );
  }

  function renderGap(gap: GapLine) {
    switch (gap.kind) {
      case 'retrieval_error':
        // Reuses the card-level wording and testid: from the reader's side
        // this is the same event, just confined to one row.
        return (
          <p className="text-sm text-text-muted" data-testid="logging-gap-error">
            {t('loggingGap.retrievalError')}
          </p>
        );
      case 'not_enough_data':
        return (
          <p className="text-sm text-text-muted" data-testid="logging-gap-not-enough-data">
            {t('loggingGap.notEnoughData')}
          </p>
        );
      case 'on_track':
        // Deliberately not "all good": the silence rule this state comes from
        // compares the log against the weight trend and checks nothing else —
        // not whether weight moves toward the goal, not whether intake is
        // sane. The praise is real but scoped to what was actually measured.
        return (
          <div data-testid="logging-gap-on-track">
            <p className="text-sm font-medium text-text">
              <span aria-hidden="true">✓ </span>
              {t('loggingGap.onTrack')}
            </p>
            <p className="text-xs text-text-muted mt-0.5">{t('loggingGap.onTrackDetail')}</p>
          </div>
        );
      case 'gap': {
        // Never a negative-to-negative range (spec's "Logging Gap Card
        // content and placement" requirement): a negative value means Mean
        // Logged Intake exceeds Implied Intake, rendered with the absolute
        // range and direction-aware copy instead.
        const unlogged = gap.value >= 0;
        const lower = Math.round(Math.abs(gap.value) - gap.interval);
        const upper = Math.round(Math.abs(gap.value) + gap.interval);
        const range = `${lower}–${upper}`;
        return (
          <p className="text-sm font-medium text-text" data-testid="logging-gap-value">
            {interpolate(t(unlogged ? 'loggingGap.unlogged' : 'loggingGap.loggedMore'), { range })}
          </p>
        );
      }
    }
  }

  function renderContent() {
    switch (state.kind) {
      case 'loading':
        return <p className={`text-sm text-text-muted py-2${dim}`} data-testid="logging-gap-loading">{t('loggingGap.loading')}</p>;
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
      case 'ready':
        return (
          <>
            {renderToday(state.today)}
            <div className={`mt-2 pt-2 border-t border-border${dim}`}>{renderGap(state.gap)}</div>
          </>
        );
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
