'use client';
import { FormEvent, useState } from 'react';
import { api, ApiError, DataType } from '@/lib/api';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  type: DataType;
  onSuccess: () => void;
  // Present only when this instance is a shortcut into a type other than
  // the page it's mounted on (see the weight page's "Set goal" shortcut,
  // task 4.2) — an inline form for the page's own type has nothing to
  // cancel back out of.
  onCancel?: () => void;
}

// One reusable Add-record form, mounted on each allowlisted type's own page
// (weight/height/weight_goal) and reused as a modal-less inline panel for
// the weight page's "Set goal" shortcut (task 4.2) by passing a different
// `type`. POSTs to the write-allowlisted `/api/data/{type}` endpoint added
// in task 2; `time` is left out of the request entirely when blank so the
// backend's own "defaults to now()" behavior applies, rather than the form
// re-deriving "now" itself.
export default function AddRecordForm({ type, onSuccess, onCancel }: Props) {
  const [value, setValue] = useState('');
  const [time, setTime] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const numeric = Number(value);
    if (!value.trim() || !Number.isFinite(numeric) || numeric <= 0) {
      setError('Enter a positive number');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await api.createRecord(type, {
        value: numeric,
        ...(time ? { time: new Date(time).toISOString() } : {}),
      });
      setValue('');
      setTime('');
      onSuccess();
    } catch (err) {
      setError(err instanceof ApiError || err instanceof Error ? err.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-wrap items-end gap-3 bg-bg-elevated rounded-[12px] border border-border p-4 mb-4"
    >
      <label className="flex flex-col gap-1">
        <span className="text-xs text-text-muted">Value</span>
        <input
          type="number"
          step="any"
          value={value}
          onChange={e => setValue(e.target.value)}
          className="w-28 border border-border rounded-md px-2 py-1.5 text-sm bg-bg text-text"
          required
        />
      </label>
      <label className="flex flex-col gap-1">
        <span className="text-xs text-text-muted">Time (optional)</span>
        <input
          type="datetime-local"
          value={time}
          onChange={e => setTime(e.target.value)}
          className="border border-border rounded-md px-2 py-1.5 text-sm bg-bg text-text"
        />
      </label>
      <TapTarget
        type="submit"
        disabled={saving}
        className="rounded-md text-sm font-medium bg-accent text-bg-elevated px-4 py-1.5 disabled:opacity-50"
      >
        {saving ? 'Saving…' : 'Add'}
      </TapTarget>
      {onCancel && (
        <TapTarget
          type="button"
          onClick={onCancel}
          className="rounded-md text-sm font-medium bg-border text-text px-4 py-1.5"
        >
          Cancel
        </TapTarget>
      )}
      {error && <p className="w-full text-sm text-red-600 dark:text-red-400">{error}</p>}
    </form>
  );
}
