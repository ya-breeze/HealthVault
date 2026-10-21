'use client';
import { useEffect, useRef, useState } from 'react';
import { ApiError, FoodMeal } from '@/lib/api';
import { useLanguage } from '@/components/LanguageContext';
import { dateLocaleFor, interpolate, macroSourceLabel } from '@/lib/i18n';
import { loggedDayLabel, loggedDayTime } from '@/lib/loggedDay';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  meal: FoodMeal;
  timezone: string;
  busy: boolean;
  onAction: (action: 'apply' | 'dismiss') => Promise<FoodMeal>;
  onRefresh: () => Promise<FoodMeal>;
}

export default function RecentMealSuggestion({ meal, timezone, busy, onAction, onRefresh }: Props) {
  const { t, language } = useLanguage();
  const inFlight = useRef(false);
  const [error, setError] = useState<string | null>(null);
  const [stale, setStale] = useState(false);
  // Refreshing portions cannot repair a changed source snapshot. Only a
  // newly analyzed suggestion (or dismissal) replaces that snapshot.
  const suggestionKey = JSON.stringify(meal.history_suggestion);
  useEffect(() => {
    setStale(false);
    setError(null);
  }, [suggestionKey]);
  const suggestion = meal.history_suggestion;
  if (!suggestion || meal.status !== 'pending_review') return null;
  const sourceDate = new Date(suggestion.source_logged_at);
  const date = `${loggedDayLabel(sourceDate, dateLocaleFor(language), timezone)}, ${loggedDayTime(sourceDate, dateLocaleFor(language), timezone)}`;

  const run = async (action: 'apply' | 'dismiss' | 'refresh') => {
    if (inFlight.current) return;
    inFlight.current = true;
    setError(null);
    try {
      if (action === 'refresh') await onRefresh();
      else if (meal.history_version) await onAction(action);
      else throw new Error(t('mealHistory.stale'));
      if (action !== 'refresh') setStale(false);
      else if (stale) setError(t('mealHistory.stale'));
    } catch (err) {
      const conflict = (err instanceof ApiError || (err as { name?: string })?.name === 'ApiError')
        && (err as ApiError).status === 409;
      if (conflict) setStale(true);
      setError(conflict ? t('mealHistory.stale') : err instanceof Error ? err.message : t('mealHistory.failed'));
    } finally {
      inFlight.current = false;
    }
  };

  return (
    <section data-testid="history-suggestion" className="hv-card mb-6 border-accent/40" aria-labelledby="history-suggestion-title">
      <h2 id="history-suggestion-title" className="text-lg font-semibold text-text">{t('mealHistory.suggestionTitle')}</h2>
      <TapTarget as="a" href={`/food/review/?meal=${encodeURIComponent(suggestion.source_meal_id)}`}
        className="mt-2 inline-flex flex-col justify-center text-sm text-accent break-words">
        <span>{suggestion.source_name || t('review.mealFallbackName')}</span>
        <span className="text-xs text-text-muted">{interpolate(t('mealHistory.sourceDate'), { date })}</span>
      </TapTarget>
      <h3 className="mt-3 text-sm font-medium text-text">{t('mealHistory.sourceComposition')}</h3>
      <ul className="mt-2 space-y-2 text-sm text-text" data-testid="history-suggestion-composition">
        {suggestion.source_items?.map((item, index) => (
          <li key={index} className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>{item.name} · {item.weight_grams} {t('unit.grams')}</span>
            <span className="rounded bg-bg px-1.5 py-0.5 text-xs text-text-muted">{macroSourceLabel(t, item.macro_source)}</span>
          </li>
        ))}
      </ul>
      <h3 className="mt-4 text-sm font-medium text-text">{t('mealHistory.foodsToReuse')}</h3>
      <ul className="mt-2 space-y-1 text-sm text-text" data-testid="history-suggestion-mappings">
        {suggestion.mappings.map(mapping => {
          const target = meal.items?.find(item => item.id === mapping.item_id);
          return <li key={mapping.item_id}>
            {mapping.source_name}{target ? ` · ${target.weight_grams} ${t('unit.grams')}` : ''}
          </li>;
        })}
      </ul>
      <p className="mt-3 text-sm text-text-muted">{t('mealHistory.explanation')}</p>
      <p className="mt-2 text-xs text-text-muted">{t('mealHistory.uncertainty')}</p>
      <div className="mt-4 flex flex-wrap gap-3">
        <TapTarget data-testid="history-suggestion-apply" onClick={() => run('apply')}
          disabled={busy || stale || !meal.history_version}
          className="rounded-xl px-4 text-sm font-medium bg-accent text-bg-elevated disabled:opacity-50">
          {t('mealHistory.use')}
        </TapTarget>
        <TapTarget data-testid="history-suggestion-dismiss" onClick={() => run('dismiss')}
          disabled={busy || !meal.history_version}
          className="rounded-xl px-4 text-sm text-text border border-border disabled:opacity-50">
          {t('mealHistory.dismiss')}
        </TapTarget>
      </div>
      {error && <p role="alert" className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {(stale || !meal.history_version) && (
        <TapTarget onClick={() => run('refresh')} disabled={busy} className="mt-2 text-sm text-accent disabled:opacity-50">
          {t('mealHistory.refresh')}
        </TapTarget>
      )}
    </section>
  );
}
