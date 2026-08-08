'use client';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import ReviewClient from './ReviewClient';

// output: 'export' means a [id] dynamic segment can't be generated for a
// runtime UUID, so the meal is addressed via a query param instead:
// /food/review/?meal=<uuid>.
function ReviewPageInner() {
  const searchParams = useSearchParams();
  const mealId = searchParams.get('meal');

  if (!mealId) {
    return <p className="p-6 text-gray-500 dark:text-gray-400 text-center text-sm">No meal specified.</p>;
  }
  return <ReviewClient mealId={mealId} />;
}

export default function FoodReviewPage() {
  return (
    <Suspense fallback={<div className="p-6 text-gray-500">Loading...</div>}>
      <ReviewPageInner />
    </Suspense>
  );
}
