'use client';
import { useEffect, useState } from 'react';
import { api, ApiError, FoodItem, FoodMeal, FoodSearchResult } from '@/lib/api';
import ItemResolver from './ItemResolver';
import TapTarget from '@/components/ui/TapTarget';
import CanonicalNameLabel from '@/components/food/CanonicalNameLabel';
import { useLanguage } from '@/components/LanguageContext';

interface Props {
  mealId: string;
  item: FoodItem;
  // Takes a thunk, not an already-started request — see ReviewClient's
  // applyMealUpdate doc comment for why (issue order must be commit order,
  // enforced by the parent controlling when each request actually starts).
  onUpdated: (issue: () => Promise<FoodMeal>, label?: string) => Promise<FoodMeal>;
  // See openspec/specs/expert-mode "Per-Screen Expert Mode Toggle" — held by
  // the parent screen, not this row, so every row on the screen reflects the
  // same toggle state.
  expertMode: boolean;
}

const SOURCE_COLOR: Record<string, string> = {
  reference: 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300',
  manual: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  // Distinct from both "manual" (a value the user typed) and "none"
  // (nothing at all) — an estimated item has real macro values, but they're
  // a model guess, not a database fact or something the user confirmed.
  estimated: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  none: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300',
};

// Which reference field a "reference"-sourced item is actually bound to —
// distinct from macro_source, which only says "reference" vs "manual" vs
// "none" and doesn't say which of USDA/Open Food Facts/a custom food it
// came from. Shown alongside the existing macro_source badge so a Czech
// product match (real label data) is visibly different from a generic USDA
// estimate when confirming a meal.
function referenceSourceLabel(item: FoodItem): string | null {
  if (item.off_code) return 'Open Food Facts';
  if (item.fdc_id) return 'USDA';
  if (item.custom_food_id) return 'Your custom food';
  return null;
}

