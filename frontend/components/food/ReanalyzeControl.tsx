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

interface Props {
  mealId: string;
  onReanalyzed: (issue: () => Promise<FoodMeal>) => Promise<FoodMeal>;
}

type Mode = 'hint' | 'expert';
type ExpertRow = { name: string; weight: string };

const blankRow = (): ExpertRow => ({ name: '', weight: '' });

export default function ReanalyzeControl({ mealId, onReanalyzed }: Props) {
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
        setError('A hint is required');
        return null;
      }
      if (unicodeLength(trimmed) > MAX_HINT_LENGTH) {
        setError(`Hint must be at most ${MAX_HINT_LENGTH} characters`);
        return null;
      }
      return { hint: trimmed };
    }

    const normalized: ExpertComponentInput[] = [];
    let combinedLength = 0;
    for (const [index, row] of components.entries()) {
      const name = row.name.trim();
      if (!name) {
        setError(`Ingredient ${index + 1} needs a name`);
        return null;
      }
      if (unicodeLength(name) > MAX_COMPONENT_NAME_LENGTH) {
        setError(`Ingredient names must be at most ${MAX_COMPONENT_NAME_LENGTH} characters`);
        return null;
      }
      combinedLength += unicodeLength(name);
      const component: ExpertComponentInput = { name };
      if (row.weight.trim()) {
        const weight = Number(row.weight);
        if (!Number.isFinite(weight) || weight <= 0) {
          setError(`Ingredient ${index + 1} weight must be greater than zero`);
          return null;
        }
        component.weight_grams = weight;
      }
      normalized.push(component);
    }
    if (combinedLength > MAX_COMBINED_COMPONENT_NAME_LENGTH) {
      setError(`Combined ingredient names must be at most ${MAX_COMBINED_COMPONENT_NAME_LENGTH} characters`);
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
      await onReanalyzed(() => api.reanalyzeMeal(mealId, input));
      close();
    } catch (err) {
      const isFailed = err instanceof ReanalyzeFailedError || (err as { name?: string })?.name === 'ReanalyzeFailedError';
      const isSuperseded =
        err instanceof ReanalyzeSupersededError || (err as { name?: string })?.name === 'ReanalyzeSupersededError';
      if (isFailed) {
        setError('Reanalysis failed — the meal is unchanged. You can try again.');
      } else if (isSuperseded) {
        try {
          await onReanalyzed(() => api.getMeal(mealId));
          setError('Another operation took over this meal while reanalyzing — showing its current state.');
        } catch {
          setError('Another operation took over this meal, and refreshing failed. Reload to see its current state.');
        }
      } else {
        setError(err instanceof Error ? err.message : 'Reanalysis failed');
      }
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="mt-3 min-h-12 w-full rounded-lg border border-gray-300 text-sm font-medium text-gray-600 hover:border-blue-400 hover:text-blue-600 dark:border-gray-600 dark:text-gray-300 dark:hover:text-blue-400"
      >
        Improve analysis
      </button>
    );
  }

  return (
    <div className="mt-3 rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-gray-700 dark:bg-gray-800">
      <p className="text-sm font-medium text-gray-900 dark:text-white">Improve analysis</p>
      <p className="mt-1 text-xs text-amber-700 dark:text-amber-400">
        This replaces all current items and requires review again.
      </p>
      <div className="mt-3 grid grid-cols-2 rounded-lg bg-gray-100 p-1 dark:bg-gray-700" role="tablist" aria-label="Analysis mode">
        {(['hint', 'expert'] as const).map(value => (
          <button
            key={value}
            type="button"
            role="tab"
            aria-selected={mode === value}
            onClick={() => { setMode(value); setError(null); }}
            className={`min-h-11 rounded-md text-sm font-medium ${mode === value ? 'bg-white text-blue-700 shadow-sm dark:bg-gray-800 dark:text-blue-400' : 'text-gray-600 dark:text-gray-300'}`}
          >
            {value === 'hint' ? 'Hint' : 'Expert'}
          </button>
        ))}
      </div>

      {mode === 'hint' ? (
        <div className="mt-3">
          <label htmlFor="reanalyze-hint" className="text-sm font-medium text-gray-900 dark:text-white">What did the model miss?</label>
          <textarea
            id="reanalyze-hint"
            value={hint}
            onChange={event => setHint(event.target.value)}
            placeholder="e.g. grilled chicken with red beans"
            rows={3}
            className="mt-2 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
          />
          <p className="mt-1 text-right text-xs text-gray-400">{normalizedUnicodeLength(hint)}/{MAX_HINT_LENGTH}</p>
        </div>
      ) : (
        <div className="mt-3 space-y-3">
          <p className="text-xs text-gray-500 dark:text-gray-400">Name each ingredient. Supplied weights are used exactly; blank weights are estimated from the photo.</p>
          {components.map((row, index) => (
            <div key={index} className="rounded-lg border border-gray-200 p-3 dark:border-gray-600">
              <div className="grid grid-cols-[minmax(0,1fr)_7rem] gap-2">
                <div>
                  <label htmlFor={`component-name-${index}`} className="text-xs font-medium text-gray-600 dark:text-gray-300">Ingredient {index + 1}</label>
                  <input id={`component-name-${index}`} value={row.name} onChange={event => setComponents(current => current.map((item, i) => i === index ? { ...item, name: event.target.value } : item))} placeholder="Red beans" className="mt-1 min-h-11 w-full rounded-md border border-gray-300 bg-white px-2 text-sm text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100" />
                </div>
                <div>
                  <label htmlFor={`component-weight-${index}`} className="text-xs font-medium text-gray-600 dark:text-gray-300">Grams</label>
                  <input id={`component-weight-${index}`} type="number" inputMode="decimal" min="0.01" step="any" value={row.weight} onChange={event => setComponents(current => current.map((item, i) => i === index ? { ...item, weight: event.target.value } : item))} placeholder="Auto" className="mt-1 min-h-11 w-full rounded-md border border-gray-300 bg-white px-2 text-sm text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100" />
                </div>
              </div>
              {components.length > 1 && (
                <button type="button" onClick={() => setComponents(current => current.filter((_, i) => i !== index))} className="mt-2 min-h-11 text-xs font-medium text-red-600 dark:text-red-400">Remove ingredient</button>
              )}
            </div>
          ))}
          {components.length < MAX_EXPERT_COMPONENTS && (
            <button type="button" onClick={() => setComponents(current => [...current, blankRow()])} className="min-h-11 w-full rounded-lg border border-dashed border-gray-300 text-sm font-medium text-gray-600 dark:border-gray-600 dark:text-gray-300">Add ingredient</button>
          )}
        </div>
      )}

      {error && <p className="mt-3 text-xs text-red-600 dark:text-red-400">{error}</p>}
      <div className="mt-4 grid grid-cols-2 gap-2">
        <button type="button" onClick={close} disabled={busy} className="min-h-12 rounded-lg border border-gray-300 text-sm font-medium text-gray-600 dark:border-gray-600 dark:text-gray-300">Cancel</button>
        <button type="button" onClick={submit} disabled={busy} className="min-h-12 rounded-lg bg-blue-600 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50">{busy ? 'Reanalyzing…' : 'Reanalyze'}</button>
      </div>
    </div>
  );
}
