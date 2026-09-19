'use client';

import { useEffect, useState, type ReactNode } from 'react';
import Link from 'next/link';
import type { Dictionary, LanguageCode } from '@/lib/i18n';
import { interpolate, pluralForm } from '@/lib/i18n';
import { api } from '@/lib/api';
import {
  FOOD_LOG_HISTORY_WINDOW_DAYS,
  OUTCOME_COLOR,
  addDays,
  resolveFoodLogHistoryWindow,
  summarizeFoodLogHistory,
  type FoodLogHistoryDay,
  type FoodLogHistoryOutcome,
  type FoodLogHistorySummary,
} from '@/lib/foodLogHistory';
import { useLanguage } from './LanguageContext';
import TapTarget from './ui/TapTarget';
import { EyeIcon, EyeOffIcon } from './icons';

interface FoodLogHistoryCardProps {
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

type ContentState =
  | { kind: 'loading' }
  | { kind: 'ready'; summary: FoodLogHistorySummary }
  | { kind: 'retrieval_error' };

function expectedDates(from: string): string[] {
  return Array.from({ length: FOOD_LOG_HISTORY_WINDOW_DAYS }, (_, i) => addDays(from, i));
}

const OUTCOME_DISPLAY: Record<FoodLogHistoryOutcome, { labelKey: keyof Dictionary; symbol: string }> = {
  counted: { labelKey: 'foodLogHistory.outcome.counted', symbol: '✓' },
  no_food: { labelKey: 'foodLogHistory.outcome.noFood', symbol: '∅' },
  needs_attention: { labelKey: 'foodLogHistory.outcome.needsAttention', symbol: '!' },
};

function weekdayLabel(date: string, language: LanguageCode): string {
  return new Intl.DateTimeFormat(language === 'ru' ? 'ru' : undefined, { weekday: 'short', timeZone: 'UTC' }).format(
    new Date(`${date}T12:00:00.000Z`),
  );
}

function SummaryCount({ testId, outcome, children }: { testId: string; outcome: FoodLogHistoryOutcome; children: ReactNode }) {
  return (
    <span className="flex items-center gap-1.5" data-testid={testId}>
      <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: OUTCOME_COLOR[outcome] }} aria-hidden="true" />
      {children}
    </span>
  );
}

/**
 * Food Card for the seven completed Logged Days. The two range requests are
 * intentionally owned by this card: the dashboard already supplies the
 * stored timezone, and a malformed or partial join is unavailable rather
 * than evidence that a day had no food.
 */
