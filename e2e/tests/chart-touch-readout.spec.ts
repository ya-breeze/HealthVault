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

// Playwright leaves its virtual mouse wherever the last click landed, and
// login()'s sign-in button sits at roughly the same place a 390px-wide data
// detail page later draws its chart. Chromium fires a hover mousemove when new
// content renders under a stationary cursor, so the tooltip can already be open
// before a test touches anything — which would let every tap case below pass
// with the production change reverted. Park the mouse off the chart and prove
// the tooltip is closed, so whatever the touch then opens is the touch's doing.
async function parkMouseAwayFromChart(page: Page) {
  await page.mouse.move(0, 0);
  await expect(tooltip(page)).toBeHidden();
}

// Chromium answers a CDP tap with compatibility mouse events — an instrumented
// run records mouseover/mouseenter/mousemove/mousedown/mouseup/click arriving
// right behind touchstart/touchmove/touchend — and that mousemove alone opens
// the tooltip. Every tap assertion below would therefore pass with the
// production seed reverted, which is the vacuous case task 4 rules out.
// Swallowing mouse events at document capture stops them before React's root
// listener (and so Recharts) ever sees them, leaving the touch path as the only
// thing that can open the tooltip. Install it after parkMouseAwayFromChart,
// which needs a real mouseleave to get through.
//
// This is necessary but not sufficient: a tap also focuses the chart's <svg>,
// and Recharts' accessibility layer opens the tooltip at index 0 on focus with
// no pointer involved at all. That is why every case below mocks several points
// and asserts the one under the finger while asserting index 0's value is
// absent — "a tooltip appeared" on its own proves nothing.
async function isolateTouchFromCompatibilityMouseEvents(page: Page) {
  await page.evaluate(() => {
    const types = ['mouseover', 'mouseout', 'mouseenter', 'mouseleave', 'mousemove', 'mousedown', 'mouseup', 'click'];
    for (const type of types) {
      document.addEventListener(type, e => e.stopPropagation(), true);
    }
  });
}

// A point at `fractionX` across the plot area, vertically centred. Taken from
// the cartesian grid rather than the chart surface for two reasons: the
// surface's leftmost ~65px are the Y axis, which Recharts' isInCartesianRange
// excludes, and a fraction of the plot area maps directly onto the series, so
// a case can say which data point it means to touch.
async function plotPoint(page: Page, fractionX: number): Promise<{ x: number; y: number }> {
  const grid = await page.locator('.recharts-cartesian-grid').first().boundingBox();
  expect(grid, 'the plot area should have a bounding box').not.toBeNull();
  return { x: grid!.x + grid!.width * fractionX, y: grid!.y + grid!.height / 2 };
}

