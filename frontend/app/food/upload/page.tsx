'use client';
import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { api } from '@/lib/api';
import CameraCapture from '@/components/food/CameraCapture';
import MealCookingContextInputs, { defaultMealCookingContext } from '@/components/food/MealCookingContextInputs';
import { useLanguage } from '@/components/LanguageContext';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import TapTarget from '@/components/ui/TapTarget';
import { MAX_HINT_LENGTH, normalizedUnicodeLength, unicodeLength } from '@/lib/foodGuidance';

export default function FoodUploadPage() {
  const router = useRouter();
  const { t } = useLanguage();
  const uploadInFlight = useRef(false);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState('');

  useEffect(() => {
    if (!selectedFile) { setPreviewUrl(''); return; }
    const url = URL.createObjectURL(selectedFile);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [selectedFile]);
  const fileRef = useRef<HTMLInputElement>(null);
  const [showCamera, setShowCamera] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHint, setShowHint] = useState(false);
  const [hint, setHint] = useState('');
  const [cookingContext, setCookingContext] = useState(defaultMealCookingContext);

  const upload = async () => {
    if (!selectedFile || uploadInFlight.current) return;
    const trimmedHint = hint.trim();
    if (unicodeLength(trimmedHint) > MAX_HINT_LENGTH) {
      setError(`Hint must be at most ${MAX_HINT_LENGTH} characters`);
      return;
    }
    uploadInFlight.current = true;
    setUploading(true);
    setError(null);
    try {
      const meal = await api.uploadMeal(selectedFile, trimmedHint, cookingContext);
      router.push(`/food/review/?meal=${meal.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed');
      uploadInFlight.current = false;
      setUploading(false);
    }
  };

  const stagePhoto = (file: File) => {
    setSelectedFile(file);
    setShowHint(true);
    setError(null);
  };

  const handleFilePicked = () => {
    const file = fileRef.current?.files?.[0];
    if (file) stagePhoto(file);
    if (fileRef.current) fileRef.current.value = '';
  };

  return (
    <AuthenticatedShell className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <main className="max-w-md mx-auto px-6 py-10">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-6">Log a Meal</h1>
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
            {selectedFile && previewUrl && (
              <div className="rounded-xl border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-800">
                {/* The selected photo is a local object URL, released when replaced or removed. */}
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src={previewUrl} alt={t('photoDraft.preview')} data-testid="photo-preview"
                  className="max-h-64 w-full rounded-lg object-contain" />
                <TapTarget onClick={() => { setSelectedFile(null); setError(null); }}
                  data-testid="photo-remove"
                  className="mt-2 w-full text-sm text-gray-600 dark:text-gray-300">
                  {t('photoDraft.remove')}
                </TapTarget>
              </div>
            )}
            <MealCookingContextInputs value={cookingContext} onChange={setCookingContext} />
            {showHint ? (
              <div className="rounded-xl border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
                <label htmlFor="meal-hint" className="block text-sm font-medium text-gray-900 dark:text-white">
                  Photo hint <span className="font-normal text-gray-500 dark:text-gray-400">(optional)</span>
                </label>
                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Mention anything the photo may not make clear, such as “grilled chicken with red beans.”
                </p>
                <textarea
                  id="meal-hint"
                  value={hint}
                  onChange={event => setHint(event.target.value)}
                  rows={3}
                  placeholder="What should the model know?"
                  className="mt-3 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-base text-gray-900 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                />
                <p className="mt-1 text-right text-xs text-gray-400">
                  {normalizedUnicodeLength(hint)}/{MAX_HINT_LENGTH}
                </p>
              </div>
            ) : (
              <TapTarget
                onClick={() => setShowHint(true)}
                className="rounded-lg border border-dashed border-gray-300 px-4 text-sm font-medium text-gray-600 hover:border-blue-400 hover:text-blue-600 dark:border-gray-600 dark:text-gray-300 dark:hover:text-blue-400"
              >
                Add a hint (optional)
              </TapTarget>
            )}
            {selectedFile && (
              <TapTarget onClick={upload} data-testid="photo-analyze"
                className="rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white">
                {t('photoDraft.analyze')}
              </TapTarget>
            )}
            <TapTarget
              onClick={() => setShowCamera(true)}
              className="rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white"
            >
              Take Photo
            </TapTarget>
            <TapTarget
              onClick={() => fileRef.current?.click()}
              className="rounded-lg text-sm font-medium bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-700"
            >
              Choose Photo
            </TapTarget>
            <input
              ref={fileRef}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              className="hidden"
              onChange={handleFilePicked}
            />
            <TapTarget
              as={Link}
              href="/food/manual/"
              className="flex items-center justify-center text-center text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
            >
              Enter manually instead
            </TapTarget>
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
            stagePhoto(file);
          }}
        />
      )}
    </AuthenticatedShell>
  );
}