export default function FoodLogHistoryCard({
  timezone,
  editing,
  onMoveUp,
  onMoveDown,
  moveUpDisabled,
  moveDownDisabled,
  hidden,
  onToggleHidden,
  controlsDisabled,
}: FoodLogHistoryCardProps) {
  const { t, language } = useLanguage();
  const [state, setState] = useState<ContentState>({ kind: 'loading' });
  const dim = editing && hidden ? ' opacity-40' : '';

  useEffect(() => {
    let cancelled = false;
    setState({ kind: 'loading' });

    (async () => {
      try {
        const window = resolveFoodLogHistoryWindow(new Date(), timezone);
        const dates = expectedDates(window.from);
        const [completeness, dailyTotals] = await Promise.all([
          api.getCompleteness(window.from, window.to),
          api.getFoodDailyTotals(window.from, window.to),
        ]);
        const summary = summarizeFoodLogHistory(completeness, dailyTotals, dates);
        if (!cancelled) setState({ kind: 'ready', summary });
      } catch {
        if (!cancelled) setState({ kind: 'retrieval_error' });
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [timezone]);

  function dayDescription(day: FoodLogHistoryDay): string {
    const outcome = t(OUTCOME_DISPLAY[day.outcome].labelKey);
    if (day.unconfirmedMeals === 0) {
      return interpolate(t('foodLogHistory.dayDescription'), {
        date: day.date,
        outcome,
        occasions: day.occasionCount,
      });
    }
    const template = pluralForm(language, day.unconfirmedMeals, {
      one: t('foodLogHistory.dayDescriptionWithUnconfirmed.one'),
      few: t('foodLogHistory.dayDescriptionWithUnconfirmed.few'),
      many: t('foodLogHistory.dayDescriptionWithUnconfirmed.many'),
      other: t('foodLogHistory.dayDescriptionWithUnconfirmed.other'),
    });
    return interpolate(template, {
      date: day.date,
      outcome,
      occasions: day.occasionCount,
      unconfirmed: day.unconfirmedMeals,
    });
  }

  function renderDay(day: FoodLogHistoryDay) {
    const color = OUTCOME_COLOR[day.outcome];
    return (
      <div
        key={day.date}
        className="flex min-w-0 flex-1 flex-col items-center gap-1 rounded-md border px-1.5 py-2 text-center"
        style={{
          borderColor: color,
          backgroundColor: `color-mix(in srgb, ${color} 16%, transparent)`,
        }}
        data-testid={`food-log-history-day-${day.date}`}
        data-outcome={day.outcome}
        role="img"
        aria-label={dayDescription(day)}
        title={dayDescription(day)}
      >
        <span className="text-[11px] text-text-muted">{weekdayLabel(day.date, language)}</span>
        <span className="font-[family-name:var(--font-data)] text-lg font-bold" style={{ color }} aria-hidden="true">
          {OUTCOME_DISPLAY[day.outcome].symbol}
        </span>
      </div>
    );
  }

  function renderContent() {
    switch (state.kind) {
      case 'loading':
        return <p className={`text-sm text-text-muted py-2${dim}`} data-testid="food-log-history-loading">{t('foodLogHistory.loading')}</p>;
      case 'retrieval_error':
        return <p className={`text-sm text-text-muted py-2${dim}`} data-testid="food-log-history-error">{t('foodLogHistory.retrievalError')}</p>;
      case 'ready':
        return (
          <>
            <div className={`grid grid-cols-3 gap-x-3 gap-y-1 text-xs tabular-nums${dim}`} data-testid="food-log-history-summary">
              <SummaryCount testId="food-log-history-counted" outcome="counted">{interpolate(t('foodLogHistory.countedDays'), { count: state.summary.countedDays })}</SummaryCount>
              <SummaryCount testId="food-log-history-no-food" outcome="no_food">{interpolate(t('foodLogHistory.noFoodDays'), { count: state.summary.noFoodDays })}</SummaryCount>
              <SummaryCount testId="food-log-history-needs-attention" outcome="needs_attention">{interpolate(t('foodLogHistory.needsAttentionDays'), { count: state.summary.needsAttentionDays })}</SummaryCount>
            </div>
            <div className={`mt-3 flex gap-1.5${dim}`} data-testid="food-log-history-strip">
              {state.summary.days.map(renderDay)}
            </div>
          </>
        );
    }
  }

  const inner = (
    <>
      <div className={`flex items-center justify-between gap-1 mb-2${dim}`}>
        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide flex items-center gap-1.5 text-accent">
          <span className="w-1.5 h-1.5 rounded-full bg-accent" />
          {t('foodLogHistory.title')}
        </p>
      </div>
      {editing && (
        <div className="flex flex-wrap items-center justify-end gap-1.5 mb-2">
          <TapTarget
            onClick={onMoveUp}
            disabled={moveUpDisabled || controlsDisabled}
            aria-label={interpolate(t('vitals.moveUp'), { metric: t('foodLogHistory.title') })}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↑
          </TapTarget>
          <TapTarget
            onClick={onMoveDown}
            disabled={moveDownDisabled || controlsDisabled}
            aria-label={interpolate(t('vitals.moveDown'), { metric: t('foodLogHistory.title') })}
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ↓
          </TapTarget>
          <TapTarget
            onClick={onToggleHidden}
            disabled={controlsDisabled}
            aria-label={interpolate(t(hidden ? 'dashboard.showCard' : 'dashboard.hideCard'), { metric: t('foodLogHistory.title') })}
            data-testid="food-log-history-card-visibility"
            className="flex items-center justify-center rounded-md border border-border bg-bg text-text disabled:opacity-30 disabled:cursor-not-allowed"
          >
            {hidden ? <EyeOffIcon className="w-4 h-4" /> : <EyeIcon className="w-4 h-4" />}
          </TapTarget>
        </div>
      )}
      {renderContent()}
    </>
  );

  const commonProps = {
    className: 'col-span-2 sm:col-span-4 block bg-bg-elevated border border-border rounded-[10px] px-3.5 py-3',
    'data-testid': 'food-log-history-card',
    'data-hidden': editing ? (hidden ? 'true' : 'false') : undefined,
  };

  if (editing) return <div {...commonProps}>{inner}</div>;
  return <Link href="/food/history/" {...commonProps}>{inner}</Link>;
}
