'use client';
import { useState } from 'react';

interface Props {
  questions: string[];
  onSubmit: (answers: string[]) => Promise<void>;
}

// Text-only by construction: no photo is shown or sent here, matching the
// backend's clarify endpoint, which never re-sends the image.
export default function ClarifyModal({ questions, onSubmit }: Props) {
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
      setError(err instanceof Error ? err.message : 'Failed to submit answers');
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-lg max-w-md w-full p-6">
        <p className="text-sm font-semibold text-gray-900 dark:text-white mb-1">
          A couple of questions
        </p>
        <p className="text-xs text-gray-500 dark:text-gray-400 mb-4">
          The model wasn&apos;t sure about your meal. No photo is sent for this — just your answers.
        </p>

        <div className="flex flex-col gap-4">
          {questions.map((q, i) => (
            <label key={i} className="block">
              <span className="text-sm text-gray-700 dark:text-gray-300">{q}</span>
              <input
                type="text"
                value={answers[i]}
                onChange={e => {
                  const next = [...answers];
                  next[i] = e.target.value;
                  setAnswers(next);
                }}
                className="mt-1 w-full border border-gray-300 dark:border-gray-600 rounded-md px-3 py-2 text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </label>
          ))}
        </div>

        {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

        <button
          onClick={handleSubmit}
          disabled={!allAnswered || submitting}
          className="mt-5 w-full py-2.5 rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {submitting ? 'Submitting…' : 'Submit'}
        </button>
      </div>
    </div>
  );
}
