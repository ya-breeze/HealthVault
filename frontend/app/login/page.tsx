'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { api, ApiError } from '@/lib/api';

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
