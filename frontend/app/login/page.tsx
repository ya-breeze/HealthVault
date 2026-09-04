'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, ApiError } from '@/lib/api';
import { accessSignInSuppressed, clearAccessSignInSuppression } from '@/lib/session';

// Renders the limiter's retry hint as something a person can act on. Seconds
// below a minute, whole minutes above it — "try again in 90 seconds" reads
// worse than "in 2 minutes", and the backoff schedule reaches 30 minutes.
function formatRetryAfter(seconds: number): string {
  if (seconds < 60) return `${Math.max(1, seconds)} second${seconds === 1 ? '' : 's'}`;
  const minutes = Math.ceil(seconds / 60);
  return `${minutes} minute${minutes === 1 ? '' : 's'}`;
}

// A 429 from /auth/login is a lockout, not a bad password, and the two need
// different words: telling someone their credentials are invalid when they
// have just been locked out sends them round the same loop, escalating the
// backoff each time. The body carries retry_after_seconds; if it is missing
// or unparsable the message still has to say "locked out", just without a
// duration.
function lockoutMessage(body: string): string {
  let seconds = 0;
  try {
    const parsed = JSON.parse(body) as { retry_after_seconds?: number };
    if (typeof parsed.retry_after_seconds === 'number') seconds = parsed.retry_after_seconds;
  } catch {
    // Not the structured body — fall through to the durationless message.
  }
  if (seconds > 0) return `Too many sign-in attempts. Try again in ${formatRetryAfter(seconds)}.`;
  return 'Too many sign-in attempts. Try again shortly.';
}

export default function LoginPage() {
  const router = useRouter();
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
          ? 'Your Google account is not authorized for HealthVault.'
          : 'Verification with Google failed.'
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
        setError(lockoutMessage(err.message));
        return;
      }
      setError('Invalid credentials');
    }
  };

  return (
    <main className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900">
      <form onSubmit={handleSubmit} className="bg-white dark:bg-gray-800 p-8 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 w-80 space-y-4">
        <h1 className="text-2xl font-bold text-center text-gray-900 dark:text-white">HealthVault</h1>
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
              className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 font-medium text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
            >
              Continue with Google
            </button>
            <div className="text-center text-xs text-gray-400 dark:text-gray-500">or</div>
          </div>
        )}
        {error && <p className="text-red-500 dark:text-red-400 text-sm">{error}</p>}
        <input
          className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
          placeholder="Username"
          value={username}
          onChange={e => setUsername(e.target.value)}
          required
        />
        <input
          className="w-full border border-gray-300 dark:border-gray-600 rounded-lg px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500"
          type="password"
          placeholder="Password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          required
        />
        <button className="w-full bg-blue-600 hover:bg-blue-700 text-white rounded-lg px-3 py-2 font-medium transition-colors">
          Sign in
        </button>
      </form>
    </main>
  );
}
