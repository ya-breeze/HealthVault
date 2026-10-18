import { request } from '@playwright/test';
import { BASE_URL } from './target';
import { webhookAuthHeaders } from './webhook';

// The disposable database starts empty. Seed only the records that existing
// dashboard and chart tests previously inherited from the long-lived WIP DB.
export default async function seedDisposableData() {
  const rawTarget = process.env.BASE_URL;
  const target = rawTarget ? new URL(rawTarget) : null;
  if (!target || target.protocol !== 'http:' ||
      !/^e2e-[a-z0-9-]+-nginx$/.test(target.hostname) ||
      (target.port && target.port !== '80')) {
    throw new Error('disposable E2E seeding requires the temporary e2e nginx target');
  }
  const client = await request.newContext({ baseURL: BASE_URL });
  try {
    const now = new Date();
    for (let month = 0; month < 6; month++) {
      const time = new Date(now.getTime() - (2 + month * 30) * 24 * 60 * 60 * 1000);
      time.setUTCHours(12, 0, 0, 0);
      const end = new Date(time.getTime() + 60 * 60 * 1000);
      const response = await client.post('/webhook/alice', {
        headers: webhookAuthHeaders(),
        data: {
          timestamp: time.toISOString(),
          app_version: 'disposable-e2e',
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
      if (response.status() !== 204) {
        throw new Error(`disposable E2E seed failed: webhook returned ${response.status()}`);
      }
    }
  } finally {
    await client.dispose();
  }
}