// Item editing is reachable whenever this row is rendered — the parent only
// shows the items list for pending_review and confirmed meals, both of
// which the backend now accepts item mutations for (see PatchMealItem's
// editableMealStatus guard), so there is no read-only state to represent
// here anymore.
export default function MealItemRow({ mealId, item, onUpdated, expertMode }: Props) {
  const { t } = useLanguage();
  const SOURCE_LABEL: Record<string, string> = {
    reference: t('item.sourceReference'),
    manual: t('item.sourceManual'),
    estimated: t('item.sourceEstimated'),
    none: t('item.sourceNone'),
  };
  const [weight, setWeight] = useState(item.weight_grams);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resolving, setResolving] = useState(item.macro_source === 'none');

  // Keep the weight draft in sync with the authoritative item prop. A
  // sibling request for this same item (e.g. a bind PATCH from ItemResolver
  // winning a race against this row's own in-flight weight PATCH — see
  // commitWeight's 409 handling below) can change item.weight_grams from
  // outside this component; useState's initial value is only used on first
  // render, so without this the input would keep showing whatever it held
  // before the win, not the food that's actually now bound.
  useEffect(() => {
    setWeight(item.weight_grams);
  }, [item.weight_grams]);

  const commitWeight = async () => {
    if (weight === item.weight_grams) return;
    // Mirrors PatchMealItem's server-side check: a non-positive weight on a
    // reference- or estimated-sourced item would rescale it to negative or
    // zero macros (see ApplyProfile/ApplyEstimatedProfile), and the backend
    // now rejects both. Only applies to those two sources — a manual item's
    // weight is metadata, not a scale factor, so it's not restricted here.
    if ((item.macro_source === 'reference' || item.macro_source === 'estimated') && weight <= 0) {
      setError(`Weight must be positive for a${item.macro_source === 'estimated' ? 'n estimated' : ' matched'} item`);
      setWeight(item.weight_grams);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onUpdated(() => api.patchMealItem(mealId, item.id, { weight_grams: weight }), 'Item updated');
    } catch (err) {
      // Checks both instanceof and the explicit .name tag — see ApiError's
      // doc comment in lib/api.ts for why instanceof alone isn't trusted.
      const isConflict =
        (err instanceof ApiError || (err as { name?: string })?.name === 'ApiError') &&
        (err as ApiError).status === 409;
      if (isConflict) {
        // This request's own view of the item was stale by the time it
        // landed — something else changed it first. That doesn't mean the
        // item prop this component already has is current: the winning
        // edit might have come from another browser tab, another device, or
        // a direct API call, none of which trigger a sibling onUpdated call
        // in this page — so the prop-sync effect above may never run.
        // Refetch explicitly (through onUpdated, so it's ordered against
        // any other sibling mutation too) and only claim "current value"
        // once that actually lands; a refetch failure gets its own distinct
        // warning rather than a false claim.
        try {
          await onUpdated(() => api.getMeal(mealId), 'Refreshed with latest change');
          setError('This item was just changed by another edit — showing its current value.');
        } catch {
          setError('This item was just changed by another edit, and refreshing failed — this view may be stale. Reload the page to see what changed.');
        }
      } else {
        setError(err instanceof Error ? err.message : 'Failed to update weight');
        setWeight(item.weight_grams);
      }
    } finally {
      setSaving(false);
    }
  };

  const handleBind = async (r: FoodSearchResult) => {
    // Binding always scales the chosen food's profile by the current weight
    // field — see the commitWeight comment above for why a non-positive
    // value can't be sent. Thrown here (not setError) since ItemResolver's
    // own try/catch around onBind is what surfaces it.
    if (weight <= 0) {
      throw new Error('Weight must be positive to match a food');
    }
    await onUpdated(() => api.patchMealItem(mealId, item.id, {
      fdc_id: r.fdc_id,
      custom_food_id: r.custom_food_id,
      weight_grams: weight,
      name: r.name,
    }), 'Item updated');
    setResolving(false);
  };

  const handleManual = async (
    name: string,
    macros: Omit<Parameters<typeof api.patchMealItem>[2], 'manual' | 'name' | 'save_as_custom_food'>,
    saveAsCustomFood: boolean
  ) => {
    await onUpdated(() => api.patchMealItem(mealId, item.id, {
      manual: true, name, save_as_custom_food: saveAsCustomFood, ...macros,
    }), 'Item updated');
    setResolving(false);
  };

  const handleDelete = async () => {
    setDeleting(true);
    setError(null);
    try {
      await onUpdated(() => api.deleteMealItem(mealId, item.id), 'Item removed');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete item');
      setDeleting(false);
      setConfirmingDelete(false);
    }
  };

  return (
    <div className="py-3 border-b border-gray-100 dark:border-gray-700 last:border-b-0">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-gray-900 dark:text-white break-words">{item.name}</p>
          <CanonicalNameLabel expertMode={expertMode} canonicalName={item.canonical_name} />
          <span className={`inline-block mt-1 text-[11px] px-1.5 py-0.5 rounded ${SOURCE_COLOR[item.macro_source]}`}>
            {SOURCE_LABEL[item.macro_source]}
          </span>
          {referenceSourceLabel(item) && (
            <span className="inline-block mt-1 ml-1 text-[11px] px-1.5 py-0.5 rounded bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300">
              {referenceSourceLabel(item)}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1 flex-shrink-0">
          <input
            type="number"
            step="any"
            min={0}
            value={weight}
            disabled={saving || deleting}
            onChange={e => setWeight(Number(e.target.value))}
            onBlur={commitWeight}
            className="w-20 border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 text-base text-right bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 disabled:opacity-60"
          />
          <span className="text-xs text-gray-400">g</span>
          {confirmingDelete ? (
            <div className="ml-1 flex items-center gap-1">
              <TapTarget
                onClick={handleDelete}
                disabled={deleting}
                className="px-2 rounded text-xs font-medium bg-red-600 hover:bg-red-700 text-white disabled:opacity-50"
              >
                {deleting ? '…' : t('item.confirm')}
              </TapTarget>
              <TapTarget
                onClick={() => setConfirmingDelete(false)}
                disabled={deleting}
                className="px-2 rounded text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 disabled:opacity-50"
              >
                {t('item.cancel')}
              </TapTarget>
            </div>
          ) : (
            <TapTarget
              onClick={() => setConfirmingDelete(true)}
              title={t('item.deleteTitle')}
              className="ml-1 flex items-center justify-center rounded text-lg leading-none text-gray-400 hover:text-red-600 hover:bg-red-50 dark:hover:text-red-400 dark:hover:bg-red-900/20"
            >
              ×
            </TapTarget>
          )}
        </div>
      </div>

      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {Math.round(item.calories)} kcal · P {item.protein_grams.toFixed(1)}g · C {item.carbs_grams.toFixed(1)}g · F{' '}
        {item.fat_grams.toFixed(1)}g
      </div>

      {!resolving && (
        <TapTarget
          onClick={() => setResolving(true)}
          className={
            item.macro_source === 'none' || item.macro_source === 'estimated'
              ? 'mt-2 flex items-center text-xs font-medium text-amber-700 dark:text-amber-400 hover:underline'
              : 'mt-2 flex items-center text-xs font-medium text-gray-500 dark:text-gray-400 hover:underline'
          }
        >
          {item.macro_source === 'none'
            ? t('item.resolve')
            : item.macro_source === 'estimated'
              ? t('item.verifyEstimate')
              : t('item.changeMatch')}
        </TapTarget>
      )}
      {resolving && (
        <ItemResolver itemName={item.name} onBind={handleBind} onManual={handleManual} allowSaveAsCustomFood />
      )}

      {error && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
