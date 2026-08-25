'use client';
import { useEffect, useState } from 'react';
import { api, ApiError } from '@/lib/api';
import { useToast } from '@/components/Toast';
import { useLanguage } from '@/components/LanguageContext';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  // The raw stored `timezone` setting, undefined when the user has never
  // set one — distinct from the resolved 'UTC' default the history page
  // itself groups by, so the panel can prefill the browser's own zone
  // instead of silently defaulting the select to UTC (tasks.md 9.1).
  timezone: string | undefined;
  usualMealsPerDay: number;
  onSaved: (next: { timezone: string; usualMealsPerDay: number }) => void;
}

function supportedTimezones(): string[] {
  if (typeof Intl.supportedValuesOf === 'function') {
    try {
      return Intl.supportedValuesOf('timeZone');
    } catch {
      // Falls through to the UTC-only fallback below.
    }
  }
  return ['UTC'];
}

// Collapsed-by-default panel at the top of /food/history for the two
// settings that drive Day Completeness: `timezone` (the Logged Day
// boundary) and `usual_meals_per_day` (the completeness threshold). Saves
// through api.updateSettings' read-modify-write so this panel can never
// clobber dashboard_order/display_language (tasks.md 9.2), and reports the
// saved values back to the page so it can refetch grouping/completeness
// without a reload (tasks.md 9.3).
export default function HistorySettingsPanel({ timezone, usualMealsPerDay, onSaved }: Props) {
  const { t } = useLanguage();
  const { showToast } = useToast();
  const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

  const [open, setOpen] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [tzValue, setTzValue] = useState(timezone ?? browserZone);
  const [mealsValue, setMealsValue] = useState(String(usualMealsPerDay));
  const [saving, setSaving] = useState(false);

  // The page's own settings fetch resolves after this panel has already
  // mounted with its browser-zone default, so sync once it lands — but
  // only while the user hasn't touched a field yet, so an in-progress edit
  // is never overwritten out from under them.
  useEffect(() => {
    if (!dirty) {
      setTzValue(timezone ?? browserZone);
      setMealsValue(String(usualMealsPerDay));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timezone, usualMealsPerDay, dirty]);

  const zones = supportedTimezones();
  if (!zones.includes(browserZone)) zones.push(browserZone);
  if (!zones.includes(tzValue)) zones.push(tzValue);
  zones.sort();

  const handleSave = async () => {
    const meals = parseInt(mealsValue, 10);
    if (!tzValue || !Number.isInteger(meals) || meals < 1) {
      showToast(t('historySettings.invalidInput'), 'error');
      return;
    }
    setSaving(true);
    try {
      await api.updateSettings({ timezone: tzValue, usual_meals_per_day: meals });
      setDirty(false);
      onSaved({ timezone: tzValue, usualMealsPerDay: meals });
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : t('historySettings.saveFailed'), 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mb-4 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 overflow-hidden">
      <TapTarget
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200"
      >
        <span>{t('historySettings.title')}</span>
        <span className="text-gray-400" aria-hidden="true">{open ? '▲' : '▼'}</span>
      </TapTarget>
      {open && (
        <div className="px-4 pb-4 pt-3 space-y-3 border-t border-gray-100 dark:border-gray-700">
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300">
            {t('historySettings.timezoneLabel')}
            <select
              value={tzValue}
              onChange={e => {
                setDirty(true);
                setTzValue(e.target.value);
              }}
              className="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 text-sm text-gray-900 dark:text-white px-2 py-1.5"
            >
              {zones.map(z => (
                <option key={z} value={z}>
                  {z}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300">
            {t('historySettings.mealsPerDayLabel')}
            <input
              type="number"
              min={1}
              value={mealsValue}
              onChange={e => {
                setDirty(true);
                setMealsValue(e.target.value);
              }}
              className="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 text-sm text-gray-900 dark:text-white px-2 py-1.5"
            />
          </label>
          <TapTarget
            onClick={handleSave}
            disabled={saving}
            className="w-full rounded-lg text-sm font-medium border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-blue-400 hover:text-blue-600 dark:hover:text-blue-400 disabled:opacity-50 py-1.5"
          >
            {saving ? t('historySettings.saving') : t('historySettings.save')}
          </TapTarget>
        </div>
      )}
    </div>
  );
}
