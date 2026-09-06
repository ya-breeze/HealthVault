'use client';
import { useState } from 'react';
import { api, ApiError, DayCompletenessState } from '@/lib/api';
import { useToast } from '@/components/Toast';
import { useLanguage } from '@/components/LanguageContext';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  date: string;
  state: DayCompletenessState | undefined;
  onChange: (date: string, state: DayCompletenessState) => void;
}

// Renders the per-day Day Completeness affordance on the history page
// (design.md §6 "Frontend surface"): nothing for `complete` (or an
// `incomplete`/not-yet-fetched day — the latter includes the caller's
// current Logged Day, which is never fetched in the first place, see
// food-day-completeness tasks.md 8.1), a "Complete" badge plus an unconfirm
// control for `confirmed_complete`, and a "Mark day complete" button for
// `unconfirmed`.
export default function DayCompletenessControl({ date, state, onChange }: Props) {
  const { t } = useLanguage();
  const { showToast } = useToast();
  const [pending, setPending] = useState(false);

  if (state === undefined || state === 'complete' || state === 'incomplete') return null;

  const handleConfirm = async () => {
    setPending(true);
    try {
      await api.confirmDay(date);
      onChange(date, 'confirmed_complete');
    } catch (err) {
      // Local state is left untouched on failure — the badge/button must
      // keep reflecting the last known-good server state rather than
      // optimistically flipping.
      showToast(err instanceof ApiError ? err.message : t('completeness.confirmFailed'), 'error');
    } finally {
      setPending(false);
    }
  };

  const handleUnconfirm = async () => {
    setPending(true);
    try {
      await api.unconfirmDay(date);
      onChange(date, 'unconfirmed');
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : t('completeness.unconfirmFailed'), 'error');
    } finally {
      setPending(false);
    }
  };

  // `whitespace-nowrap` throughout: this control sits in the history page's
  // wrapping day-header row, where its job is to drop whole onto its own line
  // when it cannot share one with the date. Allowed to wrap internally it does
  // the opposite — it compresses into the gap and breaks its own label across
  // two lines, which is the collision the two-row header exists to end.
  if (state === 'confirmed_complete') {
    return (
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium px-2 py-0.5 rounded-full whitespace-nowrap bg-green-100 dark:bg-green-900/40 text-green-700 dark:text-green-300">
          {t('completeness.completeBadge')}
        </span>
        <TapTarget
          onClick={handleUnconfirm}
          disabled={pending}
          className="flex items-center whitespace-nowrap text-xs text-gray-500 dark:text-gray-400 hover:underline disabled:opacity-50"
        >
          {pending ? t('completeness.updating') : t('completeness.unconfirm')}
        </TapTarget>
      </div>
    );
  }

  return (
    <TapTarget
      onClick={handleConfirm}
      disabled={pending}
      className="flex items-center whitespace-nowrap text-xs font-medium px-2 rounded-full border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:border-blue-400 hover:text-blue-600 dark:hover:text-blue-400 disabled:opacity-50"
    >
      {pending ? t('completeness.updating') : t('completeness.markComplete')}
    </TapTarget>
  );
}
