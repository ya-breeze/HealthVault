'use client';
import { useState } from 'react';
import { api, FoodItem, FoodMeal, FoodSearchResult } from '@/lib/api';
import ItemResolver from './ItemResolver';

interface Props {
  mealId: string;
  item: FoodItem;
  onUpdated: (meal: FoodMeal) => void;
}

const SOURCE_LABEL: Record<string, string> = {
  reference: 'Matched',
  manual: 'Manual',
  none: 'Unresolved',
};

const SOURCE_COLOR: Record<string, string> = {
  reference: 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300',
  manual: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  none: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
};

// Item editing is reachable whenever this row is rendered — the parent only
// shows the items list for pending_review and confirmed meals, both of
// which the backend now accepts item mutations for (see PatchMealItem's
// editableMealStatus guard), so there is no read-only state to represent
// here anymore.
export default function MealItemRow({ mealId, item, onUpdated }: Props) {
  const [weight, setWeight] = useState(item.weight_grams);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resolving, setResolving] = useState(item.macro_source === 'none');

  const commitWeight = async () => {
    if (weight === item.weight_grams) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await api.patchMealItem(mealId, item.id, { weight_grams: weight });
      onUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update weight');
      setWeight(item.weight_grams);
    } finally {
      setSaving(false);
    }
  };

  const handleBind = async (r: FoodSearchResult) => {
    const updated = await api.patchMealItem(mealId, item.id, {
      fdc_id: r.fdc_id,
      custom_food_id: r.custom_food_id,
      weight_grams: weight,
      name: r.name,
    });
    onUpdated(updated);
    setResolving(false);
  };

  const handleManual = async (
    name: string,
    macros: Omit<Parameters<typeof api.patchMealItem>[2], 'manual' | 'name'>
  ) => {
    const updated = await api.patchMealItem(mealId, item.id, { manual: true, name, ...macros });
    onUpdated(updated);
    setResolving(false);
  };

  const handleDelete = async () => {
    setDeleting(true);
    setError(null);
    try {
      const updated = await api.deleteMealItem(mealId, item.id);
      onUpdated(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete item');
      setDeleting(false);
    }
  };

  return (
    <div className="py-3 border-b border-gray-100 dark:border-gray-700 last:border-b-0">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-gray-900 dark:text-white break-words">{item.name}</p>
          <span className={`inline-block mt-1 text-[11px] px-1.5 py-0.5 rounded ${SOURCE_COLOR[item.macro_source]}`}>
            {SOURCE_LABEL[item.macro_source]}
          </span>
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          <input
            type="number"
            step="any"
            value={weight}
            disabled={saving || deleting}
            onChange={e => setWeight(Number(e.target.value))}
            onBlur={commitWeight}
            className="w-20 border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 text-sm text-right bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 disabled:opacity-60"
          />
          <span className="text-xs text-gray-400">g</span>
          <button
            onClick={handleDelete}
            disabled={deleting}
            title="Delete item"
            className="ml-1 text-gray-400 hover:text-red-600 dark:hover:text-red-400 disabled:opacity-50"
          >
            ×
          </button>
        </div>
      </div>

      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {Math.round(item.calories)} kcal · P {item.protein_grams.toFixed(1)}g · C {item.carbs_grams.toFixed(1)}g · F{' '}
        {item.fat_grams.toFixed(1)}g
      </div>

      {!resolving && (
        <button
          onClick={() => setResolving(true)}
          className={
            item.macro_source === 'none'
              ? 'mt-2 text-xs font-medium text-amber-700 dark:text-amber-400 hover:underline'
              : 'mt-2 text-xs font-medium text-gray-500 dark:text-gray-400 hover:underline'
          }
        >
          {item.macro_source === 'none' ? 'Resolve this item' : 'Change match'}
        </button>
      )}
      {resolving && <ItemResolver itemName={item.name} onBind={handleBind} onManual={handleManual} />}

      {error && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
