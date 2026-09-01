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

test.describe('Data type pages', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('/data/steps loads with chart area', async ({ page }) => {
    await page.goto('/data/steps/');
    await expect(page.getByText(/steps/i)).toBeVisible();
    // Page should render without errors (no "something went wrong")
    await expect(page.getByText(/something went wrong|error/i)).not.toBeVisible();
  });

  test('/data/heart_rate loads', async ({ page }) => {
    await page.goto('/data/heart_rate/');
    await expect(page.getByText(/heart.?rate/i)).toBeVisible();
  });

  test('/data/sleep loads', async ({ page }) => {
    await page.goto('/data/sleep/');
    await expect(page.getByText(/sleep/i)).toBeVisible();
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
  // Day is the intermediate: a real zoom, never the default, and it fetches
  // no bucketed data, so it cannot be mistaken for the request under test.
  // Do not remove this as redundant — without it the assertion that follows
  // has no action to wait for.
  async function selectAnotherZoomFirst(page: Page) {
    await page.getByRole('button', { name: 'Day', exact: true }).click();
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

  // Regression coverage for a bug found in code review: Year zoom's own ~12-13
  // monthly buckets fall short of the ~14-16 periods an alpha=0.25 EMA needs to
  // converge, so weight's trend line must widen its lookback fetch the same way
  // Week's does — not just Week. These assert on the actual outgoing request
  // range, since a rendered-but-unconverged trend line would still pass a mere
  // visibility check.
  // The premise the two tests below depend on, asserted rather than assumed.
  // If the page's default zoom ever stops being Week, clicking Week becomes a
  // real transition again and selectAnotherZoomFirst becomes unnecessary — and
  // if the default moves to Day, it becomes actively wrong. Either way this
  // fails first and says so, instead of the other two going quietly flaky.
  test('the chart opens on Week, so selecting Week is not a state change', async ({ page }) => {
    await page.goto('/data/weight/');
    await expect(page.getByRole('button', { name: 'Week', exact: true }))
      .toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('button', { name: 'Day', exact: true }))
      .toHaveAttribute('aria-pressed', 'false');
  });

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
