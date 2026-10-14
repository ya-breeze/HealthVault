'use client';
import { ChangeEvent, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, UserSettings } from '@/lib/api';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import AddRecordForm from '@/components/AddRecordForm';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { SUPPORTED_LANGUAGES, LanguageCode, type Dictionary } from '@/lib/i18n';
import { useToast } from '@/components/Toast';

type ActivityOverride = NonNullable<UserSettings['activity_override']>;

// Display labels for the manual activity-tier override. Deliberately not
// derived from the enum values themselves — 'active' and 'very_active' do
// not positionally match their tier names ("Very active" / "Extra active").
// See design.md's activity_override -> tier-name table.
const ACTIVITY_TIERS: [ActivityOverride, keyof Dictionary][] = [
  ['sedentary', 'redesign.pages.settings.activity.sedentary'],
  ['light', 'redesign.pages.settings.activity.light'],
  ['moderate', 'redesign.pages.settings.activity.moderate'],
  ['active', 'redesign.pages.settings.activity.active'],
  ['very_active', 'redesign.pages.settings.activity.very_active'],
];

const ACTIVITY_OVERRIDE_VALUES = new Set(ACTIVITY_TIERS.map(([value]) => value));
const BIRTHDATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;
const MIN_PROFILE_AGE_YEARS = 5;
const MAX_PROFILE_AGE_YEARS = 120;

// Mirrors the backend's parseBirthdate (backend/pkg/server/user_profile.go):
// strict YYYY-MM-DD (no rollover like 2025-02-31), no future dates, and a
// calendar age within [5, 120]. Kept in sync so the UI never reports
// "Profile saved" for a value the backend will silently treat as absent
// (user-profile spec's "interpreted, not assumed" contract).
function isValidBirthdate(raw: string): boolean {
  if (!BIRTHDATE_PATTERN.test(raw)) return false;
  const [year, month, day] = raw.split('-').map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCFullYear() !== year || date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) {
    return false;
  }

  const now = new Date();
  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  if (date.getTime() > today.getTime()) return false;

  let age = today.getUTCFullYear() - date.getUTCFullYear();
  const beforeBirthdayThisYear =
    today.getUTCMonth() < date.getUTCMonth() ||
    (today.getUTCMonth() === date.getUTCMonth() && today.getUTCDate() < date.getUTCDate());
  if (beforeBirthdayThisYear) age--;

  return age >= MIN_PROFILE_AGE_YEARS && age <= MAX_PROFILE_AGE_YEARS;
}

// The settings blob is schema-agnostic at the storage layer (any client can
// PUT arbitrary JSON), so a fetched value may not match its declared type.
// Normalize to '' rather than trusting it as a valid typed field — otherwise
// a stale/invalid value can look "set" to the required-field check even
// though the backend treats it as absent (see user-profile spec's
// "interpreted, not assumed" contract).
function normalizeBirthdate(raw: string | undefined): string {
  if (!raw || !isValidBirthdate(raw)) return '';
  return raw;
}

function normalizeSex(raw: string | undefined): '' | 'male' | 'female' {
  return raw === 'male' || raw === 'female' ? raw : '';
}

function normalizeActivityOverride(raw: string | undefined): '' | ActivityOverride {
  return raw && ACTIVITY_OVERRIDE_VALUES.has(raw as ActivityOverride) ? (raw as ActivityOverride) : '';
}

