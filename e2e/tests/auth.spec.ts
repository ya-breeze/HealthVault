import { test, expect, chromium } from '@playwright/test';
import type { BrowserContext, Page } from '@playwright/test';
import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { BASE_URL } from './helpers/target';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';

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

// navigator.locks (the cross-tab refresh coordination mechanism) requires a
// secure context, which a plain-HTTP LAN origin like hcw-wip never is —
// Chromium's --unsafely-treat-insecure-origin-as-secure flag does not
// reliably grant one for an IP-based origin in this Playwright/Chromium
// build (verified empirically: it left window.isSecureContext false). A
// genuine `localhost`/`127.0.0.1` origin, by contrast, is unconditionally a
// secure context per spec with no flag needed — confirmed the same way. So
// this proxies hcw-wip through a local `127.0.0.1` listener, giving the test
// a real secure-context origin that exercises the exact Web Locks code path
// production's HTTPS deployment actually uses.
function startLocalProxy(targetOrigin: string): Promise<{ url: string; close: () => Promise<void> }> {
  const target = new URL(targetOrigin);
  return new Promise(resolve => {
    const server = http.createServer((req, res) => {
      const proxyReq = http.request(
        {
          host: target.hostname,
          port: target.port,
          path: req.url,
          method: req.method,
          headers: { ...req.headers, host: target.host },
        },
        proxyRes => {
          res.writeHead(proxyRes.statusCode || 502, proxyRes.headers);
          proxyRes.pipe(res);
        }
      );
      req.pipe(proxyReq);
      proxyReq.on('error', () => res.destroy());
    });
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address() as AddressInfo;
      resolve({
        url: `http://127.0.0.1:${port}`,
        close: () => new Promise(r => server.close(() => r())),
      });
    });
  });
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
    const proxy = await startLocalProxy(BASE_URL);
    const browser = await chromium.launch();
    const context = await browser.newContext({ baseURL: proxy.url });
    try {
      const pageA = await context.newPage();
      await login(pageA);
      // networkidle, not just the URL, so no request from the initial load
      // (e.g. Dashboard's second-effect vitals fetches) is still in flight
      // when we corrupt cookies below — an in-flight request from before
      // corruption wouldn't itself 401, but it could otherwise obscure
      // exactly when this tab's state actually settled.
      await pageA.waitForLoadState('networkidle');

      const pageB = await context.newPage();
      await pageB.goto('/');
      await expect(pageB).toHaveURL('/');
      await pageB.waitForLoadState('networkidle');

      // This test's whole premise is that the proxied origin is a genuine
      // secure context so navigator.locks is actually active — assert that
      // directly, so a future change to the proxy or to Chromium's secure-
      // context rules fails loudly here instead of silently degrading this
      // test to (only) exercising the same-tab-only fallback.
      const capabilities = await pageA.evaluate(() => ({
        isSecureContext: window.isSecureContext,
        hasLocks: typeof navigator.locks !== 'undefined',
      }));
      expect(capabilities).toEqual({ isSecureContext: true, hasLocks: true });

      let refreshCalls = 0;
      await context.route('**/api/auth/refresh', route => {
        refreshCalls += 1;
        route.continue();
      });

      // Force a genuine race instead of relying on luck: hold each tab's
      // first GET /api/users/me (what both Header and Dashboard fire on
      // mount, and what will 401 once cookies are corrupted) until BOTH
      // tabs' requests have arrived, then release them together. Without
      // this, "one refresh call" could happen simply because tab B's
      // request landed after tab A's refresh had already completed and
      // refreshed the shared cookie — a passing result that proves nothing
      // about the Web Lock actually coalescing a real race.
      let pageAArrived = false;
      let pageBArrived = false;
      let releaseBarrier: () => void;
      const barrier = new Promise<void>(resolve => {
        releaseBarrier = resolve;
      });
      const releaseIfBothArrived = () => {
        if (pageAArrived && pageBArrived) releaseBarrier();
      };
      await pageA.route('**/api/users/me', async route => {
        pageAArrived = true;
        releaseIfBothArrived();
        await barrier;
        await route.continue();
      });
      await pageB.route('**/api/users/me', async route => {
        pageBArrived = true;
        releaseIfBothArrived();
        await barrier;
        await route.continue();
      });

      await corruptAuthCookies(context);
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
      await proxy.close();
    }
  });
});
