import { request, type APIRequestContext } from '@playwright/test';
import { BASE_URL } from './target';
import { webhookAuthHeaders } from './webhook';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';

const HISTORY_TYPES = ['steps', 'heart_rate', 'heart_rate_variability', 'sleep', 'distance',
  'weight', 'blood_pressure', 'oxygen_saturation', 'vo2_max'] as const;
type HistoryType = typeof HISTORY_TYPES[number];

async function loggedInClient(): Promise<APIRequestContext> {
  const client = await request.newContext({ baseURL: BASE_URL });
  const login = await client.post('/api/auth/login', { data: { username: USER, password: PASS } });
  if (!login.ok()) {
    await client.dispose();
    throw new Error(`history fixture: login failed with ${login.status()}`);
  }
  return client;
}

async function idsOf(client: APIRequestContext, type: HistoryType): Promise<Set<string>> {
  const res = await client.get(`/api/data/${type}?from=2000-01-01T00:00:00Z&to=2100-01-01T00:00:00Z`);
  if (!res.ok()) throw new Error(`history fixture: GET ${type} returned ${res.status()}`);
  return new Set((await res.json() as { id: string }[]).map(r => r.id));
}

/** Records created by seedHistory, for removeHistory. */
export type HistoryHandle = Record<string, string[]>;

/**
 * Creates about six months of sparse health history (one day of every primary
 * metric per month) for the test user through the real webhook, so specs that
 * look at charts, trends and vitals do not depend on data from anywhere else.
 * Call from `test.beforeAll`, and pass the result to removeHistory in `afterAll`.
 */
export async function seedHistory(): Promise<HistoryHandle> {
  const client = await loggedInClient();
  try {
    const before = new Map<HistoryType, Set<string>>();
    for (const type of HISTORY_TYPES) before.set(type, await idsOf(client, type));

    const now = new Date();
    for (let month = 0; month < 6; month++) {
      const time = new Date(now.getTime() - (2 + month * 30) * 24 * 60 * 60 * 1000);
      time.setUTCHours(12, 0, 0, 0);
      const end = new Date(time.getTime() + 60 * 60 * 1000);
      const response = await client.post(`/webhook/${USER}`, {
        headers: webhookAuthHeaders(),
        data: {
          timestamp: time.toISOString(),
          app_version: 'e2e-history',
          steps: [{ count: 6000 + month * 300, start_time: time.toISOString(), end_time: end.toISOString() }],
          heart_rate: [{ bpm: 68 + month, time: time.toISOString() }],
          heart_rate_variability: [{ rmssd_millis: 38 + month, time: time.toISOString() }],
          sleep: [{ session_end_time: end.toISOString(), duration_seconds: 7 * 60 * 60, stages: [] }],
          distance: [{ meters: 4500 + month * 100, start_time: time.toISOString(), end_time: end.toISOString() }],
          weight: [{ kilograms: 76 + month, time: time.toISOString() }],
          blood_pressure: [{ systolic: 118 + month, diastolic: 76 + month, time: time.toISOString() }],
          oxygen_saturation: [{ percentage: 97, time: time.toISOString() }],
          vo2_max: [{ ml_per_kg_per_min: 42 + month, time: time.toISOString() }],
        },
      });
      if (response.status() !== 204) throw new Error(`history fixture: webhook returned ${response.status()}`);
    }

    const handle: HistoryHandle = {};
    for (const type of HISTORY_TYPES) {
      const after = await idsOf(client, type);
      handle[type] = [...after].filter(id => !before.get(type)!.has(id));
    }
    return handle;
  } finally {
    await client.dispose();
  }
}

/** Deletes exactly the records seedHistory created. */
export async function removeHistory(handle: HistoryHandle | undefined): Promise<void> {
  if (!handle) return;
  const client = await loggedInClient();
  try {
    for (const [type, ids] of Object.entries(handle)) {
      for (const id of ids) {
        const res = await client.delete(`/api/data/${type}/${id}`);
        if (!res.ok() && res.status() !== 404) {
          throw new Error(`history fixture: DELETE ${type}/${id} returned ${res.status()}`);
        }
      }
    }
  } finally {
    await client.dispose();
  }
}
