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

// Shared cleanup for tests that reorder the vitals grid: pushes Weight back
// to the last position (the default order), so a predictable grid is left
// for later tests. Every step is best-effort and swallows its own failure —
// used from a `finally` block, so one broken step (e.g. the page was left
// mid-reorder by a failed assertion above it) must not hide the real
// assertion failure that triggered the cleanup.
async function restoreDefaultOrder(page: Page) {
  const editOrderBtn = page.getByRole('button', { name: 'Edit order' });
  if (await editOrderBtn.isVisible().catch(() => false)) {
    await editOrderBtn.click().catch(() => {});
  }
  const moveWeightDown = page.getByRole('button', { name: /move weight down/i });
  for (let i = 0; i < 8; i++) {
    if (await moveWeightDown.isDisabled().catch(() => true)) break;
    await moveWeightDown.click().catch(() => {});
  }
  await page.getByRole('button', { name: 'Done' }).click().catch(() => {});
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

test.describe('Needs-attention indicator', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('shown and links to history when meals need attention', async ({ page }) => {
    await page.route('**/api/food/meals/needs-attention-count', route =>
      route.fulfill({ json: { count: 3 } })
    );
    await page.goto('/');

    const indicator = page.getByText('3 meals need attention');
    await expect(indicator).toBeVisible();
    await indicator.click();
    await expect(page).toHaveURL(/\/food\/history/);
  });

  test('uses singular wording for a count of one', async ({ page }) => {
    await page.route('**/api/food/meals/needs-attention-count', route =>
      route.fulfill({ json: { count: 1 } })
    );
    await page.goto('/');
    await expect(page.getByText('1 meal needs attention')).toBeVisible();
  });

  test('hidden when no meals need attention', async ({ page }) => {
    await page.route('**/api/food/meals/needs-attention-count', route =>
      route.fulfill({ json: { count: 0 } })
    );
    await page.goto('/');
    await expect(page.getByText(/needs? attention/)).not.toBeVisible();
  });
});

test.describe('Dashboard card reorder', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('reordering a card via Edit mode persists across reload', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    try {
      // Outside edit mode there are no reorder controls.
      await expect(page.getByRole('button', { name: /move weight up/i })).not.toBeVisible();

      await page.getByRole('button', { name: 'Edit order' }).click();

      // Move the Weight card to the very front, regardless of its current
      // position (this test may run after a prior run left a custom order).
      const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
      for (let i = 0; i < 8; i++) {
        if (await moveWeightUp.isDisabled()) break;
        await moveWeightUp.click();
      }
      await expect(moveWeightUp).toBeDisabled();
      await expect(grid.getByTestId('vital-card-weight')).toBeVisible();

      await page.getByRole('button', { name: 'Done' }).click();
      await expect(page.getByRole('button', { name: 'Edit order' })).toBeVisible();

      // First card in the grid should now be Weight.
      const firstCardBefore = grid.locator('> *').first();
      await expect(firstCardBefore).toHaveAttribute('data-testid', 'vital-card-weight');

      await page.reload();
      const firstCardAfter = page.getByTestId('vitals-grid').locator('> *').first();
      await expect(firstCardAfter).toHaveAttribute('data-testid', 'vital-card-weight');
    } finally {
      // Restore the default order so this test (and others relying on a
      // predictable grid) starts clean on the next run, even if an
      // assertion above failed partway through and left the page in an
      // unknown state (still in edit mode, mid-reorder, etc.).
      await restoreDefaultOrder(page);
    }
  });

  test('move-up is disabled on the first card and move-down on the last', async ({ page }) => {
    await page.getByRole('button', { name: 'Edit order' }).click();
    const grid = page.getByTestId('vitals-grid');
    const cards = grid.locator('> *');
    const first = cards.first();
    const last = cards.last();

    await expect(first.getByRole('button', { name: /move .* up/i })).toBeDisabled();
    await expect(last.getByRole('button', { name: /move .* down/i })).toBeDisabled();

    await page.getByRole('button', { name: 'Done' }).click();
  });
});

test.describe('Settings lost-update race', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  // Regression for a lost-update race fixed in LanguageContext.tsx/page.tsx:
  // the dashboard-order editor and the language switcher each kept an
  // independent cached UserSettings and PUT'd a read-modify-write built from
  // it, with no shared store between them. Saving a reorder and then
  // switching language in the same session (no navigation in between, so
  // the switcher's own cache never refreshed) used to PUT a stale
  // pre-reorder snapshot and silently clobber the just-saved dashboard_order.
  test('reordering cards then switching language in the same session persists both', async ({ page }) => {
    try {
      await page.getByRole('button', { name: 'Edit order' }).click();
      const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
      for (let i = 0; i < 8; i++) {
        if (await moveWeightUp.isDisabled()) break;
        await moveWeightUp.click();
      }
      await page.getByRole('button', { name: 'Done' }).click();
      await expect(page.getByRole('button', { name: 'Edit order' })).toBeVisible();

      // No navigation here — this is the exact sequence that used to revert
      // the reorder above.
      await page.locator('#display-language').selectOption('ru');
      await expect(page.locator('#display-language')).toHaveValue('ru');

      await page.reload();

      const firstCardAfter = page.getByTestId('vitals-grid').locator('> *').first();
      await expect(firstCardAfter).toHaveAttribute('data-testid', 'vital-card-weight');
      await expect(page.locator('#display-language')).toHaveValue('ru');
    } finally {
      // Restore English + default order so later tests (which assert on
      // English label text and a predictable card order) aren't affected,
      // even if an assertion above failed partway through.
      await page.locator('#display-language').selectOption('en').catch(() => {});
      await restoreDefaultOrder(page);
    }
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
