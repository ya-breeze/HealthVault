'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { numberLocaleFor } from '@/lib/i18n';
import { api, CustomFood, CustomFoodInput } from '@/lib/api';
import CustomFoodModal from '@/components/food/CustomFoodModal';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import ExpertModeToggle from '@/components/food/ExpertModeToggle';
import CanonicalNameLabel from '@/components/food/CanonicalNameLabel';

export default function CustomFoodsPage() {
  const router = useRouter();
  const { t, language } = useLanguage();
  const [query, setQuery] = useState('');
  const [foods, setFoods] = useState<CustomFood[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<CustomFood | 'new' | null>(null);
  const [pendingDeleteId, setPendingDeleteId] = useState<string | null>(null);
  const [expertMode, setExpertMode] = useState(false);

  const load = () => {
    api.listCustomFoods()
      .then(setFoods)
      .catch(err => {
        if (err instanceof Error && err.message.includes('401')) {
          router.push('/login');
          return;
        }
        setError(err instanceof Error ? err.message : t('customFood.loadFailed'));
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
      setError(err instanceof Error ? err.message : t('customFood.deleteFailed'));
    } finally {
      setPendingDeleteId(null);
    }
  };

  return (
    <AuthenticatedShell className="min-h-screen bg-bg">
      <main className="hv-page">
        <div className="hv-page-heading">
          <div><h1>{t('customFood.title')}</h1><p>{t('redesign.entry.catalogSubtitle')}</p></div>
        <TapTarget
          onClick={() => setEditing('new')}
          className="rounded-xl px-5 text-sm font-semibold bg-accent hover:bg-accent/90 text-bg-elevated"
        >
          {t('customFood.addNew')}
        </TapTarget>
        </div>
        <nav aria-label={t('redesign.entry.sectionsLabel')} className="mb-6 flex flex-wrap gap-2">
          <TapTarget as={Link} href="/food/history/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.diary')}</TapTarget>
          <TapTarget as={Link} href="/food/upload/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.photo')}</TapTarget>
          <TapTarget as={Link} href="/food/manual/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.manualModes')}</TapTarget>
        </nav>

        {error && <p role="alert" className="mb-4 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <section className="hv-card">
          <div className="mb-6 flex flex-wrap items-end justify-between gap-5">
            <label className="w-full max-w-md text-sm font-medium text-text">
              {t('redesign.entry.search')}
              <input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder={t('redesign.entry.foodName')} className="mt-2 w-full rounded-xl border border-border bg-bg-elevated px-4 py-3 text-base text-text" />
            </label>
            <ExpertModeToggle checked={expertMode} onChange={setExpertMode} />
          </div>

          <div className="divide-y divide-border">
            {foods === null && (
              <p className="p-4 text-sm text-text-muted text-center">{t('review.loading')}</p>
            )}
            {foods?.length === 0 && (
              <p className="p-4 text-sm text-text-muted text-center">{t('customFood.noneYet')}</p>
            )}
            {foods && foods.length > 0 && !foods.some(f => f.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())) && <p className="py-10 text-center text-sm text-text-muted">{t('redesign.entry.noMatchingFoods')}</p>}
            {foods?.filter(f => f.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())).map(f => (
              <div key={f.id} className="flex flex-wrap items-center justify-between gap-4 py-5">
                <div className="min-w-0 flex-1 basis-48">
                  <p className="break-words text-sm font-semibold text-text">{f.name}</p>
                  <CanonicalNameLabel expertMode={expertMode} canonicalName={f.canonical_name} />
                  <p className="text-xs text-text-muted">
                    {Math.round(f.calories_per_100g)} {t('unit.kcalPer100g')}
                    {' · '}{t('item.fiber')} {Number.isFinite(f.dietary_fiber_per_100g) ? f.dietary_fiber_per_100g.toLocaleString(numberLocaleFor(language), { maximumFractionDigits: 1 }) : '—'} {t('redesign.entry.gramsPer100g')}
                  </p>
                </div>
                <div className="flex shrink-0 flex-wrap items-center gap-3">
                  <TapTarget
                    onClick={() => setEditing(f)}
                    className="flex items-center text-xs text-accent hover:underline"
                  >
                    {t('customFood.edit')}
                  </TapTarget>
                  {pendingDeleteId === f.id ? (
                    <span className="flex items-center gap-1">
                      <TapTarget
                        onClick={() => handleDelete(f.id)}
                        className="text-xs px-2 rounded bg-red-600 text-white hover:bg-red-700"
                      >
                        {t('item.confirm')}
                      </TapTarget>
                      <TapTarget
                        onClick={() => setPendingDeleteId(null)}
                        className="text-xs px-2 rounded bg-bg text-text"
                      >
                        {t('item.cancel')}
                      </TapTarget>
                    </span>
                  ) : (
                    <TapTarget
                      onClick={() => setPendingDeleteId(f.id)}
                      className="flex items-center text-xs text-red-500 hover:text-red-700"
                    >
                      {t('customFood.delete')}
                    </TapTarget>
                  )}
                </div>
              </div>
            ))}
          </div>
        </section>
      </main>

      {editing && (
        <CustomFoodModal
          initial={editing === 'new' ? undefined : editing}
          onSave={handleSave}
          onClose={() => setEditing(null)}
        />
      )}
    </AuthenticatedShell>
  );
}
