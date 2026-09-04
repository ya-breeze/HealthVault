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
// A page's own fixed submit/confirm bar (components/ui/BottomActionBar.tsx).
// Addressed by its test id rather than by the `.fixed.z-30` Tailwind pair it
// also carries: a z-index or layout tweak in that component would otherwise
// turn every assertion here into "element not found", which says nothing about
// the occlusion these tests exist to catch.
const submitBar = (page: Page) => page.getByTestId('bottom-action-bar');
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

  // dashboard-ui's "Mobile header is reduced to title and user", whose
  // single-row guarantee runs down to 320px — the narrowest supported width,
  // and where the old seven-control row wrapped worst.
  test('the header is one row of title and user at 320px, with the shed controls gone', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 844 });
    await login(page);

    const title = await boxOf(page.getByRole('link', { name: 'HealthVault' }), 'app title');
    const badge = await boxOf(page.getByTestId('user-badge'), 'user badge');
    // Same row, not merely both present: a wrapped header would put the
    // badge a full row below the title while both stayed visible.
    expect(Math.abs((title.y + title.height / 2) - (badge.y + badge.height / 2)),
      'title and badge should share a row').toBeLessThan(4);

    const header = await boxOf(page.locator('header').first(), 'header');
    expect(header.height).toBeLessThan(HEADER_HEIGHT_CEILING);

    for (const id of ['webhook', 'custom-foods', 'import', 'settings', 'logout']) {
      await expect(page.locator(`header [data-nav-control="${id}"]`), `header ${id}`).toBeHidden();
    }
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

  // AuthenticatedShell is rendered inside each page component, not in a
  // layout, so every client-side navigation remounts it and re-runs its
  // session fetch. It seeds its state from a module-level cache precisely so
  // that the chrome it gates does not blink out for the length of that
  // request — the bar vanishing under the finger that just tapped it, and the
  // body jumping up by the header's height and back. Samples every animation
  // frame rather than polling: the gap this guards against spans a network
  // round-trip, which is many frames, but a `toBeVisible` between navigations
  // would auto-retry straight past it. Found in code review.
  test('the header and the bar never blink out during a navigation', async ({ page }) => {
    await login(page);
    await expect(bar(page)).toBeVisible();

    // Held open deliberately: the gap this guards against lasts exactly as
    // long as /users/me does, so against a fast local stack the unfixed code
    // could slip through in a frame or two. With the request stalled, the
    // only way to render a frame of chrome is to have had the session
    // already — which is the fix. Applied after login so it delays the
    // remounts, not the first resolution that fills the cache.
    await page.route('**/users/me', async route => {
      await new Promise(resolve => setTimeout(resolve, 800));
      await route.continue();
    });

    await page.evaluate(() => {
      const w = window as unknown as { __frames?: number; __framesWithoutChrome?: number };
      w.__frames = 0;
      w.__framesWithoutChrome = 0;
      const sample = () => {
        w.__frames!++;
        const hasBar = !!document.querySelector('[data-testid="bottom-nav"]');
        const hasHeader = !!document.querySelector('header');
        if (!hasBar || !hasHeader) w.__framesWithoutChrome!++;
        requestAnimationFrame(sample);
      };
      requestAnimationFrame(sample);
    });

    for (const [id, url] of [
      ['history', '/food/history/'],
      ['manual', '/food/manual/'],
      ['home', '/'],
    ] as const) {
      await destination(page, id).click();
      await expect(page).toHaveURL(url);
    }

    const { frames, missed } = await page.evaluate(() => {
      const w = window as unknown as { __frames: number; __framesWithoutChrome: number };
      return { frames: w.__frames, missed: w.__framesWithoutChrome };
    });
    // A sampler that never ran would report zero misses too.
    expect(frames, 'frames sampled').toBeGreaterThan(10);
    expect(missed, 'frames rendered without a header or without the bar').toBe(0);
  });

  // `TapTarget` gives the app title `min-w-12`, which displaces the
  // `min-width: auto` that otherwise stops a flex item shrinking below its
  // own text — and the mobile row is `flex-nowrap`. The username is the
  // variable-length half of the row, so it is substituted here rather than
  // waiting for a user whose name happens to be long enough. Found in code
  // review.
  // idea-268: the dashboard's own Log food row duplicated the bar's Photo,
  // Manual and History destinations on a phone, so it is hidden below `sm`
  // and only the bar offers those three routes at this width.
  test('the log-food block is hidden and the bar carries its three destinations', async ({ page }) => {
    await login(page);
    await expect(page.getByTestId('log-food-links')).toBeHidden();
    await expect(bar(page)).toBeVisible();
    for (const id of ['photo', 'manual', 'history'] as const) {
      await expect(destination(page, id), `${id} destination`).toBeVisible();
    }
  });

  // idea-268: with the row gone, the section that follows it (More Data)
  // moves up by the full height of the heading, row and gap. Compared
  // against the same element's desktop position (where the row is still on
  // screen ahead of it) rather than a pixel constant, in the style of the
  // occlusion tests below.
  test('with the block hidden, the more-data section sits above where it renders at desktop widths', async ({ page }) => {
    // Presence responses omit no type in production, but an empty map is
    // enough here: hasPresence treats an absent key as present, which is all
    // this test needs to guarantee the section renders regardless of seed data.
    await page.route('**/api/data-types/presence', route => route.fulfill({ json: {} }));
    await login(page);
    const moreData = page.getByTestId('more-data');
    await expect(moreData).toBeVisible();
    const mobileTop = (await boxOf(moreData, 'more-data (mobile, log-food-links hidden)')).y;

    await page.setViewportSize(DESKTOP_VIEWPORT);
    await expect(page.getByTestId('log-food-links')).toBeVisible();
    const desktopTop = (await boxOf(moreData, 'more-data (desktop, log-food-links visible)')).y;

    expect(mobileTop, 'more-data top edge should sit above its desktop position').toBeLessThan(desktopTop);
  });

  test('a long username crowds the badge, never the app title', async ({ page }) => {
    await page.route('**/users/me', async route => {
      const response = await route.fetch();
      const body = await response.json();
      await route.fulfill({ response, json: { ...body, username: 'a-rather-long-username-indeed' } });
    });
    await login(page);
    await page.setViewportSize({ width: 320, height: 844 });

    const title = page.locator('header a[href="/"]').first();
    const badge = page.getByTestId('user-badge');
    await expect(title).toBeVisible();
    const titleBox = await boxOf(title, 'app title');
    const badgeBox = await boxOf(badge, 'user badge');

    expect(intersects(titleBox, badgeBox), 'title and badge should not overlap').toBe(false);
    expect(titleBox.x + titleBox.width, 'title right edge').toBeLessThanOrEqual(320);
    const header = await boxOf(page.locator('header'), 'header');
    expect(header.height, 'header height with a long username').toBeLessThan(HEADER_HEIGHT_CEILING);
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

  // idea-268: the mirror of the mobile-viewport assertion above — at desktop
  // widths the bar is gone and the header carries no food route, so the
  // in-body block is the only way to reach any of the three, and must stay.
  test('the log-food block is visible and offers all three food routes while the bar is hidden', async ({ page }) => {
    await login(page);
    const logFoodLinks = page.getByTestId('log-food-links');
    await expect(logFoodLinks).toBeVisible();
    await expect(bar(page)).toBeHidden();
    for (const href of ['/food/upload/', '/food/manual/', '/food/history/']) {
      await expect(logFoodLinks.locator(`a[href="${href}"]`), href).toBeVisible();
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

    // `evaluateAll` samples the DOM once and never retries, so it has to be
    // given something to wait for: without this the read lands before React
    // paints the header and returns [], failing the guard below rather than
    // the comparison this test is about.
    await expect(page.locator('header [data-nav-control]').first()).toBeAttached();
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

  // The sheet is `aria-modal`, so focus must not reach the header behind it.
  // The webhook block is `select-all` and invites a click, and clicking a
  // non-focusable element parks `activeElement` on <body> — the case the
  // forward half of the trap originally did not handle. Found in code review.
  test('keeps focus inside itself after a click on non-focusable content', async ({ page }) => {
    await login(page);
    await destination(page, 'more').click();
    const sheet = page.getByTestId('more-sheet');
    await expect(sheet).toBeVisible();

    await sheet.locator('code').click();
    for (const key of ['Tab', 'Tab', 'Shift+Tab']) {
      await page.keyboard.press(key);
      const inside = await sheet.evaluate(el => el.contains(document.activeElement));
      expect(inside, `focus should stay in the sheet after ${key}`).toBe(true);
    }
  });

  // A drag that starts in the panel and ends on the backdrop dispatches its
  // click at their common ancestor — the backdrop — so selecting the webhook
  // URL used to dismiss the sheet mid-selection. Found in code review.
  test('a drag-selection released over the backdrop does not dismiss it', async ({ page }) => {
    await login(page);
    await destination(page, 'more').click();
    const sheet = page.getByTestId('more-sheet');
    await expect(sheet).toBeVisible();

    const code = await boxOf(sheet.locator('code'), 'webhook URL');
    await page.mouse.move(code.x + 4, code.y + code.height / 2);
    await page.mouse.down();
    await page.mouse.move(code.x + code.width - 4, code.y + code.height / 2, { steps: 5 });
    // Up on the backdrop, well above the panel.
    await page.mouse.move(MOBILE_VIEWPORT.width / 2, 20, { steps: 5 });
    await page.mouse.up();

    await expect(sheet).toBeVisible();
  });

  // Nothing else closes the sheet when the viewport crosses the breakpoint,
  // and a phone rotated to landscape is past `sm` — where the header carries
  // these same five controls itself. Found in code review.
  test('closes when the viewport crosses to a width that has no More destination', async ({ page }) => {
    await login(page);
    await destination(page, 'more').click();
    await expect(page.getByTestId('more-sheet')).toBeVisible();

    await page.setViewportSize({ width: 844, height: 390 });

    await expect(page.getByTestId('more-sheet')).toHaveCount(0);
    await expect(page.locator('header [data-nav-control="settings"]')).toBeVisible();
  });

  // api.logout throws on any non-2xx and both logout controls await it before
  // routing, so without a catch a failure was an unhandled rejection: no
  // navigation, no message, and nothing to tell the user they are still
  // logged in. Found in code review.
  test('reports a failed logout instead of failing silently', async ({ page }) => {
    await login(page);
    await page.route('**/auth/logout', route => route.fulfill({ status: 500, body: 'nope' }));

    await destination(page, 'more').click();
    await page.getByTestId('more-sheet').locator('[data-nav-control="logout"]').click();

    await expect(page.getByRole('status')).toBeVisible();
    await expect(page).not.toHaveURL(/\/login/);
    // The session is untouched, so the app still works.
    await page.unroute('**/auth/logout');
    await page.keyboard.press('Escape');
    await destination(page, 'history').click();
    await expect(page).toHaveURL('/food/history/');
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
    // The fixed submit bar (and its "Save Meal" button) now belongs to the
    // structured item-by-item form, behind a collapsed disclosure — see the
    // description-first rework of this page — so it only exists once opened.
    await page.getByTestId('describe-structured-toggle').click();
    const submit = page.getByRole('button', { name: 'Save Meal' });
    await expect(submit).toBeVisible();
    // boundingBox() returns null for an element that has not laid out yet, and
    // unlike expect() it does not poll. The submit button being visible does
    // not imply its fixed container and the navigation bar are — the bar in
    // particular waits on api.me(), since AuthenticatedShell renders it only
    // once the session resolves. Waiting on both is also what makes a missing
    // bar report itself as "element not found" rather than as a null bounding
    // box: the first framing of this test's intermittent failure hid a real
    // auth bug behind a geometry error. See docs/specs/stabilize-flaky-e2e.md.
    await expect(submitBar(page)).toBeVisible();
    await expect(bar(page)).toBeVisible();
    expect(intersects(
      await boxOf(submitBar(page), 'submit bar'),
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
      await boxOf(submitBar(page), 'review submit bar'),
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

  // mobile-navigation's "End of a long scrolled list is fully visible" — the
  // in-flow half of the clearance, which the two submit-bar cases above do
  // not exercise because both of those are `position: fixed`. The tall block
  // is injected rather than relying on the seeded account happening to have
  // enough history to overflow the fold; it is ordinary in-flow content of
  // the page either way. Asserts against the content edge rather than a last
  // element, so it holds whatever the page renders.
  test('the end of a page scrolled to its limit clears the bar', async ({ page }) => {
    await login(page);
    await page.goto('/food/history/');
    await expect(bar(page)).toBeVisible();

    await page.evaluate(() => {
      const marker = document.createElement('div');
      marker.id = 'scroll-extent-marker';
      marker.style.cssText = 'height:2000px;background:linear-gradient(#0000,#0001)';
      document.querySelector('[data-testid="shell-content"]')!.appendChild(marker);
    });
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    expect(await page.evaluate(() => window.scrollY), 'page should have scrolled').toBeGreaterThan(0);

    const { contentBottom, navTop } = await page.evaluate(() => {
      const shell = document.querySelector('[data-testid="shell-content"]')!;
      const rect = shell.getBoundingClientRect();
      // The reserved space is padding on this element, so the last pixel of
      // content is its padding box bottom less that padding.
      const padding = parseFloat(getComputedStyle(shell).paddingBottom);
      return {
        contentBottom: rect.bottom - padding,
        navTop: document.querySelector('[data-testid="bottom-nav"]')!.getBoundingClientRect().top,
      };
    });
    expect(contentBottom, 'bottom of the page\'s own content at full scroll')
      .toBeLessThanOrEqual(navTop + 0.5);
  });

  // tasks.md 4.7: the tokens resolve as intended in both regimes. With a
  // zero safe-area inset in headless Chromium the two differ only in the
  // offset, which is the half this can settle; the inset half is manual.
  test('the clearance token resolves to the bar\'s height on mobile and to nothing on desktop', async ({ page }) => {
    await login(page);
    await page.goto('/food/manual/');
    // See the previous test: the fixed submit bar only exists once the
    // structured form's disclosure is opened.
    await page.getByTestId('describe-structured-toggle').click();

    const read = async () => page.evaluate(() => {
      const shell = document.querySelector('[data-testid="shell-content"]')!;
      const submit = document.querySelector('[data-testid="bottom-action-bar"]')!;
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
    // The shell withholds its chrome until the session resolves, so the bar
    // is not in the document the instant the dashboard URL settles. The
    // evaluate below is a one-shot read with no auto-retry of its own.
    await expect(bar(page)).toBeVisible();

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

// ADR-011: a page's own bottom action bar (the review page's confirm bar
// here) sits between the toast and the navigation bar, so the toast has to
// clear it too — not just the navigation bar, which the describe block above
// already covers.
test.describe('a toast does not occlude a page\'s own bottom action bar', () => {
  const MEAL_ID = 'nav-bar-mock-meal';

  async function mockPendingReviewMeal(page: Page) {
    const item = {
      id: 'nav-bar-mock-item', meal_id: MEAL_ID, name: 'Nav Bar Mock Item',
      macro_source: 'reference', weight_grams: 150, confidence: 1,
      calories: 200, protein_grams: 5, carbs_grams: 20, fat_grams: 5,
      sugar_grams: 1, sodium_grams: 1, dietary_fiber_grams: 1,
    };
    const meal = {
      id: MEAL_ID, photo_path: 'fake/path.jpg', status: 'pending_review',
      logged_at: new Date().toISOString(), name: 'Nav Bar Mock Meal', clarify_round: 0, clarify_log: '',
      calories: 200, protein_grams: 5, carbs_grams: 20, fat_grams: 5,
      sugar_grams: 1, sodium_grams: 1, dietary_fiber_grams: 1, items: [item],
    };
    await page.route(`**/api/food/meals/${MEAL_ID}`, route =>
      route.request().method() === 'GET' ? route.fulfill({ json: meal }) : route.continue()
    );
    await page.route(`**/api/food/meals/${MEAL_ID}/items/*`, route =>
      route.fulfill({ json: { ...meal, items: [{ ...item, weight_grams: 200 }] } })
    );
  }

  // Changes the item's weight (raising a toast, same as a real weight edit
  // on the review page) and asserts the toast clears the confirm bar, and
  // that the confirm button is still enabled and hit-testable while the
  // toast is on screen — the assertion a stacking-order-only fix would not
  // satisfy, since the button would still be invisible under the toast.
  async function assertToastClearsBarAndConfirmIsClickable(page: Page) {
    const weightInput = page.locator('input[type="number"]').first();
    await expect(weightInput).toBeVisible();
    await weightInput.fill('200');
    await weightInput.blur();

    const toast = page.getByRole('status').first();
    await expect(toast).toBeVisible();
    const barBox = await boxOf(page.getByTestId('bottom-action-bar'), 'bottom action bar');
    const toastBox = await boxOf(toast, 'toast');
    expect(intersects(toastBox, barBox), 'toast overlaps the bottom action bar').toBe(false);

    const confirm = page.getByRole('button', { name: /confirm meal/i });
    await expect(confirm).toBeEnabled();
    await confirm.click({ trial: true });
  }

  test('at the 390x844 mobile viewport', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await login(page);
    await mockPendingReviewMeal(page);
    await page.goto(`/food/review/?meal=${MEAL_ID}`);
    await assertToastClearsBarAndConfirmIsClickable(page);
  });

  // --nav-block is 0px here and the bar sits at the screen edge, which is
  // the other regime ADR-008/ADR-011's offset arithmetic has to hold in.
  test('at the 1280x800 desktop viewport, where --nav-block is 0px', async ({ page }) => {
    await page.setViewportSize(DESKTOP_VIEWPORT);
    await login(page);
    await mockPendingReviewMeal(page);
    await page.goto(`/food/review/?meal=${MEAL_ID}`);
    await assertToastClearsBarAndConfirmIsClickable(page);
  });

  // The review page renders no BottomActionBar for a 'failed' meal (only
  // 'pending_review' shows the confirm bar), and Retry raises the same
  // success toast as a weight edit. With no bar registered the toast must
  // fall back to exactly its pre-ADR-011 offset — about 1rem above the
  // navigation bar's top edge — rather than sitting at `bottom: 0`.
  test('with no bar on screen, the toast returns to its base offset', async ({ page }) => {
    await page.setViewportSize(MOBILE_VIEWPORT);
    await login(page);

    const mealId = 'nav-bar-mock-failed-meal';
    const meal = {
      id: mealId, photo_path: 'fake/path.jpg', status: 'failed',
      logged_at: new Date().toISOString(), name: 'Nav Bar Mock Failed Meal', clarify_round: 0, clarify_log: '',
      calories: 0, protein_grams: 0, carbs_grams: 0, fat_grams: 0,
      sugar_grams: 0, sodium_grams: 0, dietary_fiber_grams: 0, items: [],
    };
    await page.route(`**/api/food/meals/${mealId}`, route =>
      route.request().method() === 'GET' ? route.fulfill({ json: meal }) : route.continue()
    );
    await page.route(`**/api/food/meals/${mealId}/retry`, route =>
      route.fulfill({ json: { ...meal, status: 'processing' } })
    );

    await page.goto(`/food/review/?meal=${mealId}`);
    await expect(page.getByTestId('bottom-action-bar')).toHaveCount(0);

    await page.getByRole('button', { name: 'Retry' }).click();
    const toast = page.getByRole('status').first();
    await expect(toast).toBeVisible();

    const navBox = await boxOf(bar(page), 'navigation bar');
    const toastBox = await boxOf(toast, 'toast');
    const gap = navBox.y - (toastBox.y + toastBox.height);
    expect(gap, 'toast bottom edge should sit about 1rem above the navigation bar\'s top edge')
      .toBeGreaterThan(8);
    expect(gap, 'toast bottom edge should sit about 1rem above the navigation bar\'s top edge')
      .toBeLessThan(24);
  });
});
