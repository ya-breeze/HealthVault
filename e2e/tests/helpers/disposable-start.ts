import { request } from '@playwright/test';
import { BASE_URL } from './target';

// The only data a disposable run starts with is the two bootstrap users
// (HCW_SEED_USERS in the e2e_lite profile): users cannot be created through
// the app, so they are the one thing the tests cannot make themselves.
// Everything else is created by the spec that needs it (see helpers/history.ts
// for the shared health-history fixture). This guard fails the run fast if the
// database is not otherwise empty, which would hide a spec that silently
// relies on leftover data.
const BOOTSTRAP_USERS = [
  { username: 'alice', password: 'pass1' },
  { username: 'bob', password: 'pass2' },
];
const TYPES = ['steps', 'heart_rate', 'heart_rate_variability', 'sleep', 'distance', 'weight',
  'height', 'weight_goal', 'blood_pressure', 'oxygen_saturation', 'vo2_max', 'food_meal'];

export default async function assertEmptyDisposableStart() {
  const rawTarget = process.env.BASE_URL;
  const target = rawTarget ? new URL(rawTarget) : null;
  if (!target || target.protocol !== 'http:' ||
      !/^e2e-[a-z0-9-]+-nginx$/.test(target.hostname) ||
      (target.port && target.port !== '80')) {
    throw new Error('disposable E2E start check requires the temporary e2e nginx target');
  }
  for (const user of BOOTSTRAP_USERS) {
    const client = await request.newContext({ baseURL: BASE_URL });
    try {
      const login = await client.post('/api/auth/login', { data: user });
      if (!login.ok()) throw new Error(`bootstrap user ${user.username} cannot log in: ${login.status()}`);
      for (const type of TYPES) {
        const res = await client.get(`/api/data/${type}?from=2000-01-01T00:00:00Z&to=2100-01-01T00:00:00Z`);
        if (!res.ok()) throw new Error(`start check: GET /api/data/${type} returned ${res.status()}`);
        const rows = await res.json();
        if (Array.isArray(rows) && rows.length > 0) {
          throw new Error(`disposable E2E must start empty, but ${user.username} already has ${rows.length} ${type} row(s)`);
        }
      }
    } finally {
      await client.dispose();
    }
  }
}
