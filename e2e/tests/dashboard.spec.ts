import { test, expect, type Page } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
const BASE_URL = process.env.BASE_URL || 'http://192.168.1.54:8888';

// Mirrors DATA_TYPES (frontend/lib/api.ts) / typeRegistry (backend/pkg/server/api.go):
// every key a real `/api/data-types/presence` response has exactly one entry
// for. Kept here, rather than imported, so this file documents independently
// of the production code which types a presence fixture must cover to look
// like a real (non-fail-open) response.
const ALL_DATA_TYPES = [
  'steps', 'heart_rate', 'heart_rate_variability', 'sleep', 'distance',
  'active_calories', 'total_calories', 'weight', 'height', 'blood_pressure',
  'blood_glucose', 'oxygen_saturation', 'body_temperature', 'skin_temperature',
  'respiratory_rate', 'resting_heart_rate', 'exercise', 'hydration', 'nutrition',
  'basal_metabolic_rate', 'body_fat', 'lean_body_mass', 'vo2_max', 'bone_mass',
  'speed', 'food_meal',
];

const PRIMARY_METRIC_TYPES = [
  'steps', 'heart_rate', 'sleep', 'heart_rate_variability', 'distance', 'weight',
  'blood_pressure', 'oxygen_saturation',
];

const SECONDARY_TYPES = ALL_DATA_TYPES.filter(t => !PRIMARY_METRIC_TYPES.includes(t));

