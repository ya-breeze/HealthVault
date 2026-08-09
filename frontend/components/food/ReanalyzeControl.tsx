'use client';
import { useState } from 'react';
import { api, FoodMeal, ReanalyzeFailedError } from '@/lib/api';

interface Props {
  mealId: string;
  onReanalyzed: (meal: FoodMeal) => void;
}

const MAX_HINT_LENGTH = 500;

// Available whenever the backend accepts it (failed, pending_review,
// confirmed — see the Hint-Driven Reanalysis requirement). A successful
// reanalysis replaces all items and, for a confirmed meal, reverts its
// status back to pending_review/pending_clarification — so this warns
// before submitting. A failed attempt (HTTP 502, surfaced as
// ReanalyzeFailedError) leaves the meal exactly as it was; no refetch is
// needed on that path.
export default function ReanalyzeControl({ mealId, onReanalyzed }: Props) {
  const [open, setOpen] = useState(false);
  const [hint, setHint] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    const trimmed = hint.trim();
    if (!trimmed) {
      setError('A hint is required');
      return;
    }
    if (trimmed.length > MAX_HINT_LENGTH) {
      setError(`Hint must be at most ${MAX_HINT_LENGTH} characters`);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const updated = await api.reanalyzeMeal(mealId, trimmed);
      onReanalyzed(updated);
      setOpen(false);
      setHint('');
    } catch (err) {
      if (err instanceof ReanalyzeFailedError) {
        setError('Reanalysis failed — the meal is unchanged. You can try again.');
      } else {
        setError(err instanceof Error ? err.message : 'Reanalysis failed');
      }
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="mt-3 w-full py-2 rounded-lg text-sm font-medium border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-blue-400 hover:text-blue-600 dark:hover:text-blue-400"
      >
        Reanalyze with a hint
      </button>
    );
  }

  return (
    <div className="mt-3 bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-4">
      <p className="text-sm font-medium text-gray-900 dark:text-white mb-1">Reanalyze with a hint</p>
      <p className="text-xs text-amber-700 dark:text-amber-400 mb-2">
        This replaces all current items with a fresh analysis. If this meal is confirmed, it will
        revert to needing review and confirmation again.
      </p>
      <textarea
        value={hint}
        onChange={e => setHint(e.target.value)}
        maxLength={MAX_HINT_LENGTH}
        placeholder="e.g. this is chicken and rice, not berries"
        rows={2}
        className="w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
      />
      <div className="flex justify-end gap-2 mt-2">
        <button
          onClick={() => { setOpen(false); setHint(''); setError(null); }}
          className="text-xs text-gray-500 dark:text-gray-400 hover:underline"
        >
          Cancel
        </button>
        <button
          onClick={submit}
          disabled={busy}
          className="px-3 py-1.5 rounded-md text-xs font-medium bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50"
        >
          {busy ? 'Reanalyzing…' : 'Reanalyze'}
        </button>
      </div>
      {error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
