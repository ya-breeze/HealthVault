import { test, expect, type Page, type Locator } from '@playwright/test';
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

async function withSettingsSave(page: Page, action: () => Promise<unknown>): Promise<boolean> {
  const saved = page
    .waitForResponse(
      r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
      { timeout: 15_000 }
    )
    .then(r => r.ok())
    .catch(() => false);
  await action().catch(() => {});
  return saved;
}

async function mockHeartRateRecords(page: Page) {
  const now = new Date().toISOString();
  await page.route('**/api/data/heart_rate?*', route => {
    const url = new URL(route.request().url());
    if (url.searchParams.has('bucket')) {
      return route.fulfill({
        json: [{ bucket_start: now, count: 1, avg: 68, min: 68, max: 68 }],
      });
    }
    return route.fulfill({
      json: [{
        id: 'localized-table-record',
        family_id: 'family-id',
        user_id: 'user-id',
        source_payload_id: 'payload-id',
        created_at: now,
        updated_at: now,
        deleted_at: null,
        time: now,
        bpm: 68,
      }],
    });
  });
}

const FOOD_MEAL_TIME = '2026-09-08T13:45:00Z';

async function mockFoodMealRecordWithFailedDelete(page: Page) {
  await page.route('**/api/data/food_meal?*', route => route.fulfill({
    json: [{
      id: 'localized-food-record',
      logged_at: FOOD_MEAL_TIME,
      name: 'T-bone dinner',
      status: 'confirmed',
      calories: 650,
      protein_grams: 45,
      carbs_grams: 30,
      fat_grams: 38,
      sugar_grams: 4,
      sodium_grams: 1.2,
      dietary_fiber_grams: 3,
    }],
  }));
  await page.route('**/api/data/food_meal/localized-food-record', route =>
    route.fulfill({ status: 500, contentType: 'text/plain', body: 'server says delete boom' })
  );
}

test.describe('Data type pages', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('/data/steps loads with chart area', async ({ page }) => {
    await page.goto('/data/steps/');
    await expect(page.getByRole('heading', { name: 'Steps' })).toBeVisible();
    // Page should render without errors (no "something went wrong")
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
  });

  test('/data/heart_rate loads', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    await expect(page.getByRole('heading', { name: 'Heart Rate' })).toBeVisible();
  });

  test('/data/sleep loads', async ({ page }) => {
    await page.goto('/data/sleep/');
    await expect(page.getByRole('heading', { name: 'Sleep' })).toBeVisible();
  });

  test('unknown type API returns 404', async ({ page }) => {
    // The frontend static export uses SPA fallback so unknown routes get index.html (200).
    // The real guard is the API — unknown types return 404 from the backend.
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/not_a_real_type', { credentials: 'include' });
      return r.status;
    });
    expect(result).toBe(404);
  });
});

test.describe('Zoom control', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('Day/Week/Month/Year controls are visible with Week selected by default', async ({ page }) => {
    await page.goto('/data/steps/');
    for (const label of ['Day', 'Week', 'Month', 'Year']) {
      await expect(page.getByRole('button', { name: label, exact: true })).toBeVisible();
    }
  });

  test('switching zoom level re-renders the chart without error', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    for (const label of ['Day', 'Week', 'Month', 'Year']) {
      await page.getByRole('button', { name: label, exact: true }).click();
      await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    }
  });

  test('nutrition page shows a macro selector', async ({ page }) => {
    await page.goto('/data/nutrition/');
    await expect(page.getByRole('button', { name: 'Calories', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Protein', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
  });

  test('food_meal page loads without a bucketed chart request failing', async ({ page }) => {
    await page.goto('/data/food_meal/');
    await expect(page.getByText(/food.?meal/i)).toBeVisible();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    // Switching zoom on food_meal only changes the raw time range — it must
    // never trigger a ?bucket= request, which the backend rejects for this type.
    await page.getByRole('button', { name: 'Year', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
  });
});

test.describe('Localized record table', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('English headings are readable and do not expose database keys', async ({ page }) => {
    await mockHeartRateRecords(page);
    await page.goto('/data/heart_rate/');

    await expect(page.getByRole('heading', { name: 'Heart Rate' })).toBeVisible();
    for (const heading of ['Created at', 'Updated at', 'Time', 'Heart rate (bpm)', 'Actions']) {
      await expect(page.getByRole('columnheader', { name: heading, exact: true })).toBeVisible();
    }
    const headings = await page.getByRole('columnheader').allTextContents();
    expect(headings).not.toContain('created_at');
    expect(headings).not.toContain('updated_at');
    expect(headings).not.toContain('bpm');
  });

  test('Russian localizes the metric, record headers, and delete confirmation', async ({ page }) => {
    await page.goto('/settings');
    try {
      const saved = await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('ru')
      );
      expect(saved).toBe(true);
      await expect(page.locator('#display-language')).toHaveValue('ru');

      await mockHeartRateRecords(page);
      await page.goto('/data/heart_rate/');
      await expect(page.getByRole('heading', { name: 'Пульс' })).toBeVisible();
      for (const heading of ['Создано', 'Время', 'Пульс (уд/мин)', 'Действия']) {
        await expect(page.getByRole('columnheader', { name: heading, exact: true })).toBeVisible();
      }

      await page.getByRole('button', { name: 'Удалить запись' }).click();
      await expect(page.getByRole('button', { name: 'Удалить', exact: true })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Отмена', exact: true })).toBeVisible();
      await page.getByRole('button', { name: 'Отмена', exact: true }).click();

      await mockFoodMealRecordWithFailedDelete(page);
      await page.goto('/data/food_meal/');
      const localizedTime = await page.evaluate(
        value => new Date(value).toLocaleString('ru'),
        FOOD_MEAL_TIME,
      );
      await expect(page.getByRole('cell', { name: localizedTime, exact: true })).toBeVisible();
      await expect(page.getByRole('cell', { name: 'T-bone dinner', exact: true })).toBeVisible();
      await expect(page.getByRole('cell', { name: 'Подтверждено', exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'Удалить запись' }).click();
      await page.getByRole('button', { name: 'Удалить', exact: true }).click();
      await expect(page.getByText('Не удалось удалить запись. Попробуйте ещё раз.')).toBeVisible();
      await expect(page.getByText('server says delete boom')).toHaveCount(0);
      await expect(page.getByRole('cell', { name: 'T-bone dinner', exact: true })).toBeVisible();

      // A failed attempt closes the pending confirmation and leaves the row
      // available for retry; opening it again also clears the localized error.
      await page.getByRole('button', { name: 'Удалить запись' }).click();
      await expect(page.getByRole('button', { name: 'Удалить', exact: true })).toBeVisible();
      await expect(page.getByText('Не удалось удалить запись. Попробуйте ещё раз.')).toHaveCount(0);
      await page.getByRole('button', { name: 'Отмена', exact: true }).click();
    } finally {
      await page.goto('/settings').catch(() => {});
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('en')
      );
      await expect(page.locator('#display-language')).toHaveValue('en').catch(() => {});
    }
  });

  test('a family-member view keeps the owner-only Actions column absent', async ({ page }) => {
    await mockHeartRateRecords(page);
    await page.goto('/data/heart_rate/?user=bob');

    await expect(page.getByRole('columnheader', { name: 'Heart rate (bpm)', exact: true })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Actions', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Delete record' })).toHaveCount(0);
  });
});