export default function SettingsPage() {
  const router = useRouter();
  // updateSettings, not api.updateSettings — queues this form's PUT behind
  // LanguageContext's own claim() alongside the Display Language switcher's
  // reads/writes, so the two can't race and silently clobber each other's
  // save. See design.md's "third settings writer" decision.
  const { t, language, setLanguage, updateSettings } = useLanguage();
  const { showToast } = useToast();

  const [birthdate, setBirthdate] = useState('');
  const [fiberTarget, setFiberTarget] = useState('');
  const [savingFiber, setSavingFiber] = useState(false);
  const [sex, setSex] = useState<'' | 'male' | 'female'>('');
  const [activityOverride, setActivityOverride] = useState<'' | ActivityOverride>('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [changingLanguage, setChangingLanguage] = useState(false);
  // Unlike the Weight page's own "Set height"/"Set goal" shortcuts (task 4.2), neither toggle
  // here is gated on whether a record already exists — this section's whole purpose is to stay
  // available after that one-time onboarding shortcut has already served its purpose and retired
  // (height) or regardless of it (goal weight, which never had that gate). See
  // docs/specs/a-permanent-way-to-change-height-and-go.md's Why.
  const [showHeightForm, setShowHeightForm] = useState(false);
  const [showGoalForm, setShowGoalForm] = useState(false);

  const handleLanguageChange = async (code: LanguageCode) => {
    if (code === language) return;
    setChangingLanguage(true);
    try {
      await setLanguage(code);
    } catch {
      showToast(t('header.languageChangeFailed'), 'error');
    } finally {
      setChangingLanguage(false);
    }
  };

  useEffect(() => {
    api.getSettings()
      .then(s => {
        setBirthdate(normalizeBirthdate(s.birthdate));
        setFiberTarget(typeof s.fiber_target_grams === 'number' && Number.isInteger(s.fiber_target_grams) && s.fiber_target_grams >= 1 && s.fiber_target_grams <= 200 ? String(s.fiber_target_grams) : '');
        setSex(normalizeSex(s.sex));
        setActivityOverride(normalizeActivityOverride(s.activity_override));
      })
      .catch(err => {
        if (err instanceof Error && err.message.includes('401')) {
          router.push('/login');
          return;
        }
        setError(t('redesign.pages.settings.loadFailed'));
      })
      .finally(() => setLoading(false));
  }, [router]);

  const handleSave = async () => {
    if (!birthdate || !sex) {
      setError(t('redesign.pages.settings.required'));
      return;
    }
    if (!isValidBirthdate(birthdate)) {
      setError(t('redesign.pages.settings.birthdateInvalid'));
      return;
    }
    setError(null);
    setSaving(true);
    try {
      await updateSettings({
        birthdate,
        sex,
        activity_override: activityOverride || undefined,
      });
      showToast(t('redesign.pages.settings.saved'), 'success');
    } catch {
      showToast(t('redesign.pages.settings.saveFailed'), 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleSaveFiber = async () => {
    const fiber = fiberTarget.trim() ? Number(fiberTarget) : undefined;
    if (fiber !== undefined && (!Number.isInteger(fiber) || fiber < 1 || fiber > 200)) {
      setError(t('redesign.pages.settings.fiberInvalid'));
      return;
    }
    setError(null);
    setSavingFiber(true);
    try {
      await updateSettings({fiber_target_grams: fiber});
      showToast(t('redesign.pages.settings.fiberSaved'), 'success');
    } catch {
      showToast(t('redesign.pages.settings.fiberSaveFailed'), 'error');
    } finally {setSavingFiber(false);}
  };

  return (
    <AuthenticatedShell className="min-h-screen bg-bg">
      <main className="hv-page">
        <div className="hv-page-heading"><div><h1>{t('header.settings')}</h1><p className="text-text-muted mt-2">{t('redesign.pages.settings.subtitle')}</p></div></div>
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)] items-start">

        <section className="hv-card">
          <h2 className="text-lg font-semibold text-text mb-6">{t('redesign.pages.settings.profile')}</h2>

          {loading ? (
            <p className="text-sm text-text-muted">{t('review.loading')}</p>
          ) : (
            <div className="grid gap-5 sm:grid-cols-2">
              <label className="block min-w-0">
                <span className="text-xs text-text-muted" id="display-language-label">
                  {t('header.language')}
                </span>
                <TapTarget
                  as="select"
                  id="display-language"
                  aria-labelledby="display-language-label"
                  value={language}
                  disabled={changingLanguage}
                  onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                    handleLanguageChange(e.target.value as LanguageCode)
                  }
                  className="mt-1 w-full border border-border rounded-xl px-3 bg-bg text-text disabled:opacity-60"
                >
                  {SUPPORTED_LANGUAGES.map(l => (
                    <option key={l.code} value={l.code}>{l.label}</option>
                  ))}
                </TapTarget>
              </label>

              <label className="block min-w-0">
                <span className="text-xs text-text-muted">{t('redesign.pages.settings.birthdate')}</span>
                <TapTarget
                  as="input"
                  type="date"
                  value={birthdate}
                  onChange={(e: ChangeEvent<HTMLInputElement>) => setBirthdate(e.target.value)}
                  className="mt-1 w-full border border-border rounded-xl px-3 bg-bg text-text"
                />
              </label>

              <label className="block min-w-0">
                <span className="text-xs text-text-muted">{t('redesign.pages.settings.sex')}</span>
                <TapTarget
                  as="select"
                  value={sex}
                  onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                    setSex(e.target.value as '' | 'male' | 'female')
                  }
                  className="mt-1 w-full border border-border rounded-xl px-3 bg-bg text-text"
                >
                  <option value="">{t('redesign.pages.settings.select')}</option>
                  <option value="male">{t('redesign.pages.settings.male')}</option>
                  <option value="female">{t('redesign.pages.settings.female')}</option>
                </TapTarget>
              </label>

              <label className="block min-w-0">
                <span className="text-xs text-text-muted">{t('redesign.pages.settings.activity')}</span>
                <TapTarget
                  as="select"
                  value={activityOverride}
                  onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                    setActivityOverride(e.target.value as '' | ActivityOverride)
                  }
                  className="mt-1 w-full border border-border rounded-xl px-3 bg-bg text-text"
                >
                  <option value="">{t('redesign.pages.settings.automatic')}</option>
                  {ACTIVITY_TIERS.map(([value, label]) => (
                    <option key={value} value={value}>{t(label)}</option>
                  ))}
                </TapTarget>
              </label>

              <div className="sm:col-span-2">
                <label htmlFor="fiber-target" className="text-xs text-text-muted">{t('redesign.pages.settings.fiberLabel')}</label>
                <TapTarget as="input" id="fiber-target" aria-describedby="fiber-target-help" type="number" min={1} max={200} step={1} value={fiberTarget}
                  onChange={(e: ChangeEvent<HTMLInputElement>) => setFiberTarget(e.target.value)}
                  placeholder={t('redesign.pages.settings.fiberDefault')}
                  className="mt-1 w-full border border-border rounded-xl px-3 bg-bg text-text" />
                <p id="fiber-target-help" className="mt-2 text-xs leading-relaxed text-text-muted">{t('redesign.pages.settings.fiberHelp')}</p>
                <TapTarget onClick={handleSaveFiber} disabled={loading || savingFiber} data-testid="save-fiber-target" className="mt-3 rounded-xl bg-accent text-bg-elevated px-4 text-sm font-semibold disabled:opacity-50">{savingFiber ? t('addRecord.saving') : t('redesign.pages.settings.saveFiber')}</TapTarget>

              </div>

              {error && <p role="alert" className="sm:col-span-2 text-sm text-red-600 dark:text-red-400">{error}</p>}

              <TapTarget
                onClick={handleSave}
                disabled={saving}
                className="sm:col-span-2 w-full rounded-xl text-sm font-semibold bg-accent text-bg-elevated hover:opacity-90 disabled:opacity-50"
              >
                {saving ? t('addRecord.saving') : t('resolver.save')}
              </TapTarget>
            </div>
          )}
        </section>

        {/*
          A permanent counterpart to the Weight page's "Set height"/"Set goal" shortcuts
          (task 4.2) — those exist to help a user with zero records discover the write path
          once, and the height one deliberately retires after its first use (see
          e2e/tests/data-types.spec.ts and this section's own spec). Neither toggle here is
          gated on whether a record already exists: this section's whole point is to stay
          reachable afterward, and independently of the profile fetch above.
        */}
        <section className="hv-card">
          <h2 className="text-lg font-semibold text-text mb-6">{t('redesign.pages.settings.measurements')}</h2>

          <div className="space-y-3">
            {showHeightForm ? (
              <AddRecordForm
                type="height"
                onSuccess={() => setShowHeightForm(false)}
                onCancel={() => setShowHeightForm(false)}
              />
            ) : (
              <TapTarget
                type="button"
                onClick={() => setShowHeightForm(true)}
                data-testid="settings-set-height"
                className="rounded-xl text-sm font-medium bg-border text-text px-4 py-1.5"
              >
                {t('dataDetail.setHeight')}
              </TapTarget>
            )}

            {showGoalForm ? (
              <AddRecordForm
                type="weight_goal"
                onSuccess={() => setShowGoalForm(false)}
                onCancel={() => setShowGoalForm(false)}
              />
            ) : (
              <TapTarget
                type="button"
                onClick={() => setShowGoalForm(true)}
                data-testid="settings-set-goal"
                className="rounded-xl text-sm font-medium bg-border text-text px-4 py-1.5"
              >
                {t('dataDetail.setGoal')}
              </TapTarget>
            )}
          </div>
        </section>
        </div>
      </main>
    </AuthenticatedShell>
  );
}
