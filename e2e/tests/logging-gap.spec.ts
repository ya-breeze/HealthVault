import { test, expect, type Page, type APIRequestContext } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
// Like completeness.spec.ts, these tests PUT settings that change `timezone`
// (needed so the fixture dates below line up exactly with what
// resolveLoggingGapWindow computes) — default to the WIP stack rather than
// risking a no-env run landing on prod; override with BASE_URL to target
// something else deliberately.
const BASE_URL = process.env.BASE_URL || 'http://192.168.1.54:8892';

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

async function cookieHeader(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  return cookies.map(c => `${c.name}=${c.value}`).join('; ');
}

async function getSettings(request: APIRequestContext, cookies: string): Promise<Record<string, unknown>> {
  const res = await request.get(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies } });
  return res.json();
}

// Full (non-merging) write, mirroring completeness.spec.ts's putSettings —
// used to force a known timezone baseline before a test and restore the
// account's prior settings afterward.
async function putSettings(request: APIRequestContext, cookies: string, settings: Record<string, unknown>) {
  const res = await request.put(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies }, data: settings });
  if (!res.ok()) throw new Error(`failed to write settings: ${res.status()} ${await res.text()}`);
}

function isoDateOnly(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function addUTCDays(dateStr: string, days: number): string {
  const d = new Date(`${dateStr}T00:00:00.000Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return isoDateOnly(d);
}

// Mirrors frontend/lib/loggingGap.ts's resolveLoggingGapWindow with timezone
// forced to 'UTC' (every test below forces the account's timezone the same
// way first), so the window computed here is exactly what the card itself
// will request.
function loggingGapWindow(): { windowStart: string; windowEnd: string; leadInStart: string } {
  const today = isoDateOnly(new Date());
  const windowEnd = addUTCDays(today, -1);
  const windowStart = addUTCDays(windowEnd, -27);
  const leadInStart = addUTCDays(windowStart, -30);
  return { windowStart, windowEnd, leadInStart };
}

function dateRange(from: string, to: string): string[] {
  const dates: string[] = [];
  let cursor = from;
  for (;;) {
    dates.push(cursor);
    if (cursor === to) break;
    cursor = addUTCDays(cursor, 1);
  }
  return dates;
}

// One weigh-in per calendar day from `leadInStart` to `windowEnd`, changing
// by `dailyDeltaKg` per day — a perfectly linear series so the card's EMA has
// converged to the true slope well before the visible window starts (30 days
// of lead-in at alpha=0.25, matching design.md decision 4).
function buildWeightSeries(leadInStart: string, windowEnd: string, startKg: number, dailyDeltaKg: number) {
  return dateRange(leadInStart, windowEnd).map((date, i) => ({
    time: `${date}T08:00:00.000Z`,
    kilograms: Number((startKg + dailyDeltaKg * i).toFixed(2)),
  }));
}

interface LoggingGapFixture {
  weight?: { time: string; kilograms: number }[];
  weightStatus?: number;
  nutritionTargetCalories?: number;
  nutritionTargetUnmetReason?: string;
  completeness?: { date: string; state: string }[];
  dailyTotals?: { date: string; calories: number; unconfirmed_meals: number }[];
}

// Mocks the four requests LoggingGapCard fetches (task 5.1) so its content
// state is fully deterministic, independent of whatever real weight/food
// history the shared `alice` account happens to hold. Also independent of
// what the account's own dashboard_order/hidden state is, since routing is
// per-page rather than a real backend mutation.
async function mockLoggingGapApis(page: Page, fixture: LoggingGapFixture) {
  await page.route('**/api/data/weight**', route => {
    if (fixture.weightStatus) {
      return route.fulfill({ status: fixture.weightStatus, json: { error: 'boom' } });
    }
    return route.fulfill({ json: fixture.weight ?? [] });
  });
  await page.route('**/api/users/me/nutrition-target', route => {
    if (fixture.nutritionTargetUnmetReason) {
      return route.fulfill({ status: 422, json: { error: fixture.nutritionTargetUnmetReason } });
    }
    return route.fulfill({
      json: {
        calories: fixture.nutritionTargetCalories ?? 2500,
        protein_grams: 150,
        carbs_grams: 250,
        fat_grams: 70,
        measured_weight_kg: 80,
        goal_weight_kg: 75,
        height_m: 1.75,
        age_years: 30,
        sex: 'male',
        activity_multiplier: 1.4,
        activity_tier: 'moderate',
      },
    });
  });
  await page.route('**/api/food/completeness**', route => route.fulfill({ json: fixture.completeness ?? [] }));
  await page.route('**/api/food/daily-totals**', route => route.fulfill({ json: fixture.dailyTotals ?? [] }));
}

// A fixture producing a clearly out-of-interval gap (8.1): steady weight
// loss (0.2 kg/day, well under the 2 kg/day outlier cap) against a low,
// constant logged-calorie history, every window day 'complete' with all of
// its meals confirmed, so the hard floor never fires and Mean Logged Intake
// is well below Implied Intake.
function clearGapFixture(): LoggingGapFixture {
  const { windowStart, windowEnd, leadInStart } = loggingGapWindow();
  const windowDates = dateRange(windowStart, windowEnd);
  return {
    weight: buildWeightSeries(leadInStart, windowEnd, 100, -0.2),
    nutritionTargetCalories: 2500,
    completeness: windowDates.map(date => ({ date, state: 'complete' })),
    dailyTotals: windowDates.map(date => ({ date, calories: 500, unconfirmed_meals: 0 })),
  };
}

// The regression the `unconfirmed_meals` gate exists for: the same steady
// weight loss and the same 'complete' days as clearGapFixture, but every
// day's meals failed vision and so summed to 0 kcal. Before the gate, those
// zeros averaged into Mean Logged Intake and produced a confident ~2500
// kcal/day "not logged" figure out of a food log that simply hadn't
// processed. Every day is now disqualified, so the valid-day floor fires
// instead.
function unconfirmedMealsFixture(): LoggingGapFixture {
  const { windowStart, windowEnd, leadInStart } = loggingGapWindow();
  const windowDates = dateRange(windowStart, windowEnd);
  return {
    weight: buildWeightSeries(leadInStart, windowEnd, 100, -0.2),
    nutritionTargetCalories: 2500,
    completeness: windowDates.map(date => ({ date, state: 'complete' })),
    dailyTotals: windowDates.map(date => ({ date, calories: 0, unconfirmed_meals: 3 })),
  };
}

// A fixture with nothing logged at all (8.2): hard floor's `n < 2` raw
// survivors condition fires immediately, before any regression is attempted.
function noDataFixture(): LoggingGapFixture {
  return { weight: [], completeness: [], dailyTotals: [] };
}

test.describe('Logging Gap Card', () => {
  test('a clear gap renders as a kcal/day range with both caveats, never a bare number', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, clearGapFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card).toBeVisible();
      const value = card.getByTestId('logging-gap-value');
      await expect(value).toBeVisible({ timeout: 15_000 });
      // A range, not a bare point estimate: two numbers joined by an en dash.
      await expect(value).toContainText(/\d+\s*[–-]\s*\d+/);
      await expect(value).toContainText(/kcal\/day/);

      await expect(card).toContainText('Logged intake is estimated from photo recognition');
      await expect(card).toContainText("doesn't separately account for error in your activity multiplier");
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('a freshly-empty account shows "not enough data yet"', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, noDataFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-not-enough-data')).toBeVisible({ timeout: 15_000 });
      await expect(card).toContainText('Not enough data yet');
      await expect(card.getByTestId('logging-gap-value')).toHaveCount(0);
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('Complete days whose meals never reached confirmed show "not enough data yet", not a fabricated gap', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, unconfirmedMealsFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-not-enough-data')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('logging-gap-value')).toHaveCount(0);
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('a user with weight and food history but no goal weight sees the "complete your profile" state', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      // Weight/food history is populated (via the same clear-gap fixture) to
      // match the scenario's framing, but is irrelevant to the outcome here:
      // getNutritionTarget's 422 rejects the whole Promise.all before any of
      // it is inspected (task 5.1).
      await mockLoggingGapApis(page, { ...clearGapFixture(), nutritionTargetUnmetReason: 'missing_goal_weight' });
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      const unmet = card.getByTestId('logging-gap-target-unmet');
      await expect(unmet).toBeVisible({ timeout: 15_000 });
      await expect(unmet).toContainText('Complete your profile and goal weight');
      const link = unmet.getByRole('link', { name: 'Update now' });
      await expect(link).toBeVisible();
      // missing_goal_weight routes to the weight page (goal weight is set from
      // the weight detail page's Add Record form), not the profile settings
      // screen — see LoggingGapCard.tsx's unmetReasonHref.
      await expect(link).toHaveAttribute('href', '/data/weight/');
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('a non-422 failure from one of the four requests shows "temporarily unavailable", not "not enough data yet"', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      // Weight request 500s; the other three would otherwise succeed with a
      // perfectly good clear-gap fixture, proving the failure — not a data
      // shortfall — is what drives the state.
      await mockLoggingGapApis(page, { ...clearGapFixture(), weightStatus: 500 });
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-error')).toBeVisible({ timeout: 15_000 });
      await expect(card).toContainText('Temporarily unavailable');
      await expect(card.getByTestId('logging-gap-not-enough-data')).toHaveCount(0);
    } finally {
      await putSettings(request, cookies, original);
    }
  });
});

// Restores the Logging Gap card to its default state (visible, last among
// the vitals-grid cards — see PRIMARY_METRICS in frontend/lib/vitals.ts) so a
// failed assertion in one test doesn't leak a hidden/reordered card into the
// next. Best-effort throughout, matching dashboard.spec.ts's
// restoreDefaultOrder/restoreAllVisible: this runs from `finally` and
// `beforeEach`, so one broken step must not mask the real failure.
async function restoreLoggingGapDefault(page: Page) {
  const customizeBtn = page.getByRole('button', { name: 'Customize' });
  const doneBtn = page.getByRole('button', { name: 'Done' });
  await expect(customizeBtn.or(doneBtn)).toBeEnabled({ timeout: 10_000 }).catch(() => {});

  const enteredEditMode = !(await doneBtn.isVisible().catch(() => false));
  if (enteredEditMode) {
    if (!(await customizeBtn.isEnabled().catch(() => false))) return;
    await customizeBtn.click({ timeout: 5_000 }).catch(() => {});
    await expect(doneBtn).toBeVisible({ timeout: 5_000 }).catch(() => {});
  }

  let changed = false;
  const showToggle = page.getByRole('button', { name: 'Show Logging Gap' });
  if (await showToggle.isVisible().catch(() => false)) {
    await showToggle.click().catch(() => {});
    changed = true;
  }

  const moveDown = page.getByRole('button', { name: 'Move Logging Gap down' });
  for (let i = 0; i < 9; i++) {
    if (await moveDown.isDisabled().catch(() => true)) break;
    await moveDown.click().catch(() => {});
    changed = true;
  }

  if (!changed && enteredEditMode) {
    await page.goto('/').catch(() => {});
    return;
  }
  if (await doneBtn.isVisible().catch(() => false)) {
    const saved = page
      .waitForResponse(r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT', {
        timeout: 15_000,
      })
      .catch(() => {});
    await doneBtn.click().catch(() => {});
    await saved;
  }
}

test.describe('Logging Gap Card in Edit mode', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    // No timezone dependency here — Edit mode's reorder/visibility
    // mechanics don't care what content state the card is showing, so this
    // describe block leaves the account's real timezone (and real weight/
    // food history) untouched.
    await mockLoggingGapApis(page, noDataFixture());
    await page.goto('/');
    await restoreLoggingGapDefault(page);
  });

  test('hiding the card removes it from the read-only grid and persists across reload', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');
    try {
      await expect(grid.getByTestId('logging-gap-card')).toBeVisible();

      await page.getByRole('button', { name: 'Customize' }).click();
      await page.getByRole('button', { name: 'Hide Logging Gap' }).click();

      const saved = page.waitForResponse(
        r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
        { timeout: 15_000 }
      );
      await page.getByRole('button', { name: 'Done' }).click();
      await saved;

      await expect(grid.getByTestId('logging-gap-card')).toHaveCount(0);

      await page.reload();
      await expect(page.getByTestId('vitals-grid').getByTestId('logging-gap-card')).toHaveCount(0);
    } finally {
      await restoreLoggingGapDefault(page);
    }
  });

  test('moving the card to the front persists across reload, alongside the other vital cards', async ({ page }) => {
    const grid = page.getByTestId('vitals-grid');
    try {
      await page.getByRole('button', { name: 'Customize' }).click();

      const moveUp = page.getByRole('button', { name: 'Move Logging Gap up' });
      for (let i = 0; i < 9; i++) {
        if (await moveUp.isDisabled()) break;
        await moveUp.click();
      }
      await expect(moveUp).toBeDisabled();

      const saved = page.waitForResponse(
        r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
        { timeout: 15_000 }
      );
      await page.getByRole('button', { name: 'Done' }).click();
      await saved;

      await expect(grid.locator('> *').first()).toHaveAttribute('data-testid', 'logging-gap-card');

      await page.reload();
      await expect(page.getByTestId('vitals-grid').locator('> *').first()).toHaveAttribute(
        'data-testid',
        'logging-gap-card'
      );
      // The other vital cards are still all present alongside it — reordering
      // one card must not have dropped any other.
      await expect(page.getByTestId('vital-card-weight')).toBeVisible();
      await expect(page.getByTestId('vital-card-steps')).toBeVisible();
    } finally {
      await restoreLoggingGapDefault(page);
    }
  });
});
