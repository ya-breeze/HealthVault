'use client';
import { useEffect, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts';
import { api } from '@/lib/api';

interface Props {
  type: string;
}

export default function DataTypeClient({ type }: Props) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const userParam = searchParams.get('user') ?? undefined;

  const [records, setRecords] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const [from, setFrom] = useState(() => {
    const d = new Date();
    d.setDate(d.getDate() - 7);
    return d.toISOString().slice(0, 10);
  });
  const [to, setTo] = useState(() => new Date().toISOString().slice(0, 10));

  useEffect(() => {
    setLoading(true);
    api.data(type, `${from}T00:00:00Z`, `${to}T23:59:59Z`, userParam)
      .then(data => {
        setRecords(data);
        setLoading(false);
      })
      .catch(() => router.push('/login'));
  }, [type, from, to, userParam, router]);

  const numericKey = records.length > 0
    ? Object.entries(records[0]).find(([k, v]) =>
        typeof v === 'number' && !k.endsWith('_id') && k !== 'id'
      )?.[0]
    : undefined;

  const timeKey = records.length > 0
    ? (['time', 'start_time', 'timestamp'].find(k => k in records[0]))
    : undefined;

  const displayColumns = records.length > 0
    ? Object.keys(records[0]).filter(
        k => !['id', 'family_id', 'user_id', 'source_payload_id', 'deleted_at'].includes(k)
      )
    : [];

  const fromMs = new Date(`${from}T00:00:00Z`).getTime();
  const toMs = new Date(`${to}T23:59:59Z`).getTime();

  const chartData = timeKey
    ? records.map(r => ({ ...r, [timeKey]: new Date(r[timeKey] as string).getTime() }))
    : records;

  const handleConfirmDelete = async (id: string) => {
    try {
      await api.deleteRecord(type, id);
      setRecords(prev => prev.filter(r => r.id !== id));
    } finally {
      setPendingDeleteId(null);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="max-w-4xl mx-auto flex items-center gap-4">
          <a href="/" className="text-blue-600 dark:text-blue-400 hover:underline text-sm">&#8592; Dashboard</a>
          <h1 className="text-xl font-bold capitalize text-gray-900 dark:text-white">{type.replace(/_/g, ' ')}</h1>
        </div>
      </header>

      <main className="max-w-4xl mx-auto px-6 py-8">
        <div className="flex gap-3 mb-6">
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            From{' '}
            <input
              type="date"
              value={from}
              onChange={e => setFrom(e.target.value)}
              className="border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </label>
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            To{' '}
            <input
              type="date"
              value={to}
              onChange={e => setTo(e.target.value)}
              className="border border-gray-300 dark:border-gray-600 rounded-md px-2 py-1 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </label>
        </div>

        {numericKey && timeKey && records.length > 0 && (
          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-4 mb-6">
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#374151" opacity={0.3} />
                <XAxis
                  dataKey={timeKey}
                  type="number"
                  scale="time"
                  domain={[fromMs, toMs]}
                  tickFormatter={(v: number) => new Date(v).toLocaleDateString()}
                  tick={{ fill: '#9ca3af', fontSize: 12 }}
                />
                <YAxis tick={{ fill: '#9ca3af', fontSize: 12 }} />
                <Tooltip
                  labelFormatter={(v: unknown) => new Date(v as number).toLocaleString()}
                  contentStyle={{
                    backgroundColor: 'var(--color-background, #1f2937)',
                    border: '1px solid #374151',
                    borderRadius: '8px',
                    color: '#f9fafb',
                  }}
                />
                <Line type="monotone" dataKey={numericKey} stroke="#3b82f6" dot={true} strokeWidth={2} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}

        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 overflow-auto">
          {loading ? (
            <p className="p-6 text-gray-500 dark:text-gray-400 text-center text-sm">Loading...</p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-700 border-b border-gray-200 dark:border-gray-600">
                <tr>
                  {displayColumns.map(k => (
                    <th key={k} className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300 text-xs uppercase tracking-wider">
                      {k}
                    </th>
                  ))}
                  <th className="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300 text-xs uppercase tracking-wider">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
                {records.map((r, i) => {
                  const id = r.id as string;
                  const isPending = id === pendingDeleteId;
                  return (
                    <tr
                      key={i}
                      className={isPending
                        ? 'bg-red-50 dark:bg-red-900/20'
                        : 'hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors'}
                    >
                      {displayColumns.map(k => (
                        <td key={k} className="px-4 py-3 text-gray-900 dark:text-gray-200">
                          {typeof r[k] === 'string' && (r[k] as string).includes('T')
                            ? new Date(r[k] as string).toLocaleString()
                            : String(r[k] ?? '')}
                        </td>
                      ))}
                      <td className="px-4 py-3">
                        {isPending ? (
                          <span className="flex items-center gap-2">
                            <button
                              onClick={() => handleConfirmDelete(id)}
                              className="text-xs px-2 py-1 rounded bg-red-600 text-white hover:bg-red-700"
                            >
                              Confirm
                            </button>
                            <button
                              onClick={() => setPendingDeleteId(null)}
                              className="text-xs px-2 py-1 rounded bg-gray-200 dark:bg-gray-600 text-gray-800 dark:text-gray-200 hover:bg-gray-300 dark:hover:bg-gray-500"
                            >
                              Cancel
                            </button>
                          </span>
                        ) : (
                          <button
                            onClick={() => setPendingDeleteId(id)}
                            aria-label="Delete record"
                            className="text-gray-400 hover:text-red-500 transition-colors"
                          >
                            🗑
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          {!loading && records.length === 0 && (
            <p className="p-6 text-gray-500 dark:text-gray-400 text-center text-sm">No data in this range.</p>
          )}
        </div>
      </main>
    </div>
  );
}
