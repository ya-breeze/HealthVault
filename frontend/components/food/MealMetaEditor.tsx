'use client';
import { useState } from 'react';
import { api, FoodMeal } from '@/lib/api';

interface Props {
  meal: FoodMeal;
  onUpdated: (meal: FoodMeal) => void;
}

// datetime-local inputs work in local time as 'YYYY-MM-DDTHH:mm'.
function toLocalInputValue(iso: string): string {
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Lets the owner correct a meal's name/logged_at independently of
// confirming it (PATCH /api/food/meals/{id}) — previously logged_at could
// only be fixed once, via the confirm request body.
export default function MealMetaEditor({ meal, onUpdated }: Props) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(meal.name);
  const [loggedAt, setLoggedAt] = useState(toLocalInputValue(meal.logged_at));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const updated = await api.patchMeal(meal.id, {
        name: name.trim() || undefined,
        logged_at: new Date(loggedAt).toISOString(),
      });
      onUpdated(updated);
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update meal');
    } finally {
      setBusy(false);
    }
  };

  if (!editing) {
    return (
      <button
        onClick={() => setEditing(true)}
        className="text-xs text-gray-500 dark:text-gray-400 hover:underline"
      >
        Edit name/time
      </button>
    );
  }

  return (
    <div className="mt-2 space-y-2">
      <input
        type="text"
        value={name}
        onChange={e => setName(e.target.value)}
        placeholder="Meal name"
        className="w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
      />
      <input
        type="datetime-local"
        value={loggedAt}
        onChange={e => setLoggedAt(e.target.value)}
        className="w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
      />
      <div className="flex justify-end gap-2">
        <button
          onClick={() => setEditing(false)}
          className="text-xs text-gray-500 dark:text-gray-400 hover:underline"
        >
          Cancel
        </button>
        <button
          onClick={save}
          disabled={busy}
          className="px-3 py-1.5 rounded-md text-xs font-medium bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50"
        >
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
      {error && <p className="text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
