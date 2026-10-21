'use client';
import { useEffect, useState } from 'react';
import { api, DayCompletenessState, MealSummary } from '@/lib/api';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import TapTarget from '@/components/ui/TapTarget';
import RepeatMealButton from '@/components/food/RepeatMealButton';
import { canRepeatMeal } from '@/lib/mealHistory';
import DayCompletenessControl from '@/components/food/DayCompletenessControl';
import HistorySettingsPanel from '@/components/food/HistorySettingsPanel';
import { useLanguage } from '@/components/LanguageContext';
import { dateLocaleFor, mealStatusLabel } from '@/lib/i18n';
import { useLatest } from '@/lib/useLatest';
import { loggedDayKey, loggedDayLabel, loggedDayTime } from '@/lib/loggedDay';
import { splitRangeIntoWindows } from '@/lib/completeness';

const PAGE_SIZE = 50;

interface DayTotals {
  calories: number;
  protein: number;
  carbs: number;
  fat: number;
  fiber: number | null;
  confirmedMeals: number;
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
// The visible label uses the same `timezone` as the key (via loggedDayLabel)
// rather than the browser's own zone, so a meal near the day boundary is
// never grouped under one date while its section heading names another.
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
        label: loggedDayLabel(d, locale, timezone),
        meals: [],
        totals: { calories: 0, protein: 0, carbs: 0, fat: 0, fiber: 0, confirmedMeals: 0 },
      });
    }

    const group = groups[idx];
    group.meals.push(meal);
    if (meal.status === 'confirmed') {
      group.totals.calories += meal.calories;
      group.totals.protein += meal.protein_grams;
      group.totals.carbs += meal.carbs_grams;
      group.totals.fat += meal.fat_grams;
      group.totals.confirmedMeals += 1;
      group.totals.fiber = group.totals.fiber === null || meal.dietary_fiber_grams == null
        ? null : group.totals.fiber + meal.dietary_fiber_grams;
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
  // The raw stored `timezone` setting (undefined when never set), kept
  // separate from `timezone` above so the settings panel can prefill the
  // browser's own zone instead of the grouping default of 'UTC'.
  const [storedTimezone, setStoredTimezone] = useState<string | undefined>(undefined);
  const [usualMealsPerDay, setUsualMealsPerDay] = useState<number>(3);
  const [completeness, setCompleteness] = useState<Record<string, DayCompletenessState>>({});
  // Bumped on every settings-panel save so the completeness effect below
  // refetches even when only usual_meals_per_day changed (timezone, its
  // other dependency, may be unchanged) — tasks.md 9.3.
  const [completenessRefreshKey, setCompletenessRefreshKey] = useState(0);

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
        setStoredTimezone(settings.timezone);
        if (typeof settings.usual_meals_per_day === 'number' && settings.usual_meals_per_day > 0) {
          setUsualMealsPerDay(settings.usual_meals_per_day);
        }
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

  // Fetches Day Completeness state for the currently-loaded range (initial
  // load and each "load older") whenever it changes, excluding the caller's
  // current Logged Day — the backend's range endpoint never returns an entry
  // for it anyway (design.md §6 "Frontend surface"). The loaded range has no
  // depth limit, so it can exceed the endpoint's 92-day cap; split into
  // consecutive windows and fetch them in parallel rather than one
  // (potentially-400ing) oversized call (tasks.md 8.1).
  useEffect(() => {
    const todayKey = loggedDayKey(new Date(), timezone);
    const dateKeys = Array.from(new Set(meals.map(m => loggedDayKey(new Date(m.logged_at), timezone))))
      .filter(k => k !== todayKey);
    if (dateKeys.length === 0) return;
    const from = dateKeys.reduce((a, b) => (a < b ? a : b));
    const to = dateKeys.reduce((a, b) => (a > b ? a : b));

    let cancelled = false;
    Promise.all(splitRangeIntoWindows(from, to).map(w => api.getCompleteness(w.from, w.to)))
      .then(results => {
        if (cancelled) return;
        setCompleteness(prev => {
          const next = { ...prev };
          for (const window of results) for (const entry of window) next[entry.date] = entry.state;
          return next;
        });
      })
      .catch(() => {
        // Best-effort: a failed fetch just leaves those days' badges/controls
        // unrendered rather than blocking the meal list itself.
      });
    return () => {
      cancelled = true;
    };
  }, [meals, timezone, completenessRefreshKey]);

  const handleCompletenessChange = (date: string, state: DayCompletenessState) => {
    setCompleteness(prev => ({ ...prev, [date]: state }));
  };

  // Settings-panel save handler (tasks.md 9.3): applies the just-saved
  // values to this page's own state so day grouping (derived from
  // `timezone` on every render) and the completeness fetch above both pick
  // up the change immediately, with no reload.
  const handleSettingsSaved = (next: { timezone: string; usualMealsPerDay: number }) => {
    setTimezone(next.timezone);
    setStoredTimezone(next.timezone);
    setUsualMealsPerDay(next.usualMealsPerDay);
    setCompletenessRefreshKey(k => k + 1);
  };

  const dayGroups = groupByDay(meals, dateLocaleFor(language), timezone);

  return (
    <AuthenticatedShell className="min-h-screen bg-bg">
      <main className="hv-page">
        <div className="hv-page-heading">
          <div><h1>{t('history.title')}</h1><p className="text-sm text-text-muted mt-2">{t('redesign.review.mealsNutrientsAndDiaryCompleteness')}</p></div>
          <TapTarget as="a" href="/food/upload/" className="hidden sm:inline-flex items-center rounded-xl bg-accent text-bg-elevated px-5 text-sm font-semibold">{t('redesign.review.addFood')}</TapTarget>
        </div>

        <HistorySettingsPanel
          timezone={storedTimezone}
          usualMealsPerDay={usualMealsPerDay}
          onSaved={handleSettingsSaved}
        />

        {loading && <p className="text-sm text-text-muted text-center py-6">{t('review.loading')}</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400 mb-3">{error}</p>}
        {!loading && meals.length === 0 && !error && (
          <p className="text-sm text-text-muted text-center py-6">{t('history.noMeals')}</p>
        )}

        {dayGroups.map(day => (
          <section key={day.dateKey} data-testid="food-day-section" data-date={day.dateKey} className="hv-card mb-6">
            <div data-testid="day-header" className="mb-3 pb-4 border-b border-border">
              {/* Two rows, because the date, the completeness control and the day's
                  totals cannot share one at a phone width — their combined natural
                  width is roughly 457px against 342px of content at 390px, and the
                  control is a TapTarget that cannot shrink below its 48px minimum.
                  `flex-wrap` is what makes the narrow case degrade cleanly: when the
                  date and the control cannot share a line, the control drops whole
                  onto its own line instead of compressing into the gap and
                  re-wrapping inside its own pill. */}
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-sm font-semibold text-text">{day.label}</h2>
                <DayCompletenessControl
                  date={day.dateKey}
                  state={completeness[day.dateKey]}
                  onChange={handleCompletenessChange}
                />
              </div>
              <div data-testid="day-total" className="flex flex-wrap items-baseline gap-x-3 gap-y-2 mt-3">
                <span data-testid="day-total-calories" className="text-sm font-semibold text-text tabular-nums">
                  {Math.round(day.totals.calories)} {t('unit.kcal')}
                </span>
                <span data-testid="day-total-macros" className="text-xs text-text-muted tabular-nums">
                  {t('unit.proteinShort')}{' '}
                  {Math.round(day.totals.protein)}{t('unit.grams')} · {t('unit.carbsShort')}{' '}
                  {Math.round(day.totals.carbs)}{t('unit.grams')} · {t('unit.fatShort')}{' '}
                  {Math.round(day.totals.fat)}{t('unit.grams')}
                </span>
                <span data-testid="day-total-fiber" className="text-xs text-accent tabular-nums">
                  {t('item.fiber')}{' '}{day.totals.confirmedMeals === 0 || day.totals.fiber === null ? '—' : `${day.totals.fiber.toFixed(1)}${t('unit.grams')}`}
                </span>
              </div>
            </div>
            <div className="bg-bg-elevated rounded-xl shadow-sm border border-border divide-y divide-border">
              {day.meals.map(meal => (
                <div key={meal.id} data-testid={`meal-history-row-${meal.id}`}>
                  <TapTarget
                    as="a"
                    href={`/food/review/?meal=${meal.id}`}
                    className="flex items-center justify-between gap-3 px-2 py-4 hover:bg-bg transition-colors"
                  >
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-text truncate">
                        {meal.name || t('review.mealFallbackName')}
                      </p>
                      {/* Time only: the day header directly above already names the
                          day, so a full date here is noise — and it has to be the
                          stored zone, not the browser's, or a row can show a time
                          belonging to a different day than the header it sits under.
                          See loggedDayTime. */}
                      <p data-testid="meal-meta" className="text-xs text-text-muted">
                        {loggedDayTime(new Date(meal.logged_at), dateLocaleFor(language), timezone)} · {mealStatusLabel(t, meal.status)}
                      </p>
                  </div>
                  {meal.status === 'confirmed' && (
                    <span className="text-sm font-semibold text-text tabular-nums flex-shrink-0">
                      {Math.round(meal.calories)} {t('unit.kcal')}
                      <span className="block text-xs font-normal text-text-muted mt-1" data-testid={`meal-row-fiber-${meal.id}`}>
                        {t('item.fiber')}{' '}{meal.dietary_fiber_grams == null ? '—' : `${meal.dietary_fiber_grams.toFixed(1)}${t('unit.grams')}`}
                      </span>
                    </span>
                  )}
                </TapTarget>
                {canRepeatMeal(meal, timezone) && <RepeatMealButton mealId={meal.id} />}
                </div>
              ))}
            </div>
          </section>
        ))}

        {hasMore && meals.length > 0 && (
          <TapTarget
            onClick={loadMore}
            disabled={loadingMore}
            className="mt-4 w-full rounded-lg text-sm font-medium border border-border text-text-muted hover:border-accent hover:text-accent disabled:opacity-50"
          >
            {loadingMore ? t('history.loadingMore') : t('history.loadOlder')}
          </TapTarget>
        )}
      </main>
    </AuthenticatedShell>
  );
}