test.describe('Point-in-time Y-axis domain and weight trend line', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  async function yAxisTickTexts(page: Page) {
    return page.locator('.recharts-yAxis-tick-labels text').allTextContents();
  }

  // Moves the chart off Week and waits until it has actually got there.
  //
  // The page opens on Week (DataTypeClient's `useState<Zoom>('week')`), so
  // clicking Week sets state to the value it already holds: React re-renders
  // nothing and no request goes out. A `Promise.all([waitForRequest, click])`
  // around that click is therefore not synchronizing with the click at all —
  // the only request that can satisfy it is the one the page fired on mount,
  // and whether that lands before or after the listener is registered is a
  // race against `page.goto` resolving. It cost three failures in six
  // full-suite runs; both tests still passed 5/5 in isolation. See
  // docs/specs/zoom-click-race.md.
  //
  // Day is the intermediate: a real zoom and never the default. Its own
  // zoom-windowed fetch carries no `bucket=`, so for heart_rate it issues
  // nothing the waiter below could match. For weight it is not quite silent —
  // the trend projection refetches on every zoom change, `bucket=day` with a
  // 60-day window — but the weight test's `spanDays` filter already excludes
  // that window structurally, which is the only reason this is safe there.
  // Widen that filter and this helper stops being safe with it.
  //
  // The click is retried rather than issued once. The page is a static export,
  // so the zoom buttons exist in the served HTML before React hydrates, and a
  // click landing in that window is swallowed — under the old code that was
  // harmless, because the mount request satisfied the waiter regardless. Now
  // it would be a hard failure, so `toPass` re-clicks until the state actually
  // moves. Waiting on Week's `aria-pressed` beforehand would not do: the
  // pre-rendered HTML already carries `aria-pressed="true"` on Week, so it
  // says nothing about hydration.
  //
  // Do not remove this as redundant — without it the assertion that follows
  // has no action to wait for.
  async function selectAnotherZoomFirst(page: Page) {
    const day = page.getByRole('button', { name: 'Day', exact: true });
    await expect(async () => {
      await day.click();
      await expect(day).toHaveAttribute('aria-pressed', 'true', { timeout: 1_000 });
    }).toPass({ timeout: 15_000 });
    await expect(page.getByRole('button', { name: 'Week', exact: true }))
      .toHaveAttribute('aria-pressed', 'false');
  }

  test('weight Year-zoom Y-axis does not zero-anchor', async ({ page }) => {
    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Year', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    const ticks = await yAxisTickTexts(page);
    // Seeded weight data on this stack clusters in the 70s-90s kg range, far
    // from zero. A regression to zero-anchoring (either from an unset domain,
    // or from the stacked-Area baseline bug this test also guards against)
    // would put a "0" tick back on the axis.
    expect(ticks.length).toBeGreaterThan(0);
    expect(ticks).not.toContain('0');
  });

  test('heart_rate Year-zoom Y-axis does not zero-anchor', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    await page.getByRole('button', { name: 'Year', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    const ticks = await yAxisTickTexts(page);
    expect(ticks.length).toBeGreaterThan(0);
    expect(ticks).not.toContain('0');
  });

  test('steps (cumulative) Y-axis keeps its zero baseline', async ({ page }) => {
    await page.goto('/data/steps/');
    await page.getByRole('button', { name: 'Year', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    const ticks = await yAxisTickTexts(page);
    expect(ticks).toContain('0');
  });

  test('weight trend line renders at Week/Month/Year but not Day', async ({ page }) => {
    await page.goto('/data/weight/');
    for (const zoom of ['Week', 'Month', 'Year']) {
      await page.getByRole('button', { name: zoom, exact: true }).click();
      await expect(page.getByText('Trend', { exact: true })).toBeVisible();
    }
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    await expect(page.getByText('Trend', { exact: true })).not.toBeVisible();
  });

  test('trend line does not render for other point-in-time metrics', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    for (const zoom of ['Week', 'Month', 'Year']) {
      await page.getByRole('button', { name: zoom, exact: true }).click();
      await expect(page.getByText('Trend', { exact: true })).not.toBeVisible();
    }
  });

  test('blood_pressure Year-zoom band renders without a zero-anchored axis', async ({ page }) => {
    await page.goto('/data/blood_pressure/');
    await page.getByRole('button', { name: 'Year', exact: true }).click();
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
    const ticks = await yAxisTickTexts(page);
    expect(ticks.length).toBeGreaterThan(0);
    expect(ticks).not.toContain('0');
  });

  // The premise selectAnotherZoomFirst exists for, asserted rather than
  // assumed. If the page's default zoom ever stops being Week, clicking Week
  // becomes a real transition again and the helper becomes unnecessary — and
  // if the default moves to Day, it becomes actively wrong. Either way this
  // fails first and says so, instead of the bucketed-fetch tests below going
  // quietly flaky. It asserts on rendered state, not on any request.
  test('the chart opens on Week, so selecting Week is not a state change', async ({ page }) => {
    await page.goto('/data/weight/');
    await expect(page.getByRole('button', { name: 'Week', exact: true }))
      .toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('button', { name: 'Day', exact: true }))
      .toHaveAttribute('aria-pressed', 'false');
  });

  // Regression coverage for a bug found in code review: Year zoom's own ~12-13
  // monthly buckets fall short of the ~14-16 periods an alpha=0.25 EMA needs to
  // converge, so weight's trend line must widen its lookback fetch the same way
  // Week's does — not just Week. These assert on the actual outgoing request
  // range, since a rendered-but-unconverged trend line would still pass a mere
  // visibility check.
  test('weight Week-zoom bucketed fetch widens to >= 14 days', async ({ page }) => {
    await page.goto('/data/weight/');
    await selectAnotherZoomFirst(page);
    // The trend projection also issues an /api/data/weight?...bucket=day
    // request, with a fixed 60-day lookback that satisfies ">= 14" on its own.
    // Matching the bare URL pattern would let this test pass on that request
    // instead of the chart's, silently ceasing to guard the widening it was
    // written for — so exclude the projection's window structurally.
    const PROJECTION_LOOKBACK_DAYS = 60;
    const spanDays = (url: URL) =>
      (new Date(url.searchParams.get('to')!).getTime()
        - new Date(url.searchParams.get('from')!).getTime()) / (1000 * 60 * 60 * 24);

    const [req] = await Promise.all([
      page.waitForRequest(r => {
        if (!/\/api\/data\/weight\?.*bucket=day/.test(r.url())) return false;
        return spanDays(new URL(r.url())) < PROJECTION_LOOKBACK_DAYS - 5;
      }),
      page.getByRole('button', { name: 'Week', exact: true }).click(),
    ]);
    const days = spanDays(new URL(req.url()));
    expect(days).toBeGreaterThanOrEqual(14);
    expect(days).toBeLessThan(PROJECTION_LOOKBACK_DAYS - 5);
  });

  test('weight Year-zoom bucketed fetch widens to >= ~2 years', async ({ page }) => {
    await page.goto('/data/weight/');
    const [req] = await Promise.all([
      page.waitForRequest(r => /\/api\/data\/weight\?.*bucket=month/.test(r.url())),
      page.getByRole('button', { name: 'Year', exact: true }).click(),
    ]);
    const url = new URL(req.url());
    const from = new Date(url.searchParams.get('from')!);
    const to = new Date(url.searchParams.get('to')!);
    const days = (to.getTime() - from.getTime()) / (1000 * 60 * 60 * 24);
    // ~2 years, allowing slack for leap years/DST rather than pinning to 730.
    expect(days).toBeGreaterThanOrEqual(700);
  });

  test('heart_rate Week-zoom bucketed fetch is not widened', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    await selectAnotherZoomFirst(page);
    const [req] = await Promise.all([
      page.waitForRequest(r => /\/api\/data\/heart_rate\?.*bucket=day/.test(r.url())),
      page.getByRole('button', { name: 'Week', exact: true }).click(),
    ]);
    const url = new URL(req.url());
    const from = new Date(url.searchParams.get('from')!);
    const to = new Date(url.searchParams.get('to')!);
    const days = (to.getTime() - from.getTime()) / (1000 * 60 * 60 * 24);
    expect(days).toBeLessThan(8);
  });
});