// Builds a full, every-type-covered presence map (a real response never omits
// a type), defaulting every entry to present and applying `overrides` on top.
function presenceFixture(overrides: Record<string, boolean> = {}): Record<string, boolean> {
  const map: Record<string, boolean> = {};
  for (const type of ALL_DATA_TYPES) map[type] = true;
  return { ...map, ...overrides };
}

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
//
// Returns whether a matching PUT was actually observed, so a caller that
// conditions further mutation on "did this really persist" (see the
// presence-gap reorder test's `finally`) has something to check instead of
// assuming the write landed. `false` covers both a failed `action()` and a
// response that never arrived in time.
async function withSettingsSave(page: Page, action: () => Promise<unknown>): Promise<boolean> {
  const saved = page
    .waitForResponse(
      r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
      { timeout: 15_000 }
    )
    // A non-OK response (e.g. a 500) means api.updateSettings threw and the
    // server never persisted the new order — callers that gate cleanup on
    // this return value (like the presence-gap reorder test) must treat that
    // the same as no PUT having fired at all. Found in code review.
    .then(r => r.ok())
    .catch(() => false);
  await action().catch(() => {});
  return saved;
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
    // Repair any hidden card leaked by a failed visibility test before
    // asserting on grid contents: every test below assumes the full grid, and
    // a leaked hidden card fails them for reasons unrelated to what they test.
    await restoreAllVisible(page);
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
    // Repair any hidden card leaked by a failed visibility test before
    // asserting on grid contents: every test below assumes the full grid, and
    // a leaked hidden card fails them for reasons unrelated to what they test.
    await restoreAllVisible(page);
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
  const doneBtn = page.getByRole('button', { name: 'Done' });

  // Wait for the settings load to settle before deciding anything. Until it
  // does, Customize renders *disabled* and the grid isn't rendered at all, so
  // a one-shot isEnabled()/count() here reads the loading state, concludes
  // there is nothing to restore, and turns this whole helper into a silent
  // no-op — which is exactly what it exists to prevent. `expect` retries;
  // `isEnabled()` does not.
  await expect(customizeBtn.or(doneBtn)).toBeEnabled({ timeout: 10_000 }).catch(() => {});

  // Edit mode is identified by Done being on screen, not by Customize being
  // enabled: a disabled Customize is ambiguous between "still loading" and
  // "load failed", and neither means we are editing.
  const enteredEditMode = !(await doneBtn.isVisible().catch(() => false));
  if (enteredEditMode) {
    // Guarded, and with a short timeout. On the settings-load-failure path
    // Customize renders permanently disabled, so the wait above burns its full
    // 10s and an unguarded click would then block on actionability until the
    // 30s test timeout — killing the run in beforeEach instead of letting the
    // .catch() fall through.
    if (!(await customizeBtn.isEnabled().catch(() => false))) return;
    await customizeBtn.click({ timeout: 5_000 }).catch(() => {});
    // Wait for the editor to actually commit before counting below. click()
    // resolves once the event is dispatched, not once React has re-rendered,
    // and count() does not retry — so querying straight after the click can
    // read zero toggles, break the loop on iteration 0, and take the
    // "nothing was hidden" early return. That is the same silent-no-op this
    // helper has now been bitten by twice.
    await expect(doneBtn).toBeVisible({ timeout: 5_000 }).catch(() => {});
  }

  // Edit mode renders hidden cards too, so their toggles are reachable here.
  // Counting hidden cards directly — rather than checking the read-only grid
  // for a full complement — keeps this independent of how many primary
  // metrics there are. An earlier version compared against a hardcoded 8, so
  // adding a 9th metric would have made a run with exactly one card hidden
  // look complete and turned this cleanup into a silent no-op.
  //
  // Bounded loop rather than while(count): a toggle that fails to clear would
  // otherwise spin forever inside a cleanup.
  const hiddenToggles = page.locator('[data-hidden="true"] [data-testid$="-visibility"]');
  let restored = 0;
  for (let i = 0; i < 20; i++) {
    if ((await hiddenToggles.count().catch(() => 0)) === 0) break;
    await hiddenToggles.first().click().catch(() => {});
    restored++;
  }

  // A hidden More Data section is a second, independent flag on the same
  // shared account (see UserSettings.more_data_hidden) — it needs the same
  // cleanup as the per-card `hidden` flag above, in the same edit session,
  // or it leaks into every later test that asserts on the `more-data` testid
  // exactly the way a leaked hidden card does.
  const hiddenMoreData = page.locator('[data-testid="more-data"][data-hidden="true"] [data-testid="more-data-visibility"]');
  if ((await hiddenMoreData.count().catch(() => 0)) > 0) {
    await hiddenMoreData.click().catch(() => {});
    restored++;
  }

  if (restored === 0 && enteredEditMode) {
    // Nothing was hidden and we opened the editor purely to look. Leave via a
    // navigation instead of Done: handleDone PUTs unconditionally, so clicking
    // it here would write settings on every single test just to confirm there
    // was nothing to fix.
    await page.goto('/').catch(() => {});
    return;
  }
  if (await doneBtn.isVisible().catch(() => false)) {
    await withSettingsSave(page, () => doneBtn.click());
  }
}

