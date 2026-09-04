import { test, expect } from '@playwright/test';
import type { Page } from '@playwright/test';

// The e2e target is plain HTTP on the LAN and never passes through
// Cloudflare, so no test in this file can present a real Access Assertion —
// there is no tunnel here to mint one, and forging a signature Cloudflare's
// published keys would accept is not something a test can do either.
//
// What this file covers instead is the feature-off behaviour every
// deployment without Cloudflare in front must have: the exchange endpoint
// 404s, the password form still renders and isn't blocked by the mount-time
// exchange attempt, and login/logout are otherwise unchanged. Real
// verification coverage — valid/expired/wrong-audience/wrong-issuer/
// alg:none/unknown-kid/unreachable-JWKS tokens — lives in Go unit tests
// against a real signed JWT and an httptest JWKS server: see
// backend/pkg/cfaccess/verifier_test.go and
// backend/pkg/server/auth_cf_access_test.go.

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

test.describe('Cf-Access, feature off', () => {
  test('POST /api/auth/cf-access answers 404 and sets no cookies', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/api/auth/cf-access`);
    expect(resp.status()).toBe(404);
    expect(resp.headers()['set-cookie']).toBeUndefined();
  });

  test('login page still renders the password form and does not hang on the mount-time exchange', async ({ page }) => {
    await page.goto('/login/');
    // The mount-time exchange 404s in the background; the form must already
    // be usable rather than waiting on that round trip to settle.
    await expect(page.getByPlaceholder(/username/i)).toBeVisible();
    await expect(page.getByPlaceholder(/password/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /sign in|login/i })).toBeVisible();
  });

  test('password login still reaches the dashboard, and logout lands on /login and stays there', async ({ page }) => {
    await login(page);
    await expect(page).toHaveURL('/');

    await page.locator('header [data-nav-control="logout"]').click();
    await page.waitForURL(/\/login/);

    // Stays there: the mount-time exchange attempt on this fresh /login load
    // must not silently re-authenticate and bounce back to /. It can't
    // succeed here regardless (the endpoint 404s), but this also guards the
    // suppression flag useLogout sets — without it, a deployment where the
    // exchange *does* succeed would undo logout entirely. networkidle, not
    // just the URL, so the mount-time exchange request has actually settled
    // before asserting nothing came of it.
    await page.waitForLoadState('networkidle');
    await expect(page).toHaveURL(/\/login/);
  });
});
