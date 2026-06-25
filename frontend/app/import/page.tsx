'use client';
import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';

const TYPE_LABELS: Record<string, string> = {
  heart_rate: 'Heart Rate',
  steps: 'Steps',
  sleep: 'Sleep',
  exercise: 'Exercise',
  distance: 'Distance',
  total_calories: 'Total Calories',
  oxygen_saturation: 'Oxygen Saturation',
  speed: 'Speed',
};

export default function ImportPage() {
  const router = useRouter();
  const fileRef = useRef<HTMLInputElement>(null);
  const [loading, setLoading] = useState(false);
  const [counts, setCounts] = useState<Record<string, number> | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleImport = async () => {
    const file = fileRef.current?.files?.[0];
    if (!file) return;

    setLoading(true);
    setError(null);
    setCounts(null);

    try {
      const result = await api.importHealthConnect(file);
      setCounts(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="max-w-2xl mx-auto flex justify-between items-center">
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">HealthVault</h1>
          <button
            onClick={() => router.push('/')}
            className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 font-medium"
          >
            ← Dashboard
          </button>
        </div>
      </header>

      <main className="max-w-2xl mx-auto px-6 py-10">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-1">Import Health Connect</h2>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-8">
          Upload the zip archive exported from Android&apos;s Health Connect app to import your health history.
        </p>

        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-6">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Health Connect archive (.zip)
          </label>
          <input
            ref={fileRef}
            type="file"
            accept=".zip"
            className="block w-full text-sm text-gray-700 dark:text-gray-300
              file:mr-4 file:py-2 file:px-4
              file:rounded-lg file:border-0
              file:text-sm file:font-medium
              file:bg-blue-50 file:text-blue-700
              dark:file:bg-blue-900 dark:file:text-blue-200
              hover:file:bg-blue-100 dark:hover:file:bg-blue-800
              file:cursor-pointer cursor-pointer"
          />

          <button
            onClick={handleImport}
            disabled={loading}
            className={`mt-4 w-full py-2.5 rounded-lg text-sm font-medium transition-all ${
              loading
                ? 'bg-gray-100 dark:bg-gray-700 text-gray-400 dark:text-gray-500 cursor-not-allowed'
                : 'bg-blue-600 hover:bg-blue-700 text-white'
            }`}
          >
            {loading ? 'Importing…' : 'Import'}
          </button>
        </div>

        {error && (
          <div className="mt-6 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-xl p-4">
            <p className="text-sm font-medium text-red-700 dark:text-red-400">Import failed</p>
            <p className="text-sm text-red-600 dark:text-red-300 mt-1 font-mono break-all">{error}</p>
          </div>
        )}

        {counts && (
          <div className="mt-6 bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-100 dark:border-gray-700">
              <p className="text-sm font-semibold text-gray-900 dark:text-white">Import complete</p>
            </div>
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-gray-50 dark:bg-gray-900/50">
                  <th className="text-left px-6 py-3 font-medium text-gray-500 dark:text-gray-400">Type</th>
                  <th className="text-right px-6 py-3 font-medium text-gray-500 dark:text-gray-400">Records</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
                {Object.entries(counts).map(([key, count]) => (
                  <tr key={key}>
                    <td className="px-6 py-3 text-gray-700 dark:text-gray-300">
                      {TYPE_LABELS[key] ?? key.replace(/_/g, ' ')}
                    </td>
                    <td className="px-6 py-3 text-right font-mono text-gray-900 dark:text-white">
                      {count.toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </div>
  );
}
