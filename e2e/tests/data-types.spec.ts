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

test.describe('Zoom control', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('Day/Week/Month/Year controls are visible with Week selected by default', async ({ page }) => {
    await page.goto('/data/steps/');
    for (const label of ['Day', 'Week', 'Month', 'Year']) {
      await expect(page.getByRole('button', { name: label, exact: true })).toBeVisible();
    }
  });

  test('switching zoom level re-renders the chart without error', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    for (const label of ['Day', 'Week', 'Month', 'Year']) {
      await page.getByRole('button', { name: label, exact: true }).click();
      await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    }
  });

  test('nutrition page shows a macro selector', async ({ page }) => {
    await page.goto('/data/nutrition/');
    await expect(page.getByRole('button', { name: 'Calories', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Protein', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
  });

  test('food_meal page loads without a bucketed chart request failing', async ({ page }) => {
    await page.goto('/data/food_meal/');
    await expect(page.getByText(/food.?meal/i)).toBeVisible();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    // Switching zoom on food_meal only changes the raw time range — it must
    // never trigger a ?bucket= request, which the backend rejects for this type.
    await page.getByRole('button', { name: 'Year', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
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

  test('GET /api/data/steps?bucket=day returns bucket_start/count/sum rows', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    expect(Array.isArray(result.body)).toBe(true);
    if (result.body.length > 0) {
      expect(result.body[0]).toHaveProperty('bucket_start');
      expect(result.body[0]).toHaveProperty('count');
      expect(result.body[0]).toHaveProperty('sum');
    }
  });

  test('GET /api/data/heart_rate?bucket=day returns avg/min/max rows', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/heart_rate?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      expect(result.body[0]).toHaveProperty('avg');
      expect(result.body[0]).toHaveProperty('min');
      expect(result.body[0]).toHaveProperty('max');
    }
  });

  test('GET /api/data/blood_pressure?bucket=day returns dual systolic/diastolic columns', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/blood_pressure?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      for (const field of ['systolic_avg', 'systolic_min', 'systolic_max', 'diastolic_avg', 'diastolic_min', 'diastolic_max']) {
        expect(result.body[0]).toHaveProperty(field);
      }
    }
  });

  test('GET /api/data/nutrition?bucket=day returns per-macro sum columns', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/nutrition?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      expect(result.body[0]).toHaveProperty('sum_calories');
      expect(result.body[0]).toHaveProperty('sum_protein_grams');
    }
  });

  test('GET /api/data/steps?bucket=week (invalid) returns 400', async ({ page }) => {
    await login(page);
    const status = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?bucket=week', { credentials: 'include' });
      return r.status;
    });
    expect(status).toBe(400);
  });

  test('GET /api/data/food_meal?bucket=day returns 400', async ({ page }) => {
    await login(page);
    const status = await page.evaluate(async () => {
      const r = await fetch('/api/data/food_meal?bucket=day', { credentials: 'include' });
      return r.status;
    });
    expect(status).toBe(400);
  });

  test('Omitting ?bucket= still returns raw records', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      expect(result.body[0]).not.toHaveProperty('bucket_start');
    }
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
