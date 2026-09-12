'use client';
import { useLanguage } from '@/components/LanguageContext';

// The <Suspense> fallback for DataTypeClient (page.tsx) — a client component
// so it can read the selected Display Language before DataTypeClient itself
// has mounted. Distinct from weightContextStatus === 'loading' inside
// DataTypeClient, which stays intentionally silent until the weight context
// settles; this is the route-level fallback shown before any client code has
// run at all.
export default function DataTypeLoading() {
  const { t } = useLanguage();
  return <div className="p-6 text-gray-500">{t('dataDetail.loading')}</div>;
}
