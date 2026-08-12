'use client';
import { useState } from 'react';
import { api, FoodSearchResult } from '@/lib/api';

interface Props {
  itemName: string;
  onBind: (result: FoodSearchResult) => Promise<void>;
  onManual: (name: string, macros: {
    calories: number; protein_grams: number; carbs_grams: number; fat_grams: number;
    sugar_grams: number; sodium_grams: number; dietary_fiber_grams: number;
  }) => Promise<void>;
}

// The review UI for correcting an item's food match: search for a reference
// food to bind (the item's displayed name updates to the match's real name),
// or fall back to entering a name and macros directly (e.g. from a package
// label). Reachable for any item, matched or not, until the meal is
// confirmed — not just ones the vision model left unresolved.
export default function ItemResolver({ itemName, onBind, onManual }: Props) {
  const [mode, setMode] = useState<'search' | 'manual'>('search');
  const [query, setQuery] = useState(itemName);
  const [results, setResults] = useState<FoodSearchResult[] | null>(null);
  const [searching, setSearching] = useState(false);
  // Guards onBind/onManual specifically (not search): both trigger a real
  // mutation (a PATCH or POST to the backend) with no idempotency key, so a
  // double-click on a result — or a click followed quickly by another —
  // before the first request's response closes this component can fire two
  // requests. For AddItemForm's create context that's not merely wasted
  // work: each POST allocates its own new item ID, so both succeed and the
  // meal ends up with the same item twice.
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [manualName, setManualName] = useState(itemName);
  const [macros, setMacros] = useState({
    calories: 0, protein_grams: 0, carbs_grams: 0, fat_grams: 0,
    sugar_grams: 0, sodium_grams: 0, dietary_fiber_grams: 0,
  });

  const search = async () => {
    setSearching(true);
    setError(null);
    try {
      const res = await api.searchFood(query);
      setResults(res.results);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
    } finally {
      setSearching(false);
    }
  };

  const handleBind = async (r: FoodSearchResult) => {
    if (submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      await onBind(r);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to bind food');
    } finally {
      setSubmitting(false);
    }
  };

  const handleManualSubmit = async () => {
    if (submitting) return;
    setError(null);
    const name = manualName.trim();
    if (!name) {
      setError('Name is required');
      return;
    }
    setSubmitting(true);
    try {
      await onManual(name, macros);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save macros');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="mt-2 border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 rounded-lg p-3">
      <div className="flex gap-2 mb-2 text-xs">
        <button
          onClick={() => setMode('search')}
          className={`px-2 py-1 rounded ${mode === 'search' ? 'bg-amber-600 text-white' : 'text-amber-700 dark:text-amber-300'}`}
        >
          Search
        </button>
        <button
          onClick={() => setMode('manual')}
          className={`px-2 py-1 rounded ${mode === 'manual' ? 'bg-amber-600 text-white' : 'text-amber-700 dark:text-amber-300'}`}
        >
          Enter macros
        </button>
      </div>

      {mode === 'search' ? (
        <div>
          <div className="flex gap-2">
            <input
              type="text"
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && search()}
              className="flex-1 border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
            <button
              onClick={search}
              disabled={searching}
              className="px-3 py-1.5 rounded-md text-xs font-medium bg-amber-600 hover:bg-amber-700 text-white disabled:opacity-50"
            >
              {searching ? '…' : 'Search'}
            </button>
          </div>
          {results && (
            <ul className="mt-2 max-h-48 overflow-auto divide-y divide-amber-100 dark:divide-amber-900">
              {results.length === 0 && (
                <li className="py-2 text-xs text-gray-500 dark:text-gray-400">No matches found.</li>
              )}
              {results.map((r, i) => (
                <li key={i}>
                  <button
                    onClick={() => handleBind(r)}
                    disabled={submitting}
                    className="w-full text-left py-1.5 text-xs text-gray-700 dark:text-gray-200 hover:text-blue-600 dark:hover:text-blue-400 disabled:opacity-50"
                  >
                    {r.name}{' '}
                    <span className="text-gray-400">
                      ({Math.round(r.profile.calories_per_100g)} kcal/100g
                      {r.source === 'custom' ? ', your custom food' : ''})
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-2">
          <label className="col-span-2 text-xs text-gray-600 dark:text-gray-300">
            Name
            <input
              type="text"
              value={manualName}
              onChange={e => setManualName(e.target.value)}
              className="mt-0.5 w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
          </label>
          {(
            [
              ['calories', 'Calories'], ['protein_grams', 'Protein (g)'], ['carbs_grams', 'Carbs (g)'],
              ['fat_grams', 'Fat (g)'], ['sugar_grams', 'Sugar (g)'], ['sodium_grams', 'Sodium (g)'],
              ['dietary_fiber_grams', 'Fiber (g)'],
            ] as const
          ).map(([key, label]) => (
            <label key={key} className="text-xs text-gray-600 dark:text-gray-300">
              {label}
              <input
                type="number"
                step="any"
                value={macros[key]}
                onChange={e => setMacros({ ...macros, [key]: Number(e.target.value) })}
                className="mt-0.5 w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
              />
            </label>
          ))}
          <button
            onClick={handleManualSubmit}
            disabled={submitting}
            className="col-span-2 mt-1 py-1.5 rounded-md text-xs font-medium bg-amber-600 hover:bg-amber-700 text-white disabled:opacity-50"
          >
            {submitting ? 'Saving…' : 'Save'}
          </button>
        </div>
      )}

      {error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
