'use client';
import { useRef, useState } from 'react';
import { api } from '@/lib/api';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import { useLanguage } from '@/components/LanguageContext';
import { numberLocaleFor, type Dictionary } from '@/lib/i18n';

const TYPE_LABELS: Partial<Record<string, keyof Dictionary>> = {
  heart_rate: 'metric.heart_rate', steps: 'metric.steps', sleep: 'metric.sleep',
  exercise: 'metric.exercise', distance: 'metric.distance', total_calories: 'metric.total_calories',
  oxygen_saturation: 'metric.oxygen_saturation', speed: 'metric.speed',
  weight: 'metric.weight', body_fat: 'metric.body_fat',
};

interface ImportCardProps {
  title: string;
  description: string;
  accept: string;
  label: string;
  onImport: (file: File) => Promise<Record<string, number>>;
}

function ImportCard({ title, description, accept, label, onImport }: ImportCardProps) {
  const { t, language } = useLanguage();
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
      const result = await onImport(file);
      setCounts(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('redesign.pages.import.failed'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <section className="hv-card min-w-0">
      <h2 className="text-lg font-semibold text-text mb-2">{title}</h2>
      <p className="text-sm text-text-muted leading-relaxed mb-6">{description}</p>

      <div className="rounded-2xl border border-dashed border-border bg-bg p-5">
        <label className="block text-sm font-medium text-text mb-3">
          {label}
        <input
          ref={fileRef}
          type="file"
          accept={accept}
          className="mt-3 block min-w-0 w-full text-sm text-text-muted
            file:mr-3 file:py-3 file:px-4
            file:rounded-xl file:border-0
            file:text-sm file:font-semibold
            file:bg-bg-elevated file:text-accent
            hover:file:opacity-80
            file:cursor-pointer cursor-pointer"
        />
        </label>

        <button
          onClick={handleImport}
          disabled={loading}
          className={`mt-4 min-h-12 w-full py-3 rounded-xl text-sm font-semibold transition-all ${
            loading
              ? 'bg-border text-text-muted cursor-not-allowed'
              : 'bg-accent hover:opacity-90 text-bg-elevated'
          }`}
        >
          {loading ? t('redesign.pages.import.importing') : t('redesign.pages.import.submit')}
        </button>
      </div>

      {error && (
        <div role="alert" className="mt-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-xl p-4">
          <p className="text-sm font-medium text-red-700 dark:text-red-400">{t('redesign.pages.import.failed')}</p>
          <p className="text-sm text-red-600 dark:text-red-300 mt-1 font-mono break-all">{error}</p>
        </div>
      )}

      {counts && (
        <div className="mt-5 bg-bg-elevated rounded-xl border border-border overflow-auto">
          <div className="px-5 py-4 border-b border-border">
            <p role="status" className="text-sm font-semibold text-text">{t('redesign.pages.import.complete')}</p>
          </div>
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-bg">
                <th className="text-left px-5 py-3 font-medium text-text-muted">{t('redesign.pages.import.type')}</th>
                <th className="text-right px-5 py-3 font-medium text-text-muted">{t('redesign.pages.import.records')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {Object.entries(counts).map(([key, count]) => (
                <tr key={key}>
                  <td className="px-5 py-3 text-text">
                    {TYPE_LABELS[key] ? t(TYPE_LABELS[key]) : key.replace(/_/g, ' ')}
                  </td>
                  <td className="px-5 py-3 text-right tabular-nums text-text">
                    {count.toLocaleString(numberLocaleFor(language))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

export default function ImportPage() {
  const { t } = useLanguage();
  return (
    <AuthenticatedShell className="min-h-screen bg-bg">
      <main className="hv-page">
        <div className="hv-page-heading"><div><h1>{t('redesign.pages.import.title')}</h1><p className="text-text-muted mt-2">{t('redesign.pages.import.subtitle')}</p></div></div>
        <div className="grid gap-6 xl:grid-cols-2 items-start">
        <ImportCard
          title={t('redesign.pages.import.healthConnectTitle')}
          description={t('redesign.pages.import.healthConnectDescription')}
          accept=".zip"
          label={t('redesign.pages.import.healthConnectFile')}
          onImport={api.importHealthConnect}
        />

        <ImportCard
          title={t('redesign.pages.import.libraTitle')}
          description={t('redesign.pages.import.libraDescription')}
          accept=".csv"
          label={t('redesign.pages.import.libraFile')}
          onImport={api.importLibra}
        />
        </div>
      </main>
    </AuthenticatedShell>
  );
}
