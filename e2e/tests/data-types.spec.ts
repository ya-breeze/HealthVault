import { test, expect, type Page } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
const BASE_URL = process.env.BASE_URL || 'http://192.168.1.54:8888';

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

test.describe('Data type pages', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('/data/steps loads with chart area', async ({ page }) => {
    await page.goto('/data/steps/');
    await expect(page.getByText(/steps/i)).toBeVisible();
    // Page should render without errors (no "something went wrong")
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
  });

  test('/data/heart_rate loads', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    await expect(page.getByText(/heart.?rate/i)).toBeVisible();
  });

  test('/data/sleep loads', async ({ page }) => {
    await page.goto('/data/sleep/');
    await expect(page.getByText(/sleep/i)).toBeVisible();
  });

  test('unknown type API returns 404', async ({ page }) => {
    // The frontend static export uses SPA fallback so unknown routes get index.html (200).
    // The real guard is the API — unknown types return 404 from the backend.
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/not_a_real_type', { credentials: 'include' });
      return r.status;
    });
    expect(result).toBe(404);
  });
});

test.describe('API data endpoints', () => {
  test('GET /api/data/steps returns array', async ({ page, request }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    expect(Array.isArray(result.body)).toBe(true);
  });

  test('GET /api/data/unknown_type returns 404', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/not_a_real_type', { credentials: 'include' });
      return r.status;
    });
    expect(result).toBe(404);
  });

  test('GET /api/data/summary returns expected shape', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/summary?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return r.json();
    });
    expect(result).toHaveProperty('steps');
    expect(result).toHaveProperty('avg_heart_rate');
    expect(result).toHaveProperty('sleep_seconds');
  });
});

test.describe('Webhook endpoint', () => {
  test('POST /webhook/alice with valid payload returns 204', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/webhook/${USER}`, {
      data: {
        timestamp: new Date().toISOString(),
        app_version: 'e2e-test-1.0',
        heart_rate: [{ bpm: 65, time: new Date().toISOString() }],
      },
    });
    expect(resp.status()).toBe(204);
  });

  test('POST /webhook/nonexistent_user returns 404', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/webhook/nonexistent_user_xyz`, {
      data: { timestamp: new Date().toISOString(), app_version: '1.0' },
    });
    expect(resp.status()).toBe(404);
  });

  test('POST /webhook/alice with invalid JSON returns 400', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/webhook/${USER}`, {
      headers: { 'Content-Type': 'application/json' },
      data: 'not valid json{{{',
    });
    expect(resp.status()).toBe(400);
  });
});
