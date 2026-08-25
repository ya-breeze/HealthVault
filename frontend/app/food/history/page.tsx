'use client';
import { useEffect, useState } from 'react';
import { api, MealSummary } from '@/lib/api';
import Header from '@/components/Header';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { dateLocaleFor, mealStatusLabel } from '@/lib/i18n';
import { useLatest } from '@/lib/useLatest';
import { loggedDayKey } from '@/lib/loggedDay';

const PAGE_SIZE = 50;

interface DayTotals {
  calories: number;
  protein: number;
  carbs: number;
  fat: number;
}

interface DayGroup {
  dateKey: string;
  label: string;
  meals: MealSummary[];
  totals: DayTotals;
}

// Groups an already logged_at-DESC-sorted meal list into per-local-calendar-
// day sections. Because the input is sorted, a given day's meals are always
// contiguous, so a single linear pass (grouping by first-seen order) is
// enough — no separate sort step, and re-running this over a longer list
// after "load older" naturally merges new meals into the right existing
// section or appends new sections in the correct place. Totals sum only
// `confirmed` meals — others have no final nutrition numbers yet (see
// food-meal-history's "Daily total sums only confirmed meals" scenario).
// `locale` and `timezone` are threaded in rather than read from a hook, since
// this runs outside the component. See dateLocaleFor: it is undefined for
// English, which is exactly what Intl wants in order to keep honouring the
// browser's regional date preference. `timezone` groups by the user's Logged
// Day (see lib/loggedDay.ts), the same boundary the backend's completeness
// endpoint uses, so a day section's key lines up with that endpoint's `date`.
function groupByDay(meals: MealSummary[], locale: string | undefined, timezone: string | undefined): DayGroup[] {
  const groups: DayGroup[] = [];
  const indexByKey = new Map<string, number>();

  for (const meal of meals) {
    const d = new Date(meal.logged_at);
    const dateKey = loggedDayKey(d, timezone);

    let idx = indexByKey.get(dateKey);
    if (idx === undefined) {
      idx = groups.length;
      indexByKey.set(dateKey, idx);
      groups.push({
        dateKey,
        label: d.toLocaleDateString(locale, { weekday: 'long', month: 'short', day: 'numeric' }),
        meals: [],
        totals: { calories: 0, protein: 0, carbs: 0, fat: 0 },
      });
    }

    const group = groups[idx];
    group.meals.push(meal);
    if (meal.status === 'confirmed') {
      group.totals.calories += meal.calories;
      group.totals.protein += meal.protein_grams;
      group.totals.carbs += meal.carbs_grams;
      group.totals.fat += meal.fat_grams;
    }
  }

  return groups;
}

// Meal history: browse past meals of any status and open one for review or
// editing. Previously there was no entry point into a meal beyond the
// direct upload flow or a hand-typed /food/review/?meal=<uuid> URL.
export default function FoodHistoryPage() {
  const { t, language } = useLanguage();
  const [meals, setMeals] = useState<MealSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [timezone, setTimezone] = useState<string>('UTC');

  // Read through a ref, deliberately not listed as a dependency: `t` changes
  // identity on every language switch, and this effect replaces the whole
  // meal list with page 1. Depending on it meant switching language mid-browse
  // silently discarded every "Load older" page the user had accumulated and
  // jumped the view back to the top, with no loading indicator to explain it.
  // Found in code review. See lib/useLatest.
  const tRef = useLatest(t);

  useEffect(() => {
    api.getSettings()
      .then(settings => {
        if (settings.timezone) setTimezone(settings.timezone);
      })
      .catch(() => {
        // Falls back to the 'UTC' default already in state — a settings
        // fetch failure shouldn't block the meal list itself from rendering.
      });
  }, []);

  useEffect(() => {
    api.listMeals({ limit: PAGE_SIZE })
      .then(rows => {
        setMeals(rows);
        // The (logged_at, id) keyset cursor never defers or splits a tied
        // group — it returns up to PAGE_SIZE rows, full stop. A short page
        // therefore reliably proves there's nothing more to fetch.
        setHasMore(rows.length === PAGE_SIZE);
      })
      .catch(err => setError(err instanceof Error ? err.message : tRef.current('history.loadFailed')))
      .finally(() => setLoading(false));
  }, [tRef]);

  const loadMore = async () => {
    if (meals.length === 0) return;
    setLoadingMore(true);
    setError(null);
    try {
      const oldest = meals[meals.length - 1];
      const rows = await api.listMeals({ limit: PAGE_SIZE, before: oldest.logged_at, beforeId: oldest.id });
      setMeals(m => [...m, ...rows]);
      setHasMore(rows.length === PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('history.loadMoreFailed'));
    } finally {
      setLoadingMore(false);
    }
  };

  const dayGroups = groupByDay(meals, dateLocaleFor(language), timezone);

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <Header />

      <main className="max-w-md mx-auto px-6 py-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-4">{t('history.title')}</h1>

        {loading && <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-6">{t('review.loading')}</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400 mb-3">{error}</p>}
        {!loading && meals.length === 0 && !error && (
          <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-6">{t('history.noMeals')}</p>
        )}

        {dayGroups.map(day => (
          <div key={day.dateKey} className="mb-5">
            <div className="flex items-baseline justify-between mb-2 px-1">
              <h2 className="text-sm font-semibold text-gray-900 dark:text-white">{day.label}</h2>
              <span className="text-sm text-gray-500 dark:text-gray-400">
                {Math.round(day.totals.calories)} {t('unit.kcal')} · {t('unit.proteinShort')}{' '}
                {Math.round(day.totals.protein)}{t('unit.grams')} · {t('unit.carbsShort')}{' '}
                {Math.round(day.totals.carbs)}{t('unit.grams')} · {t('unit.fatShort')}{' '}
                {Math.round(day.totals.fat)}{t('unit.grams')}
              </span>
            </div>
            <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 divide-y divide-gray-100 dark:divide-gray-700">
              {day.meals.map(meal => (
                <TapTarget
                  key={meal.id}
                  as="a"
                  href={`/food/review/?meal=${meal.id}`}
                  className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-gray-900 dark:text-white truncate">
                      {meal.name || t('review.mealFallbackName')}
                    </p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">
                      {new Date(meal.logged_at).toLocaleString(dateLocaleFor(language))} · {mealStatusLabel(t, meal.status)}
                    </p>
                  </div>
                  {meal.status === 'confirmed' && (
                    <span className="text-sm font-semibold text-gray-900 dark:text-white flex-shrink-0">
                      {Math.round(meal.calories)} {t('unit.kcal')}
                    </span>
                  )}
                </TapTarget>
              ))}
            </div>
          </div>
        ))}

        {hasMore && meals.length > 0 && (
          <TapTarget
            onClick={loadMore}
            disabled={loadingMore}
            className="mt-4 w-full rounded-lg text-sm font-medium border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-blue-400 hover:text-blue-600 dark:hover:text-blue-400 disabled:opacity-50"
          >
            {loadingMore ? t('history.loadingMore') : t('history.loadOlder')}
          </TapTarget>
        )}
      </main>
    </div>
  );
}
