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

// Runs `action` and waits for the settings PUT it triggers to actually come
// back from the server, rather than only for the click/selectOption that
// starts it. Both settings writers in the app (the language switcher and the
// dashboard-order editor) do a GET-then-PUT via api.updateSettings, so the
// UI-level interaction resolves well before the write lands; a test that ends
// there has its context torn down with the request still in flight, and the
// change it thought it had made — or, in a `finally`, un-made — may never
// reach the shared seeded account. Best-effort by design: this is only ever
// used for cleanup, so a missing response times out quietly instead of
// masking the assertion failure that triggered the cleanup. Found in code
// review.
async function withSettingsSave(page: Page, action: () => Promise<unknown>) {
  const saved = page
    .waitForResponse(
      r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
      { timeout: 15_000 }
    )
    .catch(() => null);
  await action().catch(() => {});
  await saved;
}

// Shared cleanup for tests that reorder the vitals grid: puts Weight back
// where PRIMARY_METRICS has it, so a predictable grid is left for later tests.
// Every step is best-effort and swallows its own failure — used from a
// `finally` block, so one broken step (e.g. the page was left mid-reorder by a
// failed assertion above it) must not hide the real assertion failure that
// triggered the cleanup.
//
// Weight is only moved back to *last* by pressing down, which is not the
// default: PRIMARY_METRICS (frontend/lib/vitals.ts) puts weight 6th of 8, with
// blood_pressure and oxygen_saturation after it. Hence the two ups below. This
// used to stop at the bottom while its name and comment both promised the
// default order — harmless only because no test yet asserts the seeded grid
// order, and actively misleading to the first one that does. Found in code
// review.
//
// Correct because Weight is the only card any test in this file moves, so
// everything else is still in default relative order when this runs.
async function restoreDefaultOrder(page: Page) {
  const customizeBtn = page.getByRole('button', { name: 'Customize' });
  // Waits before asking, because isVisible() answers immediately and never
  // retries. Callers reach here straight after switching Display Language back
  // to English, and withSettingsSave resolves on the PUT *response* — the
  // React re-render that relabels this button from "Настроить" follows
  // it. Asking without waiting can therefore catch the Russian label, read
  // "no editor here", and skip the whole restore this function exists to
  // perform. Bounded at 5s rather than the default so the genuine
  // no-editor-open case below still returns promptly. Found in code review.
  await customizeBtn.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => null);
  // Returns early rather than clicking a Done button that isn't there: with
  // no editor open there is no save to wait for, and withSettingsSave would
  // otherwise sit out its full timeout waiting for a PUT nothing will send.
  if (!(await customizeBtn.isVisible().catch(() => false))) return;
  await customizeBtn.click().catch(() => {});
  const moveWeightDown = page.getByRole('button', { name: /move weight down/i });
  for (let i = 0; i < 8; i++) {
    if (await moveWeightDown.isDisabled().catch(() => true)) break;
    await moveWeightDown.click().catch(() => {});
  }
  // Now last; lift it back over oxygen_saturation and blood_pressure into its
  // default 6th slot.
  const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
  for (let i = 0; i < 2; i++) {
    if (await moveWeightUp.isDisabled().catch(() => true)) break;
    await moveWeightUp.click().catch(() => {});
  }
  // handleDone (app/page.tsx) PUTs unconditionally, so this always has a
  // response to wait for.
  await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
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

      await page.getByRole('button', { name: 'Customize' }).click();

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
      await expect(page.getByRole('button', { name: 'Customize' })).toBeVisible();

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

  // Regression for a bug fixed in app/page.tsx: the settings-load effect
  // listed `t` in its dependency array (to satisfy exhaustive-deps), and `t`
  // is useCallback(..., [language]), so switching Display Language re-ran the
  // effect and overwrote `order` with the *stored* order — discarding an
  // in-progress rearrangement. Because `editing` stays true, the user is left
  // in edit mode looking at the reverted order, and a following Done persists
  // it as though they had chosen it.
  //
  // Distinct from the "persists both" test below, which clicks Done *before*
  // switching language and so exercises the saved-state race instead. Here
  // Done is deliberately never clicked before the switch: the unsaved editing
  // state is the whole point.
  test('switching language mid-reorder keeps the unsaved order', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
      for (let i = 0; i < 8; i++) {
        if (await moveWeightUp.isDisabled()) break;
        await moveWeightUp.click();
      }
      await expect(grid.locator('> *').first()).toHaveAttribute('data-testid', 'vital-card-weight');

      // The switcher lives in this page's own Header, so this is reachable
      // without leaving edit mode.
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('ru')
      );
      await expect(page.locator('#display-language')).toHaveValue('ru');

      // Still first. Before the fix this reverted to the stored order, so the
      // first card was Steps.
      await expect(grid.locator('> *').first()).toHaveAttribute('data-testid', 'vital-card-weight');
      // And still in edit mode — the state the reverted order would have been
      // saved from.
      await expect(page.getByRole('button', { name: 'Готово' })).toBeVisible();
    } finally {
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('en')
      );
      // Waits for the re-render, not just the PUT: the select's value is bound
      // to the provider's `language` state, so seeing 'en' here proves the
      // English labels are on screen. Without it the Done lookup below — an
      // isVisible() that does not retry — can run while the button still reads
      // "Готово", conclude edit mode is closed, and leave it open, which in
      // turn makes restoreDefaultOrder find no "Customize" button and skip
      // the restore. Found in code review.
      await expect(page.locator('#display-language')).toHaveValue('en');
      // Leaves edit mode if it is still open, then restores the default
      // order. Awaited through to the PUT for the same reason as everywhere
      // else in this file: restoreDefaultOrder immediately re-reads settings,
      // and a write still in flight would race it.
      const done = page.getByRole('button', { name: 'Done' });
      if (await done.isVisible().catch(() => false)) {
        await withSettingsSave(page, () => done.click());
      }
      await restoreDefaultOrder(page);
    }
  });

  test('move-up is disabled on the first card and move-down on the last', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize' }).click();
    const grid = page.getByTestId('vitals-grid');
    const cards = grid.locator('> *');
    const first = cards.first();
    const last = cards.last();

    await expect(first.getByRole('button', { name: /move .* up/i })).toBeDisabled();
    await expect(last.getByRole('button', { name: /move .* down/i })).toBeDisabled();

    await page.getByRole('button', { name: 'Done' }).click();
  });
});