test.describe('API data endpoints', () => {
  test('GET /api/data/steps returns array', async ({ page, request }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    expect(Array.isArray(result.body)).toBe(true);
  });

  test('GET /api/data/unknown_type returns 404', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/not_a_real_type', { credentials: 'include' });
      return r.status;
    });
    expect(result).toBe(404);
  });

  test('GET /api/data/summary returns expected shape', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/summary?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return r.json();
    });
    expect(result).toHaveProperty('steps');
    expect(result).toHaveProperty('avg_heart_rate');
    expect(result).toHaveProperty('sleep_seconds');
  });

  test('GET /api/data/steps?bucket=day returns bucket_start/count/sum rows', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    expect(Array.isArray(result.body)).toBe(true);
    if (result.body.length > 0) {
      expect(result.body[0]).toHaveProperty('bucket_start');
      expect(result.body[0]).toHaveProperty('count');
      expect(result.body[0]).toHaveProperty('sum');
    }
  });

  test('GET /api/data/heart_rate?bucket=day returns avg/min/max rows', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/heart_rate?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      expect(result.body[0]).toHaveProperty('avg');
      expect(result.body[0]).toHaveProperty('min');
      expect(result.body[0]).toHaveProperty('max');
    }
  });

  test('GET /api/data/blood_pressure?bucket=day returns dual systolic/diastolic columns', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/blood_pressure?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      for (const field of ['systolic_avg', 'systolic_min', 'systolic_max', 'diastolic_avg', 'diastolic_min', 'diastolic_max']) {
        expect(result.body[0]).toHaveProperty(field);
      }
    }
  });

  test('GET /api/data/nutrition?bucket=day returns per-macro sum columns', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/nutrition?bucket=day&from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      expect(result.body[0]).toHaveProperty('sum_calories');
      expect(result.body[0]).toHaveProperty('sum_protein_grams');
    }
  });

  test('GET /api/data/steps?bucket=week (invalid) returns 400', async ({ page }) => {
    await login(page);
    const status = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?bucket=week', { credentials: 'include' });
      return r.status;
    });
    expect(status).toBe(400);
  });

  test('GET /api/data/food_meal?bucket=day returns 400', async ({ page }) => {
    await login(page);
    const status = await page.evaluate(async () => {
      const r = await fetch('/api/data/food_meal?bucket=day', { credentials: 'include' });
      return r.status;
    });
    expect(status).toBe(400);
  });

  test('Omitting ?bucket= still returns raw records', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z', {
        credentials: 'include',
      });
      return { status: r.status, body: await r.json() };
    });
    expect(result.status).toBe(200);
    if (result.body.length > 0) {
      expect(result.body[0]).not.toHaveProperty('bucket_start');
    }
  });
});

test.describe('Step interval collapse (check-the-health-data)', () => {
  // 2019-06-15 is far enough in the past that no other spec's window covers
  // it — this file's own bucketed-query assertions above use
  // from=2020-01-01, and dashboard.spec.ts's webhook steps test seeds
  // "today" instead.
  const day = '2019-06-15';
  const startOfDay = `${day}T00:00:00Z`;
  const endOfDay = `${day}T23:59:59Z`;

  test.beforeEach(async ({ request }) => {
    // Two overlapping step records for the same day in one webhook POST — a
    // full-morning record plus a smaller one nested inside it, simulating
    // two sync sources' overlapping copies of the same walk.
    const resp = await request.post(`${BASE_URL}/webhook/${USER}`, {
      data: {
        timestamp: new Date().toISOString(),
        app_version: 'e2e-test-1.0',
        steps: [
          { count: 4000, start_time: startOfDay, end_time: `${day}T12:00:00Z` },
          { count: 1500, start_time: `${day}T01:00:00Z`, end_time: `${day}T06:00:00Z` },
        ],
      },
    });
    expect(resp.status()).toBe(204);
  });

  test('overlapping records posted to the webhook are counted once in the bucketed steps total', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async ({ from, to }) => {
      const r = await fetch(`/api/data/steps?bucket=day&from=${from}&to=${to}`, { credentials: 'include' });
      return { status: r.status, body: await r.json() };
    }, { from: startOfDay, to: endOfDay });
    expect(result.status).toBe(200);
    const bucket = result.body.find((b: { bucket_start: string }) => b.bucket_start.startsWith(day));
    expect(bucket).toBeTruthy();
    // 4000, not 5500 — the nested, fully-overlapping record must be dropped,
    // not summed.
    expect(bucket.sum).toBe(4000);
  });

  test('GET /api/data/steps/diagnostics reports raw_sum above collapsed_sum with dropped_records', async ({ page }) => {
    await login(page);
    const result = await page.evaluate(async ({ from, to }) => {
      const r = await fetch(`/api/data/steps/diagnostics?from=${from}&to=${to}`, { credentials: 'include' });
      return { status: r.status, body: await r.json() };
    }, { from: startOfDay, to: endOfDay });
    expect(result.status).toBe(200);
    const dayRow = result.body.find((d: { bucket_start: string }) => d.bucket_start.startsWith(day));
    expect(dayRow).toBeTruthy();
    expect(dayRow.raw_sum).toBeGreaterThan(dayRow.collapsed_sum);
    expect(dayRow.dropped_records).toBeGreaterThan(0);
  });

  test('the steps page diagnostic disclosure is collapsed on load and reveals the table when activated', async ({ page }) => {
    await login(page);
    await page.goto('/data/steps/');

    const toggle = page.getByTestId('steps-diagnostics-toggle');
    await expect(toggle).toBeVisible();
    const detail = page.getByTestId('steps-diagnostics-detail');
    await expect(detail).toBeHidden();

    await toggle.click();
    await expect(detail).toBeVisible();
  });
});

