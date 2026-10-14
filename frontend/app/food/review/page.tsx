'use client';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import ReviewClient from './ReviewClient';
import { useLanguage } from '@/components/LanguageContext';

// output: 'export' means a [id] dynamic segment can't be generated for a
// runtime UUID, so the meal is addressed via a query param instead:
// /food/review/?meal=<uuid>.
function ReviewPageInner() {
  const { t } = useLanguage();
  const searchParams = useSearchParams();
  const mealId = searchParams.get('meal');

  if (!mealId) {
    return <p className="p-6 text-text-muted text-center text-sm">{t('redesign.review.noMealSpecified')}</p>;
  }
  return <ReviewClient mealId={mealId} />;
}

export default function FoodReviewPage() {
  const { t } = useLanguage();
  return (
    <Suspense fallback={<div className="p-6 text-text-muted">{t('review.loading')}</div>}>
      <ReviewPageInner />
    </Suspense>
  );
}
