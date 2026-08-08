'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api, CustomFood, CustomFoodInput } from '@/lib/api';
import CustomFoodModal from '@/components/food/CustomFoodModal';

export default function CustomFoodsPage() {
  const router = useRouter();
  const [foods, setFoods] = useState<CustomFood[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<CustomFood | 'new' | null>(null);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);

  const load = () => {
    api.listCustomFoods()
      .then(setFoods)
      .catch(err => {
        if (err instanceof Error && err.message.includes('401')) {
          router.push('/login');
          return;
        }
        setError(err instanceof Error ? err.message : 'Failed to load custom foods');
      });
  };

  useEffect(load, [router]);

  const handleSave = async (input: CustomFoodInput) => {
    if (editing === 'new') {
      await api.createCustomFood(input);
    } else if (editing) {
      await api.updateCustomFood(editing.id, input);
    }
    setEditing(null);
    load();
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteCustomFood(id);
      setFoods(prev => (prev ? prev.filter(f => f.id !== id) : prev));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setPendingDeleteId(null);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="max-w-md mx-auto flex items-center gap-4">
          <Link href="/" className="text-blue-600 dark:text-blue-400 hover:underline text-sm">
            &#8592; Dashboard
          </Link>
          <h1 className="text-xl font-bold text-gray-900 dark:text-white">Custom Foods</h1>
        </div>
      </header>

      <main className="max-w-md mx-auto px-6 py-6">
        <button
          onClick={() => setEditing('new')}
          className="w-full mb-4 py-2.5 rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white"
        >
          + Add Custom Food
        </button>

        {error && <p className="mb-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 divide-y divide-gray-100 dark:divide-gray-700">
          {foods === null && (
            <p className="p-4 text-sm text-gray-500 dark:text-gray-400 text-center">Loading…</p>
          )}
          {foods?.length === 0 && (
            <p className="p-4 text-sm text-gray-500 dark:text-gray-400 text-center">No custom foods yet.</p>
          )}
          {foods?.map(f => (
            <div key={f.id} className="p-4 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-white">{f.name}</p>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  {Math.round(f.calories_per_100g)} kcal/100g
                </p>
              </div>
              <div className="flex items-center gap-3">
                <button
                  onClick={() => setEditing(f)}
                  className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
                >
                  Edit
                </button>
                {pendingDeleteId === f.id ? (
                  <span className="flex items-center gap-1">
                    <button
                      onClick={() => handleDelete(f.id)}
                      className="text-xs px-2 py-1 rounded bg-red-600 text-white hover:bg-red-700"
                    >
                      Confirm
                    </button>
                    <button
                      onClick={() => setPendingDeleteId(null)}
                      className="text-xs px-2 py-1 rounded bg-gray-200 dark:bg-gray-600 text-gray-800 dark:text-gray-200"
                    >
                      Cancel
                    </button>
                  </span>
                ) : (
                  <button
                    onClick={() => setPendingDeleteId(f.id)}
                    className="text-xs text-red-500 hover:text-red-700"
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </main>

      {editing && (
        <CustomFoodModal
          initial={editing === 'new' ? undefined : editing}
          onSave={handleSave}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}