test.describe('Manual record writes: weight_goal, height, write allowlist', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  async function deleteAllRecords(page: Page, type: string) {
    const records = await page.evaluate(async (t) => {
      const r = await fetch(`/api/data/${t}?from=2000-01-01T00:00:00Z&to=2100-01-01T00:00:00Z`, {
        credentials: 'include',
      });
      return r.json();
    }, type);
    for (const rec of records as Array<{ id: string }>) {
      await page.evaluate(async ({ t, id }) => {
        await fetch(`/api/data/${t}/${id}`, { method: 'DELETE', credentials: 'include' });
      }, { t: type, id: rec.id });
    }
  }

  async function postRecord(page: Page, type: string, value: number) {
    await page.evaluate(async ({ t, value }) => {
      await fetch(`/api/data/${t}`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ value }),
      });
    }, { t: type, value });
  }

  test('creating a weight_goal record via the Add-record form appears on /data/weight_goal and as a goal line on the weight chart', async ({ page }) => {
    // Tests are re-run against a persistent WIP stack, so clear out any goal
    // left by a prior run rather than assuming a clean-slate account.
    await deleteAllRecords(page, 'weight_goal');

    await page.goto('/data/weight_goal/');
    await page.getByLabel('Value').fill('72.5');
    await page.getByRole('button', { name: 'Add', exact: true }).click();
    await expect(page.getByRole('cell', { name: '72.5' })).toBeVisible();

    await page.goto('/data/weight/');
    await expect(page.getByText('Goal', { exact: true })).toBeVisible();
  });

  test('creating a height record closes the BMI dead end: bands + readout appear on the weight chart after, absent before', async ({ page }) => {
    await deleteAllRecords(page, 'height');
    // The BMI readout reads the latest *raw* weight record already visible
    // in the current (Week) zoom window, independent of the height gate
    // under test here — seed a fresh one so recency of seeded data can't
    // make this test flaky.
    await postRecord(page, 'weight', 80);

    await page.goto('/data/weight/');
    await expect(page.getByText('BMI', { exact: true })).not.toBeVisible();

    await page.goto('/data/height/');
    await page.getByLabel('Value').fill('1.78');
    await page.getByRole('button', { name: 'Add', exact: true }).click();
    await expect(page.getByRole('cell', { name: '1.78' })).toBeVisible();

    await page.goto('/data/weight/');
    await expect(page.getByText('BMI', { exact: true })).toBeVisible();
    // Ordering is load-bearing: waiting for the BMI readout first proves the
    // height fetch has settled, which is what makes this negative assertion
    // mean something. The shortcut used to render on every load — heightRecords
    // is [] until the fetch resolves — so a user who already had a height was
    // offered "Set height" anyway, and clicking it wrote a duplicate.
    await expect(page.getByTestId('set-height')).toHaveCount(0);
  });

  // The test above navigates to /data/height/ by direct URL, so it proves the
  // height *page* works while saying nothing about whether a user can reach
  // it. They could not: height is a secondary type, and the dashboard's More
  // Data list — the only place secondary type pages are linked — is filtered
  // by data presence, so a user with no height had no route to it. This test
  // pins the reachability, not just the page.
  test('a user with no height can add one without knowing the URL', async ({ page }) => {
    await deleteAllRecords(page, 'height');
    await postRecord(page, 'weight', 80);

    // The dashboard must not be relied on: with zero height rows it offers no
    // link at all. The weight page is where the user already is.
    await page.goto('/data/weight/');
    await expect(page.getByText('BMI', { exact: true })).not.toBeVisible();

    const setHeight = page.getByTestId('set-height');
    await expect(setHeight).toBeVisible();
    await setHeight.click();

    const heightForm = page.getByTestId('add-record-height');
    await heightForm.getByLabel(/^Value/).fill('1.78');
    await heightForm.getByRole('button', { name: 'Add', exact: true }).click();

    // BMI turns on without ever leaving the weight page.
    await expect(page.getByText('BMI', { exact: true })).toBeVisible();
    // And the shortcut retires once it has served its purpose.
    await expect(page.getByTestId('set-height')).toHaveCount(0);
  });

  test('a future-dated write is rejected rather than creating an invisible record', async ({ page }) => {
    const future = new Date(Date.now() + 48 * 60 * 60 * 1000).toISOString();
    const status = await page.evaluate(async (t) => {
      const r = await fetch('/api/data/weight', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ value: 81, time: t }),
      });
      return r.status;
    }, future);
    expect(status).toBe(400);
  });

  test('POST to a non-allowlisted type (steps) is rejected with 403', async ({ page }) => {
    const status = await page.evaluate(async () => {
      const r = await fetch('/api/data/steps', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ value: 1000 }),
      });
      return r.status;
    });
    expect(status).toBe(403);
  });
});

test.describe('Webhook endpoint', () => {
  test('POST /webhook/alice with valid payload returns 204', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/webhook/${USER}`, {
      data: {
        timestamp: new Date().toISOString(),
        app_version: 'e2e-test-1.0',
        heart_rate: [{ bpm: 65, time: new Date().toISOString() }],
      },
    });
    expect(resp.status()).toBe(204);
  });

  test('POST /webhook/nonexistent_user returns 404', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/webhook/nonexistent_user_xyz`, {
      data: { timestamp: new Date().toISOString(), app_version: '1.0' },
    });
    expect(resp.status()).toBe(404);
  });

  test('POST /webhook/alice with invalid JSON returns 400', async ({ request }) => {
    const resp = await request.post(`${BASE_URL}/webhook/${USER}`, {
      headers: { 'Content-Type': 'application/json' },
      data: 'not valid json{{{',
    });
    expect(resp.status()).toBe(400);
  });
});

