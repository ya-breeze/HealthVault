'use client';
import { useState } from 'react';
import { api, FoodMeal, FoodSearchResult } from '@/lib/api';
import ItemResolver from './ItemResolver';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  mealId: string;
  // Takes a thunk, not an already-started request — see ReviewClient's
  // applyMealUpdate doc comment for why (issue order must be commit order,
  // enforced by the parent controlling when each request actually starts).
  onAdded: (issue: () => Promise<FoodMeal>, label?: string) => Promise<FoodMeal>;
  // Passed straight through to ItemResolver — this form sits on the review
  // screen, whose Expert Mode toggle governs every custom food listed on it,
  // including the ones in this form's own search results. This component's
  // own chrome stays English (see lib/i18n/en.ts's scope comment); only the
  // Canonical Name sub-labels on its results are affected.
  expertMode?: boolean;
}

// Reuses ItemResolver's search/manual flows (see MealItemRow), pointed at
// POST .../items instead of PATCH .../items/{item_id} — a new item has no
// existing weight to fall back on, so this collects one directly.
export default function AddItemForm({ mealId, onAdded, expertMode = false }: Props) {
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
      await onAdded(() => api.createMealItem(mealId, {
        name: r.name,
        fdc_id: r.fdc_id,
        custom_food_id: r.custom_food_id,
        weight_grams: weight,
      }), 'Item added');
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
      await onAdded(() => api.createMealItem(mealId, { manual: true, name, weight_grams: weight, ...macros }), 'Item added');
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add item');
    } finally {
      setCreating(false);
    }
  };

  if (!open) {
    return (
      <TapTarget
        onClick={() => setOpen(true)}
        className="mt-4 w-full rounded-xl text-sm font-medium border border-dashed border-border text-text-muted hover:border-accent hover:text-accent"
      >
        + Add item
      </TapTarget>
    );
  }

  return (
    <div className="mt-5 bg-bg-elevated rounded-2xl border border-border p-5">
      <div className="flex items-center justify-between mb-2">
        <p className="text-sm font-medium text-text">Add item</p>
        <div className="flex items-center gap-1">
          <input
            type="number"
            step="any"
            value={weight}
            onChange={e => setWeight(Number(e.target.value))}
            className="w-20 border border-border rounded-xl px-2 py-1 text-base text-right bg-bg-elevated text-text"
          />
          <span className="text-xs text-text-muted">g</span>
        </div>
      </div>
      <ItemResolver itemName="" onBind={handleBind} onManual={handleManual} expertMode={expertMode} />
      {error && <p className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>}
      <TapTarget
        onClick={() => setOpen(false)}
        disabled={creating}
        className="mt-2 flex items-center text-xs text-text-muted hover:underline disabled:opacity-50"
      >
        Cancel
      </TapTarget>
    </div>
  );
}
