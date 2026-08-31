'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, ManualMealItemInput } from '@/lib/api';
import ManualItemEditor from '@/components/food/ManualItemEditor';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate } from '@/lib/i18n';
import { MAX_DESCRIPTION_LENGTH, normalizedUnicodeLength, unicodeLength } from '@/lib/foodGuidance';

function emptyItem(): ManualMealItemInput {
  return { name: '', source: 'reference' };
}

export default function ManualMealPage() {
  const router = useRouter();
  const { t } = useLanguage();

  // Shared by both submit paths below — the description-first path leads
  // with these, and the demoted structured form reuses them rather than
  // duplicating its own copy.
  const [name, setName] = useState('');
  const [loggedAt, setLoggedAt] = useState(() => new Date().toISOString().slice(0, 16));

  const [description, setDescription] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [showStructured, setShowStructured] = useState(false);
  const [items, setItems] = useState<ManualMealItemInput[]>([emptyItem()]);
  const [structuredSaving, setStructuredSaving] = useState(false);
  const [structuredError, setStructuredError] = useState<string | null>(null);

  const updateItem = (index: number, item: ManualMealItemInput) =>
    setItems(prev => prev.map((it, i) => (i === index ? item : it)));

  const removeItem = (index: number) => setItems(prev => prev.filter((_, i) => i !== index));

  const handleDescribeSubmit = async () => {
    setError(null);
    const trimmed = description.trim();
    if (!trimmed) {
      setError(t('describe.emptyError'));
      return;
    }
    if (unicodeLength(trimmed) > MAX_DESCRIPTION_LENGTH) {
      setError(interpolate(t('describe.tooLongError'), { max: MAX_DESCRIPTION_LENGTH }));
      return;
    }
    setSaving(true);
    try {
      const meal = await api.describeMeal({
        description: trimmed,
        name: name || undefined,
        logged_at: new Date(loggedAt).toISOString(),
      });
      router.push(`/food/review/?meal=${meal.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('describe.saveFailed'));
      setSaving(false);
    }
  };

  // Unchanged from before this page was reworked: same request shape, same
  // POST /api/food/meals/manual call, same validation and error fallback —
  // only its place on the page (behind the disclosure below) and its shared
  // name/loggedAt state have changed.
  const handleStructuredSubmit = async () => {
    setStructuredError(null);
    if (items.length === 0) {
      setStructuredError('Add at least one item.');
      return;
    }
    setStructuredSaving(true);
    try {
      const meal = await api.createManualMeal({
        name: name || undefined,
        logged_at: new Date(loggedAt).toISOString(),
        items,
      });
      router.push(`/food/review/?meal=${meal.id}`);
    } catch (err) {
      setStructuredError(err instanceof Error ? err.message : 'Failed to save meal');
      setStructuredSaving(false);
    }
  };

  return (
    <AuthenticatedShell className="min-h-screen bg-gray-50 dark:bg-gray-900 pb-24">
      <main className="max-w-md mx-auto px-6 py-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-4">{t('describe.title')}</h1>

        <div className="flex flex-col sm:flex-row gap-2 mb-4">
          <label className="flex-1 text-sm text-gray-700 dark:text-gray-300">
            {t('describe.nameLabel')}
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              className="mt-1 w-full border border-gray-300 dark:border-gray-600 rounded-md px-3 py-2 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
          </label>
          <label className="text-sm text-gray-700 dark:text-gray-300">
            {t('describe.whenLabel')}
            <input
              type="datetime-local"
              value={loggedAt}
              onChange={e => setLoggedAt(e.target.value)}
              className="mt-1 w-full border border-gray-300 dark:border-gray-600 rounded-md px-2 py-2 text-base bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
          </label>
        </div>

        <label htmlFor="meal-description" className="block text-sm font-medium text-gray-900 dark:text-white">
          {t('describe.textareaLabel')}
        </label>
        <textarea
          id="meal-description"
          value={description}
          onChange={e => setDescription(e.target.value)}
          rows={4}
          placeholder={t('describe.placeholder')}
          className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-base text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
        />
        <p className="mt-1 text-right text-xs text-gray-400">
          {normalizedUnicodeLength(description)}/{MAX_DESCRIPTION_LENGTH}
        </p>
        <p className="mt-1 mb-4 text-sm text-gray-500 dark:text-gray-400">{t('describe.disclosure')}</p>

        {error && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <TapTarget
          onClick={handleDescribeSubmit}
          disabled={saving}
          className="w-full mb-6 rounded-lg text-sm font-medium bg-green-600 hover:bg-green-700 text-white disabled:opacity-50"
        >
          {saving ? t('describe.submitting') : t('describe.submit')}
        </TapTarget>

        {showStructured ? (
          <div className="border-t border-gray-200 dark:border-gray-700 pt-4">
            <h2 className="text-base font-semibold text-gray-900 dark:text-white mb-3">Log a Meal Manually</h2>
            <div className="flex flex-col gap-3 mb-4">
              {items.map((item, i) => (
                <ManualItemEditor key={i} index={i} item={item} onChange={updateItem} onRemove={removeItem} />
              ))}
            </div>

            <TapTarget
              onClick={() => setItems(prev => [...prev, emptyItem()])}
              className="w-full mb-4 rounded-lg text-sm font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600"
            >
              + Add item
            </TapTarget>

            {structuredError && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{structuredError}</p>}
          </div>
        ) : (
          <TapTarget
            onClick={() => setShowStructured(true)}
            className="w-full rounded-lg border border-dashed border-gray-300 px-4 text-sm font-medium text-gray-600 hover:border-blue-400 hover:text-blue-600 dark:border-gray-600 dark:text-gray-300 dark:hover:text-blue-400"
          >
            {t('describe.structuredToggle')}
          </TapTarget>
        )}
      </main>

      {/* Only rendered once the structured form is disclosed: this bar, and
          the "Save Meal" button inside it, belong to the structured
          item-by-item submit — the description path above has its own
          inline submit button instead. Anchored above the bottom navigation
          bar, not at the viewport edge: `/food/manual/` is one of the bar's
          own destinations, so without the offset the bar lands on this
          submit button. A padding on the shell cannot do this — a fixed
          element is out of flow relative to the viewport, not to its
          ancestor. Its own bottom padding keeps the safe-area inset for the
          desktop case, where no bar is beneath it to absorb it; below the
          breakpoint `--edge-inset-b` is `0px` because `--nav-block` already
          carries it. */}
      {showStructured && (
        <div className="fixed bottom-[var(--nav-block)] left-0 right-0 z-30 bg-gray-50/95 dark:bg-gray-900/95 backdrop-blur border-t border-gray-200 dark:border-gray-700 px-6 py-3 pb-[calc(0.75rem+var(--edge-inset-b))]">
          <div className="max-w-md mx-auto">
            <TapTarget
              onClick={handleStructuredSubmit}
              disabled={structuredSaving}
              className="w-full rounded-lg text-sm font-medium bg-green-600 hover:bg-green-700 text-white disabled:opacity-50"
            >
              {structuredSaving ? 'Saving…' : 'Save Meal'}
            </TapTarget>
          </div>
        </div>
      )}
    </AuthenticatedShell>
  );
}