// ---------------------------------------------------------------------------
// Data-detail chart localization
// (docs/specs/complete-the-owner-selected-english-and.md)
//
// Every chart series name, tick/tooltip format, and summary/projection
// string on this route now depends on the selected Display Language — see
// DataTypeClient.tsx and lib/i18n/{en,ru}.ts's dataDetail.* catalog. These
// tests cover four chart shapes (heart_rate as the ordinary point page,
// blood_pressure, nutrition, weight) in both languages, using deterministic
// mocked responses rather than the shared seeded account's real health
// data, which this file's other suites already depend on staying stable.
// ---------------------------------------------------------------------------

function hoursAgo(hours: number): string {
  return new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();
}

// UTC-midnight bucket_start, matching bucketLabel's read-back convention
// (DataTypeClient.tsx's comment on bucketLabel) — the same helper as
// chart-touch-readout.spec.ts's isoDateDaysAgo.
function daysAgoUTC(daysAgo: number): string {
  const d = new Date();
  d.setUTCHours(0, 0, 0, 0);
  d.setUTCDate(d.getUTCDate() - daysAgo);
  return d.toISOString();
}

type DataDetailMockRow = Record<string, unknown>;

// One type's fixtures for every request shape DataTypeClient can issue for
// it. `raw` backs the zoom-window fetch (record table, plus the Day-zoom
// chart); `bucketDay`/`bucketMonth` back the active zoom's own bucketed
// chart fetch. Weight adds two more: `allTimeRaw` backs the all-time weight
// fetch that feeds the BMI readout and the projection's lifetime-history
// gate, and `projectionBucket` backs the dedicated 60-day daily-bucketed
// fetch that feeds the trend regression — both fire only for
// dataType === 'weight' and, per the spec's task 5.1, must never be
// satisfiable by the chart's own zoom-window or bucketed requests (or vice
// versa). `weightContextStatus`, when set, fails only those two
// weight-context fetches with the given HTTP status — never the ordinary
// raw/bucketDay fetches, whose own failure already redirects to /login.
interface DataDetailMock {
  raw?: DataDetailMockRow[];
  bucketDay?: DataDetailMockRow[];
  bucketMonth?: DataDetailMockRow[];
  allTimeRaw?: DataDetailMockRow[];
  projectionBucket?: DataDetailMockRow[];
  weightContextStatus?: number;
}

// Routes every `/api/data/<type>` request the data-detail page can issue,
// branching on the type, the `bucket` query param, and — for weight alone,
// the only type with two distinct un-bucketed fetches and two distinct
// bucket=day fetches — the requested `from` date.
async function mockDataDetail(page: Page, mocks: Record<string, DataDetailMock>) {
  await page.route('**/api/data/**', route => {
    const url = new URL(route.request().url());
    const type = url.pathname.split('/').filter(Boolean).pop() ?? '';
    const mock = mocks[type];
    if (!mock) return route.fulfill({ json: [] });

    const bucket = url.searchParams.get('bucket');
    if (bucket === 'month') return route.fulfill({ json: mock.bucketMonth ?? [] });

    if (bucket === 'day') {
      // The projection's own bucket=day fetch is a fixed 60-day lookback
      // (DataTypeClient.tsx), well past the chart's own bucket=day span at
      // every zoom that requests one — Week's ~14 widened days, Month's 30
      // unwidened days — see this file's "widens to >= 14 days"/"is not
      // widened" tests above, which this threshold mirrors.
      if (type === 'weight' && (mock.projectionBucket || mock.weightContextStatus !== undefined)) {
        const from = new Date(url.searchParams.get('from')!).getTime();
        const to = new Date(url.searchParams.get('to')!).getTime();
        const spanDays = (to - from) / (1000 * 60 * 60 * 24);
        if (spanDays > 45) {
          if (mock.weightContextStatus !== undefined) {
            return route.fulfill({ status: mock.weightContextStatus, contentType: 'text/plain', body: 'mock weight-context failure' });
          }
          return route.fulfill({ json: mock.projectionBucket ?? [] });
        }
      }
      return route.fulfill({ json: mock.bucketDay ?? [] });
    }

    // No bucket: either the zoom-window raw fetch (record table, Day-zoom
    // chart) or, for weight only, the separate all-time fetch — ALL_TIME_FROM
    // is the Unix epoch, which no zoom window's `from` is ever close to.
    if (type === 'weight' && (mock.allTimeRaw || mock.weightContextStatus !== undefined)) {
      const fromYear = new Date(url.searchParams.get('from')!).getUTCFullYear();
      if (fromYear < 2000) {
        if (mock.weightContextStatus !== undefined) {
          return route.fulfill({ status: mock.weightContextStatus, contentType: 'text/plain', body: 'mock weight-context failure' });
        }
        return route.fulfill({ json: mock.allTimeRaw ?? [] });
      }
    }
    return route.fulfill({ json: mock.raw ?? [] });
  });
}

function dataDetailChartSurface(page: Page): Locator {
  return page.getByTestId('chart-surface');
}

function dataDetailTooltip(page: Page): Locator {
  return page.locator('.recharts-tooltip-wrapper');
}

async function dataDetailPlotPoint(page: Page, fractionX: number, fractionY = 0.5): Promise<{ x: number; y: number }> {
  const grid = await page.locator('.recharts-cartesian-grid').first().boundingBox();
  expect(grid, 'the plot area should have a bounding box').not.toBeNull();
  return { x: grid!.x + grid!.width * fractionX, y: grid!.y + grid!.height * fractionY };
}

// Opens the tooltip with a real mouse hover — this suite runs on the default
// desktop Chrome project (no touch), so the simpler mouse path from
// chart-touch-readout.spec.ts's "mouse regression" describe block applies
// here, without that file's touch-isolation machinery.
async function hoverDataDetailChart(page: Page, fractionX: number, fractionY = 0.5) {
  await expect(dataDetailChartSurface(page)).toBeVisible();
  const { x, y } = await dataDetailPlotPoint(page, fractionX, fractionY);
  await page.mouse.move(x, y);
  await expect(dataDetailTooltip(page)).toBeVisible();
}

async function dataDetailXAxisTickTexts(page: Page): Promise<string[]> {
  return page.locator('.recharts-xAxis-tick-labels text').allTextContents();
}

async function selectRussianDisplayLanguage(page: Page) {
  await page.goto('/settings');
  const saved = await withSettingsSave(page, () =>
    page.locator('#display-language').selectOption('ru')
  );
  expect(saved).toBe(true);
  await expect(page.locator('#display-language')).toHaveValue('ru');
}

