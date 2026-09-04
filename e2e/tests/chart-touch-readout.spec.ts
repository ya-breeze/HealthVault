import { test, expect, type Page, type Locator } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';

const MOBILE_VIEWPORT = { width: 390, height: 844 };
// DefaultTooltipContent's formatter tuple renders as `[value, name]` — see
// formatTooltipValue in DataTypeClient.tsx, which joins a min-max band with
// this exact separator (an en dash, padded with spaces on both sides).
const EN_DASH_RANGE = (min: string, max: string) => `${min} – ${max}`;

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

type MockRow = Record<string, unknown>;
interface TypeMock {
  raw?: MockRow[];
  bucketDay?: MockRow[];
  bucketMonth?: MockRow[];
}

// Routes every `/api/data/<type>` read the data detail page makes, branching
// on the `bucket` query param the same way DataTypeClient's own fetches do —
// see mockDataRecords in mobile-tap-targets.spec.ts for the one-bucket-only
// precedent this generalizes to raw + day + month.
async function mockData(page: Page, mocks: Record<string, TypeMock>) {
  await page.route('**/api/data/**', route => {
    const url = new URL(route.request().url());
    const type = url.pathname.split('/').filter(Boolean).pop() ?? '';
    const bucket = url.searchParams.get('bucket');
    const mock = mocks[type];
    if (!mock) return route.fulfill({ json: [] });
    const rows = bucket === 'day' ? (mock.bucketDay ?? [])
      : bucket === 'month' ? (mock.bucketMonth ?? [])
      : (mock.raw ?? []);
    return route.fulfill({ json: rows });
  });
}

function isoHoursAgo(hours: number): string {
  return new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();
}

// UTC-midnight bucket_start, matching bucketLabel's read-back convention
// (DataTypeClient.tsx's comment on bucketLabel).
function isoDateDaysAgo(daysAgo: number): string {
  const d = new Date();
  d.setUTCHours(0, 0, 0, 0);
  d.setUTCDate(d.getUTCDate() - daysAgo);
  return d.toISOString();
}

function chartSurface(page: Page): Locator {
  return page.getByTestId('chart-surface');
}

function tooltip(page: Page): Locator {
  return page.locator('.recharts-tooltip-wrapper');
}

async function centerOf(locator: Locator): Promise<{ x: number; y: number }> {
  const box = await locator.boundingBox();
  expect(box, 'element should have a bounding box').not.toBeNull();
  return { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 };
}

// Dispatches one real touch event using the browser's own Touch/TouchEvent
// constructors, per task 4's requirement — Playwright's page.touchscreen
// only offers a single tap, not a multi-step drag with readings in between.
async function touchDispatch(
  page: Page, testId: string, type: 'touchstart' | 'touchmove' | 'touchend', x: number, y: number
) {
  await page.evaluate(
    ({ testId, type, x, y }) => {
      const el = document.querySelector(`[data-testid="${testId}"]`);
      if (!el) throw new Error(`no element with data-testid="${testId}"`);
      const touch = new Touch({ identifier: 1, target: el, clientX: x, clientY: y, pageX: x, pageY: y });
      const touches = type === 'touchend' ? [] : [touch];
      el.dispatchEvent(new TouchEvent(type, {
        bubbles: true, cancelable: true, touches, targetTouches: touches, changedTouches: [touch],
      }));
    },
    { testId, type, x, y }
  );
}

