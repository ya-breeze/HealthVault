'use client';

import type { MealCookingContext } from '@/lib/api';
import { useLanguage } from '@/components/LanguageContext';

export const defaultMealCookingContext = (): MealCookingContext => ({ low_added_salt: true });

interface Props {
  value: MealCookingContext;
  onChange: (value: MealCookingContext) => void;
}

export default function MealCookingContextInputs({ value, onChange }: Props) {
  const { t } = useLanguage();
  return (
    <fieldset className="rounded-2xl border border-border bg-bg px-5 py-3">
      <legend className="px-1 text-sm font-medium text-text">{t('cookingContext.title')}</legend>
      <label className="flex min-h-12 cursor-pointer items-center gap-3 text-sm text-text">
        <input type="checkbox" checked={value.low_added_salt} data-testid="meal-low-added-salt"
          onChange={event => onChange({ low_added_salt: event.target.checked })}
          className="h-5 w-5 accent-accent" />
        {t('cookingContext.lowAddedSalt')}
      </label>
      <p className="mb-2 text-xs text-text-muted">{t('cookingContext.hint')}</p>
    </fieldset>
  );
}
