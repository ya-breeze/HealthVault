'use client';
import { useRef, useState } from 'react';
import { api, FoodSearchResult } from '@/lib/api';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  itemName: string;
  onBind: (result: FoodSearchResult) => Promise<void>;
  onManual: (name: string, macros: {
    calories: number; protein_grams: number; carbs_grams: number; fat_grams: number;
    sugar_grams: number; sodium_grams: number; dietary_fiber_grams: number;
  }, saveAsCustomFood: boolean) => Promise<void>;
  // Shows a "save as reusable food" checkbox alongside the manual-macros
  // form — only meaningful for a correction to an existing item (see
  // MealItemRow), not for adding a brand-new one (AddItemForm), which POSTs
  // to an endpoint that doesn't accept this option.
  allowSaveAsCustomFood?: boolean;
}

// The review UI for correcting an item's food match: search for a reference
// food to bind (the item's displayed name updates to the match's real name),
// or fall back to entering a name and macros directly (e.g. from a package
// label). Reachable for any item, matched or not, until the meal is
// confirmed — not just ones the vision model left unresolved.
export default function ItemResolver({ itemName, onBind, onManual, allowSaveAsCustomFood }: Props) {
  const [mode, setMode] = useState<'search' | 'manual'>('search');
  const [query, setQuery] = useState(itemName);
  const [results, setResults] = useState<FoodSearchResult[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [translatedQuery, setTranslatedQuery] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<string | null>(null);
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
  const [saveAsCustomFood, setSaveAsCustomFood] = useState(false);
  // search and refresh both write to the shared results/translatedQuery
  // state below, so a slower, older response (e.g. a refresh outlived by a
  // newer search) must not clobber a newer one's results. Each call captures
  // the sequence number current when it starts and only applies its
  // response if that's still the latest — a stale response is dropped
  // entirely, though searching/refreshing still clear on their own request's
  // completion regardless of staleness.
  const requestSeq = useRef(0);

  const search = async () => {
    const seq = ++requestSeq.current;
    setSearching(true);
    setError(null);
    setRefreshError(null);
    try {
      const res = await api.searchFood(query);
      if (seq === requestSeq.current) {
        setResults(res.results);
        setTranslatedQuery(res.translated_query ?? null);
      }
    } catch (err) {
      if (seq === requestSeq.current) {
        setError(err instanceof Error ? err.message : 'Search failed');
      }
    } finally {
      setSearching(false);
    }
  };

  // Re-translates the query, discarding the cached mapping — for when a
  // previous translation turned out wrong (e.g. "овсянка" resolved to the
  // wrong food). Results always reflect this request's fresh response; the
  // "Searched as" text is only replaced when the response actually names a
  // translation, so a refresh that comes back without one (already-correct
  // input, or a translation failure on the server) doesn't blank out the
  // last known-good term.
  const refresh = async () => {
    const seq = ++requestSeq.current;
    setRefreshing(true);
    setRefreshError(null);
    try {
      const res = await api.searchFood(query, undefined, undefined, true);
      if (seq === requestSeq.current) {
        setResults(res.results);
        if (res.translated_query) {
          setTranslatedQuery(res.translated_query);
        } else {
          setRefreshError('Could not refresh the translation — showing the previous term.');
        }
      }
    } catch (err) {
      if (seq === requestSeq.current) {
        setRefreshError(err instanceof Error ? err.message : 'Refresh failed');
      }
    } finally {
      setRefreshing(false);
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
      await onManual(name, macros, saveAsCustomFood);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save macros');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="mt-2 border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 rounded-lg p-3">
      <div className="flex gap-2 mb-2 text-xs">
        <TapTarget
          onClick={() => setMode('search')}
          className={`px-2 pb-1 border-b-2 ${mode === 'search' ? 'border-amber-600 text-amber-900 dark:text-amber-200' : 'border-transparent text-amber-700 dark:text-amber-300'}`}
        >
          Search food
        </TapTarget>
        <TapTarget
          onClick={() => setMode('manual')}
          className={`px-2 pb-1 border-b-2 ${mode === 'manual' ? 'border-amber-600 text-amber-900 dark:text-amber-200' : 'border-transparent text-amber-700 dark:text-amber-300'}`}
        >
          Enter macros
        </TapTarget>
      </div>

      {mode === 'search' ? (
        <div>
          <div className="flex gap-2 items-center">
            <input
              type="text"
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && search()}
              className="flex-1 border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1.5 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
            <TapTarget
              onClick={search}
              disabled={searching}
              className="px-3 rounded-md text-xs font-medium bg-amber-600 hover:bg-amber-700 text-white disabled:opacity-50"
            >
              {searching ? '…' : 'Search'}
            </TapTarget>
          </div>
          <p className="mt-1 text-[10px] text-gray-400 dark:text-gray-500">
            New search terms may be sent to an external model provider for translation.
          </p>
          {translatedQuery && (
            <div className="mt-1 flex items-center gap-1.5 text-[11px] text-amber-700 dark:text-amber-300">
              <span>
                Searched as: <strong>{translatedQuery}</strong>
              </span>
              <TapTarget
                onClick={refresh}
                disabled={refreshing}
                aria-label="Refresh translation"
                title="Refresh translation"
                className="flex items-center justify-center disabled:opacity-50"
              >
                {refreshing ? '…' : '🔄'}
              </TapTarget>
            </div>
          )}
          {refreshError && <p className="mt-1 text-[11px] text-red-600 dark:text-red-400">{refreshError}</p>}
          {results && (
            <ul className="mt-2 max-h-64 overflow-auto divide-y divide-amber-100 dark:divide-amber-900">
              {results.length === 0 && (
                <li className="py-2 text-xs text-gray-500 dark:text-gray-400">No matches found.</li>
              )}
              {results.map((r, i) => (
                <li key={i}>
                  <TapTarget
                    onClick={() => handleBind(r)}
                    disabled={submitting}
                    className="w-full flex items-center text-left text-xs text-gray-700 dark:text-gray-200 hover:text-blue-600 dark:hover:text-blue-400 disabled:opacity-50"
                  >
                    {r.name}{' '}
                    <span className="text-gray-400">
                      ({Math.round(r.profile.calories_per_100g)} kcal/100g
                      {r.source === 'custom' ? ', your custom food' : ''})
                    </span>
                  </TapTarget>
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
          {allowSaveAsCustomFood && (
            <label className="col-span-2 flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-300">
              <input
                type="checkbox"
                checked={saveAsCustomFood}
                onChange={e => setSaveAsCustomFood(e.target.checked)}
                className="h-4 w-4"
              />
              Save as a reusable food, so a future photo of this dish can match it automatically
            </label>
          )}
          <TapTarget
            onClick={handleManualSubmit}
            disabled={submitting}
            className="col-span-2 mt-1 rounded-md text-xs font-medium bg-amber-600 hover:bg-amber-700 text-white disabled:opacity-50"
          >
            {submitting ? 'Saving…' : 'Save'}
          </TapTarget>
        </div>
      )}

      {error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
