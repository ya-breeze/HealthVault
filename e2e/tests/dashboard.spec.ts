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

  test('shows summary cards', async ({ page }) => {
    await expect(page.getByText('Steps (last 7 days)')).toBeVisible();
    await expect(page.getByText('Avg Heart Rate')).toBeVisible();
    await expect(page.getByText('Sleep (last night)')).toBeVisible();
  });

  test('shows logged-in username', async ({ page }) => {
    await expect(page.locator('select')).toHaveValue(USER);
  });

  test('has links to data type pages', async ({ page }) => {
    const stepsLink = page.getByRole('link', { name: 'steps' }).first();
    await expect(stepsLink).toBeVisible();
    await stepsLink.click();
    await expect(page).toHaveURL(/\/data\/steps/);
  });
});

test.describe('Webhook ingest + dashboard', () => {
  test('webhook POST stores data visible in dashboard summary', async ({ page, request }) => {
    const ts = new Date().toISOString();
    const stepCount = Math.floor(Math.random() * 5000) + 3000;

    // POST a fresh webhook payload
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

    // Login and check dashboard shows non-zero steps
    await login(page);
    // The summary fetches the last 7 days, so our step record should appear
    await expect(page.getByText('Steps (last 7 days)')).toBeVisible();
    // Verify the API summary includes our steps
    const summaryResp = await request.get(`${BASE_URL}/api/data/summary`, {
      params: { from: startOfDay, to: endOfDay, user: USER },
    });
    // Note: summary requires auth cookie — use page context instead
    const summary = await page.evaluate(async (params) => {
      const r = await fetch(`/api/data/summary?from=${params.from}&to=${params.to}&user=${params.user}`, {
        credentials: 'include',
      });
      return r.json();
    }, { from: startOfDay, to: endOfDay, user: USER });
    expect(summary.steps).toBeGreaterThanOrEqual(stepCount);
  });
});
