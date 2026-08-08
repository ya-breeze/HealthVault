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

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('shows the vitals grid with all 8 primary metrics', async ({ page }) => {
    for (const label of ['Steps', 'Heart Rate', 'Sleep', 'HRV', 'Distance', 'Weight', 'Blood Pressure', 'Oxygen Sat.']) {
      await expect(page.getByText(label, { exact: true })).toBeVisible();
    }
  });

  test('shows logged-in username in the header', async ({ page }) => {
    await expect(page.getByText(USER, { exact: true })).toBeVisible();
  });

  test('vitals grid card links to its data page', async ({ page }) => {
    await page.locator('a[href="/data/steps/"]').first().click();
    await expect(page).toHaveURL(/\/data\/steps/);
  });

  test('secondary "more data" row links to a non-primary data page', async ({ page }) => {
    const link = page.locator('a[href="/data/vo2_max/"]').first();
    await expect(link).toBeVisible();
    await link.click();
    await expect(page).toHaveURL(/\/data\/vo2_max/);
  });
});

test.describe('Webhook ingest + dashboard', () => {
  test('webhook POST is reflected in the steps vital card and its bucketed API response', async ({ page, request }) => {
    const ts = new Date().toISOString();
    const stepCount = Math.floor(Math.random() * 5000) + 3000;

    const today = new Date();
    const startOfDay = new Date(today.getFullYear(), today.getMonth(), today.getDate()).toISOString();
    const endOfDay = new Date(today.getFullYear(), today.getMonth(), today.getDate(), 23, 59, 59).toISOString();

    const resp = await request.post(`${BASE_URL}/webhook/${USER}`, {
      data: {
        timestamp: ts,
        app_version: 'e2e-test',
        steps: [{ count: stepCount, start_time: startOfDay, end_time: endOfDay }],
      },
    });
    expect(resp.status()).toBe(204);

    await login(page);
    await expect(page.getByText('Steps', { exact: true })).toBeVisible();

    // The dashboard's vitals grid fetches ?bucket=day — confirm today's bucket
    // reflects the posted steps rather than scraping a formatted number from the DOM.
    const bucketed = await page.evaluate(async (params) => {
      const r = await fetch(`/api/data/steps?bucket=day&from=${params.from}&to=${params.to}`, {
        credentials: 'include',
      });
      return r.json();
    }, { from: startOfDay, to: endOfDay });
    expect(Array.isArray(bucketed)).toBe(true);
    const total = bucketed.reduce((sum: number, b: { sum?: number }) => sum + (b.sum ?? 0), 0);
    expect(total).toBeGreaterThanOrEqual(stepCount);
  });
});
