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
    <AuthenticatedShell className="min-h-screen bg-bg">
      <main className="hv-page">
        <div className="hv-page-heading">
          <div><h1>{t('redesign.entry.uploadTitle')}</h1>
            <p>{t('redesign.entry.uploadSubtitle')}</p>
          </div>
          <TapTarget as={Link} href="/food/history/" className="inline-flex items-center justify-center rounded-xl border border-border bg-bg-elevated px-5 text-sm font-semibold text-text">{t('redesign.entry.backToDiary')}</TapTarget>
        </div>
        <nav aria-label={t('redesign.entry.optionsLabel')} className="mb-6 flex flex-wrap gap-2">
          <TapTarget as={Link} href="/food/upload/" aria-current="page" className="inline-flex items-center justify-center rounded-xl bg-accent/10 px-5 text-sm font-semibold text-accent">{t('redesign.entry.photo')}</TapTarget>
          <TapTarget as={Link} href="/food/manual/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.manualModes')}</TapTarget>
          <TapTarget as={Link} href="/food/custom/" className="inline-flex items-center justify-center rounded-xl px-5 text-sm text-text-muted hover:bg-bg-elevated">{t('redesign.entry.catalog')}</TapTarget>
        </nav>
        <section className="hv-card mx-auto max-w-3xl">
          <p className="text-sm text-text-muted mb-6">
            Take or upload a photo of your meal. The photo is sent to an external AI model (OpenAI)
            to identify foods and estimate portions — review and confirm before it&apos;s logged.
          </p>

          {uploading ? (
            <div className="rounded-2xl border border-border bg-bg p-10 text-center" role="status">
              <p className="text-sm text-text-muted">Analyzing your photo…</p>
              <p className="text-xs text-text-muted mt-1">This can take up to a minute.</p>
            </div>
          ) : (
            <div className="flex flex-col gap-5">
              {selectedFile && previewUrl && (
                <div className="rounded-2xl border border-border bg-bg p-4">
                  {/* The selected photo is a local object URL, released when replaced or removed. */}
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={previewUrl} alt={t('photoDraft.preview')} data-testid="photo-preview"
                    className="max-h-64 w-full rounded-xl object-contain" />
                  <TapTarget onClick={() => { setSelectedFile(null); setError(null); }}
                    data-testid="photo-remove"
                    className="mt-2 w-full text-sm text-text-muted">
                    {t('photoDraft.remove')}
                  </TapTarget>
                </div>
              )}
              <MealCookingContextInputs value={cookingContext} onChange={setCookingContext} />
              {showHint ? (
                <div className="rounded-2xl border border-border bg-bg-elevated p-5">
                  <label htmlFor="meal-hint" className="block text-sm font-medium text-text">
                    Photo hint <span className="font-normal text-text-muted">(optional)</span>
                  </label>
                  <p className="mt-1 text-xs text-text-muted">
                    Mention anything the photo may not make clear, such as “grilled chicken with red beans.”
                  </p>
                  <textarea
                    id="meal-hint"
                    value={hint}
                    onChange={event => setHint(event.target.value)}
                    rows={3}
                    placeholder="What should the model know?"
                    className="mt-3 w-full rounded-xl border border-border bg-bg-elevated px-3 py-2 text-base text-text"
                  />
                  <p className="mt-1 text-right text-xs text-text-muted">
                    {normalizedUnicodeLength(hint)}/{MAX_HINT_LENGTH}
                  </p>
                </div>
              ) : (
                <TapTarget
                  onClick={() => setShowHint(true)}
                  className="rounded-xl border border-dashed border-border px-4 text-sm font-medium text-text-muted hover:border-accent hover:text-accent"
                >
                  Add a hint (optional)
                </TapTarget>
              )}
              {selectedFile && (
                <TapTarget onClick={upload} data-testid="photo-analyze"
                  className="rounded-xl text-sm font-medium bg-accent hover:bg-accent/90 text-bg-elevated">
                  {t('photoDraft.analyze')}
                </TapTarget>
              )}
              <TapTarget
                onClick={() => setShowCamera(true)}
                className="rounded-xl text-sm font-medium bg-accent hover:bg-accent/90 text-bg-elevated"
              >
                Take Photo
              </TapTarget>
              <TapTarget
                onClick={() => fileRef.current?.click()}
                className="rounded-xl text-sm font-medium bg-bg-elevated border border-border text-text hover:bg-bg"
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
                className="flex items-center justify-center text-center text-sm text-text-muted hover:text-text"
              >
                Enter manually instead
              </TapTarget>
            </div>
          )}

          {error && (
            <div role="alert" className="mt-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-xl p-4">
              <p className="text-sm text-red-700 dark:text-red-300">{error}</p>
            </div>
          )}
        </section>
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
