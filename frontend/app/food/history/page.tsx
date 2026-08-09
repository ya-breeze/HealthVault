'use client';
import { useEffect, useState } from 'react';
import { api, MealSummary } from '@/lib/api';
import Header from '@/components/Header';

const STATUS_LABEL: Record<string, string> = {
  processing: 'Analyzing…',
  pending_clarification: 'Needs clarification',
  pending_review: 'Review needed',
  confirmed: 'Confirmed',
  failed: 'Analysis failed',
};

const PAGE_SIZE = 50;

// Meal history: browse past meals of any status and open one for review or
// editing. Previously there was no entry point into a meal beyond the
// direct upload flow or a hand-typed /food/review/?meal=<uuid> URL.
export default function FoodHistoryPage() {
  const [meals, setMeals] = useState<MealSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);

  useEffect(() => {
    api.listMeals({ limit: PAGE_SIZE })
      .then(rows => {
        setMeals(rows);
        // The (logged_at, id) keyset cursor never defers or splits a tied
        // group — it returns up to PAGE_SIZE rows, full stop. A short page
        // therefore reliably proves there's nothing more to fetch.
        setHasMore(rows.length === PAGE_SIZE);
      })
      .catch(err => setError(err instanceof Error ? err.message : 'Failed to load meals'))
      .finally(() => setLoading(false));
  }, []);

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
      setError(err instanceof Error ? err.message : 'Failed to load more meals');
    } finally {
      setLoadingMore(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <Header />

      <main className="max-w-md mx-auto px-6 py-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Meal History</h1>

        {loading && <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-6">Loading…</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400 mb-3">{error}</p>}
        {!loading && meals.length === 0 && !error && (
          <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-6">No meals logged yet.</p>
        )}

        {meals.length > 0 && (
          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 divide-y divide-gray-100 dark:divide-gray-700">
            {meals.map(meal => (
              <a
                key={meal.id}
                href={`/food/review/?meal=${meal.id}`}
                className="flex items-center justify-between gap-3 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900 dark:text-white truncate">
                    {meal.name || 'Meal'}
                  </p>
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    {new Date(meal.logged_at).toLocaleString()} · {STATUS_LABEL[meal.status] ?? meal.status}
                  </p>
                </div>
                {meal.status === 'confirmed' && (
                  <span className="text-sm font-semibold text-gray-900 dark:text-white flex-shrink-0">
                    {Math.round(meal.calories)} kcal
                  </span>
                )}
              </a>
            ))}
          </div>
        )}

        {hasMore && meals.length > 0 && (
          <button
            onClick={loadMore}
            disabled={loadingMore}
            className="mt-4 w-full py-2 rounded-lg text-sm font-medium border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-blue-400 hover:text-blue-600 dark:hover:text-blue-400 disabled:opacity-50"
          >
            {loadingMore ? 'Loading…' : 'Load older'}
          </button>
        )}
      </main>
    </div>
  );
}
