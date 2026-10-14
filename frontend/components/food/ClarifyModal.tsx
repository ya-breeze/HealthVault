'use client';
import { ReactNode, useState } from 'react';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';

interface Props {
  questions: string[];
  onSubmit: (answers: string[]) => Promise<void>;
  // This modal's fixed inset-0 backdrop fully covers the rest of the review
  // page, so a meal stuck in pending_clarification (e.g. an accidental
  // upload) would otherwise have no way to be deleted short of answering
  // questions the user may not want to answer — this renders that escape
  // hatch inside the modal itself. Optional so this component stays usable
  // without coupling it to ReviewClient's delete plumbing.
  deleteControl?: ReactNode;
}

// Text-only by construction: no photo is shown or sent here, matching the
// backend's clarify endpoint, which never re-sends the image.
export default function ClarifyModal({ questions, onSubmit, deleteControl }: Props) {
  const { t } = useLanguage();
  const [answers, setAnswers] = useState<string[]>(() => questions.map(() => ''));
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const allAnswered = answers.every(a => a.trim() !== '');

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit(answers);
    } catch (err) {
      setError(err instanceof Error ? err.message : (t('redesign.review.failedToSubmitAnswers')));
      setSubmitting(false);
    }
  };

  return (
    <div data-testid="clarify-modal" className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4">
      <div className="bg-bg-elevated rounded-xl shadow-lg max-w-lg w-full p-6 max-h-[85vh] overflow-y-auto">
        <p className="text-sm font-semibold text-text mb-1">
          {t('redesign.review.aCoupleOfQuestions')}
        </p>
        <p className="text-xs text-text-muted mb-4">
          {t('redesign.review.theModelWasNotSureAboutYour')}
        </p>

        <div className="flex flex-col gap-4">
          {questions.map((q, i) => (
            <label key={i} className="block">
              <span className="text-sm text-text">{q}</span>
              <input
                type="text"
                value={answers[i]}
                onChange={e => {
                  const next = [...answers];
                  next[i] = e.target.value;
                  setAnswers(next);
                }}
                className="mt-1 w-full border border-border rounded-md px-3 py-2 text-sm bg-bg-elevated text-text focus:outline-none focus:ring-2 focus:ring-accent"
              />
            </label>
          ))}
        </div>

        {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <TapTarget
          onClick={handleSubmit}
          disabled={!allAnswered || submitting}
          className="mt-5 w-full rounded-lg text-sm font-medium bg-accent hover:opacity-90 text-bg-elevated disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {submitting ? (t('redesign.review.submitting')) : (t('redesign.review.submit'))}
        </TapTarget>

        {deleteControl && (
          <div className="mt-5 pt-4 border-t border-border">
            {deleteControl}
          </div>
        )}
      </div>
    </div>
  );
}
