'use client';
import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api } from '@/lib/api';
import CameraCapture from '@/components/food/CameraCapture';

export default function FoodUploadPage() {
  const router = useRouter();
  const fileRef = useRef<HTMLInputElement>(null);
  const [showCamera, setShowCamera] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const upload = async (file: File) => {
    setUploading(true);
    setError(null);
    try {
      const meal = await api.uploadMeal(file);
      router.push(`/food/review/?meal=${meal.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed');
      setUploading(false);
    }
  };

  const handleFilePicked = () => {
    const file = fileRef.current?.files?.[0];
    if (file) upload(file);
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="max-w-md mx-auto flex items-center gap-4">
          <Link href="/" className="text-blue-600 dark:text-blue-400 hover:underline text-sm">
            &#8592; Dashboard
          </Link>
          <h1 className="text-xl font-bold text-gray-900 dark:text-white">Log a Meal</h1>
        </div>
      </header>

      <main className="max-w-md mx-auto px-6 py-10">
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">
          Take or upload a photo of your meal. The photo is sent to an external AI model (OpenAI)
          to identify foods and estimate portions — review and confirm before it&apos;s logged.
        </p>

        {uploading ? (
          <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 p-8 text-center">
            <p className="text-sm text-gray-600 dark:text-gray-300">Analyzing your photo…</p>
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">This can take up to a minute.</p>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            <button
              onClick={() => setShowCamera(true)}
              className="py-3 rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white"
            >
              Take Photo
            </button>
            <button
              onClick={() => fileRef.current?.click()}
              className="py-3 rounded-lg text-sm font-medium bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-700"
            >
              Choose Photo
            </button>
            <input
              ref={fileRef}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              className="hidden"
              onChange={handleFilePicked}
            />
            <Link
              href="/food/manual/"
              className="text-center py-2 text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
            >
              Enter manually instead
            </Link>
          </div>
        )}

        {error && (
          <div className="mt-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-xl p-4">
            <p className="text-sm text-red-700 dark:text-red-300">{error}</p>
          </div>
        )}
      </main>

      {showCamera && (
        <CameraCapture
          onClose={() => setShowCamera(false)}
          onCapture={file => {
            setShowCamera(false);
            upload(file);
          }}
        />
      )}
    </div>
  );
}