test.describe('Dashboard card visibility', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    // Normalize before, not only after. Every test below toggles visibility
    // relative to the current state, so a card left hidden by an earlier
    // failure (cleanup is best-effort, and `retries` re-runs on a mutated
    // account) would otherwise invert the toggle and fail every subsequent
    // run with a misleading message. The sibling reorder test guards the same
    // way by moving Weight to the front "regardless of its current position".
    await restoreAllVisible(page);
  });

  test('hiding a card removes it from the grid and persists across reload', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    try {
      // Outside edit mode there is no visibility control, same as the reorder
      // arrows. Anchored on the card being present first: the load gate means
      // nothing renders until the settings GET resolves (and beforeEach may
      // have just navigated), so a bare not.toBeVisible() here would pass
      // against an empty page and would still pass if the toggle leaked into
      // read-only mode.
      await expect(grid.getByTestId('vital-card-sleep')).toBeVisible();
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
      // Anchor on a card that should be there first. The grid is not rendered
      // at all until the settings GET resolves, so a bare toHaveCount(0)
      // straight after reload passes against the not-yet-rendered grid — it
      // would still pass if the server had dropped the `hidden` flag entirely.
      const reloadedGrid = page.getByTestId('vitals-grid');
      await expect(reloadedGrid.getByTestId('vital-card-steps')).toBeVisible();
      await expect(reloadedGrid.getByTestId('vital-card-sleep')).toHaveCount(0);
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

  test('settings saved in the pre-visibility shape still load', async ({ page }) => {
    // The bare-string shape is what every account written before this feature
    // holds, so it is the path a real user takes on their first load after
    // deploy — and, until this test, the only spec scenario asserted by
    // nothing (the frontend has no unit-test runner). Served via a route mock
    // rather than by writing it, because the app can no longer produce it.
    await page.route('**/api/users/me/settings', route => {
      if (route.request().method() !== 'GET') return route.fallback();
      return route.fulfill({ json: { dashboard_order: ['weight', 'steps'] } });
    });
    await page.goto('/');

    const grid = page.getByTestId('vitals-grid');
    // Named entries keep their saved order and are treated as visible...
    await expect(grid.locator('> *').nth(0)).toHaveAttribute('data-testid', 'vital-card-weight');
    await expect(grid.locator('> *').nth(1)).toHaveAttribute('data-testid', 'vital-card-steps');
    // ...and the metrics the old shape never mentioned are appended, visible,
    // rather than being dropped or defaulting to hidden.
    await expect(grid.locator('> *')).toHaveCount(8);
    await expect(grid.getByTestId('vital-card-sleep')).toBeVisible();
    await expect(page.getByTestId('vitals-grid-empty')).toHaveCount(0);
  });

  test('no card is rendered until the saved visibility is known', async ({ page }) => {
    // Hold the settings GET open, then fail it. `order` defaults to
    // every-card-visible, so a grid rendered before this resolves would show
    // cards the user hid — briefly on a slow load, and for the whole session
    // on a failure, with Customize disabled and no way to re-hide them.
    let release: () => void = () => {};
    const held = new Promise<void>(resolve => { release = resolve; });
    await page.route('**/api/users/me/settings', async route => {
      if (route.request().method() !== 'GET') return route.fallback();
      await held;
      await route.fulfill({ status: 500, json: { error: 'boom' } });
    });

    await page.goto('/');

    // While in flight: a placeholder holds the slot, and no card exists.
    await expect(page.getByTestId('vitals-grid-loading')).toBeVisible();
    await expect(page.getByTestId('vital-card-steps')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Customize' })).toBeDisabled();

    release();

    // After failure: still no cards, and the placeholder says so.
    await expect(page.getByTestId('vitals-grid-error')).toBeVisible();
    await expect(page.getByTestId('vital-card-steps')).toHaveCount(0);
    await expect(page.getByTestId('vitals-grid')).toHaveCount(0);

    // The failure is recoverable in-page: drop the mock and retry, rather
    // than leaving the dashboard stuck on an error paragraph for the whole
    // session with no way back but a manual reload.
    await page.unroute('**/api/users/me/settings');
    await page.getByTestId('vitals-grid-retry').click();
    await expect(page.getByTestId('vitals-grid').getByTestId('vital-card-steps')).toBeVisible();
    await expect(page.getByTestId('vitals-grid-error')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Customize' })).toBeEnabled();
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
    // Repair any hidden card leaked by a failed visibility test before
    // asserting on grid contents: every test below assumes the full grid, and
    // a leaked hidden card fails them for reasons unrelated to what they test.
    await restoreAllVisible(page);
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

// hide-unrecorded-data-types: presence (whether a type has ever had a row,
// not just whether it has recent data) is fetched from
// `/api/data-types/presence` and drives which cards/pills the dashboard shows
// at all. Every test here mocks that route directly rather than depending on
// which types the seeded account happens to have real rows for, so the
// covered scenarios (a type gaining/losing presence, a failed/incomplete
// fetch, a pending fetch) are exercised deterministically regardless of
// seed data.
test.describe('Data-type presence filtering', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    // Same reasoning as the other describe blocks: a card left hidden by an
    // earlier failed test must not be mistaken for presence filtering here.
    await restoreAllVisible(page);
  });

  test('a primary metric with zero data ever is hidden from the read-only vitals grid', async ({ page }) => {
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture({ sleep: false }) })
    );
    await page.goto('/');

    const grid = page.getByTestId('vitals-grid');
    await expect(grid.getByTestId('vital-card-steps')).toBeVisible();
    await expect(grid.getByTestId('vital-card-sleep')).toHaveCount(0);
  });

  // The idea-5 regression: a metric that has data, just none in the 7-day
  // sparkline window, must stay on the grid (with its own "No data"
  // in-card state) rather than being hidden — hiding is a presence-only
  // decision, not a recent-data one.
  test('a primary metric with data outside the 7-day window but presence stays shown', async ({ page }) => {
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture({ sleep: true }) })
    );
    await page.route('**/api/data/sleep**', route => route.fulfill({ json: [] }));
    await page.goto('/');

    const grid = page.getByTestId('vitals-grid');
    const sleepCard = grid.getByTestId('vital-card-sleep');
    await expect(sleepCard).toBeVisible();
    await expect(sleepCard.getByText('No data')).toBeVisible();
  });

  test('a zero-presence metric does not appear in edit/Customize mode\'s reorder/show-hide list', async ({ page }) => {
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture({ sleep: false }) })
    );
    await page.goto('/');

    await page.getByRole('button', { name: 'Customize' }).click();
    await expect(page.getByTestId('vital-card-sleep-visibility')).toHaveCount(0);
    await expect(page.locator('[data-testid$="-visibility"]')).toHaveCount(PRIMARY_METRIC_TYPES.length - 1);

    // Nothing was changed, so leave without triggering a settings write —
    // same reasoning as restoreAllVisible's own no-op exit.
    await page.goto('/');
  });

  // Hiding a middle card by presence creates a gap in the rendered list —
  // the case moveCard/toggleHidden's index-remapping exists for (see the
  // comment above them in app/page.tsx: index into the presence-filtered
  // list, resolved back onto the full stored `order`), but nothing else
  // exercises that remapping with an actual move plus reload. Reads the
  // account's *actual* current order rather than assuming PRIMARY_METRICS's
  // default — the shared seeded account's stored order may have drifted
  // from it via earlier reorder tests, and this test's own assertions don't
  // depend on which order it starts from. Found in code review.
  test('reordering a card adjacent to a presence gap resolves onto the full order, leaving the gapped type\'s own stored position untouched', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    // Baseline: capture whichever order this account currently has, then
    // pick Sleep's two neighbours to swap around the gap it will leave.
    await page.route('**/api/data-types/presence', route => route.fulfill({ json: presenceFixture() }));
    await page.goto('/');
    await expect(grid.getByTestId('vital-card-steps')).toBeVisible();
    const fullBefore = await grid.locator('> *').evaluateAll(els => els.map(e => e.getAttribute('data-testid')!));
    const sleepIdx = fullBefore.indexOf('vital-card-sleep');
    expect(sleepIdx).toBeGreaterThan(0);
    expect(sleepIdx).toBeLessThan(fullBefore.length - 1);
    const predType = fullBefore[sleepIdx - 1];
    const succType = fullBefore[sleepIdx + 1];

    // Hide Sleep by presence: predType and succType become adjacent in the
    // rendered list, with the gap it leaves behind in the middle of `order`.
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture({ sleep: false }) })
    );
    await page.reload();
    await expect(grid.getByTestId('vital-card-steps')).toBeVisible();
    const filteredBefore = await grid.locator('> *').evaluateAll(els => els.map(e => e.getAttribute('data-testid')!));
    expect(filteredBefore).toEqual(fullBefore.filter(id => id !== 'vital-card-sleep'));

    // Tracks whether the forward move actually reached the server: the
    // `finally` below re-navigates fresh (page.goto discards any unsaved
    // local state), so it must know whether the account's *persisted* order
    // was really touched before undoing it — otherwise, if the forward click
    // or its Done never persisted, "undoing" a mutation that never happened
    // would apply a fresh, unintended one to the untouched account. Found in
    // code review.
    let forwardSaved = false;
    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      // succType now renders immediately after predType — move it up, onto
      // predType's old slot. The move itself only updates local state;
      // Done is what persists it (handleDone's PUT), so withSettingsSave
      // waits on that click, not the move.
      await page.getByTestId(succType).getByRole('button', { name: /move .* up/i }).click();
      forwardSaved = await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());

      // Persisted with the gap still active.
      await page.reload();
      await expect(grid.getByTestId('vital-card-steps')).toBeVisible();
      const filteredAfter = await grid.locator('> *').evaluateAll(els => els.map(e => e.getAttribute('data-testid')!));
      const expectedFiltered = [...filteredBefore];
      const i = expectedFiltered.indexOf(succType);
      [expectedFiltered[i - 1], expectedFiltered[i]] = [expectedFiltered[i], expectedFiltered[i - 1]];
      expect(filteredAfter).toEqual(expectedFiltered);

      // Sleep's own stored position must be untouched by that swap: once it
      // regains presence, it renders back between the very two cards that
      // were just swapped around it, not shifted to the front or appended
      // at the end.
      await page.route('**/api/data-types/presence', route =>
        route.fulfill({ json: presenceFixture() })
      );
      await page.reload();
      await expect(grid.getByTestId('vital-card-sleep')).toBeVisible();
      const fullAfter = await grid.locator('> *').evaluateAll(els => els.map(e => e.getAttribute('data-testid')!));
      const expectedFull = [...fullBefore];
      const predIdx = expectedFull.indexOf(predType);
      const succIdx = expectedFull.indexOf(succType);
      [expectedFull[predIdx], expectedFull[succIdx]] = [expectedFull[succIdx], expectedFull[predIdx]];
      expect(fullAfter).toEqual(expectedFull);
    } finally {
      // Skip the inverse entirely if the forward move never persisted (e.g.
      // the move-up click or Done failed before its PUT landed): the account
      // is still in its original, untouched order, and blindly clicking
      // "move down" here would swap succType with whatever actually follows
      // it there — a new, unrelated mutation — rather than undo anything.
      if (forwardSaved) {
        // Undoes the swap through the mirror move (down instead of up) in the
        // same presence-gapped context that produced it, so it inverts
        // exactly rather than needing a different sequence of arrow clicks
        // under full presence — predType and succType are no longer adjacent
        // in the full order once Sleep is back, so a full-presence "move
        // down" would swap the wrong pair.
        await page.route('**/api/data-types/presence', route =>
          route.fulfill({ json: presenceFixture({ sleep: false }) })
        );
        await page.goto('/');
        const customizeBtn = page.getByRole('button', { name: 'Customize' });
        await customizeBtn.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => null);
        if (await customizeBtn.isVisible().catch(() => false)) {
          await customizeBtn.click().catch(() => {});
          const moveBack = page.getByTestId(succType).getByRole('button', { name: /move .* down/i });
          if (!(await moveBack.isDisabled().catch(() => true))) {
            await moveBack.click().catch(() => {});
          }
          const done = page.getByRole('button', { name: 'Done' });
          if (await done.isVisible().catch(() => false)) {
            await withSettingsSave(page, () => done.click());
          }
        }
      }
    }
  });

  test('the vitals-grid-empty-no-data placeholder renders when no primary metric has presence', async ({ page }) => {
    const overrides = Object.fromEntries(PRIMARY_METRIC_TYPES.map(type => [type, false]));
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture(overrides) })
    );
    await page.goto('/');

    // Distinct from vitals-grid-empty, which covers "hid every data-bearing
    // card via Customize" — here there is nothing to un-hide.
    await expect(page.getByTestId('vitals-grid-empty-no-data')).toBeVisible();
    await expect(page.getByTestId('vitals-grid-empty')).toHaveCount(0);
    await expect(page.getByTestId('vitals-grid')).toHaveCount(0);
  });

  test('a secondary type with zero data ever is omitted from More Data; one with data is shown', async ({ page }) => {
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture({ vo2_max: true, height: false }) })
    );
    await page.goto('/');

    const moreData = page.getByTestId('more-data');
    await expect(moreData.locator('a[href="/data/vo2_max/"]')).toBeVisible();
    await expect(moreData.locator('a[href="/data/height/"]')).toHaveCount(0);
  });

  test('the More Data section is entirely absent when no secondary type has presence', async ({ page }) => {
    const overrides = Object.fromEntries(SECONDARY_TYPES.map(type => [type, false]));
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture(overrides) })
    );
    await page.goto('/');

    await expect(page.getByTestId('more-data')).toHaveCount(0);
  });

  test('a presence-fetch failure leaves every primary and secondary type visible (fail open)', async ({ page }) => {
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ status: 500, json: { error: 'boom' } })
    );
    await page.goto('/');

    const grid = page.getByTestId('vitals-grid');
    for (const type of PRIMARY_METRIC_TYPES) {
      await expect(grid.getByTestId(`vital-card-${type}`)).toBeVisible();
    }
    await expect(page.getByTestId('more-data').locator('a[href="/data/vo2_max/"]')).toBeVisible();
  });

  test('a presence response missing an entry for a type leaves that type visible (fail open)', async ({ page }) => {
    // Only `steps` present in the response — every other type (primary and
    // secondary) is absent from the map entirely, not merely `false`.
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: { steps: true } })
    );
    await page.goto('/');

    const grid = page.getByTestId('vitals-grid');
    for (const type of PRIMARY_METRIC_TYPES) {
      await expect(grid.getByTestId(`vital-card-${type}`)).toBeVisible();
    }
    await expect(page.getByTestId('more-data').locator('a[href="/data/vo2_max/"]')).toBeVisible();
  });

  test('while the presence fetch is pending, nothing is filtered in by presence yet', async ({ page }) => {
    // Mirrors the existing settings-load gate test: hold the response open,
    // assert the loading placeholder covers the grid and More Data, then
    // release and confirm both appear.
    let release: () => void = () => {};
    const held = new Promise<void>(resolve => { release = resolve; });
    await page.route('**/api/data-types/presence', async route => {
      await held;
      await route.fulfill({ json: presenceFixture() });
    });

    await page.goto('/');

    await expect(page.getByTestId('vitals-grid-loading')).toBeVisible();
    await expect(page.getByTestId('vitals-grid')).toHaveCount(0);
    await expect(page.getByTestId('more-data')).toHaveCount(0);

    release();

    await expect(page.getByTestId('vitals-grid')).toBeVisible();
    await expect(page.getByTestId('vitals-grid-loading')).toHaveCount(0);
    await expect(page.getByTestId('more-data')).toBeVisible();
  });
});

