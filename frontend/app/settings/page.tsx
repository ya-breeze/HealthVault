'use client';
import { ChangeEvent, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, UserSettings } from '@/lib/api';
import AuthenticatedShell from '@/components/AuthenticatedShell';
import TapTarget from '@/components/ui/TapTarget';
import { useLanguage } from '@/components/LanguageContext';
import { SUPPORTED_LANGUAGES, LanguageCode } from '@/lib/i18n';
import { useToast } from '@/components/Toast';

type ActivityOverride = NonNullable<UserSettings['activity_override']>;

// Display labels for the manual activity-tier override. Deliberately not
// derived from the enum values themselves — 'active' and 'very_active' do
// not positionally match their tier names ("Very active" / "Extra active").
// See design.md's activity_override -> tier-name table.
const ACTIVITY_TIERS: [ActivityOverride, string][] = [
  ['sedentary', 'Sedentary'],
  ['light', 'Lightly active'],
  ['moderate', 'Moderately active'],
  ['active', 'Very active'],
  ['very_active', 'Extra active'],
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
  const [sex, setSex] = useState<'' | 'male' | 'female'>('');
  const [activityOverride, setActivityOverride] = useState<'' | ActivityOverride>('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [changingLanguage, setChangingLanguage] = useState(false);

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
        setSex(normalizeSex(s.sex));
        setActivityOverride(normalizeActivityOverride(s.activity_override));
      })
      .catch(err => {
        if (err instanceof Error && err.message.includes('401')) {
          router.push('/login');
          return;
        }
        setError('Failed to load your profile.');
      })
      .finally(() => setLoading(false));
  }, [router]);

  const handleSave = async () => {
    if (!birthdate || !sex) {
      setError('Birthdate and sex are required.');
      return;
    }
    if (!isValidBirthdate(birthdate)) {
      setError('Enter a valid birthdate: not in the future, and implying an age between 5 and 120.');
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
      showToast('Profile saved', 'success');
    } catch {
      showToast('Could not save your profile. Try again.', 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <AuthenticatedShell className="min-h-screen bg-bg">
      <main className="max-w-md mx-auto px-6 py-8">
        <h1 className="text-xl font-bold text-text mb-6">Settings</h1>

        <section>
          <h2 className="text-sm font-semibold text-text mb-4">Profile</h2>

          {loading ? (
            <p className="text-sm text-text-muted">Loading…</p>
          ) : (
            <div className="space-y-4">
              <label className="block">
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
                  className="mt-1 w-full border border-border rounded-md px-3 bg-bg text-text disabled:opacity-60"
                >
                  {SUPPORTED_LANGUAGES.map(l => (
                    <option key={l.code} value={l.code}>{l.label}</option>
                  ))}
                </TapTarget>
              </label>

              <label className="block">
                <span className="text-xs text-text-muted">Birthdate</span>
                <TapTarget
                  as="input"
                  type="date"
                  value={birthdate}
                  onChange={(e: ChangeEvent<HTMLInputElement>) => setBirthdate(e.target.value)}
                  className="mt-1 w-full border border-border rounded-md px-3 bg-bg text-text"
                />
              </label>

              <label className="block">
                <span className="text-xs text-text-muted">Sex</span>
                <TapTarget
                  as="select"
                  value={sex}
                  onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                    setSex(e.target.value as '' | 'male' | 'female')
                  }
                  className="mt-1 w-full border border-border rounded-md px-3 bg-bg text-text"
                >
                  <option value="">Select…</option>
                  <option value="male">Male</option>
                  <option value="female">Female</option>
                </TapTarget>
              </label>

              <label className="block">
                <span className="text-xs text-text-muted">Activity level</span>
                <TapTarget
                  as="select"
                  value={activityOverride}
                  onChange={(e: ChangeEvent<HTMLSelectElement>) =>
                    setActivityOverride(e.target.value as '' | ActivityOverride)
                  }
                  className="mt-1 w-full border border-border rounded-md px-3 bg-bg text-text"
                >
                  <option value="">Automatic (based on steps)</option>
                  {ACTIVITY_TIERS.map(([value, label]) => (
                    <option key={value} value={value}>{label}</option>
                  ))}
                </TapTarget>
              </label>

              {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

              <TapTarget
                onClick={handleSave}
                disabled={saving}
                className="w-full rounded-lg text-sm font-medium bg-accent text-bg-elevated hover:opacity-90 disabled:opacity-50"
              >
                {saving ? 'Saving…' : 'Save'}
              </TapTarget>
            </div>
          )}
        </section>
      </main>
    </AuthenticatedShell>
  );
}
