'use client';
import { useState } from 'react';
import { api, FoodMeal, FoodSearchResult } from '@/lib/api';
import ItemResolver from './ItemResolver';

interface Props {
  mealId: string;
  onAdded: (meal: FoodMeal) => void;
}

// Reuses ItemResolver's search/manual flows (see MealItemRow), pointed at
// POST .../items instead of PATCH .../items/{item_id} — a new item has no
// existing weight to fall back on, so this collects one directly.
export default function AddItemForm({ mealId, onAdded }: Props) {
  const [open, setOpen] = useState(false);
  const [weight, setWeight] = useState(100);
  const [error, setError] = useState<string | null>(null);
  // ItemResolver already guards against a double-click on the same result
  // firing two POSTs (see its own submitting state), but Cancel is this
  // component's own button, outside ItemResolver's awareness — without this,
  // Cancel could hide the form while a create is still in flight, and the
  // item would still appear in the meal once it resolves after the form (and
  // any error) is no longer visible.
  const [creating, setCreating] = useState(false);

  const handleBind = async (r: FoodSearchResult) => {
    setError(null);
    setCreating(true);
    try {
      const updated = await api.createMealItem(mealId, {
        name: r.name,
        fdc_id: r.fdc_id,
        custom_food_id: r.custom_food_id,
        weight_grams: weight,
      });
      onAdded(updated);
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add item');
    } finally {
      setCreating(false);
    }
  };

  const handleManual = async (
    name: string,
    macros: Omit<Parameters<typeof api.createMealItem>[1], 'manual' | 'name' | 'weight_grams'>
  ) => {
    setError(null);
    setCreating(true);
    try {
      const updated = await api.createMealItem(mealId, { manual: true, name, weight_grams: weight, ...macros });
      onAdded(updated);
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add item');
    } finally {
      setCreating(false);
    }
  };

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="mt-4 w-full py-2.5 rounded-lg text-sm font-medium border border-dashed border-gray-300 dark:border-gray-600 text-gray-500 dark:text-gray-400 hover:border-blue-400 hover:text-blue-600 dark:hover:text-blue-400"
      >
        + Add item
      </button>
    );
  }

  return (
    <div className="mt-4 bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-4">
      <div className="flex items-center justify-between mb-2">
        <p className="text-sm font-medium text-gray-900 dark:text-white">Add item</p>
        <div className="flex items-center gap-1">
          <input
            type="number"
            step="any"
            value={weight}
            onChange={e => setWeight(Number(e.target.value))}
            className="w-20 border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 text-sm text-right bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
          />
          <span className="text-xs text-gray-400">g</span>
        </div>
      </div>
      <ItemResolver itemName="" onBind={handleBind} onManual={handleManual} />
      {error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>}
      <button
        onClick={() => setOpen(false)}
        disabled={creating}
        className="mt-2 text-xs text-gray-500 dark:text-gray-400 hover:underline disabled:opacity-50"
      >
        Cancel
      </button>
    </div>
  );
}
