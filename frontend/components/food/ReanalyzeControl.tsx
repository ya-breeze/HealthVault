'use client';
import { useState } from 'react';
import {
  api,
  ExpertComponentInput,
  FoodMeal,
  ReanalyzeFailedError,
  ReanalyzeInput,
  ReanalyzeSupersededError,
} from '@/lib/api';
import {
  MAX_COMBINED_COMPONENT_NAME_LENGTH,
  MAX_EXPERT_COMPONENTS,
  MAX_HINT_LENGTH,
  MAX_COMPONENT_NAME_LENGTH,
  normalizedUnicodeLength,
  unicodeLength,
} from '@/lib/foodGuidance';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate } from '@/lib/i18n';

interface Props {
  mealId: string;
  // Takes a thunk, not an already-started request — see ReviewClient's
  // applyMealUpdate doc comment for why (issue order must be commit order,
  // enforced by the parent controlling when each request actually starts).
  onReanalyzed: (issue: () => Promise<FoodMeal>, label?: string) => Promise<FoodMeal>;
}

type Mode = 'hint' | 'expert';
type ExpertRow = { name: string; weight: string };

const blankRow = (): ExpertRow => ({ name: '', weight: '' });

export default function ReanalyzeControl({ mealId, onReanalyzed }: Props) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<Mode>('hint');
  const [hint, setHint] = useState('');
  const [components, setComponents] = useState<ExpertRow[]>([blankRow()]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    setOpen(false);
    setMode('hint');
    setHint('');
    setComponents([blankRow()]);
    setError(null);
  };

  const buildInput = (): ReanalyzeInput | null => {
    if (mode === 'hint') {
      const trimmed = hint.trim();
      if (!trimmed) {
        setError(t('redesign.review.aHintIsRequired'));
        return null;
      }
      if (unicodeLength(trimmed) > MAX_HINT_LENGTH) {
        setError(interpolate(t('redesign.review.hintMustBeAtMostMaxCharacters'), {max: String(MAX_HINT_LENGTH)}));
        return null;
      }
      return { hint: trimmed };
    }

    const normalized: ExpertComponentInput[] = [];
    let combinedLength = 0;
    for (const [index, row] of components.entries()) {
      const name = row.name.trim();
      if (!name) {
        setError(interpolate(t('redesign.review.ingredientIndexNeedsAName'), {index: String(index + 1)}));
        return null;
      }
      if (unicodeLength(name) > MAX_COMPONENT_NAME_LENGTH) {
        setError(interpolate(t('redesign.review.ingredientNamesMustBeAtMostMax'), {max: String(MAX_COMPONENT_NAME_LENGTH)}));
        return null;
      }
      combinedLength += unicodeLength(name);
      const component: ExpertComponentInput = { name };
      if (row.weight.trim()) {
        const weight = Number(row.weight);
        if (!Number.isFinite(weight) || weight <= 0) {
          setError(interpolate(t('redesign.review.ingredientIndexWeightMustBeGreaterThan'), {index: String(index + 1)}));
          return null;
        }
        component.weight_grams = weight;
      }
      normalized.push(component);
    }
    if (combinedLength > MAX_COMBINED_COMPONENT_NAME_LENGTH) {
      setError(interpolate(t('redesign.review.combinedIngredientNamesMustBeAtMost'), {max: String(MAX_COMBINED_COMPONENT_NAME_LENGTH)}));
      return null;
    }
    return { components: normalized };
  };

  const submit = async () => {
    const input = buildInput();
    if (!input) return;
    setBusy(true);
    setError(null);
    try {
      await onReanalyzed(() => api.reanalyzeMeal(mealId, input), t('redesign.review.reanalysisComplete'));
      close();
    } catch (err) {
      const isFailed = err instanceof ReanalyzeFailedError || (err as { name?: string })?.name === 'ReanalyzeFailedError';
      const isSuperseded =
        err instanceof ReanalyzeSupersededError || (err as { name?: string })?.name === 'ReanalyzeSupersededError';
      if (isFailed) {
        setError(t('redesign.review.reanalysisFailedTheMealIsUnchangedYou'));
      } else if (isSuperseded) {
        try {
          await onReanalyzed(() => api.getMeal(mealId), t('redesign.review.refreshedWithLatestChange'));
          setError(t('redesign.review.anotherOperationTookOverThisMealWhile'));
        } catch {
          setError(t('redesign.review.anotherOperationTookOverThisMealAnd'));
        }
      } else {
        setError(err instanceof Error ? err.message : (t('redesign.review.reanalysisFailed')));
      }
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <TapTarget
        onClick={() => setOpen(true)}
        className="mt-3 w-full rounded-lg border border-border text-sm font-medium text-text-muted hover:border-accent hover:text-accent"
      >
        {t('redesign.review.improveAnalysis')}
      </TapTarget>
    );
  }

  return (
    <div className="hv-card mt-6">
      <p className="text-sm font-medium text-text">{t('redesign.review.improveAnalysis')}</p>
      <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">
        {t('redesign.review.thisReplacesAllCurrentItemsAndRequires')}
      </p>
      <div className="mt-3 grid grid-cols-2 rounded-lg bg-bg p-1" role="tablist" aria-label={t('redesign.review.analysisMode')}>
        {(['hint', 'expert'] as const).map(value => (
          <TapTarget
            key={value}
            role="tab"
            aria-selected={mode === value}
            onClick={() => { setMode(value); setError(null); }}
            className={`rounded-md text-sm font-medium ${mode === value ? 'bg-bg-elevated text-accent shadow-sm ' : 'text-text-muted '}`}
          >
            {value === 'hint' ? (t('redesign.review.hint')) : (t('redesign.review.expert'))}
          </TapTarget>
        ))}
      </div>

      {mode === 'hint' ? (
        <div className="mt-3">
          <label htmlFor="reanalyze-hint" className="text-sm font-medium text-text">{t('redesign.review.whatDidTheModelMiss')}</label>
          <textarea
            id="reanalyze-hint"
            value={hint}
            onChange={event => setHint(event.target.value)}
            placeholder={t('redesign.review.egGrilledChickenWithRedBeans')}
            rows={3}
            className="mt-2 w-full rounded-lg border border-border bg-bg-elevated px-3 py-2 text-sm text-text"
          />
          <p className="mt-1 text-right text-xs text-text-muted">{normalizedUnicodeLength(hint)}/{MAX_HINT_LENGTH}</p>
        </div>
      ) : (
        <div className="mt-3 space-y-3">
          <p className="text-xs text-text-muted">{t('redesign.review.nameEachIngredientSuppliedWeightsAreUsed')}</p>
          {components.map((row, index) => (
            <div key={index} className="rounded-lg border border-border p-3">
              <div className="grid grid-cols-[minmax(0,1fr)_7rem] gap-2">
                <div>
                  <label htmlFor={`component-name-${index}`} className="text-xs font-medium text-text-muted">{t('redesign.review.ingredient')} {index + 1}</label>
                  <input id={`component-name-${index}`} value={row.name} onChange={event => setComponents(current => current.map((item, i) => i === index ? { ...item, name: event.target.value } : item))} placeholder={t('redesign.review.redBeans')} className="mt-1 min-h-11 w-full rounded-md border border-border bg-bg-elevated px-2 text-sm text-text" />
                </div>
                <div>
                  <label htmlFor={`component-weight-${index}`} className="text-xs font-medium text-text-muted">{t('redesign.review.grams')}</label>
                  <input id={`component-weight-${index}`} type="number" inputMode="decimal" min="0.01" step="any" value={row.weight} onChange={event => setComponents(current => current.map((item, i) => i === index ? { ...item, weight: event.target.value } : item))} placeholder={t('redesign.review.auto')} className="mt-1 min-h-11 w-full rounded-md border border-border bg-bg-elevated px-2 text-sm text-text" />
                </div>
              </div>
              {components.length > 1 && (
                <TapTarget onClick={() => setComponents(current => current.filter((_, i) => i !== index))} className="mt-2 flex items-center text-xs font-medium text-red-600 dark:text-red-400">{t('redesign.review.removeIngredient')}</TapTarget>
              )}
            </div>
          ))}
          {components.length < MAX_EXPERT_COMPONENTS && (
            <TapTarget onClick={() => setComponents(current => [...current, blankRow()])} className="w-full rounded-lg border border-dashed border-border text-sm font-medium text-text-muted">{t('redesign.review.addIngredient')}</TapTarget>
          )}
        </div>
      )}

      {error && <p className="mt-3 text-xs text-red-600 dark:text-red-400">{error}</p>}
      <div className="mt-4 grid grid-cols-2 gap-2">
        <TapTarget onClick={close} disabled={busy} className="rounded-lg border border-border text-sm font-medium text-text-muted">{t('item.cancel')}</TapTarget>
        <TapTarget onClick={submit} disabled={busy} className="rounded-lg bg-accent text-sm font-medium text-bg-elevated hover:opacity-90 disabled:opacity-50">{busy ? (t('redesign.review.reanalyzing')) : (t('redesign.review.reanalyze'))}</TapTarget>
      </div>
    </div>
  );
}
