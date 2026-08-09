import { test, expect, chromium } from '@playwright/test';
import type { BrowserContext, Page } from '@playwright/test';

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

// Overwrites the kin_access (and optionally kin_refresh) cookie with a bogus
// value, simulating an expired/invalid token while leaving login's other
// cookie(s) untouched. context.addCookies operates below the page-JS layer,
// so it can rewrite HttpOnly cookies the same way a real TTL expiry would
// leave them invalid.
async function corruptAuthCookies(context: BrowserContext, opts: { refreshToo?: boolean } = {}) {
  const cookies = await context.cookies();
  const targets = opts.refreshToo ? ['kin_access', 'kin_refresh'] : ['kin_access'];
  const patched = cookies
    .filter(c => targets.includes(c.name))
    .map(c => ({ ...c, value: 'corrupted-invalid-token' }));
  await context.addCookies(patched);
}

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

test.describe('Session refresh on 401', () => {
  test('expired access token is silently refreshed using a valid refresh token', async ({ page, context }) => {
    await login(page);
    await corruptAuthCookies(context);

    await page.reload();
    await page.waitForLoadState('networkidle');

    // Should stay on the dashboard — the 401 from the corrupted access token
    // should have been silently recovered via POST /api/auth/refresh, not
    // treated as "not logged in".
    await expect(page).toHaveURL('/');
    await expect(page.getByRole('link', { name: /healthvault/i })).toBeVisible();
  });

  test('dead refresh token still redirects to login', async ({ page, context }) => {
    await login(page);
    await corruptAuthCookies(context, { refreshToo: true });

    await page.reload();
    await page.waitForURL('**/login/**');
    expect(page.url()).toContain('login');
  });

  test('two tabs of the same browser dedupe the refresh call', async () => {
    // navigator.locks (the cross-tab coordination mechanism) requires a
    // secure context. hcw-wip is served over plain HTTP, which production
    // (reached via a public HTTPS URL) is not — so this test launches its
    // own browser with Chromium's origin-allowlist flag to exercise the same
    // code path production actually uses, rather than silently testing only
    // the same-tab-only fallback that active WIP traffic would hit.
    const browser = await chromium.launch({
      args: [`--unsafely-treat-insecure-origin-as-secure=${BASE_URL}`],
    });
    const context = await browser.newContext({ baseURL: BASE_URL });
    try {
      const pageA = await context.newPage();
      await login(pageA);

      const pageB = await context.newPage();
      await pageB.goto('/');
      await expect(pageB).toHaveURL('/');

      await corruptAuthCookies(context);

      let refreshCalls = 0;
      await context.route('**/api/auth/refresh', route => {
        refreshCalls += 1;
        route.continue();
      });

      await Promise.all([pageA.reload(), pageB.reload()]);
      await Promise.all([
        pageA.waitForLoadState('networkidle'),
        pageB.waitForLoadState('networkidle'),
      ]);

      // Neither tab should have been logged out...
      await expect(pageA).toHaveURL('/');
      await expect(pageB).toHaveURL('/');
      // ...and only one of them should have actually called /auth/refresh —
      // the other should have deduped via the Web Lock + localStorage
      // timestamp check instead of resubmitting the already-rotated token.
      expect(refreshCalls).toBe(1);
    } finally {
      await browser.close();
    }
  });
});