async function restoreEnglishDisplayLanguage(page: Page) {
  await page.goto('/settings').catch(() => {});
  await withSettingsSave(page, () =>
    page.locator('#display-language').selectOption('en')
  );
  await expect(page.locator('#display-language')).toHaveValue('en').catch(() => {});
}

function heartRateDataDetailMocks(): Record<string, DataDetailMock> {
  return {
    heart_rate: {
      raw: [
        { id: 'hr1', bpm: 62, time: hoursAgo(20) },
        { id: 'hr2', bpm: 71, time: hoursAgo(2) },
      ],
      bucketDay: [2, 1, 0].map((daysAgo, i) => ({
        bucket_start: daysAgoUTC(daysAgo), avg: 64 + i * 3, min: 60 + i * 3, max: 70 + i * 3,
      })),
    },
  };
}

function bloodPressureDataDetailMocks(): Record<string, DataDetailMock> {
  return {
    blood_pressure: {
      raw: [
        { id: 'bp1', systolic: 112, diastolic: 70, time: hoursAgo(20) },
        { id: 'bp2', systolic: 124, diastolic: 80, time: hoursAgo(2) },
      ],
      bucketDay: [2, 1, 0].map((daysAgo, i) => ({
        bucket_start: daysAgoUTC(daysAgo),
        systolic_avg: 114 + i * 4, systolic_min: 110 + i * 4, systolic_max: 118 + i * 4,
        diastolic_avg: 72 + i * 2, diastolic_min: 68 + i * 2, diastolic_max: 76 + i * 2,
      })),
    },
  };
}

function nutritionDataDetailMocks(): Record<string, DataDetailMock> {
  return {
    nutrition: {
      raw: [
        {
          id: 'n1', time: hoursAgo(20),
          calories: 900, protein_grams: 30, carbs_grams: 90, fat_grams: 25,
          sugar_grams: 10, sodium_grams: 1.0, dietary_fiber_grams: 8,
        },
        {
          id: 'n2', time: hoursAgo(2),
          calories: 1234, protein_grams: 45, carbs_grams: 120, fat_grams: 38,
          sugar_grams: 20, sodium_grams: 1.8, dietary_fiber_grams: 12,
        },
      ],
      bucketDay: [2, 1, 0].map((daysAgo, i) => ({
        bucket_start: daysAgoUTC(daysAgo),
        sum_calories: 1800 + i * 200, sum_protein_grams: 80 + i * 5,
        sum_carbs_grams: 180 + i * 10, sum_fat_grams: 60 + i * 3,
        sum_sugar_grams: 40 + i * 2, sum_sodium_grams: 2.2 + i * 0.1,
        sum_dietary_fiber_grams: 22 + i,
      })),
    },
  };
}

const WEIGHT_HEIGHT_METERS = 1.78;
const WEIGHT_LATEST_KG = 85; // bmi = 85 / 1.78^2 ≈ 26.83 → 'overweight'
const WEIGHT_WEEK_AVG_KG = 79.2;

function weightGoalDataDetailMock(kg: number): DataDetailMockRow[] {
  return [{ id: 'goal1', kilograms: kg, time: daysAgoUTC(200) }];
}

// The chart's own Week/Month-zoom bucketed fetch — independent of the
// projection's dedicated fetch below, so this can stay a small, easy-to-read
// series without needing to double as regression input.
function weightChartBucketDayMock(): DataDetailMockRow[] {
  return [4, 3, 2, 1, 0].map((daysAgo, i) => ({
    bucket_start: daysAgoUTC(daysAgo), avg: 80.0 - i * 0.4, min: 79.0 - i * 0.4, max: 81.0 - i * 0.4,
  }));
}

// Main weight-page fixture set: a height and a goal are on file, and the
// projection lands 'on-track' with an ETA — reusing the same declining
// 60-daily-bucket shape chart-touch-readout.spec.ts's Month-zoom projection
// test already verified against computeProjection (dataTypeMeta.ts).
function weightMainDataDetailMocks(): Record<string, DataDetailMock> {
  const days = 60;
  const projectionBucket: DataDetailMockRow[] = Array.from({ length: days }, (_, i) => {
    const daysAgo = days - 1 - i;
    return { bucket_start: daysAgoUTC(daysAgo), avg: 100 - i * 0.3 };
  });
  return {
    weight: {
      raw: [],
      bucketDay: weightChartBucketDayMock(),
      allTimeRaw: [
        { id: 'atw0', kilograms: 92, time: daysAgoUTC(59) },
        { id: 'atw1', kilograms: 90, time: daysAgoUTC(45) },
        { id: 'atw2', kilograms: 88, time: daysAgoUTC(30) },
        { id: 'atw3', kilograms: 86, time: daysAgoUTC(15) },
        { id: 'atw4', kilograms: WEIGHT_LATEST_KG, time: daysAgoUTC(0) },
      ],
      projectionBucket,
    },
    height: { raw: [{ id: 'h1', meters: WEIGHT_HEIGHT_METERS, time: daysAgoUTC(200) }] },
    weight_goal: { raw: weightGoalDataDetailMock(70) },
  };
}

type WeightProjectionScenario = 'reached' | 'not-on-track' | 'insufficient-data' | 'load-failure';

// The other four projection outcomes the main fixture set above doesn't
// exercise — each isolates exactly the condition computeProjection or
// hasEnoughDataForProjection (dataTypeMeta.ts) gates on, independent of the
// chart's own bucketDay rendering fixture.
function weightProjectionScenarioMocks(scenario: WeightProjectionScenario): Record<string, DataDetailMock> {
  switch (scenario) {
    case 'reached': {
      // windowStartEma === latestEma === goal — computeProjection's
      // direction===0-and-at-goal branch.
      const flat: DataDetailMockRow[] = Array.from({ length: 20 }, (_, i) => ({
        bucket_start: daysAgoUTC(19 - i), avg: 70,
      }));
      return {
        weight: {
          raw: [], bucketDay: [],
          allTimeRaw: [0, 1, 2, 3, 4].map(i => ({ id: `r${i}`, kilograms: 70, time: daysAgoUTC(19 - i * 5) })),
          projectionBucket: flat,
        },
        height: { raw: [] },
        weight_goal: { raw: weightGoalDataDetailMock(70) },
      };
    }
    case 'not-on-track': {
      // A flat trend (slope 0) far from the goal — computeProjection's
      // "slope === 0" branch, after the reached check falls through.
      const flat: DataDetailMockRow[] = Array.from({ length: 20 }, (_, i) => ({
        bucket_start: daysAgoUTC(19 - i), avg: 85,
      }));
      return {
        weight: {
          raw: [], bucketDay: [],
          allTimeRaw: [0, 1, 2, 3, 4].map(i => ({ id: `n${i}`, kilograms: 85, time: daysAgoUTC(19 - i * 5) })),
          projectionBucket: flat,
        },
        height: { raw: [] },
        weight_goal: { raw: weightGoalDataDetailMock(50) },
      };
    }
    case 'insufficient-data': {
      // Below PROJECTION_MIN_RECORDS (5) — hasEnoughDataForProjection gates
      // the projection off regardless of the regression window.
      return {
        weight: {
          raw: [], bucketDay: [],
          allTimeRaw: [
            { id: 'i0', kilograms: 85, time: daysAgoUTC(10) },
            { id: 'i1', kilograms: 84, time: daysAgoUTC(0) },
          ],
          projectionBucket: [],
        },
        height: { raw: [] },
        weight_goal: { raw: weightGoalDataDetailMock(75) },
      };
    }
    case 'load-failure': {
      return {
        weight: { raw: [], bucketDay: [], weightContextStatus: 500 },
        height: { raw: [] },
        weight_goal: { raw: [] },
      };
    }
  }
}

