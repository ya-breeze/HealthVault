'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { api, FoodMeal, pendingClarifyQuestions } from '@/lib/api';
import ClarifyModal from '@/components/food/ClarifyModal';
import MealItemRow from '@/components/food/MealItemRow';
import AddItemForm from '@/components/food/AddItemForm';
import ReanalyzeControl from '@/components/food/ReanalyzeControl';
import MealMetaEditor from '@/components/food/MealMetaEditor';
import MacroSummary from '@/components/food/MacroSummary';
import DeleteMealControl from '@/components/food/DeleteMealControl';
import Header from '@/components/Header';
import { useToast } from '@/components/Toast';

const STATUS_LABEL: Record<string, string> = {
  processing: 'Analyzing…',
  pending_clarification: 'Needs clarification',
  pending_review: 'Review needed',
  confirmed: 'Confirmed',
  failed: 'Analysis failed',
};

export default function ReviewClient({ mealId }: { mealId: string }) {
  const { showToast } = useToast();
  const [meal, setMeal] = useState<FoodMeal | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Every mutation control below (item rows, add-item, meal name/time,
  // reanalyze, retry, clarify, confirm) needs its resulting FoodMeal
  // applied to this shared state in the order its request actually
  // committed on the server — not the order responses happen to arrive in,
  // which ordinary network delay can reorder. A round-9 version of this
  // compared a ticket assigned at *issue* time, but issue order isn't
  // commit order either: the request issued first can be the one left
  // waiting on a lock while a later-issued request commits and returns
  // first, so a same-page edit that actually finished last can be the one
  // holding the true current state — discarding it by issue-order ticket
  // throws that state away. applyMealUpdate instead serializes *issuing*
  // each request in the first place: a mutation is queued behind whatever
  // is still outstanding, so by the time it's actually sent, every
  // earlier-queued mutation has already committed and been applied to
  // `meal`. That makes issue order and commit order the same thing by
  // construction, with nothing left to compare or guess — so every
  // mutating child takes a thunk (not an already-started request) as its
  // onUpdated/onAdded/onReanalyzed prop, letting this queue decide exactly
  // when each request is allowed to start.
  //
  // `label` names the toast shown on success ("Item added", "Meal
  // confirmed", ...); omitted call sites fall back to a generic "Saved".
  // A rejection shows a generic error toast and is then rethrown unchanged,
  // so each call site's own inline error handling (unaffected by this) still
  // runs — the toast is additive, never a replacement for it.
  const queueRef = useRef<Promise<unknown>>(Promise.resolve());
  const applyMealUpdate = useCallback((issue: () => Promise<FoodMeal>, label?: string): Promise<FoodMeal> => {
    const run = queueRef.current
      .then(issue)
      .then(result => {
        setMeal(result);
        showToast(label ?? 'Saved', 'success');
        return result;
      })
      .catch(err => {
        showToast('Update failed', 'error');
        throw err;
      });
    // Keep the queue moving even if this mutation failed — a rejected
    // mutation must not wedge every mutation queued behind it.
    queueRef.current = run.catch(() => undefined);
    return run;
  }, [showToast]);

  // Same ordering guarantee as applyMealUpdate above — queue *issuing*, not
  // just resolution — but without a FoodMeal result to apply, since a
  // delete leaves nothing to reconcile into `meal`. Used by
  // DeleteMealControl: waits for anything already queued ahead of the
  // delete to settle, and immediately re-claims the ref so anything queued
  // after this call chains behind the delete too, instead of racing it — a
  // one-time `await queueRef.current` snapshot doesn't cover that second
  // case, since a mutation queued while that await is still pending would
  // reassign the ref to something this call never sees.
  //
  // The returned promise also waits out `queueRef.current` *after* the
  // delete's own request settles, not just the request itself — otherwise a
  // caller that navigates away as soon as this resolves (DeleteMealControl
  // does) can leave a trailing mutation queued behind the delete still in
  // flight. That mutation is now guaranteed to fail (its meal is gone), and
  // applyMealUpdate's error toast would surface after the page has already
  // moved on to /food/history, which reads as a spurious failure following
  // a successful delete instead of what actually happened.
  const queueDelete = useCallback((issue: () => Promise<void>): Promise<void> => {
    const run = queueRef.current.then(issue);
    queueRef.current = run.catch(() => undefined);
    return run.then(() => queueRef.current).then(() => undefined);
  }, []);

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
      await applyMealUpdate(() => api.retryMeal(mealId), 'Analysis retried');
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Retry failed');
    } finally {
      setBusy(false);
    }
  };

  const handleClarify = async (answers: string[]) => {
    await applyMealUpdate(() => api.clarifyMeal(mealId, answers), 'Clarification submitted');
  };

  const handleConfirm = async () => {
    setBusy(true);
    setActionError(null);
    try {
      await applyMealUpdate(() => api.confirmMeal(mealId), 'Meal confirmed');
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
  const showClarifyModal = meal.status === 'pending_clarification' && pendingQuestions.length > 0;
  const canConfirm = meal.status === 'pending_review' && !busy;
  const showConfirmBar = meal.status === 'pending_review';
  // Manual entries are confirmed but have no stored photo — the backend
  // rejects reanalyzing those with 409, so don't offer a control that would
  // always fail for them.
  const canReanalyze =
    Boolean(meal.photo_path) &&
    (meal.status === 'failed' || meal.status === 'pending_review' || meal.status === 'confirmed');

  return (
    <div className={`min-h-screen bg-gray-50 dark:bg-gray-900 ${showConfirmBar ? 'pb-24' : ''}`}>
      <Header />

      <main className="max-w-md mx-auto px-6 py-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-1">
          {meal.name || 'Meal'}
        </h1>
        {(meal.status === 'pending_review' || meal.status === 'confirmed') && (
          <div className="mb-3">
            <MealMetaEditor meal={meal} onUpdated={applyMealUpdate} />
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
                  <MealItemRow key={item.id} mealId={mealId} item={item} onUpdated={applyMealUpdate} />
                ))
              )}
            </div>

            {items.some(item => item.off_code) && (
              <p className="mt-2 text-[11px] text-gray-400 dark:text-gray-500">
                Product data for some items from{' '}
                <a
                  href="https://world.openfoodfacts.org/"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="underline"
                >
                  Open Food Facts
                </a>
                , available under the{' '}
                <a
                  href="https://opendatacommons.org/licenses/odbl/1-0/"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="underline"
                >
                  Open Database License (ODbL)
                </a>
                .
              </p>
            )}

            <AddItemForm mealId={mealId} onAdded={applyMealUpdate} />
          </>
        )}

        {canReanalyze && <ReanalyzeControl mealId={mealId} onReanalyzed={applyMealUpdate} />}

        {actionError && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{actionError}</p>}

        {/* Hidden here only when ClarifyModal is actually covering the page
            (its own copy below is the only reachable one then) — gating on
            bare status would also hide this when pending_clarification has
            no pending questions to show, in which case the modal itself
            doesn't render and this would be the only copy. */}
        {!showClarifyModal && (
          <div className="mt-6 pt-4 border-t border-gray-200 dark:border-gray-700">
            <DeleteMealControl mealId={mealId} queueDelete={queueDelete} />
          </div>
        )}
      </main>

      {showConfirmBar && (
        <div className="fixed bottom-0 left-0 right-0 bg-gray-50/95 dark:bg-gray-900/95 backdrop-blur border-t border-gray-200 dark:border-gray-700 px-6 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))]">
          <div className="max-w-md mx-auto">
            <button
              onClick={handleConfirm}
              disabled={!canConfirm}
              className="w-full py-2.5 rounded-lg text-sm font-medium bg-green-600 hover:bg-green-700 text-white disabled:opacity-50"
            >
              {busy ? 'Confirming…' : 'Confirm Meal'}
            </button>
          </div>
        </div>
      )}

      {showClarifyModal && (
        <ClarifyModal
          questions={pendingQuestions}
          onSubmit={handleClarify}
          deleteControl={<DeleteMealControl mealId={mealId} queueDelete={queueDelete} />}
        />
      )}
    </div>
  );
}
