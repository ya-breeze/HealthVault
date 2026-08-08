'use client';
import { useState } from 'react';
import { CustomFood, CustomFoodInput } from '@/lib/api';

interface Props {
  initial?: CustomFood;
  onSave: (input: CustomFoodInput) => Promise<void>;
  onClose: () => void;
}

const FIELDS: [keyof CustomFoodInput, string][] = [
  ['calories_per_100g', 'Calories'],
  ['protein_per_100g', 'Protein (g)'],
  ['carbs_per_100g', 'Carbs (g)'],
  ['fat_per_100g', 'Fat (g)'],
  ['sugar_per_100g', 'Sugar (g)'],
  ['sodium_per_100g', 'Sodium (g)'],
  ['dietary_fiber_per_100g', 'Fiber (g)'],
];

export default function CustomFoodModal({ initial, onSave, onClose }: Props) {
  const [name, setName] = useState(initial?.name ?? '');
  const [values, setValues] = useState<Record<string, number>>({
    calories_per_100g: initial?.calories_per_100g ?? 0,
    protein_per_100g: initial?.protein_per_100g ?? 0,
    carbs_per_100g: initial?.carbs_per_100g ?? 0,
    fat_per_100g: initial?.fat_per_100g ?? 0,
    sugar_per_100g: initial?.sugar_per_100g ?? 0,
    sodium_per_100g: initial?.sodium_per_100g ?? 0,
    dietary_fiber_per_100g: initial?.dietary_fiber_per_100g ?? 0,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = async () => {
    if (!name.trim()) {
      setError('Name is required');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSave({
        name,
        calories_per_100g: values.calories_per_100g,
        protein_per_100g: values.protein_per_100g,
        carbs_per_100g: values.carbs_per_100g,
        fat_per_100g: values.fat_per_100g,
        sugar_per_100g: values.sugar_per_100g,
        sodium_per_100g: values.sodium_per_100g,
        dietary_fiber_per_100g: values.dietary_fiber_per_100g,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed');
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-lg max-w-sm w-full p-6">
        <p className="text-sm font-semibold text-gray-900 dark:text-white mb-4">
          {initial ? 'Edit Custom Food' : 'New Custom Food'}
        </p>

        <label className="block mb-3">
          <span className="text-xs text-gray-600 dark:text-gray-300">Name</span>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            className="mt-1 w-full border border-gray-300 dark:border-gray-600 rounded-md px-3 py-2 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
          />
        </label>

        <p className="text-xs text-gray-500 dark:text-gray-400 mb-2">Per 100g</p>
        <div className="grid grid-cols-2 gap-2">
          {FIELDS.map(([key, label]) => (
            <label key={key} className="text-xs text-gray-600 dark:text-gray-300">
              {label}
              <input
                type="number"
                step="any"
                value={values[key]}
                onChange={e => setValues({ ...values, [key]: Number(e.target.value) })}
                className="mt-0.5 w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
              />
            </label>
          ))}
        </div>

        {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <div className="mt-5 flex gap-2">
          <button
            onClick={onClose}
            className="flex-1 py-2 rounded-lg text-sm font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex-1 py-2 rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  );
}
