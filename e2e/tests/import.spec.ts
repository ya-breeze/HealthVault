import { test, expect } from '@playwright/test';
import * as path from 'path';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
const BASE_URL = process.env.BASE_URL || 'http://localhost:3000';

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

test.describe('Import page', () => {
  test('import nav link is visible on dashboard', async ({ page }) => {
    await login(page);
    await expect(page.getByRole('link', { name: /import/i })).toBeVisible();
  });

  test('import page loads with file input', async ({ page }) => {
    await login(page);
    await page.goto('/import/');
    await expect(page.getByRole('heading', { name: /import health connect/i })).toBeVisible();
    await expect(page.locator('input[type="file"]')).toBeVisible();
    await expect(page.getByRole('button', { name: /import/i })).toBeVisible();
  });

  test('dashboard link on import page navigates home', async ({ page }) => {
    await login(page);
    await page.goto('/import/');
    await page.getByRole('link', { name: /dashboard/i }).click();
    await expect(page).toHaveURL('/');
  });

  test('API: import health-connect rejects missing file', async ({ request }) => {
    // Login via API to get session cookie
    const loginRes = await request.post(`${BASE_URL}/api/auth/login`, {
      data: { username: USER, password: PASS },
    });
    expect(loginRes.ok()).toBeTruthy();

    // POST without a file field
    const res = await request.post(`${BASE_URL}/api/import/health-connect`, {
      multipart: {},
    });
    expect(res.status()).toBe(400);
  });
});
