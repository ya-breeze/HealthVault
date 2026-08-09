'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, FoodMeal, pendingClarifyQuestions } from '@/lib/api';
import ClarifyModal from '@/components/food/ClarifyModal';
import MealItemRow from '@/components/food/MealItemRow';
import AddItemForm from '@/components/food/AddItemForm';
import ReanalyzeControl from '@/components/food/ReanalyzeControl';
import MealMetaEditor from '@/components/food/MealMetaEditor';
import MacroSummary from '@/components/food/MacroSummary';
import Header from '@/components/Header';

const STATUS_LABEL: Record<string, string> = {
  processing: 'Analyzing…',
  pending_clarification: 'Needs clarification',
  pending_review: 'Review needed',
  confirmed: 'Confirmed',
  failed: 'Analysis failed',
};

export default function ReviewClient({ mealId }: { mealId: string }) {
  const router = useRouter();
  const [meal, setMeal] = useState<FoodMeal | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => {
    setLoading(true);
    setLoadError(null);
    api.getMeal(mealId)
      .then(setMeal)
      .catch(err => setLoadError(err instanceof Error ? err.message : 'Failed to load meal'))
      .finally(() => setLoading(false));
  };

  useEffect(load, [mealId]);

  const handleRetry = async () => {
    setBusy(true);
    setActionError(null);
    try {
      setMeal(await api.retryMeal(mealId));
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Retry failed');
    } finally {
      setBusy(false);
    }
  };

  const handleClarify = async (answers: string[]) => {
    const updated = await api.clarifyMeal(mealId, answers);
    setMeal(updated);
  };

  const handleConfirm = async () => {
    setBusy(true);
    setActionError(null);
    try {
      const updated = await api.confirmMeal(mealId);
      setMeal(updated);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Confirm failed');
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
        <Header />
        <p className="p-6 text-gray-500 dark:text-gray-400 text-center text-sm">Loading…</p>
      </div>
    );
  }
  if (loadError || !meal) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
        <Header />
        <div className="p-6 text-center">
          <p className="text-sm text-red-600 dark:text-red-400">{loadError ?? 'Meal not found'}</p>
        </div>
      </div>
    );
  }

  const items = meal.items ?? [];
  const pendingQuestions = pendingClarifyQuestions(meal);
  const canConfirm = meal.status === 'pending_review' && !busy;
  // Manual entries are confirmed but have no stored photo — the backend
  // rejects reanalyzing those with 409, so don't offer a control that would
  // always fail for them.
  const canReanalyze =
    Boolean(meal.photo_path) &&
    (meal.status === 'failed' || meal.status === 'pending_review' || meal.status === 'confirmed');

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <Header />

      <main className="max-w-md mx-auto px-6 py-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-1">
          {meal.name || 'Meal'}
        </h1>
        {(meal.status === 'pending_review' || meal.status === 'confirmed') && (
          <div className="mb-3">
            <MealMetaEditor meal={meal} onUpdated={setMeal} />
          </div>
        )}
        <div className="flex items-center justify-between mb-4">
          <span className="text-sm font-medium text-gray-600 dark:text-gray-300">
            {STATUS_LABEL[meal.status] ?? meal.status}
          </span>
          {meal.photo_path && (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={api.mealPhotoUrl(mealId)}
              alt="Meal"
              className="w-16 h-16 rounded-lg object-cover border border-gray-200 dark:border-gray-700"
            />
          )}
        </div>

        {meal.status === 'processing' && (
          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-6 text-center">
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-3">Still analyzing…</p>
            <button
              onClick={load}
              className="text-sm text-blue-600 dark:text-blue-400 hover:underline"
            >
              Refresh
            </button>
          </div>
        )}

        {meal.status === 'failed' && (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-xl p-6 text-center">
            <p className="text-sm text-red-700 dark:text-red-300 mb-3">Analysis failed.</p>
            <button
              onClick={handleRetry}
              disabled={busy}
              className="py-2 px-4 rounded-lg text-sm font-medium bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
            >
              {busy ? 'Retrying…' : 'Retry'}
            </button>
          </div>
        )}

        {(meal.status === 'pending_review' || meal.status === 'confirmed') && (
          <>
            <MacroSummary meal={meal} />
            <div className="mt-4 bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 px-4">
              {items.length === 0 ? (
                <p className="py-4 text-sm text-gray-500 dark:text-gray-400 text-center">No items.</p>
              ) : (
                items.map(item => (
                  <MealItemRow key={item.id} mealId={mealId} item={item} onUpdated={setMeal} />
                ))
              )}
            </div>

            <AddItemForm mealId={mealId} onAdded={setMeal} />

            {meal.status === 'pending_review' && (
              <button
                onClick={handleConfirm}
                disabled={!canConfirm}
                className="mt-4 w-full py-2.5 rounded-lg text-sm font-medium bg-green-600 hover:bg-green-700 text-white disabled:opacity-50"
              >
                {busy ? 'Confirming…' : 'Confirm Meal'}
              </button>
            )}
          </>
        )}

        {canReanalyze && <ReanalyzeControl mealId={mealId} onReanalyzed={setMeal} />}

        {actionError && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{actionError}</p>}
      </main>

      {meal.status === 'pending_clarification' && pendingQuestions.length > 0 && (
        <ClarifyModal questions={pendingQuestions} onSubmit={handleClarify} />
      )}
    </div>
  );
}