test.describe('Chart touch readout — mobile', () => {
  // hasTouch matters as much as the viewport here: without it Chromium's
  // Desktop Chrome project reports a fine pointer, so useCoarsePointer would
  // never flip true and position={{ y: 0 }} would never apply — see the
  // same reasoning in mobile-tap-targets.spec.ts.
  test.use({ viewport: MOBILE_VIEWPORT, hasTouch: true });

  test('weight at Day zoom: a tap shows the value under the finger', async ({ page }) => {
    await login(page);
    await mockData(page, {
      weight: { raw: [{ id: 'w1', kilograms: 82.4, time: isoHoursAgo(2) }] },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const { x, y } = await centerOf(surface);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('82.4', { exact: true })).toBeVisible();
  });

  test('blood_pressure at Day zoom: a tap shows systolic/diastolic', async ({ page }) => {
    await login(page);
    await mockData(page, {
      blood_pressure: { raw: [{ id: 'bp1', systolic: 118, diastolic: 76, time: isoHoursAgo(2) }] },
    });

    await page.goto('/data/blood_pressure/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const { x, y } = await centerOf(surface);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('118', { exact: true })).toBeVisible();
    await expect(box.getByText('76', { exact: true })).toBeVisible();
  });

  test('blood_pressure at Week zoom: a tap shows the bucket average', async ({ page }) => {
    await login(page);
    await mockData(page, {
      blood_pressure: {
        bucketDay: [{
          bucket_start: isoDateDaysAgo(2),
          systolic_avg: 122, systolic_min: 118, systolic_max: 126,
          diastolic_avg: 79, diastolic_min: 75, diastolic_max: 83,
        }],
      },
    });

    await page.goto('/data/blood_pressure/');
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const { x, y } = await centerOf(surface);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('122', { exact: true })).toBeVisible();
  });

  test('a cumulative type (steps) at Week zoom: a tap shows the bucket sum', async ({ page }) => {
    await login(page);
    await mockData(page, {
      steps: { bucketDay: [{ bucket_start: isoDateDaysAgo(2), sum: 4500 }] },
    });

    await page.goto('/data/steps/');
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const { x, y } = await centerOf(surface);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    // 4500 with zero decimals still carries en-US grouping — matches
    // formatMetricValue's toLocaleString call, not a bare String(4500).
    await expect(box.getByText('4,500', { exact: true })).toBeVisible();
  });

  test('weight at Week zoom: a tap shows the min-max band, Avg and Trend', async ({ page }) => {
    await login(page);
    await mockData(page, {
      weight: { bucketDay: [{ bucket_start: isoDateDaysAgo(2), avg: 79.4, min: 78.0, max: 81.0 }] },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const { x, y } = await centerOf(surface);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('Avg', { exact: true })).toBeVisible();
    await expect(box.getByText('79.4', { exact: true })).toBeVisible();
    await expect(box.getByText(EN_DASH_RANGE('78.0', '81.0'), { exact: true })).toBeVisible();
    await expect(box.getByText('Trend', { exact: true })).toBeVisible();
  });

  test('weight at Month zoom: the projection series is readable on a synthetic future bucket', async ({ page }) => {
    await login(page);
    // 60 daily buckets declining ~0.3kg/day — verified against
    // computeProjection (frontend/lib/dataTypeMeta.ts) to land status
    // 'on-track' with a ~44-day-out crossing, comfortably inside the
    // 365-day horizon and past hasEnoughDataForProjection's gates (60
    // records, 59-day lifetime span, a 30-point/29-day regression window).
    const days = 60;
    const bucketDay = Array.from({ length: days }, (_, i) => {
      const daysAgo = days - 1 - i;
      const kg = 100 - i * 0.3;
      return { bucket_start: isoDateDaysAgo(daysAgo), avg: kg, min: kg - 0.5, max: kg + 0.5 };
    });
    const raw = bucketDay.map((r, i) => ({ id: `w${i}`, kilograms: r.avg, time: r.bucket_start }));

    await mockData(page, {
      weight: { raw, bucketDay },
      weight_goal: { raw: [{ id: 'goal1', kilograms: 70, time: isoDateDaysAgo(0) }] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Month', exact: true }).click();
    await expect(page.getByTestId('projection-message')).toBeVisible();
    const surface = chartSurface(page);
    const box = await surface.boundingBox();
    expect(box, 'chart surface should have a bounding box').not.toBeNull();

    // The ~11 synthetic points trail the ~60 real buckets, so the far right
    // of a categorical x-axis lands on one of them.
    await page.touchscreen.tap(box!.x + box!.width * 0.96, box!.y + box!.height / 2);

    await expect(tooltip(page).getByText('Projection', { exact: true })).toBeVisible();
  });

  test('a scrub across the plot moves the readout, which survives lifting the finger', async ({ page }) => {
    await login(page);
    const rows = [4, 3, 2, 1, 0].map(daysAgo => ({
      bucket_start: isoDateDaysAgo(daysAgo),
      avg: 80 - daysAgo * 0.4, min: 79, max: 81,
    }));
    await mockData(page, {
      weight: { bucketDay: rows },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    const surface = chartSurface(page);
    const box = await surface.boundingBox();
    expect(box, 'chart surface should have a bounding box').not.toBeNull();
    const y = box!.y + box!.height / 2;
    const leftX = box!.x + box!.width * 0.1;
    const midX = box!.x + box!.width * 0.5;
    const rightX = box!.x + box!.width * 0.9;

    const label = tooltip(page).locator('.recharts-tooltip-label');

    await touchDispatch(page, 'chart-surface', 'touchstart', leftX, y);
    await expect(label).toBeVisible();
    const startLabel = await label.textContent();

    await touchDispatch(page, 'chart-surface', 'touchmove', midX, y);
    await touchDispatch(page, 'chart-surface', 'touchmove', rightX, y);
    const movedLabel = await label.textContent();
    expect(movedLabel, 'label should change as the finger moves across the plot').not.toBe(startLabel);

    // Sticky after lift: the label from the last touchmove stays on screen
    // once the finger is lifted — touch has no mouseleave to clear it.
    await touchDispatch(page, 'chart-surface', 'touchend', rightX, y);
    await expect(label).toHaveText(movedLabel ?? '');
  });

  test('the chart surface computes touch-action: pan-y, and the tooltip sits above the touch point on a coarse pointer', async ({ page }) => {
    await login(page);
    await mockData(page, {
      weight: { raw: [{ id: 'w1', kilograms: 82.4, time: isoHoursAgo(2) }] },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const touchAction = await surface.evaluate(el => getComputedStyle(el).touchAction);
    expect(touchAction).toBe('pan-y');

    const box = await surface.boundingBox();
    expect(box, 'chart surface should have a bounding box').not.toBeNull();
    const touchY = box!.y + box!.height * 0.7;
    await page.touchscreen.tap(box!.x + box!.width / 2, touchY);

    const tooltipBox = await tooltip(page).boundingBox();
    expect(tooltipBox, 'tooltip should have a bounding box').not.toBeNull();
    expect(
      tooltipBox!.y,
      'tooltip should sit near the top of the plot area, not under the touch point'
    ).toBeLessThan(touchY - 40);
  });
});

test.describe('Chart touch readout — mouse regression', () => {
  test.use({ viewport: { width: 1280, height: 900 }, hasTouch: false });

  test('a mouse hover still shows the tooltip', async ({ page }) => {
    await login(page);
    await mockData(page, {
      weight: { raw: [{ id: 'w1', kilograms: 82.4, time: isoHoursAgo(2) }] },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    const { x, y } = await centerOf(surface);
    await page.mouse.move(x, y);

    await expect(tooltip(page)).toBeVisible();
    await expect(tooltip(page).getByText('82.4', { exact: true })).toBeVisible();
  });
});
