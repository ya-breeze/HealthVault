import { test, expect } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';

test.describe('Auth', () => {
  test('login page loads', async ({ page }) => {
    await page.goto('/login/');
    await expect(page.getByRole('heading', { name: /healthvault/i })).toBeVisible();
    await expect(page.getByPlaceholder(/username/i)).toBeVisible();
    await expect(page.getByPlaceholder(/password/i)).toBeVisible();
  });

  test('login with valid credentials redirects to dashboard', async ({ page }) => {
    await page.goto('/login/');
    await page.getByPlaceholder(/username/i).fill(USER);
    await page.getByPlaceholder(/password/i).fill(PASS);
    await page.getByRole('button', { name: /sign in|login/i }).click();
    await page.waitForURL('/');
    await expect(page).toHaveURL('/');
  });

  test('login with wrong password shows error', async ({ page }) => {
    await page.goto('/login/');
    await page.getByPlaceholder(/username/i).fill(USER);
    await page.getByPlaceholder(/password/i).fill('wrongpassword');
    await page.getByRole('button', { name: /sign in|login/i }).click();
    await expect(page.getByText(/invalid|incorrect|unauthorized|error|failed/i)).toBeVisible();
  });

  test('unauthenticated access to dashboard redirects to login', async ({ page }) => {
    // Fresh context with no cookies
    await page.goto('/');
    await page.waitForURL('**/login/**');
    expect(page.url()).toContain('login');
  });
});
