import { test, expect, type Page, type Locator } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';

// 390x844 is the viewport the mobile audit measured, and the one the
// change's before/after numbers are stated against.
const MOBILE_VIEWPORT = { width: 390, height: 844 };
const DESKTOP_VIEWPORT = { width: 1280, height: 800 };

// Measured on main before this change: the shared header took 177px of a
// 844px fold. The threshold is deliberately well below that and well above
// what the reduced header actually renders at (81px), so it fails on a
// regression rather than on a one-pixel drift. See tasks.md 4.4/5.2.
const HEADER_HEIGHT_CEILING = 120;

const DESTINATIONS = ['home', 'photo', 'manual', 'history', 'more'] as const;

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

const bar = (page: Page) => page.getByTestId('bottom-nav');
const destination = (page: Page, id: string) => page.locator(`[data-nav-destination="${id}"]`);

/** True when the two boxes share any area at all. */
function intersects(a: NonNullable<Awaited<ReturnType<Locator['boundingBox']>>>,
                    b: NonNullable<Awaited<ReturnType<Locator['boundingBox']>>>) {
  return a.x < b.x + b.width && b.x < a.x + a.width
    && a.y < b.y + b.height && b.y < a.y + a.height;
}

async function boxOf(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} should have a bounding box`).not.toBeNull();
  return box!;
}

test.describe('Mobile bottom navigation — mobile viewport', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('the bar offers all five destinations on an authenticated page', async ({ page }) => {
    await login(page);
    await expect(bar(page)).toBeVisible();
    for (const id of DESTINATIONS) {
      await expect(destination(page, id), `${id} destination`).toBeVisible();
    }
  });

  test('each routed destination navigates', async ({ page }) => {
    await login(page);
    for (const [id, url] of [
      ['photo', '/food/upload/'],
      ['manual', '/food/manual/'],
      ['history', '/food/history/'],
      ['home', '/'],
    ] as const) {
      await destination(page, id).click();
      await expect(page, `${id} destination`).toHaveURL(url);
      await expect(bar(page)).toBeVisible();
    }
  });

  test('exactly the current route\'s destination is active, and none on an unlisted route', async ({ page }) => {
    await login(page);
    await page.goto('/food/history/');
    await expect(destination(page, 'history')).toHaveAttribute('aria-current', 'page');
    for (const id of DESTINATIONS.filter(d => d !== 'history')) {
      await expect(destination(page, id), `${id} destination`).not.toHaveAttribute('aria-current', 'page');
    }

    // /data/steps is not one of the five, so nothing is indicated.
    await page.goto('/data/steps/');
    await expect(bar(page)).toBeVisible();
    await expect(page.locator('[data-nav-destination][aria-current="page"]')).toHaveCount(0);
  });

  test('the header no longer takes a fifth of the fold', async ({ page }) => {
    await login(page);
    const header = await boxOf(page.locator('header').first(), 'header');
    expect(header.height).toBeLessThan(HEADER_HEIGHT_CEILING);
  });

  // The second of the two defects the audit measured: "log a meal by photo"
  // sat 1.04 folds down in the dashboard body, and existed on no other
  // screen at all. Asserted from a page that is not the dashboard as well as
  // from the dashboard, since reaching it from everywhere is the point.
  test('both logging entry points are within the fold without scrolling, from the dashboard and from a data page', async ({ page }) => {
    await login(page);
    for (const route of ['/', '/data/steps/']) {
      await page.goto(route);
      await expect(bar(page)).toBeVisible();
      for (const id of ['photo', 'manual'] as const) {
        const box = await boxOf(destination(page, id), `${id} destination on ${route}`);
        expect(box.y + box.height, `${id} bottom edge on ${route}`)
          .toBeLessThanOrEqual(MOBILE_VIEWPORT.height);
        expect(box.y, `${id} top edge on ${route}`).toBeGreaterThanOrEqual(0);
      }
      expect(await page.evaluate(() => window.scrollY), `scroll position on ${route}`).toBe(0);
    }
  });
});

test.describe('Mobile bottom navigation — desktop viewport', () => {
  test.use({ viewport: DESKTOP_VIEWPORT });

  // The bar is CSS-hidden rather than conditionally mounted, so that a
  // statically exported page serves the right navigation before hydration.
  // That is why this asserts invisibility and unreachability rather than
  // absence — toHaveCount(0) would fail here and is correct only on /login,
  // which never receives the shell at all (see the next test).
  test('the bar is present but neither visible nor focusable', async ({ page }) => {
    await login(page);
    await expect(bar(page)).toBeHidden();
    for (const id of DESTINATIONS) {
      const d = destination(page, id);
      await expect(d, `${id} destination`).toBeHidden();
      const focusable = await d.evaluate(el => {
        (el as HTMLElement).focus();
        return document.activeElement === el;
      });
      expect(focusable, `${id} destination should not be focusable`).toBe(false);
    }
  });

  test('the header still carries its full control set', async ({ page }) => {
    await login(page);
    for (const id of ['webhook', 'custom-foods', 'import', 'settings', 'logout']) {
      await expect(page.locator(`header [data-nav-control="${id}"]`), `header ${id}`).toBeVisible();
    }
  });
});

test.describe('Mobile bottom navigation — unauthenticated', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('/login carries no bar at all', async ({ page }) => {
    await page.goto('/login/');
    await expect(page.getByPlaceholder(/username/i)).toBeVisible();
    await expect(bar(page)).toHaveCount(0);
  });

  // The app is `output: 'export'` and its auth check is client-side, so a
  // visitor with no session is served an authenticated page's HTML and only
  // then redirected. A poll would race the redirect, so this records every
  // DOM state the document passes through instead.
  test('a deep link with no session never paints header or bar', async ({ page }) => {
    await page.addInitScript(() => {
      const w = window as unknown as { __sawChrome?: boolean };
      w.__sawChrome = false;
      const check = () => {
        if (document.querySelector('[data-testid="bottom-nav"], header')) w.__sawChrome = true;
      };
      new MutationObserver(check).observe(document.documentElement, { childList: true, subtree: true });
      document.addEventListener('DOMContentLoaded', check);
    });

    await page.context().clearCookies();
    await page.goto('/food/history/');
    await page.waitForURL(/\/login/);
    // The redirect is client-side, so the recording survives it.
    const sawChrome = await page.evaluate(() => (window as unknown as { __sawChrome: boolean }).__sawChrome);
    expect(sawChrome, 'authenticated chrome was painted before the redirect').toBe(false);
  });
});

test.describe('More sheet', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('opens with every control the mobile header sheds', async ({ page }) => {
    await login(page);
    await destination(page, 'more').click();
    const sheet = page.getByTestId('more-sheet');
    await expect(sheet).toBeVisible();
    await expect(sheet).toHaveAttribute('aria-modal', 'true');
    for (const id of ['webhook', 'custom-foods', 'import', 'settings', 'logout']) {
      await expect(sheet.locator(`[data-nav-control="${id}"]`), `sheet ${id}`).toBeVisible();
    }
    // The webhook control is the one that needs the session, and it is the
    // reason the shell owns the fetch rather than each surface making its own.
    await expect(sheet.getByText(new RegExp(`/webhook/${USER}$`))).toBeVisible();
  });

  // mobile-navigation's "No control is stranded on mobile". Both sets are
  // read out of the DOM and compared with each other — neither is a literal
  // maintained inside this test, so adding a control to one surface and not
  // the other fails here instead of drifting past it.
  test('the header\'s control set and the sheet\'s are the same set', async ({ page }) => {
    await login(page);
    const read = (scope: Locator) =>
      scope.locator('[data-nav-control]').evaluateAll(els =>
        els.map(el => el.getAttribute('data-nav-control')!).sort()
      );

    const headerControls = await read(page.locator('header'));
    await destination(page, 'more').click();
    await expect(page.getByTestId('more-sheet')).toBeVisible();
    const sheetControls = await read(page.getByTestId('more-sheet'));

    expect(headerControls.length).toBeGreaterThan(0);
    expect(sheetControls).toEqual(headerControls);
  });

  test('dismisses by Escape and by backdrop without navigating', async ({ page }) => {
    await login(page);
    await page.goto('/food/history/');
    const urlBefore = page.url();

    await destination(page, 'more').click();
    await expect(page.getByTestId('more-sheet')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByTestId('more-sheet')).toHaveCount(0);
    expect(page.url()).toBe(urlBefore);

    await destination(page, 'more').click();
    await expect(page.getByTestId('more-sheet')).toBeVisible();
    // Click the backdrop above the panel, which starts partway down the screen.
    await page.getByTestId('more-sheet-backdrop').click({ position: { x: 195, y: 20 } });
    await expect(page.getByTestId('more-sheet')).toHaveCount(0);
    expect(page.url()).toBe(urlBefore);
  });

  test('logout ends the session and lands on /login', async ({ page }) => {
    await login(page);
    await destination(page, 'more').click();
    await page.getByTestId('more-sheet').locator('[data-nav-control="logout"]').click();
    await page.waitForURL(/\/login/);
    // Session really ended, rather than only the route changing.
    await page.goto('/food/history/');
    await page.waitForURL(/\/login/);
  });
});

// The cases a screenshot review misses. Bounding boxes are compared against
// each other rather than against a pixel offset, so these survive the bar's
// height being settled differently.
test.describe('Bottom navigation does not occlude the app\'s own fixed elements', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('the manual-entry submit bar clears it and stays clickable', async ({ page }) => {
    await login(page);
    await page.goto('/food/manual/');
    const submit = page.getByRole('button', { name: 'Save Meal' });
    await expect(submit).toBeVisible();
    expect(intersects(
      await boxOf(page.locator('.fixed.z-30').first(), 'submit bar'),
      await boxOf(bar(page), 'navigation bar'),
    ), 'submit bar overlaps the navigation bar').toBe(false);
    // A control under the bar is unusable however the two stack, so this is
    // the assertion that a z-index-only "fix" would not satisfy.
    await expect(submit).toBeEnabled();
    await submit.click({ trial: true });
  });

  test('the review page\'s submit bar clears it, and so does a toast', async ({ page }) => {
    await login(page);
    const meal = {
      id: 'nav-mock-meal', photo_path: 'fake/path.jpg', status: 'pending_review',
      logged_at: new Date().toISOString(), name: 'Nav Mock Meal', clarify_round: 0, clarify_log: '',
      calories: 300, protein_grams: 10, carbs_grams: 20, fat_grams: 5,
      sugar_grams: 1, sodium_grams: 1, dietary_fiber_grams: 1, items: [],
    };
    await page.route('**/api/food/meals/nav-mock-meal', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: meal }) : route.continue()
    );
    await page.route('**/api/food/meals/nav-mock-meal/confirm', route =>
      route.fulfill({ json: { ...meal, status: 'confirmed' } })
    );

    await page.goto('/food/review/?meal=nav-mock-meal');
    const confirm = page.getByRole('button', { name: /confirm meal/i });
    await expect(confirm).toBeVisible();
    const navBox = await boxOf(bar(page), 'navigation bar');
    expect(intersects(
      await boxOf(page.locator('.fixed.z-30').first(), 'review submit bar'),
      navBox,
    ), 'review submit bar overlaps the navigation bar').toBe(false);

    // Confirming raises a success toast — useToast is the app-wide feedback
    // path, so without its own offset every toast in the app lands in the bar.
    await confirm.click();
    const toast = page.getByRole('status').first();
    await expect(toast).toBeVisible();
    expect(intersects(await boxOf(toast, 'toast'), navBox),
      'toast overlaps the navigation bar').toBe(false);
    await expect(toast.getByRole('button', { name: /dismiss/i })).toBeVisible();
  });

  // tasks.md 4.7: the tokens resolve as intended in both regimes. With a
  // zero safe-area inset in headless Chromium the two differ only in the
  // offset, which is the half this can settle; the inset half is manual.
  test('the clearance token resolves to the bar\'s height on mobile and to nothing on desktop', async ({ page }) => {
    await login(page);
    await page.goto('/food/manual/');

    const read = async () => page.evaluate(() => {
      const shell = document.querySelector('[data-testid="shell-content"]')!;
      const submit = document.querySelector('.fixed.z-30')!;
      const s = getComputedStyle(submit);
      return {
        shellPaddingBottom: getComputedStyle(shell).paddingBottom,
        submitBottom: s.bottom,
        submitPaddingBottom: s.paddingBottom,
      };
    });

    const mobile = await read();
    expect(parseFloat(mobile.shellPaddingBottom)).toBeGreaterThan(0);
    expect(mobile.submitBottom).toBe(mobile.shellPaddingBottom);
    // The bar beneath it absorbs the inset, so the submit bar adds none.
    expect(mobile.submitPaddingBottom).toBe('12px');

    await page.setViewportSize(DESKTOP_VIEWPORT);
    await expect(bar(page)).toBeHidden();
    const desktop = await read();
    expect(desktop.shellPaddingBottom).toBe('0px');
    expect(desktop.submitBottom).toBe('0px');
    // 0.75rem plus the inset, which headless Chromium reports as zero.
    expect(desktop.submitPaddingBottom).toBe('12px');
  });

  // The testable half of the safe-area scenario. The non-zero-inset half
  // cannot be reached here — headless Chromium reports the inset as 0 and
  // offers no way to set it — and is disclosed as a manual device check in
  // the change's tasks.md 6.2/6.2a rather than claimed as covered.
  test('the document opts into safe-area insets and the bar absorbs the bottom one', async ({ page }) => {
    await login(page);
    await expect(page.locator('meta[name="viewport"]'))
      .toHaveAttribute('content', /viewport-fit=cover/);

    const usesEnv = await page.evaluate(() => {
      const el = document.querySelector('[data-testid="bottom-nav"]');
      if (!el) return null;
      // Every style rule, at any nesting depth. Note a CSSStyleRule also
      // carries a (usually empty) `cssRules` these days, now that CSS
      // nesting exists — so "has cssRules" cannot be the test for whether a
      // rule is a container, or every style rule is discarded and this
      // silently finds nothing.
      const collect = (rules: CSSRuleList): CSSStyleRule[] =>
        [...rules].flatMap(r => [
          ...((r as CSSStyleRule).selectorText !== undefined && (r as CSSStyleRule).style
            ? [r as CSSStyleRule] : []),
          ...((r as CSSGroupingRule).cssRules ? collect((r as CSSGroupingRule).cssRules) : []),
        ]);
      return [...document.styleSheets]
        .flatMap(sheet => { try { return collect(sheet.cssRules); } catch { return []; } })
        .some(rule => {
          if (!rule.style?.paddingBottom || !/env\(/.test(rule.style.paddingBottom)) return false;
          try { return el.matches(rule.selectorText); } catch { return false; }
        });
    });
    expect(usesEnv, 'the bar\'s padding-bottom should carry an env() term').toBe(true);
  });
});
