import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { BASE_URL } from './helpers/target';

test('weather evidence is authenticated, idempotent, isolated and conservative about travel', async ({ request, playwright }) => {
  test.setTimeout(180_000);
  const secondUser = await playwright.request.newContext({ baseURL: BASE_URL });
  const anonymous = await playwright.request.newContext({ baseURL: BASE_URL });
  // A synthetic window in the retained recent archive. Jitter prevents
  // successive WIP runs from reusing an observation's exact timestamp.
  const start = new Date(Date.now() - 13 * 86_400_000);
  start.setUTCMinutes(1, Math.floor(Math.random() * 55), 0);
  const stableEnd = new Date(start.getTime() + 3 * 3_600_000);
  const travelEnd = new Date(stableEnd.getTime() + 3_600_000);
  let query = `from=${encodeURIComponent(start.toISOString())}&to=${encodeURIComponent(travelEnd.toISOString())}`;
  const evidence = {
    id: randomUUID(), observed_at: start.toISOString(),
    latitude: 50.1, longitude: 14.4, accuracy_m: 1000,
  };
  try {
    expect((await anonymous.post('/api/weather/locations', { data: evidence })).status()).toBe(401);
    expect((await anonymous.get(`/api/weather/history?${query}`)).status()).toBe(401);
    expect((await request.post(`${BASE_URL}/api/auth/login`, {
      data: { username: process.env.HCW_USER || 'alice', password: process.env.HCW_PASS || 'pass1' },
    })).ok()).toBeTruthy();
    expect((await secondUser.post('/api/auth/login', {
      data: { username: 'bob', password: 'pass2' },
    })).ok()).toBeTruthy();

    // WIP retains evidence across test runs. Pick an empty interval from
    // existing coverage so retries do not mix fixtures from earlier runs.
    const broadFrom = new Date(Date.now() - 13 * 86_400_000).toISOString();
    const broadTo = new Date(Date.now() - 86_400_000).toISOString();
    const available = await (await request.get(`${BASE_URL}/api/weather/history`, {
      params: { from: broadFrom, to: broadTo },
    })).json();
    const empty = available.gaps.filter((gap: { reason: string; from: string; to: string }) =>
      ['missing_location', 'long_gap'].includes(gap.reason) &&
      new Date(gap.to).getTime() - new Date(gap.from).getTime() > 6 * 3_600_000)
      .sort((a: { from: string; to: string }, b: { from: string; to: string }) =>
        (new Date(b.to).getTime() - new Date(b.from).getTime()) -
        (new Date(a.to).getTime() - new Date(a.from).getTime()))[0];
    expect(empty, 'an empty synthetic weather window remains available in WIP').toBeTruthy();
    start.setTime(new Date(empty.from).getTime() + 3_600_000);
    start.setUTCMinutes(1, Math.floor(Math.random() * 55), 0);
    stableEnd.setTime(start.getTime() + 3 * 3_600_000);
    travelEnd.setTime(stableEnd.getTime() + 3_600_000);
    evidence.observed_at = start.toISOString();
    query = `from=${encodeURIComponent(start.toISOString())}&to=${encodeURIComponent(travelEnd.toISOString())}`;

    expect((await request.post(`${BASE_URL}/api/weather/locations`, { data: { ...evidence, latitude: 999 } })).status()).toBe(400);
    expect((await request.post(`${BASE_URL}/api/weather/locations`, { data: { ...evidence, observed_at: 'yesterday' } })).status()).toBe(400);
    expect((await request.post(`${BASE_URL}/api/weather/locations`, { data: evidence })).status()).toBe(202);
    expect((await request.post(`${BASE_URL}/api/weather/locations`, { data: evidence })).status()).toBe(202);
    expect((await request.post(`${BASE_URL}/api/weather/locations`, { data: { ...evidence, latitude: 49.1 } })).status()).toBe(409);
    const lastId = randomUUID();
    expect((await request.post(`${BASE_URL}/api/weather/locations`, {
      data: { ...evidence, id: lastId, observed_at: stableEnd.toISOString() },
    })).status()).toBe(202);

    // Exercise the deployed enrichment worker and real provider, rather than
    // proving only that a mocked response can be saved in a unit test.
    await expect.poll(async () => {
      const response = await request.get(`${BASE_URL}/api/weather/history?${query}`);
      expect(response.status()).toBe(200);
      const history = await response.json();
      return history.hours.filter((hour: { first_observation_id: string; last_observation_id: string }) =>
        hour.first_observation_id === evidence.id && hour.last_observation_id === lastId).length;
    }, { timeout: 120_000, intervals: [1000, 3000, 5000] }).toBeGreaterThan(0);

    const history = await (await request.get(`${BASE_URL}/api/weather/history?${query}`)).json();
    const ownHours = history.hours.filter((hour: { first_observation_id: string }) => hour.first_observation_id === evidence.id);
    for (const hour of ownHours) {
      expect(Number.isFinite(hour.temperature_c)).toBe(true);
      expect(Number.isFinite(hour.surface_pressure_hpa)).toBe(true);
      expect(hour.source).toMatch(/open.?meteo/i);
      expect(typeof hour.model).toBe('string');
      expect(new Date(hour.hour).getTime()).toBeGreaterThanOrEqual(start.getTime());
      expect(new Date(hour.hour).getTime() + 3_600_000).toBeLessThanOrEqual(stableEnd.getTime());
    }

    const otherHistory = await (await secondUser.get(`/api/weather/history?${query}&user=alice`)).json();
    expect(otherHistory.hours.some((hour: { first_observation_id: string }) => hour.first_observation_id === evidence.id)).toBe(false);
    expect((await request.get(`${BASE_URL}/api/weather/history?from=bad&to=bad`)).status()).toBe(400);
    expect((await request.post(`${BASE_URL}/api/weather/locations`, {
      data: { ...evidence, id: randomUUID(), observed_at: travelEnd.toISOString(), latitude: 35.3, longitude: 25.1 },
    })).status()).toBe(202);
    const traveled = await (await request.get(`${BASE_URL}/api/weather/history?${query}`)).json();
    expect(traveled.gaps.some((gap: { reason: string; from: string; to: string }) => gap.reason === 'moving' &&
      new Date(gap.from).getTime() <= stableEnd.getTime() && new Date(gap.to).getTime() >= travelEnd.getTime())).toBe(true);

    // Late evidence of movement must revoke the formerly supported hours.
    expect((await request.post(`${BASE_URL}/api/weather/locations`, {
      data: { ...evidence, id: randomUUID(), observed_at: new Date(start.getTime() + 90 * 60_000).toISOString(), latitude: 35.3, longitude: 25.1 },
    })).status()).toBe(202);
    const repaired = await (await request.get(`${BASE_URL}/api/weather/history?${query}`)).json();
    expect(repaired.hours.some((hour: { first_observation_id: string; last_observation_id: string }) =>
      hour.first_observation_id === evidence.id && hour.last_observation_id === lastId)).toBe(false);
  } finally {
    await secondUser.dispose();
    await anonymous.dispose();
  }
});
