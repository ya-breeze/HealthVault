'use client';
import { useState } from 'react';
import { api, FoodMeal } from '@/lib/api';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  meal: FoodMeal;
  // Takes a thunk, not an already-started request — the parent
  // (ReviewClient) controls exactly when this fires so sibling mutations
  // never overlap in flight. See ReviewClient's applyMealUpdate.
  onUpdated: (issue: () => Promise<FoodMeal>, label?: string) => Promise<FoodMeal>;
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
  // toLocalInputValue truncates to minute granularity (datetime-local has no
  // seconds by default), so re-deriving logged_at from the input on every
  // save — even a name-only edit — would silently drop the meal's real
  // seconds/fractional-seconds and could shift its history ordering. Only
  // send logged_at when the user actually touched this field.
  const [loggedAtDirty, setLoggedAtDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const resetDraft = () => {
    setName(meal.name);
    setLoggedAt(toLocalInputValue(meal.logged_at));
    setLoggedAtDirty(false);
    setError(null);
  };

  const openEditor = () => {
    resetDraft();
    setEditing(true);
  };

  const cancel = () => {
    resetDraft();
    setEditing(false);
  };

  const save = async () => {
    const trimmedName = name.trim();
    const nameChanged = trimmedName !== '' && trimmedName !== meal.name;

    let loggedAtIso: string | undefined;
    if (loggedAtDirty) {
      const parsedDate = loggedAt ? new Date(loggedAt) : null;
      if (!parsedDate || Number.isNaN(parsedDate.getTime())) {
        setError('Please enter a valid date and time');
        return;
      }
      loggedAtIso = parsedDate.toISOString();
    }

    if (!nameChanged && loggedAtIso === undefined) {
      // Nothing was actually changed — close without a round trip; an empty
      // PATCH body would just be rejected by the backend anyway.
      setEditing(false);
      return;
    }

    setBusy(true);
    setError(null);
    try {
      await onUpdated(() => api.patchMeal(meal.id, {
        name: nameChanged ? trimmedName : undefined,
        logged_at: loggedAtIso,
      }), 'Meal updated');
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update meal');
    } finally {
      setBusy(false);
    }
  };

  if (!editing) {
    return (
      <TapTarget
        onClick={openEditor}
        className="flex items-center text-xs text-gray-500 dark:text-gray-400 hover:underline"
      >
        Edit name/time
      </TapTarget>
    );
  }

  return (
    <div className="mt-2 space-y-2">
      <input
        type="text"
        value={name}
        onChange={e => setName(e.target.value)}
        placeholder="Meal name"
        className="w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
      />
      <input
        type="datetime-local"
        value={loggedAt}
        onChange={e => {
          setLoggedAt(e.target.value);
          setLoggedAtDirty(true);
        }}
        className="w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
      />
      <div className="flex justify-end gap-2">
        <TapTarget
          onClick={cancel}
          className="flex items-center text-xs text-gray-500 dark:text-gray-400 hover:underline"
        >
          Cancel
        </TapTarget>
        <TapTarget
          onClick={save}
          disabled={busy}
          className="px-3 rounded-md text-xs font-medium bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50"
        >
          {busy ? 'Saving…' : 'Save'}
        </TapTarget>
      </div>
      {error && <p className="text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
