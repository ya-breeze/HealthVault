'use client';
import { FormEvent, useState } from 'react';
import { api, DataType } from '@/lib/api';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate, type Dictionary } from '@/lib/i18n';

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
// Unit and plausibility range per writable type, mirroring the API's own
// writeBounds. One generic form serves units of very different magnitude, so
// without a visible unit the natural "178" for a height is a 178-metre record
// that the old `> 0` check happily accepted.
type UnitKey = Extract<keyof Dictionary, `unit.${string}`>;

const WRITE_UNITS: Partial<Record<DataType, { unitKey: UnitKey; min: number; max: number }>> = {
  weight: { unitKey: 'unit.kg', min: 20, max: 500 },
  weight_goal: { unitKey: 'unit.kg', min: 20, max: 500 },
  height: { unitKey: 'unit.m', min: 0.5, max: 2.5 },
};

type FormError =
  | { kind: 'positive' }
  | { kind: 'range' }
  | { kind: 'saveFailed' };

// `datetime-local` wants a local-time "YYYY-MM-DDTHH:mm" string, not an ISO
// UTC one — toISOString() here would set the ceiling to the wrong instant for
// anyone not on UTC.
function maxLocalDateTime(): string {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export default function AddRecordForm({ type, onSuccess, onCancel }: Props) {
  const { t } = useLanguage();
  const spec = WRITE_UNITS[type];
  const unit = spec ? t(spec.unitKey) : undefined;
  const [value, setValue] = useState('');
  const [time, setTime] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<FormError | null>(null);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const numeric = Number(value);
    if (!value.trim() || !Number.isFinite(numeric) || numeric <= 0) {
      setError({ kind: 'positive' });
      return;
    }
    if (spec && (numeric < spec.min || numeric > spec.max)) {
      setError({ kind: 'range' });
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
    } catch {
      setError({ kind: 'saveFailed' });
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={handleSubmit}
      // Several of these can be on one page at once (the weight page renders
      // its own plus the goal/height shortcuts), so each needs to be
      // addressable on its own rather than by a shared label.
      data-testid={`add-record-${type}`}
      className="flex flex-wrap items-end gap-3 bg-bg-elevated rounded-[12px] border border-border p-4 mb-4"
    >
      <label className="flex flex-col gap-1">
        <span className="text-xs text-text-muted">
          {spec && unit
            ? interpolate(t('addRecord.valueWithUnit'), { unit })
            : t('addRecord.value')}
        </span>
        <input
          type="number"
          step="any"
          value={value}
          onChange={e => setValue(e.target.value)}
          min={spec?.min}
          max={spec?.max}
          className="w-28 border border-border rounded-md px-2 py-1.5 text-sm bg-bg text-text"
          required
        />
      </label>
      <label className="flex flex-col gap-1">
        <span className="text-xs text-text-muted">{t('addRecord.time')}</span>
        <input
          type="datetime-local"
          value={time}
          onChange={e => setTime(e.target.value)}
          // Records are only ever read up to now, so a future date would
          // create a row that is invisible everywhere and therefore also
          // un-deletable. The API rejects it too — this just surfaces the
          // limit before the round-trip.
          max={maxLocalDateTime()}
          className="border border-border rounded-md px-2 py-1.5 text-sm bg-bg text-text"
        />
      </label>
      <TapTarget
        type="submit"
        disabled={saving}
        className="rounded-md text-sm font-medium bg-accent text-bg-elevated px-4 py-1.5 disabled:opacity-50"
      >
        {saving ? t('addRecord.saving') : t('addRecord.add')}
      </TapTarget>
      {onCancel && (
        <TapTarget
          type="button"
          onClick={onCancel}
          className="rounded-md text-sm font-medium bg-border text-text px-4 py-1.5"
        >
          {t('addRecord.cancel')}
        </TapTarget>
      )}
      {error && (
        <p className="w-full text-sm text-red-600 dark:text-red-400">
          {error.kind === 'positive'
            ? t('addRecord.positiveNumber')
            : error.kind === 'range' && spec && unit
              ? interpolate(t('addRecord.range'), {
                  min: spec.min,
                  max: spec.max,
                  unit,
                })
              : t('addRecord.saveFailed')}
        </p>
      )}
    </form>
  );
}