// Dispatches one real touch event using the browser's own Touch/TouchEvent
// constructors, per task 4's requirement — Playwright's page.touchscreen
// only offers a single tap, not a multi-step drag with readings in between.
//
// The event target is resolved via elementFromPoint at (x, y), not the
// chart-surface div itself: React only invokes a handler for a real native
// event if the node it's attached to lies on the path from the event's
// actual target up to the document root. RechartsWrapper's own touchmove
// handler sits on a descendant of chart-surface (recharts-wrapper, inside
// ResponsiveContainer), so an event whose target IS chart-surface bubbles
// *up* past it and never reaches that inner listener — exactly like a real
// touch, whose target is whatever's under the finger, not the outer div.
async function touchDispatch(
  page: Page, testId: string, type: 'touchstart' | 'touchmove' | 'touchend', x: number, y: number
) {
  await page.evaluate(
    ({ testId, type, x, y }) => {
      const surface = document.querySelector(`[data-testid="${testId}"]`);
      if (!surface) throw new Error(`no element with data-testid="${testId}"`);
      const el = document.elementFromPoint(x, y);
      if (!el || !surface.contains(el)) {
        throw new Error(`(${x}, ${y}) is not inside the chart surface`);
      }
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

  // The Day-zoom X axis is a time scale spanning `from`..`to` (now minus 24h to
  // now), so a record 2h old sits at ~92% of the plot and one 20h old at ~17%.
  const DAY_ZOOM_EARLY_HOURS = 20;
  const DAY_ZOOM_LATE_HOURS = 2;

  test('weight at Day zoom: a tap shows the value under the finger', async ({ page }) => {
    await login(page);
    await mockData(page, {
      weight: {
        raw: [
          { id: 'w1', kilograms: 80.1, time: isoHoursAgo(DAY_ZOOM_EARLY_HOURS) },
          { id: 'w2', kilograms: 84.7, time: isoHoursAgo(DAY_ZOOM_LATE_HOURS) },
        ],
      },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    const { x, y } = await plotPoint(page, 0.9);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('84.7', { exact: true })).toBeVisible();
    await expect(box.getByText('80.1', { exact: true })).toHaveCount(0);
  });

  test('blood_pressure at Day zoom: a tap shows systolic/diastolic', async ({ page }) => {
    await login(page);
    await mockData(page, {
      blood_pressure: {
        raw: [
          { id: 'bp1', systolic: 104, diastolic: 62, time: isoHoursAgo(DAY_ZOOM_EARLY_HOURS) },
          { id: 'bp2', systolic: 118, diastolic: 76, time: isoHoursAgo(DAY_ZOOM_LATE_HOURS) },
        ],
      },
    });

    await page.goto('/data/blood_pressure/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    const { x, y } = await plotPoint(page, 0.9);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('118', { exact: true })).toBeVisible();
    await expect(box.getByText('76', { exact: true })).toBeVisible();
    await expect(box.getByText('104', { exact: true })).toHaveCount(0);
  });

  test('blood_pressure at Week zoom: a tap shows the bucket average', async ({ page }) => {
    await login(page);
    await mockData(page, {
      blood_pressure: {
        bucketDay: [2, 1, 0].map((daysAgo, i) => ({
          bucket_start: isoDateDaysAgo(daysAgo),
          systolic_avg: 110 + i * 6, systolic_min: 106 + i * 6, systolic_max: 114 + i * 6,
          diastolic_avg: 71 + i * 4, diastolic_min: 67 + i * 4, diastolic_max: 75 + i * 4,
        })),
      },
    });

    await page.goto('/data/blood_pressure/');
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    const { x, y } = await plotPoint(page, 0.9);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('122', { exact: true })).toBeVisible();
    await expect(box.getByText('110', { exact: true })).toHaveCount(0);
  });

  test('a cumulative type (steps) at Week zoom: a tap shows the bucket sum', async ({ page }) => {
    await login(page);
    await mockData(page, {
      steps: {
        bucketDay: [2, 1, 0].map((daysAgo, i) => ({
          bucket_start: isoDateDaysAgo(daysAgo), sum: 1200 + i * 1650,
        })),
      },
    });

    await page.goto('/data/steps/');
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    const { x, y } = await plotPoint(page, 0.9);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    // 4500 with zero decimals still carries en-US grouping — matches
    // formatMetricValue's toLocaleString call, not a bare String(4500).
    await expect(box.getByText('4,500', { exact: true })).toBeVisible();
    await expect(box.getByText('1,200', { exact: true })).toHaveCount(0);
  });

  test('weight at Week zoom: a tap shows the min-max band, Avg and Trend', async ({ page }) => {
    await login(page);
    await mockData(page, {
      weight: {
        bucketDay: [
          { bucket_start: isoDateDaysAgo(2), avg: 74.2, min: 73.0, max: 75.0 },
          { bucket_start: isoDateDaysAgo(1), avg: 76.8, min: 75.5, max: 78.0 },
          { bucket_start: isoDateDaysAgo(0), avg: 79.4, min: 78.0, max: 81.0 },
        ],
      },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    const { x, y } = await plotPoint(page, 0.9);
    await page.touchscreen.tap(x, y);

    const box = tooltip(page);
    await expect(box).toBeVisible();
    await expect(box.getByText('Avg', { exact: true })).toBeVisible();
    await expect(box.getByText('79.4', { exact: true })).toBeVisible();
    await expect(box.getByText(EN_DASH_RANGE('78.0', '81.0'), { exact: true })).toBeVisible();
    await expect(box.getByText('Trend', { exact: true })).toBeVisible();
    // The first bucket's avg — proves the readout followed the finger rather
    // than landing on index 0, which is where a focus-driven tooltip opens.
    await expect(box.getByText('74.2', { exact: true })).toHaveCount(0);
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
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    // The ~11 synthetic points trail the ~60 real buckets, so the far right
    // of a categorical x-axis lands on one of them. Index 0 carries no
    // Projection entry, so this also fails if the tooltip opens on focus
    // rather than following the finger.
    const { x, y } = await plotPoint(page, 0.97);
    await page.touchscreen.tap(x, y);

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
    await expect(surface).toBeVisible();
    // Positions come from the plot area, not the surface: 10% of the surface
    // is still inside the Y axis, which isInCartesianRange excludes, so a
    // scrub started there would open no tooltip at all.
    const { x: leftX, y } = await plotPoint(page, 0.05);
    const { x: midX } = await plotPoint(page, 0.5);
    const { x: rightX } = await plotPoint(page, 0.95);

    const label = tooltip(page).locator('.recharts-tooltip-label');

    await parkMouseAwayFromChart(page);
    await touchDispatch(page, 'chart-surface', 'touchstart', leftX, y);
    await expect(label).toBeVisible();
    const startLabel = await label.textContent();

    await touchDispatch(page, 'chart-surface', 'touchmove', midX, y);
    await touchDispatch(page, 'chart-surface', 'touchmove', rightX, y);
    // touchEventsMiddleware batches touchmove handling behind
    // requestAnimationFrame, so the label update lands a tick after the
    // dispatch call returns — an assertion that retries (not a one-shot
    // textContent() read) is what actually waits for it.
    await expect(label, 'label should change as the finger moves across the plot').not.toHaveText(startLabel ?? '');
    const movedLabel = await label.textContent();

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
    await parkMouseAwayFromChart(page);
    await isolateTouchFromCompatibilityMouseEvents(page);

    const touchY = box!.y + box!.height * 0.7;
    const { x: touchX } = await plotPoint(page, 0.5);
    await page.touchscreen.tap(touchX, touchY);

    // Recharts renders the tooltip wrapper from mount and only toggles its
    // `visibility`, laying it out at the chart's top-left corner until a
    // readout opens. Its bounding box is therefore non-null and already above
    // `touchY - 40` when nothing was read at all, so measuring it straight
    // after the tap would assert nothing. Waiting for it to be visible and to
    // carry the mocked value is what makes the check below a check on
    // `position={{ y: 0 }}` rather than on an unopened tooltip.
    const readout = tooltip(page);
    await expect(readout).toBeVisible();
    await expect(readout.getByText('82.4', { exact: true })).toBeVisible();

    const tooltipBox = await readout.boundingBox();
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
      weight: {
        raw: [
          { id: 'w1', kilograms: 80.1, time: isoHoursAgo(20) },
          { id: 'w2', kilograms: 84.7, time: isoHoursAgo(2) },
        ],
      },
      weight_goal: { raw: [] },
      height: { raw: [] },
    });

    await page.goto('/data/weight/');
    await page.getByRole('button', { name: 'Day', exact: true }).click();
    const surface = chartSurface(page);
    await expect(surface).toBeVisible();

    await parkMouseAwayFromChart(page);

    const { x, y } = await plotPoint(page, 0.9);
    await page.mouse.move(x, y);

    await expect(tooltip(page)).toBeVisible();
    await expect(tooltip(page).getByText('84.7', { exact: true })).toBeVisible();
    await expect(tooltip(page).getByText('80.1', { exact: true })).toHaveCount(0);
  });
});
