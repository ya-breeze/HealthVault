import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
// These tests PUT settings that change `timezone`, needed so the fixture dates
// below line up exactly with what resolveLoggingGapWindow computes — one of the
// reasons `target.ts` refuses to resolve a prod URL at all.
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
  summaryStatus?: number;
  nutritionTargetCalories?: number;
  // Defaults low enough (see mockLoggingGapApis) that every fixture predating
  // the sustainability warning — none of which think about BMR at all — keeps
  // its existing assertions unchanged: their logged-intake numbers all clear
  // a BMR this low with room to spare.
  nutritionTargetBmr?: number;
  // The derivation fields the top row's ⓘ disclosure reads (docs/specs/idea.md
  // task 6), given fixed defaults here so most fixtures — which only care
  // about the gap line — don't have to spell them out.
  nutritionTargetMeasuredWeightKg?: number;
  nutritionTargetGoalWeightKg?: number;
  nutritionTargetHeightM?: number;
  nutritionTargetAgeYears?: number;
  nutritionTargetSex?: 'male' | 'female';
  nutritionTargetActivityMultiplier?: number;
  nutritionTargetActivityTier?: string;
  nutritionTargetUnmetReason?: string;
  // Today's consumed totals, for the card's top row. Defaults to an untouched
  // day (all zeros), which is what most of these fixtures want — they exist to
  // pin the *gap* line, and a zeroed top row keeps them from also depending on
  // numbers they don't care about.
  today?: { calories: number; protein: number; carbs: number; fat: number };
  completeness?: { date: string; state: string }[];
  dailyTotals?: { date: string; calories: number; unconfirmed_meals: number }[];
}

