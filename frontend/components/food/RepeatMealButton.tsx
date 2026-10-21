'use client';
import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api } from '@/lib/api';
import { useLanguage } from '@/components/LanguageContext';
import TapTarget from '@/components/ui/TapTarget';

export default function RepeatMealButton({ mealId }: { mealId: string }) {
  const router = useRouter();
  const { t } = useLanguage();
  const inFlight = useRef(false);
  // Retain the same key after an error: the server may have created the
  // meal even when the response never reached this browser.
  const requestId = useRef<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const repeat = async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError(null);
    try {
      if (!requestId.current) {
        requestId.current = Array.from(crypto.getRandomValues(new Uint8Array(16)),
          byte => byte.toString(16).padStart(2, '0')).join('');
      }
      const meal = await api.repeatMeal(mealId, requestId.current);
      router.push(`/food/review/?meal=${encodeURIComponent(meal.id)}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('mealHistory.repeatFailed'));
      inFlight.current = false;
      setBusy(false);
    }
  };

  return <div className="px-2 pb-3">
    <TapTarget data-testid={`repeat-meal-${mealId}`} onClick={repeat} disabled={busy}
      className="rounded-xl border border-border px-3 text-sm text-accent disabled:opacity-50">
      {busy ? t('mealHistory.repeating') : t('mealHistory.repeat')}
    </TapTarget>
    {error && <p role="alert" className="mt-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
  </div>;
}