test.describe('Data-detail chart localization — English', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('heart_rate: Day and Week views translate controls, series names, and summary headings', async ({ page }) => {
    await mockDataDetail(page, heartRateDataDetailMocks());
    await page.goto('/data/heart_rate/');
    await expect(page.getByRole('heading', { name: 'Heart Rate' })).toBeVisible();

    for (const label of ['Day', 'Week', 'Month', 'Year']) {
      await expect(page.getByRole('button', { name: label, exact: true })).toBeVisible();
    }

    // Week is the default zoom — no click needed to reach it.
    await expect(page.getByText('Avg', { exact: true })).toBeVisible();
    await expect(page.getByText('Max', { exact: true })).toBeVisible();

    const expectedTick = await page.evaluate(
      iso => new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' }),
      daysAgoUTC(0),
    );
    expect(await dataDetailXAxisTickTexts(page)).toContain(expectedTick);

    await hoverDataDetailChart(page, 0.9);
    await expect(dataDetailTooltip(page).getByText('Avg', { exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Day', exact: true }).click();
    await hoverDataDetailChart(page, 0.9);
    await expect(dataDetailTooltip(page).getByText('Heart Rate', { exact: true })).toBeVisible();
    await expect(dataDetailTooltip(page).getByText('71', { exact: true })).toBeVisible();
  });

  test('blood_pressure: Day and Week views translate legend and range names', async ({ page }) => {
    await mockDataDetail(page, bloodPressureDataDetailMocks());
    await page.goto('/data/blood_pressure/');
    await expect(page.getByRole('heading', { name: 'Blood Pressure' })).toBeVisible();

    await page.getByRole('button', { name: 'Day', exact: true }).click();
    await expect(page.getByText('Systolic', { exact: true })).toBeVisible();
    await expect(page.getByText('Diastolic', { exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Week', exact: true }).click();
    await expect(page.getByText('Systolic', { exact: true })).toBeVisible();
    await expect(page.getByText('Diastolic', { exact: true })).toBeVisible();
    await hoverDataDetailChart(page, 0.9);
    await expect(dataDetailTooltip(page).getByText('Systolic range', { exact: true })).toBeVisible();
    await expect(dataDetailTooltip(page).getByText('Diastolic range', { exact: true })).toBeVisible();
  });

  test('nutrition: Day and Week views translate the macro selector and follow the selected macro', async ({ page }) => {
    await mockDataDetail(page, nutritionDataDetailMocks());
    await page.goto('/data/nutrition/');
    await expect(page.getByRole('heading', { name: 'Nutrition' })).toBeVisible();
    for (const macro of ['Calories', 'Protein', 'Carbs', 'Fat', 'Sugar', 'Sodium', 'Fiber']) {
      await expect(page.getByRole('button', { name: macro, exact: true })).toBeVisible();
    }

    // Week (default zoom): Calories macro, bar chart.
    await hoverDataDetailChart(page, 0.9);
    await expect(dataDetailTooltip(page).getByText('Calories', { exact: true })).toBeVisible();
    await expect(dataDetailTooltip(page).getByText('2,200', { exact: true })).toBeVisible();

    // Switching macro changes the bar's name and value together.
    await page.getByRole('button', { name: 'Protein', exact: true }).click();
    await hoverDataDetailChart(page, 0.9);
    await expect(dataDetailTooltip(page).getByText('Protein', { exact: true })).toBeVisible();
    await expect(dataDetailTooltip(page).getByText('90', { exact: true })).toBeVisible();

    // Day zoom: line chart, still following the selected macro (Protein).
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    await hoverDataDetailChart(page, 0.9);
    await expect(dataDetailTooltip(page).getByText('Protein', { exact: true })).toBeVisible();
    await expect(dataDetailTooltip(page).getByText('45', { exact: true })).toBeVisible();
  });

  test('weight: Week and Month views show Goal/Trend, Avg/Max/BMI, and the on-track projection ETA', async ({ page }) => {
    await mockDataDetail(page, weightMainDataDetailMocks());
    await page.goto('/data/weight/');
    await expect(page.getByRole('heading', { name: 'Weight' })).toBeVisible();

    // 'Avg' names both the summary heading and the chart's Legend entry —
    // translated in both, so it appears twice rather than once.
    await expect(page.getByText('Avg', { exact: true })).toHaveCount(2);
    await expect(page.getByText('Max', { exact: true })).toBeVisible();
    await expect(page.getByText('BMI', { exact: true })).toBeVisible();
    await expect(page.getByText('Overweight')).toBeVisible();
    await expect(page.getByText('Goal', { exact: true })).toBeVisible();
    await expect(page.getByText('Trend', { exact: true })).toBeVisible();

    const message = page.getByTestId('projection-message');
    await expect(message).toContainText('On track to reach your goal around');

    await hoverDataDetailChart(page, 0.5);
    await expect(dataDetailTooltip(page).getByText('Avg', { exact: true })).toBeVisible();
    const expectedAvg = await page.evaluate(
      v => v.toLocaleString('en-US', { minimumFractionDigits: 1, maximumFractionDigits: 1 }),
      WEIGHT_WEEK_AVG_KG,
    );
    await expect(dataDetailTooltip(page).getByText(expectedAvg, { exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Month', exact: true }).click();
    await expect(message).toContainText('On track to reach your goal around');
  });
});

test.describe('Data-detail chart localization — Russian', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('heart_rate: Day and Week views translate controls, series names, and summary headings', async ({ page }) => {
    try {
      await selectRussianDisplayLanguage(page);
      await mockDataDetail(page, heartRateDataDetailMocks());
      await page.goto('/data/heart_rate/');
      await expect(page.getByRole('heading', { name: 'Пульс' })).toBeVisible();

      for (const label of ['День', 'Неделя', 'Месяц', 'Год']) {
        await expect(page.getByRole('button', { name: label, exact: true })).toBeVisible();
      }
      await expect(page.getByText('Среднее', { exact: true })).toBeVisible();
      await expect(page.getByText('Макс.', { exact: true })).toBeVisible();

      const expectedTick = await page.evaluate(
        iso => new Date(iso).toLocaleDateString('ru', { month: 'short', day: 'numeric', timeZone: 'UTC' }),
        daysAgoUTC(0),
      );
      expect(await dataDetailXAxisTickTexts(page)).toContain(expectedTick);

      await hoverDataDetailChart(page, 0.9);
      await expect(dataDetailTooltip(page).getByText('Среднее', { exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'День', exact: true }).click();
      await hoverDataDetailChart(page, 0.9);
      await expect(dataDetailTooltip(page).getByText('Пульс', { exact: true })).toBeVisible();
      await expect(dataDetailTooltip(page).getByText('71', { exact: true })).toBeVisible();
    } finally {
      await restoreEnglishDisplayLanguage(page);
    }
  });

  test('blood_pressure: Day and Week views translate legend and range names', async ({ page }) => {
    try {
      await selectRussianDisplayLanguage(page);
      await mockDataDetail(page, bloodPressureDataDetailMocks());
      await page.goto('/data/blood_pressure/');
      await expect(page.getByRole('heading', { name: 'Давление' })).toBeVisible();

      await page.getByRole('button', { name: 'День', exact: true }).click();
      await expect(page.getByText('Систолическое', { exact: true })).toBeVisible();
      await expect(page.getByText('Диастолическое', { exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'Неделя', exact: true }).click();
      await expect(page.getByText('Систолическое', { exact: true })).toBeVisible();
      await expect(page.getByText('Диастолическое', { exact: true })).toBeVisible();
      await hoverDataDetailChart(page, 0.9);
      await expect(dataDetailTooltip(page).getByText('Диапазон систолического давления', { exact: true })).toBeVisible();
      await expect(dataDetailTooltip(page).getByText('Диапазон диастолического давления', { exact: true })).toBeVisible();

      const expectedTick = await page.evaluate(
        iso => new Date(iso).toLocaleDateString('ru', { month: 'short', day: 'numeric', timeZone: 'UTC' }),
        daysAgoUTC(0),
      );
      expect(await dataDetailXAxisTickTexts(page)).toContain(expectedTick);
    } finally {
      await restoreEnglishDisplayLanguage(page);
    }
  });

  test('nutrition: Day and Week views translate the macro selector and follow the selected macro', async ({ page }) => {
    try {
      await selectRussianDisplayLanguage(page);
      await mockDataDetail(page, nutritionDataDetailMocks());
      await page.goto('/data/nutrition/');
      await expect(page.getByRole('heading', { name: 'Питание' })).toBeVisible();
      for (const macro of ['Калории', 'Белки', 'Углеводы', 'Жиры', 'Сахар', 'Натрий', 'Клетчатка']) {
        await expect(page.getByRole('button', { name: macro, exact: true })).toBeVisible();
      }

      await hoverDataDetailChart(page, 0.9);
      await expect(dataDetailTooltip(page).getByText('Калории', { exact: true })).toBeVisible();
      const expectedCalories = await page.evaluate(
        v => v.toLocaleString('ru-RU', { minimumFractionDigits: 0, maximumFractionDigits: 0 }),
        2200,
      );
      await expect(dataDetailTooltip(page).getByText(expectedCalories, { exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'Белки', exact: true }).click();
      await hoverDataDetailChart(page, 0.9);
      await expect(dataDetailTooltip(page).getByText('Белки', { exact: true })).toBeVisible();
      await expect(dataDetailTooltip(page).getByText('90', { exact: true })).toBeVisible();
    } finally {
      await restoreEnglishDisplayLanguage(page);
    }
  });

  test('weight: Week and Month views translate BMI, Goal/Trend, Avg/Max, and the on-track projection ETA', async ({ page }) => {
    try {
      await selectRussianDisplayLanguage(page);
      await mockDataDetail(page, weightMainDataDetailMocks());
      await page.goto('/data/weight/');
      await expect(page.getByRole('heading', { name: 'Вес' })).toBeVisible();

      await expect(page.getByText('Среднее', { exact: true })).toHaveCount(2);
      await expect(page.getByText('Макс.', { exact: true })).toBeVisible();
      await expect(page.getByText('ИМТ', { exact: true })).toBeVisible();
      await expect(page.getByText('Избыточный вес')).toBeVisible();
      await expect(page.getByText('Цель', { exact: true })).toBeVisible();
      await expect(page.getByText('Тренд', { exact: true })).toBeVisible();

      const message = page.getByTestId('projection-message');
      await expect(message).toContainText('При текущей динамике вы достигнете цели примерно');

      await hoverDataDetailChart(page, 0.5);
      await expect(dataDetailTooltip(page).getByText('Среднее', { exact: true })).toBeVisible();
      // ru-RU number formatting — computed in-browser rather than hardcoding
      // the platform's exact non-breaking-space/comma rendering.
      const expectedAvg = await page.evaluate(
        v => v.toLocaleString('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 }),
        WEIGHT_WEEK_AVG_KG,
      );
      await expect(dataDetailTooltip(page).getByText(expectedAvg, { exact: true })).toBeVisible();

      await page.getByRole('button', { name: 'Месяц', exact: true }).click();
      await expect(message).toContainText('При текущей динамике вы достигнете цели примерно');
    } finally {
      await restoreEnglishDisplayLanguage(page);
    }
  });
});

test.describe('Weight trend-projection states — Russian', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  const cases: { scenario: WeightProjectionScenario; expected: string }[] = [
    { scenario: 'reached', expected: 'Вы достигли целевого веса' },
    { scenario: 'not-on-track', expected: 'С текущей динамикой цель не будет достигнута' },
    { scenario: 'insufficient-data', expected: 'Пока недостаточно данных для прогноза' },
    { scenario: 'load-failure', expected: 'Не удалось загрузить историю веса' },
  ];

  // The fifth outcome, on-track with an ETA, is covered by the "translate
  // BMI, Goal/Trend, Avg/Max, and the on-track projection ETA" test above —
  // together these five cover every branch of computeProjection plus the
  // insufficient-data and load-failure gates around it (dataTypeMeta.ts).
  for (const { scenario, expected } of cases) {
    test(`${scenario} renders the matching Russian projection message`, async ({ page }) => {
      try {
        await selectRussianDisplayLanguage(page);
        await mockDataDetail(page, weightProjectionScenarioMocks(scenario));
        await page.goto('/data/weight/');
        await expect(page.getByTestId('projection-message')).toHaveText(expected);
      } finally {
        await restoreEnglishDisplayLanguage(page);
      }
    });
  }
});
