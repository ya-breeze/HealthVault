'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import type { Dictionary, LanguageCode } from '@/lib/i18n';
import { interpolate } from '@/lib/i18n';
import { api } from '@/lib/api';
import {
  FOOD_LOG_HISTORY_WINDOW_DAYS,
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
  const cursor = new Date(`${from}T00:00:00.000Z`);
  if (Number.isNaN(cursor.getTime())) throw new Error('Invalid food history window start');
  return Array.from({ length: FOOD_LOG_HISTORY_WINDOW_DAYS }, () => {
    const date = cursor.toISOString().slice(0, 10);
    cursor.setUTCDate(cursor.getUTCDate() + 1);
    return date;
  });
}

function outcomeLabelKey(outcome: FoodLogHistoryOutcome): keyof Dictionary {
  switch (outcome) {
    case 'counted':
      return 'foodLogHistory.outcome.counted';
    case 'no_food':
      return 'foodLogHistory.outcome.noFood';
    case 'needs_attention':
      return 'foodLogHistory.outcome.needsAttention';
  }
}

function statusSymbol(outcome: FoodLogHistoryOutcome): string {
  switch (outcome) {
    case 'counted':
      return '✓';
    case 'no_food':
      return '∅';
    case 'needs_attention':
      return '!';
  }
}

function weekdayLabel(date: string, language: LanguageCode): string {
  return new Intl.DateTimeFormat(language === 'ru' ? 'ru' : undefined, { weekday: 'short', timeZone: 'UTC' }).format(
    new Date(`${date}T12:00:00.000Z`),
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
    const outcome = t(outcomeLabelKey(day.outcome));
    const key = day.unconfirmedMeals === 1
      ? 'foodLogHistory.dayDescriptionWithOneUnresolved'
      : day.unconfirmedMeals > 1
        ? 'foodLogHistory.dayDescriptionWithUnresolved'
        : 'foodLogHistory.dayDescription';
    return interpolate(t(key), {
      date: day.date,
      outcome,
      occasions: day.occasionCount,
      unresolved: day.unconfirmedMeals,
    });
  }

  function renderDay(day: FoodLogHistoryDay) {
    return (
      <div
        key={day.date}
        className="flex min-w-0 flex-1 flex-col items-center gap-1 rounded-md border border-border px-1.5 py-2 text-center"
        data-testid={`food-log-history-day-${day.date}`}
        data-outcome={day.outcome}
        role="img"
        aria-label={dayDescription(day)}
        title={dayDescription(day)}
      >
        <span className="text-[11px] text-text-muted">{weekdayLabel(day.date, language)}</span>
        <span className="font-[family-name:var(--font-data)] text-base font-bold" aria-hidden="true">
          {statusSymbol(day.outcome)}
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
              <span data-testid="food-log-history-counted">{interpolate(t('foodLogHistory.countedDays'), { count: state.summary.countedDays })}</span>
              <span data-testid="food-log-history-no-food">{interpolate(t('foodLogHistory.noFoodDays'), { count: state.summary.noFoodDays })}</span>
              <span data-testid="food-log-history-needs-attention">{interpolate(t('foodLogHistory.needsAttentionDays'), { count: state.summary.needsAttentionDays })}</span>
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
