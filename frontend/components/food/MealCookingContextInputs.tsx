'use client';

import type { MealCookingContext } from '@/lib/api';
import { useLanguage } from '@/components/LanguageContext';

export const defaultMealCookingContext = (): MealCookingContext => ({ homemade: true, low_added_salt: true });

interface Props {
  value: MealCookingContext;
  onChange: (value: MealCookingContext) => void;
}

export default function MealCookingContextInputs({ value, onChange }: Props) {
  const { t } = useLanguage();
  return (
    <fieldset className="rounded-xl border border-gray-200 bg-white px-4 py-2 dark:border-gray-700 dark:bg-gray-800">
      <legend className="px-1 text-sm font-medium text-gray-700 dark:text-gray-200">{t('cookingContext.title')}</legend>
      <label className="flex min-h-12 cursor-pointer items-center gap-3 text-sm text-gray-900 dark:text-white">
        <input type="checkbox" checked={value.homemade} data-testid="meal-homemade"
          onChange={event => onChange({ homemade: event.target.checked, low_added_salt: event.target.checked })}
          className="h-5 w-5 accent-green-600" />
        {t('cookingContext.homemade')}
      </label>
      <label className={`flex min-h-12 items-center gap-3 text-sm ${value.homemade ? 'cursor-pointer text-gray-900 dark:text-white' : 'text-gray-400 dark:text-gray-500'}`}>
        <input type="checkbox" checked={value.homemade && value.low_added_salt} disabled={!value.homemade}
          data-testid="meal-low-added-salt"
          onChange={event => onChange({ ...value, low_added_salt: event.target.checked })}
          className="h-5 w-5 accent-green-600" />
        {t('cookingContext.lowAddedSalt')}
      </label>
      <p className="mb-2 text-xs text-gray-500 dark:text-gray-400">{t('cookingContext.hint')}</p>
    </fieldset>
  );
}
