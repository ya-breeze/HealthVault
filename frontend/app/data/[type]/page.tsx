import { Suspense } from 'react';
import { use } from 'react';
import { DATA_TYPES } from '@/lib/api';
import DataTypeClient from './DataTypeClient';

// Generate static routes for all 24 health data types
export function generateStaticParams() {
  return DATA_TYPES.map(type => ({ type }));
}

export default function DataTypePage({
  params,
}: {
  params: Promise<{ type: string }>;
}) {
  const { type } = use(params);
  return (
    <Suspense fallback={<div className="p-6 text-gray-500">Loading...</div>}>
      <DataTypeClient type={type} />
    </Suspense>
  );
}
