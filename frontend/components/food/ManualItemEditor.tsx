'use client';
import { useRef, useState } from 'react';
import { api, FoodSearchResult, ManualMealItemInput } from '@/lib/api';
import TapTarget from '@/components/ui/TapTarget';
import { tabClass } from './tabClass';

interface Props {
  index: number;
  item: ManualMealItemInput;
  onChange: (index: number, item: ManualMealItemInput) => void;
  onRemove: (index: number) => void;
}

// One row in the manual meal entry form: a food name, a source toggle
// (reference food + weight, or macros entered directly), and the fields for
// whichever source is selected.
export default function ManualItemEditor({ index, item, onChange, onRemove }: Props) {
  const [query, setQuery] = useState(item.name);
  const [results, setResults] = useState<FoodSearchResult[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [selected, setSelected] = useState<FoodSearchResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [translatedQuery, setTranslatedQuery] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<string | null>(null);

  const update = (patch: Partial<ManualMealItemInput>) => onChange(index, { ...item, ...patch });

  // See ItemResolver's requestSeq for the same stale-response-drops contract
  // — kept in sync since both search food the same way.
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
        if (selected) {
          setSelected(null);
          update({ fdc_id: undefined, custom_food_id: undefined });
        }
      }
    } catch (err) {
      if (seq === requestSeq.current) {
        setError(err instanceof Error ? err.message : 'Search failed');
      }
    } finally {
      setSearching(false);
    }
  };

  // See ItemResolver's refresh for the same fresh-results/preserve-banner
  // contract — kept in sync since both search food the same way.
  const refresh = async () => {
    const seq = ++requestSeq.current;
    setRefreshing(true);
    setRefreshError(null);
    try {
      const res = await api.searchFood(query, undefined, undefined, true);
      if (seq === requestSeq.current) {
        setResults(res.results);
        if (selected) {
          setSelected(null);
          update({ fdc_id: undefined, custom_food_id: undefined });
        }
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

  const pick = (r: FoodSearchResult) => {
    setSelected(r);
    update({ name: query, fdc_id: r.fdc_id, custom_food_id: r.custom_food_id });
  };

  return (
    <div className="border border-border bg-bg rounded-2xl p-4 sm:p-5">
      <div className="flex justify-between items-start gap-2 mb-2">
        <input
          type="text"
          placeholder="Food name"
          value={query}
          onChange={e => {
            setQuery(e.target.value);
            setSelected(null);
            update({ name: e.target.value, fdc_id: undefined, custom_food_id: undefined });
          }}
          className="min-w-0 flex-1 border border-border rounded-xl px-3 py-3 text-base bg-bg-elevated text-text"
        />
        <TapTarget
          onClick={() => onRemove(index)}
          className="flex items-center justify-center rounded text-text-muted hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 text-base"
          aria-label="Remove item"
        >
          ✕
        </TapTarget>
      </div>

      <div className="flex gap-4 mb-2 border-b border-border text-xs" role="tablist" aria-label={`Item resolution mode for ${item.name || `item ${index + 1}`}`}>
        <TapTarget
          role="tab"
          aria-selected={item.source === 'reference'}
          onClick={() => update({ source: 'reference' })}
          className={tabClass(item.source === 'reference', 'border-accent text-accent')}
        >
          Search food
        </TapTarget>
        <TapTarget
          role="tab"
          aria-selected={item.source === 'manual'}
          onClick={() => update({ source: 'manual' })}
          className={tabClass(item.source === 'manual', 'border-accent text-accent')}
        >
          Enter macros
        </TapTarget>
      </div>

      {item.source === 'reference' ? (
        <div>
          <div className="flex gap-2 items-center">
            <input
              type="number"
              step="any"
              placeholder="grams"
              value={item.weight_grams ?? ''}
              onChange={e => update({ weight_grams: Number(e.target.value) })}
              className="flex-1 min-w-0 border border-border rounded-xl px-2 py-1 text-base bg-bg-elevated text-text"
            />
            <span className="text-xs text-text-muted">g</span>
            <TapTarget
              onClick={search}
              disabled={searching || !query}
              className="px-3 rounded-xl text-xs font-medium bg-accent hover:bg-accent/90 text-bg-elevated disabled:opacity-50"
            >
              {searching ? 'Searching…' : 'Search'}
            </TapTarget>
          </div>
          <p className="mt-1 text-[10px] text-text-muted">
            New search terms may be sent to an external model provider for translation.
          </p>
          {translatedQuery && (
            <div className="mt-1 flex items-center gap-1.5 text-[11px] text-accent">
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
          {error && <p className="mt-1 text-[11px] text-red-600 dark:text-red-400">{error}</p>}
          {selected && (
            <p className="mt-1 text-xs text-green-700 dark:text-green-400">
              Bound to {selected.name} ({Math.round(selected.profile.calories_per_100g)} kcal/100g)
            </p>
          )}
          {results && !selected && (
            <ul className="mt-2 max-h-56 overflow-auto divide-y divide-border">
              {results.length === 0 && <li className="py-1 text-xs text-text-muted">No matches.</li>}
              {results.map((r, i) => (
                <li key={i}>
                  <TapTarget
                    onClick={() => pick(r)}
                    className="w-full flex items-center text-left text-xs text-text hover:text-accent"
                  >
                    {r.name} ({Math.round(r.profile.calories_per_100g)} kcal/100g)
                  </TapTarget>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {(
            [
              ['calories', 'Calories'], ['protein_grams', 'Protein (g)'], ['carbs_grams', 'Carbs (g)'],
              ['fat_grams', 'Fat (g)'], ['sugar_grams', 'Sugar (g)'], ['sodium_grams', 'Sodium (g)'],
              ['dietary_fiber_grams', 'Fiber (g)'], ['saturated_fat_grams', 'Saturated fat (g)'],
            ] as const
          ).map(([key, label]) => (
            <label key={key} className="text-xs text-text-muted">
              {label}
              <input
                type="number"
                step="any"
                value={item[key] ?? 0}
                onChange={e => update({ [key]: Number(e.target.value) } as Partial<ManualMealItemInput>)}
                className="mt-0.5 w-full border border-border rounded-xl px-2 py-1 text-base bg-bg-elevated text-text"
              />
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
