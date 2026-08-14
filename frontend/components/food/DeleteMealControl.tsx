'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, ApiError } from '@/lib/api';
import { useToast } from '@/components/Toast';

interface Props {
  mealId: string;
  // Claims the next slot in the caller's mutation queue and returns a
  // promise for `issue`'s own settlement — mirrors ReviewClient's
  // applyMealUpdate (queue *issuing*, not just resolution), but skips
  // applying a result since a delete leaves no FoodMeal to reconcile.
  // Anything queued after this call chains behind the returned promise, so
  // it runs after the delete instead of racing it.
  queueDelete: (issue: () => Promise<void>) => Promise<void>;
}

// Shared by ReviewClient's main content and ClarifyModal — a meal stuck in
// pending_clarification is otherwise fully hidden behind the modal's opaque
// backdrop, with no way to delete it short of answering questions the user
// may not want to answer (e.g. an accidental upload).
export default function DeleteMealControl({ mealId, queueDelete }: Props) {
  const router = useRouter();
  const { showToast } = useToast();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const handleDelete = async () => {
    setDeleting(true);
    setDeleteError(null);
    try {
      await queueDelete(() => api.deleteRecord('food_meal', mealId));
    } catch (err) {
      // A 404 means the meal is already gone (e.g. deleted from another
      // tab) — treat that the same as a successful delete instead of
      // leaving the user stuck retrying a delete that can never succeed.
      // Anything else is a real failure: stay in the confirm state so the
      // user can retry.
      if (!(err instanceof ApiError && err.status === 404)) {
        setDeleteError(err instanceof Error ? err.message : 'Failed to delete meal');
        setDeleting(false);
        return;
      }
    }
    showToast('Meal deleted', 'success');
    router.push('/food/history');
    setDeleting(false);
  };

  return (
    <div>
      {confirmingDelete ? (
        <div className="flex items-center gap-2">
          <span className="text-sm text-gray-600 dark:text-gray-300">Delete this meal?</span>
          <button
            onClick={handleDelete}
            disabled={deleting}
            className="py-2 px-4 rounded-lg text-sm font-medium bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
          >
            {deleting ? 'Deleting…' : 'Confirm'}
          </button>
          <button
            onClick={() => { setDeleteError(null); setConfirmingDelete(false); }}
            disabled={deleting}
            className="py-2 px-4 rounded-lg text-sm font-medium bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 disabled:opacity-50"
          >
            Cancel
          </button>
        </div>
      ) : (
        <button
          onClick={() => { setDeleteError(null); setConfirmingDelete(true); }}
          className="text-sm font-medium text-red-600 dark:text-red-400 hover:underline"
        >
          Delete meal
        </button>
      )}
      {deleteError && <p className="mt-2 text-sm text-red-600 dark:text-red-400">{deleteError}</p>}
    </div>
  );
}
