'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, ApiError } from '@/lib/api';
import { useLanguage } from '@/components/LanguageContext';
import { interpolate, type Dictionary } from '@/lib/i18n';
import { accessSignInSuppressed, clearAccessSignInSuppression } from '@/lib/session';

// Renders the limiter's retry hint as something a person can act on. Seconds
// below a minute, whole minutes above it — "try again in 90 seconds" reads
// worse than "in 2 minutes", and the backoff schedule reaches 30 minutes.
type Translate = (key: keyof Dictionary) => string;
function formatRetryAfter(seconds: number, t: Translate): string {
  const count = seconds < 60 ? Math.max(1, seconds) : Math.ceil(seconds / 60);
  const key = seconds < 60
    ? (count === 1 ? 'redesign.pages.login.second' : 'redesign.pages.login.seconds')
    : (count === 1 ? 'redesign.pages.login.minute' : 'redesign.pages.login.minutes');
  return interpolate(t(key), { count });
}

// A 429 from /auth/login is a lockout, not a bad password, and the two need
// different words: telling someone their credentials are invalid when they
// have just been locked out sends them round the same loop, escalating the
// backoff each time. The body carries retry_after_seconds; if it is missing
// or unparsable the message still has to say "locked out", just without a
// duration.
function lockoutMessage(body: string, t: Translate): string {
  let seconds = 0;
  try {
    const parsed = JSON.parse(body) as { retry_after_seconds?: number };
    if (typeof parsed.retry_after_seconds === 'number') seconds = parsed.retry_after_seconds;
  } catch {
    // Not the structured body — fall through to the durationless message.
  }
  if (seconds > 0) return interpolate(t('redesign.pages.login.lockout'), { duration: formatRetryAfter(seconds, t) });
  return t('redesign.pages.login.lockoutUnknown');
}

export default function LoginPage() {
  const router = useRouter();
  const { t } = useLanguage();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  // Hidden by default only once a completed exchange has actually 404'd —
  // this deployment has no Cloudflare in front. Left visible while unknown
  // (including the suppressed case) so a user landing here after logout can
  // still choose to sign in through Access again.
  const [accessHidden, setAccessHidden] = useState(false);
  const [accessMessage, setAccessMessage] = useState('');

  // Attempts the Cf-Access exchange and routes home on success. Used both by
  // the mount-time auto-attempt below and by the explicit sign-in button, so
  // the two can't diverge in how they interpret the response.
  const attemptAccessSignIn = async () => {
    try {
      await api.cfAccessLogin();
      router.push('/');
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setAccessHidden(true);
        return;
      }
      setAccessMessage(
        err instanceof ApiError && err.status === 403
          ? t('redesign.pages.login.notAuthorized')
          : t('redesign.pages.login.verificationFailed')
      );
    }
  };

  // Runs once on mount so a user whose Google sign-in Cloudflare Access
  // already verified is never asked for a password they may not have.
  // Skipped entirely right after a logout — see lib/session.ts's doc comment
  // on the suppression flag — otherwise ending a session would silently
  // re-authenticate on the very next page load.
  //
  // lib/api.ts's accessExchange checks the same flag, and that check is the
  // one that actually enforces logout across the app. This one is not
  // redundant with it: it keeps a suppressed mount from rendering a sign-in
  // failure the user never asked for, since the api layer can only answer by
  // failing.
  useEffect(() => {
    if (accessSignInSuppressed()) return;
    attemptAccessSignIn();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleAccessSignIn = () => {
    clearAccessSignInSuppression();
    setAccessMessage('');
    attemptAccessSignIn();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.login(username, password);
      router.push('/');
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        setError(lockoutMessage(err.message, t));
        return;
      }
      setError(t('redesign.pages.login.invalidCredentials'));
    }
  };

  return (
    <main className="min-h-screen flex items-center justify-center bg-bg px-5 py-10">
      <form onSubmit={handleSubmit} className="hv-card w-full max-w-md space-y-5">
        <h1 className="text-3xl font-bold tracking-tight text-center text-text">HealthVault</h1>
        <p className="text-sm text-text-muted text-center">{t('redesign.pages.login.subtitle')}</p>
        {!accessHidden && (
          <div className="space-y-2">
            {accessMessage && (
              <p className="text-amber-600 dark:text-amber-400 text-sm">{accessMessage}</p>
            )}
            {/* "Continue with Google", not "Sign in with Google": the
                password form's own submit button already matches e2e's
                /sign in|login/i role query (auth.spec.ts), and a second
                match on this button would make that query ambiguous. */}
            <button
              type="button"
              onClick={handleAccessSignIn}
              className="min-h-12 w-full border border-border rounded-xl px-4 py-3 font-semibold text-text hover:bg-bg transition-colors"
            >
              {t('redesign.pages.login.google')}
            </button>
            <div className="text-center text-xs text-text-muted">{t('redesign.pages.login.or')}</div>
          </div>
        )}
        {error && <p role="alert" className="text-red-600 dark:text-red-400 text-sm">{error}</p>}
        <input
          className="min-h-12 w-full border border-border rounded-xl px-4 py-3 bg-bg-elevated text-text placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-accent"
          placeholder={t('redesign.pages.login.username')}
          aria-label={t('redesign.pages.login.username')}
          autoComplete="username"
          value={username}
          onChange={e => setUsername(e.target.value)}
          required
        />
        <input
          className="min-h-12 w-full border border-border rounded-xl px-4 py-3 bg-bg-elevated text-text placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-accent"
          type="password"
          placeholder={t('redesign.pages.login.password')}
          aria-label={t('redesign.pages.login.password')}
          autoComplete="current-password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          required
        />
        <button className="min-h-12 w-full bg-accent hover:opacity-90 text-bg-elevated rounded-xl px-4 py-3 font-semibold transition-colors">
          {t('redesign.pages.login.submit')}
        </button>
      </form>
    </main>
  );
}
