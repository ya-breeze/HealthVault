'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api, ManualMealItemInput } from '@/lib/api';
import ManualItemEditor from '@/components/food/ManualItemEditor';
import MealCookingContextInputs, { defaultMealCookingContext } from '@/components/food/MealCookingContextInputs';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import TapTarget from '@/components/ui/TapTarget';
import BottomActionBar from '@/components/ui/BottomActionBar';
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
  const [cookingContext, setCookingContext] = useState(defaultMealCookingContext);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [showStructured, setShowStructured] = useState(false);
  const [items, setItems] = useState<ManualMealItemInput[]>([emptyItem()]);
  const [structuredSaving, setStructuredSaving] = useState(false);
  const [structuredError, setStructuredError] = useState<string | null>(null);

  const updateItem = (index: number, item: ManualMealItemInput) =>
    setItems(prev => prev.map((it, i) => (i === index ? item : it)));

  const removeItem = (index: number) => setItems(prev => prev.filter((_, i) => i !== index));

  // A datetime-local input reads back as '' once the user clears it, and
  // `new Date('').toISOString()` throws RangeError — which would surface as
  // the raw, untranslated "Invalid time value" instead of saving. Both
  // endpoints treat logged_at as optional and default it to now, so an empty
  // field means "now" rather than an error.
  const loggedAtISO = () => (loggedAt ? new Date(loggedAt).toISOString() : undefined);

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
        logged_at: loggedAtISO(),
        cooking_context: cookingContext,
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
        logged_at: loggedAtISO(),
        items,
      });
      router.push(`/food/review/?meal=${meal.id}`);
    } catch (err) {
      setStructuredError(err instanceof Error ? err.message : 'Failed to save meal');
      setStructuredSaving(false);
    }
  };

  return (
    <AuthenticatedShell className="min-h-screen bg-bg pb-24">
      <main className="hv-page">
        <div className="hv-page-heading">
          <div><h1>{t('describe.title')}</h1><p>{t('redesign.entry.manualSubtitle')}</p></div>
          <TapTarget as={Link} href="/food/history/" className="inline-flex items-center justify-center rounded-xl border border-border bg-bg-elevated px-5 text-sm font-semibold text-text">{t('redesign.entry.backToDiary')}</TapTarget>
        </div>
        <nav aria-label={t('redesign.entry.optionsLabel')} className="mb-6 flex flex-wrap gap-2">
          <TapTarget as={Link} href="/food/upload/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.photo')}</TapTarget>
          <TapTarget as={Link} href="/food/manual/" aria-current="page" className="inline-flex items-center justify-center rounded-xl bg-accent/10 px-5 text-sm font-semibold text-accent">{t('redesign.entry.manualModes')}</TapTarget>
          <TapTarget as={Link} href="/food/custom/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.catalog')}</TapTarget>
        </nav>
        <section className="hv-card mx-auto max-w-3xl">

          <div className="flex flex-col sm:flex-row gap-5 mb-6">
            <label className="flex-1 text-sm text-text">
              {t('describe.nameLabel')}
              <input
                type="text"
                value={name}
                onChange={e => setName(e.target.value)}
                className="mt-1 w-full border border-border rounded-xl px-3 py-2 text-base bg-bg-elevated text-text"
              />
            </label>
            <label className="text-sm text-text">
              {t('describe.whenLabel')}
              <input
                type="datetime-local"
                value={loggedAt}
                onChange={e => setLoggedAt(e.target.value)}
                className="mt-1 w-full border border-border rounded-xl px-2 py-2 text-base bg-bg-elevated text-text"
              />
            </label>
          </div>

          <label htmlFor="meal-description" className="block text-sm font-medium text-text">
            {t('describe.textareaLabel')}
          </label>
          <textarea
            id="meal-description"
            data-testid="describe-textarea"
            value={description}
            onChange={e => setDescription(e.target.value)}
            rows={4}
            placeholder={t('describe.placeholder')}
            className="mt-1 w-full rounded-xl border border-border bg-bg-elevated px-3 py-2 text-base text-text"
          />
          <p className="mt-1 text-right text-xs text-text-muted">
            {normalizedUnicodeLength(description)}/{MAX_DESCRIPTION_LENGTH}
          </p>
          <p className="mt-1 mb-4 text-sm text-text-muted">{t('describe.disclosure')}</p>
          {!showStructured && (
            <div className="mb-4"><MealCookingContextInputs value={cookingContext} onChange={setCookingContext} /></div>
          )}

          {error && <p role="alert" className="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

          {/* Hidden while the structured form is open, so the page never shows
              two green submit buttons at once. They post to different endpoints
              with different outcomes and neither carries the other's input, so
              a user who tapped the wrong one would silently lose what they
              typed. The disclosure's own label says "instead", and the control
              below reverses it. */}
          {!showStructured && (
            <TapTarget
              onClick={handleDescribeSubmit}
              disabled={saving}
              data-testid="describe-submit"
              className="w-full mb-6 rounded-xl text-sm font-medium bg-accent hover:bg-accent/90 text-bg-elevated disabled:opacity-50"
            >
              {saving ? t('describe.submitting') : t('describe.submit')}
            </TapTarget>
          )}

          {showStructured ? (
            <div className="border-t border-border pt-6">
              <h2 className="text-base font-semibold text-text mb-3">Log a Meal Manually</h2>
              {description.trim() && (
                <p className="mb-3 text-sm text-amber-700 dark:text-amber-400">
                  {t('describe.descriptionUnusedHere')}
                </p>
              )}
              <div className="flex flex-col gap-5 mb-5">
                {items.map((item, i) => (
                  <ManualItemEditor key={i} index={i} item={item} onChange={updateItem} onRemove={removeItem} />
                ))}
              </div>

              <TapTarget
                onClick={() => setItems(prev => [...prev, emptyItem()])}
                className="w-full mb-4 rounded-xl text-sm font-medium bg-bg text-text hover:bg-bg"
              >
                + Add item
              </TapTarget>

              {structuredError && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{structuredError}</p>}

              <TapTarget
                onClick={() => setShowStructured(false)}
                data-testid="describe-structured-back"
                className="w-full rounded-xl border border-dashed border-border px-4 text-sm font-medium text-text-muted hover:border-accent hover:text-accent"
              >
                {t('describe.backToDescribe')}
              </TapTarget>
            </div>
          ) : (
            <TapTarget
              onClick={() => setShowStructured(true)}
              data-testid="describe-structured-toggle"
              className="w-full rounded-xl border border-dashed border-border px-4 text-sm font-medium text-text-muted hover:border-accent hover:text-accent"
            >
              {t('describe.structuredToggle')}
            </TapTarget>
          )}
        </section>
      </main>

      {/* Only rendered once the structured form is disclosed: this bar, and
          the "Save Meal" button inside it, belong to the structured
          item-by-item submit — the description path above has its own
          inline submit button instead.

          BottomActionBar (from main) replaces the hand-rolled fixed div this
          branch wrote: it carries the same anchoring above the navigation bar
          and the same safe-area handling, and additionally registers its
          height so the toast stack clears it. Rendering it conditionally is
          intended — the registry it writes to is keyed per mounted instance,
          so an undisclosed structured form contributes no height. */}
      {showStructured && (
        <BottomActionBar>
          <TapTarget
            onClick={handleStructuredSubmit}
            disabled={structuredSaving}
            className="w-full rounded-xl text-sm font-medium bg-accent hover:bg-accent/90 text-bg-elevated disabled:opacity-50"
          >
            {structuredSaving ? 'Saving…' : 'Save Meal'}
          </TapTarget>
        </BottomActionBar>
      )}
    </AuthenticatedShell>
  );
}
