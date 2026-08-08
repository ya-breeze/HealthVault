'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, ManualMealItemInput } from '@/lib/api';
import ManualItemEditor from '@/components/food/ManualItemEditor';
import Header from '@/components/Header';

function emptyItem(): ManualMealItemInput {
  return { name: '', source: 'reference' };
}

export default function ManualMealPage() {
  const router = useRouter();
  const [name, setName] = useState('');
  const [loggedAt, setLoggedAt] = useState(() => new Date().toISOString().slice(0, 16));
  const [items, setItems] = useState<ManualMealItemInput[]>([emptyItem()]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const updateItem = (index: number, item: ManualMealItemInput) =>
    setItems(prev => prev.map((it, i) => (i === index ? item : it)));

  const removeItem = (index: number) => setItems(prev => prev.filter((_, i) => i !== index));

  const handleSubmit = async () => {
    setError(null);
    if (items.length === 0) {
      setError('Add at least one item.');
      return;
    }
    setSaving(true);
    try {
      const meal = await api.createManualMeal({
        name: name || undefined,
        logged_at: new Date(loggedAt).toISOString(),
        items,
      });
      router.push(`/food/review/?meal=${meal.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save meal');
      setSaving(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <Header />

      <main className="max-w-md mx-auto px-6 py-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Log a Meal Manually</h1>
        <div className="flex gap-2 mb-4">
          <label className="flex-1 text-sm text-gray-700 dark:text-gray-300">
            Name (optional)
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              className="mt-1 w-full border border-gray-300 dark:border-gray-600 rounded-md px-3 py-2 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
          </label>
          <label className="text-sm text-gray-700 dark:text-gray-300">
            When
            <input
              type="datetime-local"
              value={loggedAt}
              onChange={e => setLoggedAt(e.target.value)}
              className="mt-1 border border-gray-300 dark:border-gray-600 rounded-md px-2 py-2 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
          </label>
        </div>

        <div className="flex flex-col gap-3 mb-4">
          {items.map((item, i) => (
            <ManualItemEditor key={i} index={i} item={item} onChange={updateItem} onRemove={removeItem} />
          ))}
        </div>

        <button
          onClick={() => setItems(prev => [...prev, emptyItem()])}
          className="w-full mb-4 py-2 rounded-lg text-sm font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600"
        >
          + Add item
        </button>

        {error && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <button
          onClick={handleSubmit}
          disabled={saving}
          className="w-full py-2.5 rounded-lg text-sm font-medium bg-green-600 hover:bg-green-700 text-white disabled:opacity-50"
        >
          {saving ? 'Saving…' : 'Save Meal'}
        </button>
      </main>
    </div>
  );
}
