'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, DATA_TYPES } from '@/lib/api';
import { metricColorVar } from '@/lib/tokens';
import { PRIMARY_METRICS, extractVital, VitalResult } from '@/lib/vitals';
import Header from '@/components/Header';
import VitalCard from '@/components/VitalCard';
import { CameraIcon, PencilIcon, HistoryIcon } from '@/components/icons';

const SECONDARY_TYPES = DATA_TYPES.filter(t => !PRIMARY_METRICS.some(m => m.type === t));

export default function Dashboard() {
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const [vitals, setVitals] = useState<Record<string, VitalResult | null>>({});

  useEffect(() => {
    api.me()
      .then(() => setReady(true))
      .catch(() => router.push('/login'));
  }, [router]);

  useEffect(() => {
    if (!ready) return;
    const from = (() => {
      const d = new Date();
      d.setDate(d.getDate() - 7);
      d.setUTCHours(0, 0, 0, 0);
      return d.toISOString();
    })();
    const to = new Date().toISOString();

    Promise.all(
      PRIMARY_METRICS.map(m => api.data(m.type, from, to, undefined, 'day').catch(() => []))
    ).then(results => {
      const next: Record<string, VitalResult | null> = {};
      PRIMARY_METRICS.forEach((m, i) => {
        next[m.type] = extractVital(m.type, results[i]);
      });
      setVitals(next);
    });
  }, [ready]);

  return (
    <div className="min-h-screen bg-bg">
      <Header />

      <main className="max-w-4xl mx-auto px-6 py-8">
        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          Vitals · last 7 days
        </p>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 mb-8">
          {PRIMARY_METRICS.map(m => (
            <VitalCard key={m.type} type={m.type} label={m.label} result={ready ? vitals[m.type] ?? null : null} />
          ))}
        </div>

        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          Log food
        </p>
        <div className="flex gap-2.5 mb-8">
          <a
            href="/food/upload/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <CameraIcon className="w-4 h-4 text-accent" />
            Photo
          </a>
          <a
            href="/food/manual/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <PencilIcon className="w-4 h-4 text-accent" />
            Manual
          </a>
          <a
            href="/food/history/"
            className="flex-1 bg-bg-elevated border border-border rounded-[10px] p-4 flex items-center justify-center gap-2 hover:border-accent transition-colors text-sm font-semibold text-text"
          >
            <HistoryIcon className="w-4 h-4 text-accent" />
            History
          </a>
        </div>

        <p className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide text-accent mb-3">
          More data
        </p>
        <div className="flex flex-wrap gap-2">
          {SECONDARY_TYPES.map(t => (
            <a
              key={t}
              href={`/data/${t}/`}
              className="font-[family-name:var(--font-data)] text-[11px] font-bold uppercase tracking-wide px-2.5 py-1.5 rounded-lg border border-border bg-bg-elevated hover:border-accent transition-colors flex items-center gap-1.5"
              style={{ color: metricColorVar(t) }}
            >
              <span className="w-1.5 h-1.5 rounded-full" style={{ background: metricColorVar(t) }} />
              {t.replace(/_/g, ' ')}
            </a>
          ))}
        </div>
      </main>
    </div>
  );
}