// Shared cleanup for the visibility tests: un-hides every card that is
// currently hidden and saves. Like restoreDefaultOrder, every step is
// best-effort — it runs from `finally`, so a failure here must not mask the
// assertion that triggered it. Hidden cards leak across tests worse than a
// custom order does: a card hidden by a failed test is simply absent from the
// grid for every later test that looks for it.
async function restoreAllVisible(page: Page) {
  const customizeBtn = page.getByRole('button', { name: 'Customize' });
  await customizeBtn.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => null);
  if (await customizeBtn.isVisible().catch(() => false)) {
    await customizeBtn.click().catch(() => {});
  }
  // Edit mode renders hidden cards too, so their toggles are reachable here.
  // Bounded loop rather than while(count): a toggle that fails to clear would
  // otherwise spin forever inside a cleanup.
  const hiddenToggles = page.locator('[data-hidden="true"] [data-testid$="-visibility"]');
  for (let i = 0; i < 10; i++) {
    if ((await hiddenToggles.count().catch(() => 0)) === 0) break;
    await hiddenToggles.first().click().catch(() => {});
  }
  const done = page.getByRole('button', { name: 'Done' });
  if (await done.isVisible().catch(() => false)) {
    await withSettingsSave(page, () => done.click());
  }
}

test.describe('Dashboard card visibility', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('hiding a card removes it from the grid and persists across reload', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    try {
      // Outside edit mode there is no visibility control, same as the reorder
      // arrows.
      await expect(page.getByTestId('vital-card-sleep-visibility')).not.toBeVisible();

      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByTestId('vital-card-sleep-visibility').click();
      // Still on screen while editing — dimmed, and flagged for assertions —
      // so it can be found and shown again.
      await expect(grid.getByTestId('vital-card-sleep')).toHaveAttribute('data-hidden', 'true');

      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(grid.getByTestId('vital-card-sleep')).toHaveCount(0);
      // Only Sleep went; the rest of the grid is untouched.
      await expect(grid.getByTestId('vital-card-steps')).toBeVisible();

      await page.reload();
      await expect(page.getByTestId('vitals-grid').getByTestId('vital-card-sleep')).toHaveCount(0);
    } finally {
      await restoreAllVisible(page);
    }
  });

  test('re-showing a hidden card restores its position rather than appending it', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      // Whichever card is second right now — read from the DOM rather than
      // assumed, since a previous test may have left a custom order.
      const secondTestId = await grid.locator('> *').nth(1).getAttribute('data-testid');
      expect(secondTestId).toBeTruthy();

      await page.getByTestId(`${secondTestId}-visibility`).click();
      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(grid.getByTestId(secondTestId!)).toHaveCount(0);

      // Show it again.
      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByTestId(`${secondTestId}-visibility`).click();
      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());

      // Back in slot 2, not appended to the end — the whole point of storing
      // `hidden` inline with the order instead of as a separate list.
      await expect(grid.locator('> *').nth(1)).toHaveAttribute('data-testid', secondTestId!);
    } finally {
      await restoreAllVisible(page);
    }
  });

  test('hiding every card shows a placeholder instead of an empty grid', async ({ page }) => {
    try {
      await page.getByRole('button', { name: 'Customize' }).click();

      // Hide all 8. Each toggle stays in the DOM while editing, so this can
      // walk them by index.
      const toggles = page.locator('[data-testid$="-visibility"]');
      const count = await toggles.count();
      expect(count).toBe(8);
      for (let i = 0; i < count; i++) {
        await toggles.nth(i).click();
      }

      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());

      // Hiding the last card is allowed; the grid is replaced by a message.
      await expect(page.getByTestId('vitals-grid')).toHaveCount(0);
      await expect(page.getByTestId('vitals-grid-empty')).toBeVisible();
      // And the way back is still on screen.
      await expect(page.getByRole('button', { name: 'Customize' })).toBeVisible();
    } finally {
      await restoreAllVisible(page);
    }
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
      await page.getByRole('button', { name: 'Customize' }).click();
      const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
      for (let i = 0; i < 8; i++) {
        if (await moveWeightUp.isDisabled()) break;
        await moveWeightUp.click();
      }
      await page.getByRole('button', { name: 'Done' }).click();
      await expect(page.getByRole('button', { name: 'Customize' })).toBeVisible();

      // No navigation here — this is the exact sequence that used to revert
      // the reorder above. Awaited through to the server's response for the
      // same reason as the cleanup below: the reload immediately after would
      // otherwise be free to abort the PUT in flight, failing the post-reload
      // assertion for a reason that has nothing to do with the race under
      // test.
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('ru')
      );
      await expect(page.locator('#display-language')).toHaveValue('ru');

      await page.reload();

      const firstCardAfter = page.getByTestId('vitals-grid').locator('> *').first();
      await expect(firstCardAfter).toHaveAttribute('data-testid', 'vital-card-weight');
      await expect(page.locator('#display-language')).toHaveValue('ru');
    } finally {
      // Restore English + default order so later tests (which assert on
      // English label text and a predictable card order) aren't affected,
      // even if an assertion above failed partway through.
      //
      // The restore is awaited all the way to the server's response, not just
      // to the selectOption() that starts it: switching the language fires a
      // GET and then a PUT (see api.updateSettings), and when the test ends
      // Playwright closes the context and aborts whatever is still in flight.
      // Leaving 'ru' persisted on the shared seeded account is not a
      // this-test problem — Header labels are driven by that setting, so
      // every later spec that names a header control by text
      // (mobile-tap-targets.spec.ts's 'Custom Foods'/'Import'/'Logout',
      // food.spec.ts's 'Change match'/'Load older') fails against a Russian
      // header. workers: 1 does not help here: the leak is persisted server
      // state, not concurrency. Found in code review.
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('en')
      );
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