// Reads the account's current settings, merges `patch` on top, and PUTs the
// result — a raw (typed-API-bypassing) write, used only to seed a stored
// value the TS UserSettings type wouldn't allow api.updateSettings to send
// (e.g. more_data_hidden as a string), per the strict-normalization test
// below. Runs via page.evaluate so it carries the session cookie the way a
// real client write would.
async function putRawSettings(page: Page, patch: Record<string, unknown>) {
  await page.evaluate(async (patch) => {
    const current = await fetch('/api/users/me/settings', { credentials: 'include' }).then(r => r.json());
    await fetch('/api/users/me/settings', {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...current, ...patch }),
    });
  }, patch);
}

test.describe('More Data visibility', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    // Same reasoning as the other describe blocks: a section left hidden (or
    // a card left hidden) by an earlier failed test must not be mistaken for
    // what this block itself is testing.
    await restoreAllVisible(page);
  });

  test('hiding the More Data section removes it from the read-only page, stays discoverable (dimmed) in edit mode, and persists across reload; re-showing restores it', async ({ page }) => {
    const moreData = page.getByTestId('more-data');

    try {
      await expect(moreData).toBeVisible();
      await expect(page.getByTestId('more-data-visibility')).not.toBeVisible();

      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByTestId('more-data-visibility').click();
      // Still on screen while editing — dimmed, so it can be found and shown
      // again — mirroring the vitals grid's dimmed-hidden-card treatment.
      await expect(moreData).toHaveAttribute('data-hidden', 'true');

      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      await page.reload();
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      // Re-show it.
      await page.getByRole('button', { name: 'Customize' }).click();
      await expect(page.getByTestId('more-data')).toHaveAttribute('data-hidden', 'true');
      await page.getByTestId('more-data-visibility').click();
      await expect(page.getByTestId('more-data')).toHaveAttribute('data-hidden', 'false');
      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(page.getByTestId('more-data')).toBeVisible();

      await page.reload();
      await expect(page.getByTestId('more-data')).toBeVisible();
    } finally {
      await restoreAllVisible(page);
    }
  });

  test('the More Data visibility toggle is absent from edit mode when no secondary type has presence', async ({ page }) => {
    const overrides = Object.fromEntries(SECONDARY_TYPES.map(type => [type, false]));
    await page.route('**/api/data-types/presence', route =>
      route.fulfill({ json: presenceFixture(overrides) })
    );
    await page.goto('/');

    await page.getByRole('button', { name: 'Customize' }).click();
    await expect(page.getByTestId('more-data')).toHaveCount(0);
    await expect(page.getByTestId('more-data-visibility')).toHaveCount(0);

    // Nothing was changed, so leave without triggering a settings write —
    // same reasoning as restoreAllVisible's own no-op exit.
    await page.goto('/');
  });

  test('a user-hidden More Data section stays hidden through a presence-fetch failure', async ({ page }) => {
    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByTestId('more-data-visibility').click();
      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      // Per design.md's Risks section, a user-hidden section stays hidden
      // even during the fail-open path that would otherwise show every
      // primary and secondary type.
      await page.route('**/api/data-types/presence', route =>
        route.fulfill({ status: 500, json: { error: 'boom' } })
      );
      await page.reload();
      await expect(page.getByTestId('vitals-grid')).toBeVisible();
      await expect(page.getByTestId('more-data')).toHaveCount(0);
    } finally {
      await page.unroute('**/api/data-types/presence').catch(() => {});
      await restoreAllVisible(page);
    }
  });

  test('More Data does not render before the saved hidden preference is known, and does not flash unhidden if the settings load fails', async ({ page }) => {
    // Hide it first so "stays hidden" is the persisted preference under test.
    await page.getByRole('button', { name: 'Customize' }).click();
    await page.getByTestId('more-data-visibility').click();
    await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
    await expect(page.getByTestId('more-data')).toHaveCount(0);

    try {
      let release: () => void = () => {};
      const held = new Promise<void>(resolve => { release = resolve; });
      await page.route('**/api/users/me/settings', async route => {
        if (route.request().method() !== 'GET') return route.fallback();
        await held;
        await route.fulfill({ status: 500, json: { error: 'boom' } });
      });

      await page.goto('/');

      // While in flight: not rendered, same as the vitals grid.
      await expect(page.getByTestId('vitals-grid-loading')).toBeVisible();
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      release();

      // After failure: still not rendered — a settings-load failure must not
      // flash the section unhidden.
      await expect(page.getByTestId('vitals-grid-error')).toBeVisible();
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      // Recovers in-page, and the stored "hidden" preference still holds
      // once the real settings load succeeds.
      await page.unroute('**/api/users/me/settings');
      await page.getByTestId('vitals-grid-retry').click();
      await expect(page.getByTestId('vitals-grid')).toBeVisible();
      await expect(page.getByTestId('more-data')).toHaveCount(0);
    } finally {
      await restoreAllVisible(page);
    }
  });

  // Accepted edge case from design.md's Risks section: the toggle disappears
  // if presence regresses to zero via record deletion, but the stored
  // preference is not lost — just temporarily inert.
  test('More Data and its toggle disappear from edit mode if presence drops to zero while hidden, without erroring; the stored preference survives', async ({ page }) => {
    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByTestId('more-data-visibility').click();
      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      // Drive every secondary type's presence to zero.
      const noneOverrides = Object.fromEntries(SECONDARY_TYPES.map(type => [type, false]));
      await page.route('**/api/data-types/presence', route =>
        route.fulfill({ json: presenceFixture(noneOverrides) })
      );
      await page.reload();

      await page.getByRole('button', { name: 'Customize' }).click();
      await expect(page.getByTestId('more-data')).toHaveCount(0);
      await expect(page.getByTestId('more-data-visibility')).toHaveCount(0);
      await page.goto('/');

      // Restore presence for exactly one secondary type — the section is
      // still hidden in the read-only view, i.e. the stored preference
      // survived the toggle's temporary disappearance.
      const oneTypeOverrides = Object.fromEntries(SECONDARY_TYPES.map(type => [type, type === 'vo2_max']));
      await page.route('**/api/data-types/presence', route =>
        route.fulfill({ json: presenceFixture(oneTypeOverrides) })
      );
      await page.reload();
      await expect(page.getByTestId('more-data')).toHaveCount(0);
    } finally {
      await page.unroute('**/api/data-types/presence').catch(() => {});
      await restoreAllVisible(page);
    }
  });

  // handleDone's GET-merge-PUT write path previously had a lost-update race
  // (see settings.spec.ts's "Settings lost-update race" coverage) that a
  // second key merged into the same payload could reintroduce.
  test('toggling a vitals card and the More Data section in one edit session, saved via a single Done, persists both after reload', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');

    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByTestId('vital-card-sleep-visibility').click();
      await expect(grid.getByTestId('vital-card-sleep')).toHaveAttribute('data-hidden', 'true');
      await page.getByTestId('more-data-visibility').click();
      await expect(page.getByTestId('more-data')).toHaveAttribute('data-hidden', 'true');

      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(grid.getByTestId('vital-card-sleep')).toHaveCount(0);
      await expect(page.getByTestId('more-data')).toHaveCount(0);

      await page.reload();
      await expect(grid.getByTestId('vital-card-steps')).toBeVisible();
      await expect(grid.getByTestId('vital-card-sleep')).toHaveCount(0);
      await expect(page.getByTestId('more-data')).toHaveCount(0);
    } finally {
      await restoreAllVisible(page);
    }
  });

  // Mirrors 'settings saved in the pre-visibility shape still load' above:
  // a naive `?? false` read would incorrectly treat a truthy-but-not-`true`
  // stored value as hidden. Written via a raw PUT rather than the app UI,
  // since the app itself (through its typed UserSettings) can never produce
  // this shape — but a hand-edited or pre-normalization row could.
  test('a stored more_data_hidden value that is truthy but not strictly true renders as visible, not hidden', async ({ page }) => {
    try {
      await putRawSettings(page, { more_data_hidden: 'false' });
      await page.reload();
      await expect(page.getByTestId('more-data')).toBeVisible();

      await putRawSettings(page, { more_data_hidden: 1 });
      await page.reload();
      await expect(page.getByTestId('more-data')).toBeVisible();
    } finally {
      await putRawSettings(page, { more_data_hidden: false }).catch(() => {});
    }
  });
});