// Mocks the four requests LoggingGapCard fetches (task 5.1) so its content
// state is fully deterministic, independent of whatever real weight/food
// history the shared `alice` account happens to hold. Also independent of
// what the account's own dashboard_order/hidden state is, since routing is
// per-page rather than a real backend mutation.
//
// `opts.liveDailyTotals` leaves /api/food/daily-totals unrouted so it reaches
// the real backend — see the contract test below for why one test must.
async function mockLoggingGapApis(page: Page, fixture: LoggingGapFixture, opts?: { liveDailyTotals?: boolean }) {
  await page.route('**/api/data/weight**', route => {
    if (fixture.weightStatus) {
      return route.fulfill({ status: fixture.weightStatus, json: { error: 'boom' } });
    }
    return route.fulfill({ json: fixture.weight ?? [] });
  });
  // /api/summary/today, not /api/users/me/nutrition-target: the card reads
  // today's consumed macros and the Nutrition Target from this one response.
  // Note it never answers 422 — an unavailable target is ordinary data here,
  // carried as `target.available === false`.
  await page.route('**/api/summary/today', route => {
    if (fixture.summaryStatus) {
      return route.fulfill({ status: fixture.summaryStatus, json: { error: 'boom' } });
    }
    const target = fixture.nutritionTargetUnmetReason
      ? { available: false, reason: fixture.nutritionTargetUnmetReason }
      : {
          available: true,
          calories: fixture.nutritionTargetCalories ?? 2500,
          protein_grams: 150,
          carbs_grams: 250,
          fat_grams: 70,
          bmr: fixture.nutritionTargetBmr ?? 1000,
          measured_weight_kg: fixture.nutritionTargetMeasuredWeightKg ?? 80,
          goal_weight_kg: fixture.nutritionTargetGoalWeightKg ?? 75,
          height_m: fixture.nutritionTargetHeightM ?? 1.8,
          age_years: fixture.nutritionTargetAgeYears ?? 35,
          sex: fixture.nutritionTargetSex ?? 'male',
          activity_multiplier: fixture.nutritionTargetActivityMultiplier ?? 1.55,
          activity_tier: fixture.nutritionTargetActivityTier ?? 'Moderately active',
        };
    return route.fulfill({
      json: {
        date: isoDateOnly(new Date()),
        calories_consumed: fixture.today?.calories ?? 0,
        protein_grams_consumed: fixture.today?.protein ?? 0,
        carbs_grams_consumed: fixture.today?.carbs ?? 0,
        fat_grams_consumed: fixture.today?.fat ?? 0,
        meal_count: 0,
        last_logged_at: null,
        display_language: 'en',
        target,
        recommendation: null,
      },
    });
  });
  await page.route('**/api/food/completeness**', route => route.fulfill({ json: fixture.completeness ?? [] }));
  if (!opts?.liveDailyTotals) {
    await page.route('**/api/food/daily-totals**', route => route.fulfill({ json: fixture.dailyTotals ?? [] }));
  }
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

// The case this change exists for: plenty of data, and it agrees. Weight falls
// 0.1 kg/day against a 2500 kcal target, so Implied Intake is
// 2500 - 0.1 * 7700 = 1730; the log says 1700, a difference of 30 inside an
// interval of ~250 (10% of the target, the trend term being ~0 for a perfectly
// linear series). That lands on `on_track` — not on "not enough data yet",
// which is what this fixture produced before.
function onTrackFixture(): LoggingGapFixture {
  const { windowStart, windowEnd, leadInStart } = loggingGapWindow();
  const windowDates = dateRange(windowStart, windowEnd);
  return {
    weight: buildWeightSeries(leadInStart, windowEnd, 100, -0.1),
    nutritionTargetCalories: 2500,
    today: { calories: 1200, protein: 80, carbs: 130, fat: 35 },
    completeness: windowDates.map(date => ({ date, state: 'complete' })),
    dailyTotals: windowDates.map(date => ({ date, calories: 1700, unconfirmed_meals: 0 })),
  };
}

// A weight series losing about 1.4%/week (sustainability.ts's
// MAX_SUSTAINABLE_LOSS_PCT_PER_WEEK is 1.0), well clear of the band even on
// the conservative slope+se bound: the series is built linear end-to-end, so
// the EMA has converged and the regression's own residuals — and therefore
// its standard error — are close to zero. `startKg` is chosen so weight is
// close to 100kg right at the window's end, keeping the percentage close to
// the 1.4% the -0.2 kg/day rate implies there. The logged-calorie history
// (500 kcal/day against a 2500 target) is the same as clearGapFixture's, so
// the logging gap itself reports `gap`, not `on_track` — this fixture is
// about the rate check firing alone, not about the two warnings interacting.
function tooFastLossFixture(): LoggingGapFixture {
  const { windowStart, windowEnd, leadInStart } = loggingGapWindow();
  const windowDates = dateRange(windowStart, windowEnd);
  const dailyDeltaKg = -0.2;
  const totalDays = dateRange(leadInStart, windowEnd).length - 1;
  const startKg = 100 - dailyDeltaKg * totalDays;
  return {
    weight: buildWeightSeries(leadInStart, windowEnd, startKg, dailyDeltaKg),
    nutritionTargetCalories: 2500,
    completeness: windowDates.map(date => ({ date, state: 'complete' })),
    dailyTotals: windowDates.map(date => ({ date, calories: 500, unconfirmed_meals: 0 })),
  };
}

// on_track numbers (same weight trend and logged intake as onTrackFixture,
// so the gap line reads on_track just as it does there) whose mean logged
// intake, 1700 kcal, sits clearly under a mocked BMR of 2000 — a shortfall
// past the 5% margin (threshold 1900). The loss rate stays inside the band
// (0.7%/week), so only the below-BMR line should appear.
function belowBmrFixture(): LoggingGapFixture {
  const { windowStart, windowEnd, leadInStart } = loggingGapWindow();
  const windowDates = dateRange(windowStart, windowEnd);
  return {
    weight: buildWeightSeries(leadInStart, windowEnd, 100, -0.1),
    nutritionTargetCalories: 2500,
    nutritionTargetBmr: 2000,
    completeness: windowDates.map(date => ({ date, state: 'complete' })),
    dailyTotals: windowDates.map(date => ({ date, calories: 1700, unconfirmed_meals: 0 })),
  };
}

// The gating regression (design.md's "The interaction that makes this worth
// a spec rather than an afternoon"): the same BMR (2000) and a mean logged
// intake (1200) that would clear the below-BMR margin on its own — but the
// weight trend implies 1730, a difference of 530 outside the interval, so the
// logging gap itself reports `gap`, not `on_track`. The intake check must
// stay silent because it cannot vouch for self-reported food the weight
// trend disagrees with this much.
function belowBmrGatedOffFixture(): LoggingGapFixture {
  const { windowStart, windowEnd, leadInStart } = loggingGapWindow();
  const windowDates = dateRange(windowStart, windowEnd);
  return {
    weight: buildWeightSeries(leadInStart, windowEnd, 100, -0.1),
    nutritionTargetCalories: 2500,
    nutritionTargetBmr: 2000,
    completeness: windowDates.map(date => ({ date, state: 'complete' })),
    dailyTotals: windowDates.map(date => ({ date, calories: 1200, unconfirmed_meals: 0 })),
  };
}

test.describe('Logging Gap Card', () => {
  test('a clear gap renders as a kcal/day range with both caveats behind the hint, never a bare number', async ({ page, request }) => {
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

      // The caveats are still on the card, but reaching them is now the
      // reader's choice — the default view must not show them.
      const hint = card.getByTestId('logging-gap-hint');
      const toggle = card.getByTestId('logging-gap-hint-toggle');
      await expect(hint).toBeHidden();
      await expect(toggle).toHaveAttribute('aria-expanded', 'false');

      await toggle.click();
      await expect(hint).toBeVisible();
      await expect(toggle).toHaveAttribute('aria-expanded', 'true');
      await expect(hint).toContainText('Logged intake is estimated from photo recognition');
      await expect(hint).toContainText("doesn't separately account for error in your activity multiplier");
      // The on-track sentence explains a state this card is not in.
      await expect(hint).not.toContainText('intake and weight trend agree');
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('a log that agrees with the weight trend reads as on track, not as missing data', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, onTrackFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-on-track')).toBeVisible({ timeout: 15_000 });
      await expect(card).toContainText('Your log matches your weight');
      // The distinction this state exists to draw: neither the "not enough
      // data" copy nor a gap figure may appear alongside it.
      await expect(card.getByTestId('logging-gap-not-enough-data')).toHaveCount(0);
      await expect(card.getByTestId('logging-gap-value')).toHaveCount(0);

      // The sentence that explains this state is a hint, not part of the row:
      // hidden until asked for, and first in the panel when it is.
      const hint = card.getByTestId('logging-gap-hint');
      await expect(hint).toBeHidden();
      await card.getByTestId('logging-gap-hint-toggle').click();
      await expect(hint).toBeVisible();
      await expect(hint).toContainText('intake and weight trend agree');
      await expect(hint).toContainText('Logged intake is estimated from photo recognition');
      // on_track is a real comparison against the weight trend, so the caveat
      // about the activity multiplier applies to it as much as to a gap.
      await expect(hint).toContainText("doesn't separately account for error in your activity multiplier");
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test("today's intake renders against the target, in every gap state", async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, onTrackFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      const today = card.getByTestId('nutrition-today-calories');
      await expect(today).toBeVisible({ timeout: 15_000 });
      await expect(today).toContainText('1200 / 2500 kcal');
      await expect(card.getByTestId('nutrition-today-macros')).toContainText('Protein 80/150 g');

      // The top row is not conditional on the gap resolving: the same row must
      // render when the gap line is "not enough data yet", which is the state a
      // user spends their first weeks in. Re-mocking rather than starting a
      // fresh page works because Playwright matches the most recently
      // registered route first, so this second call shadows the first.
      await mockLoggingGapApis(page, { ...noDataFixture(), today: { calories: 1200, protein: 80, carbs: 130, fat: 35 } });
      await page.goto('/');
      await expect(card.getByTestId('logging-gap-not-enough-data')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('nutrition-today-calories')).toContainText('1200 / 2500 kcal');
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test("the top row's ⓘ explains the target and opens independently of the gap line's own hint", async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, onTrackFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      const todayHint = card.getByTestId('nutrition-today-hint');
      const todayToggle = card.getByTestId('nutrition-today-hint-toggle');
      await expect(todayToggle).toBeVisible({ timeout: 15_000 });

      // Hidden by default, matching the gap line's own hint.
      await expect(todayHint).toBeHidden();
      await expect(todayToggle).toHaveAttribute('aria-expanded', 'false');

      await todayToggle.click();
      await expect(todayHint).toBeVisible();
      await expect(todayToggle).toHaveAttribute('aria-expanded', 'true');
      // The confirmed-meals rule, explaining the *first* number in the row.
      await expect(todayHint).toContainText('confirmed meals only');
      // The fixture's BMR (1000, mockLoggingGapApis' default), activity
      // multiplier (1.55) and tier label (Moderately active -> "moderately
      // active"), and goal weight (75 -> "75.0").
      await expect(todayHint).toContainText('1000 kcal');
      await expect(todayHint).toContainText('1.55');
      await expect(todayHint).toContainText('moderately active');
      await expect(todayHint).toContainText('75.0 kg');

      // Opening the top row's hint must not open the gap line's own hint.
      const gapHint = card.getByTestId('logging-gap-hint');
      await expect(gapHint).toBeHidden();

      // Close the top row's hint, then check the reverse: opening the gap
      // line's own hint must not open the top row's — the two panels are
      // independent state, not one shared toggle.
      await todayToggle.click();
      await expect(todayHint).toBeHidden();
      await card.getByTestId('logging-gap-hint-toggle').click();
      await expect(gapHint).toBeVisible();
      await expect(todayHint).toBeHidden();
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  // The one test in this file that does NOT mock /api/food/daily-totals.
  // Every other test here fulfills all four of the card's requests from
  // fixtures, which makes the card's *logic* deterministic but leaves the seam
  // this feature actually introduced — a brand-new endpoint and a brand-new
  // caller for it — untested end to end: a wrong path prefix, a renamed query
  // parameter, or a JSON tag that doesn't match the field the card reads would
  // pass every mocked test and fail only in a real browser. The other three
  // requests stay mocked so this test depends on nothing about what the shared
  // `alice` account has logged; only the daily-totals round trip is real.
  test('the card requests daily totals from the real endpoint, which answers with the shape it reads', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      const { windowStart, windowEnd } = loggingGapWindow();
      await mockLoggingGapApis(page, clearGapFixture(), { liveDailyTotals: true });

      const responsePromise = page.waitForResponse(r => r.url().includes('/api/food/daily-totals'), {
        timeout: 20_000,
      });
      await page.goto('/');
      const response = await responsePromise;

      // The URL the card built, asserted against the route the server
      // registered — path and both parameter names, not just "something 200'd".
      const url = new URL(response.url());
      expect(url.pathname).toBe('/api/food/daily-totals');
      expect(url.searchParams.get('from')).toBe(windowStart);
      expect(url.searchParams.get('to')).toBe(windowEnd);
      expect(response.status()).toBe(200);

      // DailyTotalsRange zero-fills every day in the range rather than omitting
      // the empty ones, so the window's full length comes back whatever this
      // account has logged — which is what lets the card index by date without
      // a presence check.
      const body = await response.json();
      expect(Array.isArray(body)).toBe(true);
      expect(body).toHaveLength(dateRange(windowStart, windowEnd).length);
      expect(body[0].date).toBe(windowStart);
      expect(body[body.length - 1].date).toBe(windowEnd);
      // Field names exactly as LoggingGapCard destructures them — `calories`
      // and the snake_case `unconfirmed_meals`, both numbers.
      for (const entry of body) {
        expect(typeof entry.date).toBe('string');
        expect(typeof entry.calories).toBe('number');
        expect(typeof entry.unconfirmed_meals).toBe('number');
      }

      // And the card survived the real response: it reached a terminal content
      // state rather than throwing or hanging on the spinner.
      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-loading')).toHaveCount(0, { timeout: 15_000 });
      await expect(card.getByTestId('logging-gap-error')).toHaveCount(0);
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

      // The hint travels with the row, so it is reachable here too — but it
      // carries only what applies. The photo caveat qualifies today's calorie
      // figure at the top of the card, which is on screen; the activity
      // caveat qualifies a comparison against the weight trend that produced
      // nothing here, and the on-track sentence explains a state this is not.
      const hint = card.getByTestId('logging-gap-hint');
      await expect(hint).toBeHidden();
      await card.getByTestId('logging-gap-hint-toggle').click();
      await expect(hint).toBeVisible();
      await expect(hint).toContainText('Logged intake is estimated from photo recognition');
      await expect(hint).not.toContainText("doesn't separately account for error in your activity multiplier");
      await expect(hint).not.toContainText('intake and weight trend agree');
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
      // an unavailable target ends the card before any of it is inspected,
      // because neither the today row nor the gap can be computed without one.
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

  test('a failure from one of the four requests shows "temporarily unavailable", not "not enough data yet"', async ({
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
      await mockLoggingGapApis(page, {
        ...clearGapFixture(),
        today: { calories: 1200, protein: 80, carbs: 130, fat: 35 },
        weightStatus: 500,
      });
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-error')).toBeVisible({ timeout: 15_000 });
      await expect(card).toContainText('Temporarily unavailable');
      // Never "not enough data yet": an outage is not a data shortfall, and
      // saying so would blame the user's logging for a failed request.
      await expect(card.getByTestId('logging-gap-not-enough-data')).toHaveCount(0);
      // The failure is confined to the gap line. Today's row comes from
      // /api/summary/today, which answered, so it must survive — losing it
      // here would discard the row this card leads with over an unrelated
      // request.
      await expect(card.getByTestId('nutrition-today-calories')).toContainText('1200 / 2500 kcal');
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('a failure of the summary request itself takes the whole card, since nothing can be drawn without it', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      // The other side of the split above: /api/summary/today carries both the
      // today row and the target the gap needs, so its failure leaves no row
      // to draw and no Implied Intake to derive.
      await mockLoggingGapApis(page, { ...clearGapFixture(), summaryStatus: 500 });
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-error')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('nutrition-today-calories')).toHaveCount(0);
      await expect(card.getByTestId('logging-gap-value')).toHaveCount(0);
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('a weight trend losing faster than the sustainable 1%/week shows the loss-rate warning', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, tooFastLossFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      const lossRate = card.getByTestId('nutrition-sustainability-loss-rate');
      await expect(lossRate).toBeVisible({ timeout: 15_000 });
      await expect(lossRate).toContainText(/\d+\.\d%/);
      await expect(lossRate).toContainText('faster than the sustainable 1%');
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('logged intake clearly below a mocked BMR under on_track shows the below-BMR warning, not the loss-rate one', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, belowBmrFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-on-track')).toBeVisible({ timeout: 15_000 });
      const belowBmr = card.getByTestId('nutrition-sustainability-below-bmr');
      await expect(belowBmr).toBeVisible();
      await expect(belowBmr).toContainText('1700');
      await expect(belowBmr).toContainText('2000');
      await expect(card.getByTestId('nutrition-sustainability-loss-rate')).toHaveCount(0);
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('the same intake and BMR numbers stay silent once the logging gap is a real gap, not on_track', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, belowBmrGatedOffFixture());
      await page.goto('/');

      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-value')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('nutrition-sustainability-below-bmr')).toHaveCount(0);
      await expect(card.getByTestId('nutrition-sustainability')).toHaveCount(0);
    } finally {
      await putSettings(request, cookies, original);
    }
  });

  test('the middle row is absent for on_track, not_enough_data and the gap-only retrieval error', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    try {
      await mockLoggingGapApis(page, onTrackFixture());
      await page.goto('/');
      const card = page.getByTestId('logging-gap-card');
      await expect(card.getByTestId('logging-gap-on-track')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('nutrition-sustainability')).toHaveCount(0);

      await mockLoggingGapApis(page, noDataFixture());
      await page.goto('/');
      await expect(card.getByTestId('logging-gap-not-enough-data')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('nutrition-sustainability')).toHaveCount(0);

      await mockLoggingGapApis(page, { ...clearGapFixture(), weightStatus: 500 });
      await page.goto('/');
      await expect(card.getByTestId('logging-gap-error')).toBeVisible({ timeout: 15_000 });
      await expect(card.getByTestId('nutrition-sustainability')).toHaveCount(0);
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
  const showToggle = page.getByRole('button', { name: 'Show Nutrition' });
  if (await showToggle.isVisible().catch(() => false)) {
    await showToggle.click().catch(() => {});
    changed = true;
  }

  const moveDown = page.getByRole('button', { name: 'Move Nutrition down' });
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
      await page.getByRole('button', { name: 'Hide Nutrition' }).click();

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

      const moveUp = page.getByRole('button', { name: 'Move Nutrition up' });
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
