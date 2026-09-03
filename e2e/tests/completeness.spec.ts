import { test, expect, type Page, type APIRequestContext, type Locator } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
// Unlike this suite's other spec files, these tests PUT settings that change
// `timezone` — which cascades to hard-deleting the caller's FoodDayCompletion
// rows (design.md §4 "Storage") — so a no-env run must never land on the prod
// stack (8888) by accident. Defaults to the WIP stack instead; override with
// BASE_URL to target something else deliberately.
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

// Seeds a confirmed meal at a specific instant via the manual-entry API (no
// vision call, deterministic), so Day Completeness state (which is driven
// entirely by logged_at) is fully under the test's control.
async function createMealAt(
  request: APIRequestContext,
  cookies: string,
  name: string,
  loggedAtISO: string,
  calories = 100
) {
  const res = await request.post(`${BASE_URL}/api/food/meals/manual`, {
    headers: { Cookie: cookies },
    data: { name, logged_at: loggedAtISO, items: [{ name: 'Item', source: 'manual', weight_grams: 100, calories }] },
  });
  expect(res.status()).toBe(201);
  return res.json();
}

async function deleteMeal(request: APIRequestContext, cookies: string, id: string): Promise<void> {
  const res = await request.delete(`${BASE_URL}/api/data/food_meal/${id}`, { headers: { Cookie: cookies } });
  if (!res.ok() && res.status() !== 404) {
    throw new Error(`failed to delete meal ${id}: ${res.status()} ${await res.text()}`);
  }
}

async function deleteMeals(request: APIRequestContext, cookies: string, ids: string[]): Promise<void> {
  const results = await Promise.allSettled(ids.map(id => deleteMeal(request, cookies, id)));
  // Carries each rejection's own message, not just the id. The previous
  // version reported only "failed to clean up 3/3 meals: <uuids>", which is
  // the one thing that cannot be acted on: a cleanup failure leaves meals
  // behind that go on to break *other* days' tests, and the status code that
  // would say why (401? 409? a timeout?) was discarded at exactly the moment
  // it mattered. Observed once on 2026-08-26 with no server-side log to
  // match it against.
  const failures = results.flatMap((r, i) =>
    r.status === 'rejected' ? [`${ids[i]}: ${r.reason instanceof Error ? r.reason.message : String(r.reason)}`] : []
  );
  if (failures.length > 0) {
    throw new Error(`failed to clean up ${failures.length}/${ids.length} meals:\n  ${failures.join('\n  ')}`);
  }
}

async function getSettings(request: APIRequestContext, cookies: string): Promise<Record<string, unknown>> {
  const res = await request.get(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies } });
  return res.json();
}

// Full (non-merging) write, mirroring PUT /users/me/settings' whole-document
// semantics — used to force a known baseline before a test and to restore
// the account's prior settings afterward, regardless of what the test itself
// changed via the UI.
async function putSettings(request: APIRequestContext, cookies: string, settings: Record<string, unknown>) {
  const res = await request.put(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies }, data: settings });
  if (!res.ok()) throw new Error(`failed to write settings: ${res.status()} ${await res.text()}`);
}

// Best-effort: a confirmation that was never created (or already retracted)
// 404s/204s either way, and cleanup must not fail the test over that.
async function unconfirmDate(request: APIRequestContext, cookies: string, date: string) {
  await request.delete(`${BASE_URL}/api/food/completeness/${date}/confirm`, { headers: { Cookie: cookies } }).catch(() => {});
}

// The Eating Occasion count the API currently reports for a local date.
//
// Asks the app rather than assuming: these tests run against a shared
// account whose history accumulates meals from every other spec file (and
// from runs killed before their cleanup), so a date this test picks may
// already hold occasions it did not create. A test that needs its day to sit
// *below* threshold must therefore derive the threshold from what the day
// actually holds — see thresholdLeavingDayBelow.
async function occasionCount(request: APIRequestContext, cookies: string, date: string): Promise<number> {
  const res = await request.get(`${BASE_URL}/api/food/completeness?from=${date}&to=${date}`, {
    headers: { Cookie: cookies },
  });
  if (!res.ok()) throw new Error(`failed to read completeness for ${date}: ${res.status()} ${await res.text()}`);
  const days = (await res.json()) as Array<{ date: string; occasion_count: number }>;
  return days.find(d => d.date === date)?.occasion_count ?? 0;
}

// A usual_meals_per_day value that leaves `date` exactly one occasion short
// of its threshold, so the day is Unconfirmed and renders its "Mark day
// complete" control. Call it after seeding, since the seeded meals count too.
//
// Hardcoding 3 here instead is what made this suite's failures a matter of
// which dates happened to be clean: on 2026-08-26 the day seven days back
// already held 3 occasions (two stray meals plus four leftover 'E2E Edit
// Meal' rows collapsing into one), so it was already Complete and the
// control the test waited for was correctly absent.
async function thresholdLeavingDayBelow(
  request: APIRequestContext,
  cookies: string,
  date: string
): Promise<number> {
  return (await occasionCount(request, cookies, date)) + 1;
}

// The most recent day at or before `startDaysAgo` that holds no occasions at
// all, so a test seeding one meal there owns the whole day section.
//
// The same shared-account problem thresholdLeavingDayBelow solves for the
// threshold, one step further: a test that deletes its meal and then asserts
// the day *section* disappeared needs a day whose only meal was its own. On
// 2026-09-03 the day two days back held 17 of the account's own meals, so the
// section correctly survived and the assertion failed on data rather than on
// behaviour. Walks back a bounded number of days and fails loudly rather than
// silently testing nothing.
async function findEmptyDaysAgo(
  request: APIRequestContext,
  cookies: string,
  startDaysAgo: number,
  maxLookback = 60
): Promise<number> {
  for (let d = startDaysAgo; d < startDaysAgo + maxLookback; d++) {
    if ((await occasionCount(request, cookies, isoAtUTC(d, 12).slice(0, 10))) === 0) return d;
  }
  throw new Error(
    `no empty day found between ${startDaysAgo} and ${startDaysAgo + maxLookback} days ago; ` +
    `the shared account may need its leftover E2E meals cleaned up`
  );
}

// Runs `action` and waits for the settings PUT it triggers to actually land,
// not just for the click that starts it — mirrors dashboard.spec.ts's
// withSettingsSave (the settings panel here does the same GET-then-PUT via
// api.updateSettings, so the UI interaction resolves before the write does).
async function withSettingsSave(page: Page, action: () => Promise<unknown>): Promise<void> {
  const saved = page.waitForResponse(
    r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
    { timeout: 15_000 }
  );
  await action();
  await saved;
}

// Locates the day-group section (page.tsx's `<div className="mb-5">` per
// day) containing the given meal name, so assertions about that day's
// completeness control don't accidentally match another day's section.
function daySection(page: Page, mealName: string): Locator {
  return page.locator('div.mb-5').filter({ hasText: mealName });
}

// Distinguishes this run's seeded meals from an earlier run's leftovers.
//
// Without it, a run that fails to clean up poisons the *next* run in a way
// unrelated to what is being tested: the names collide, `getByText(name,
// { exact: true })` resolves to two elements, and Playwright fails on strict
// mode instead of on the assertion. That is what turned one transient
// cleanup failure into a red retry on 2026-08-26.
const RUN_TAG = Math.random().toString(36).slice(2, 8);

function isoAtUTC(daysAgo: number, hour: number, minute = 0): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - daysAgo);
  d.setUTCHours(hour, minute, 0, 0);
  return d.toISOString();
}

test.describe('Day completeness', () => {
  // 12.1: an unconfirmed day can be confirmed and retracted from the history
  // page, both actions persist across a reload, and a day with 0 meals never
  // renders a section at all.
  test('confirming and retracting a day persists across reload; an emptied day drops its section', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    // Force a known baseline: UTC, so the date arithmetic below matches the
    // account's day boundaries regardless of what an earlier run left in its
    // settings. The threshold is set after seeding, once the day's real
    // occasion count is known.
    await putSettings(request, cookies, { ...original, timezone: 'UTC', usual_meals_per_day: 3 });

    const mealName = `E2E Completeness Unconfirmed Day ${RUN_TAG}`;
    // Seeded on a day the account holds nothing else on, because the last
    // assertion here is that deleting the meal drops the whole day section —
    // which only follows if this test's meal was the section's only one.
    const daysAgo = await findEmptyDaysAgo(request, cookies, 2);
    const meal = await createMealAt(request, cookies, mealName, isoAtUTC(daysAgo, 12));
    const date = isoAtUTC(daysAgo, 12).slice(0, 10);

    try {
      // This test asserts the day starts Unconfirmed, which requires both
      // that it sits below threshold and that no confirmation row survives
      // from an earlier run — a stray row would render "Complete"/"Unconfirm"
      // instead of the control this waits for.
      await unconfirmDate(request, cookies, date);
      await putSettings(request, cookies, {
        ...original,
        timezone: 'UTC',
        usual_meals_per_day: await thresholdLeavingDayBelow(request, cookies, date),
      });

      await page.goto('/food/history/');
      await expect(page.getByText(mealName)).toBeVisible();
      const section = daySection(page, mealName);
      const initialLabel = await section.locator('h2').innerText();

      await expect(section.getByRole('button', { name: 'Mark day complete' })).toBeVisible({ timeout: 10_000 });
      await section.getByRole('button', { name: 'Mark day complete' }).click();
      await expect(section.getByText('Complete', { exact: true })).toBeVisible();
      await expect(section.getByRole('button', { name: 'Unconfirm' })).toBeVisible();

      await page.reload();
      const sectionAfterReload = daySection(page, mealName);
      await expect(sectionAfterReload.getByText('Complete', { exact: true })).toBeVisible({ timeout: 10_000 });
      await expect(sectionAfterReload.getByRole('button', { name: 'Unconfirm' })).toBeVisible();

      await sectionAfterReload.getByRole('button', { name: 'Unconfirm' }).click();
      await expect(sectionAfterReload.getByRole('button', { name: 'Mark day complete' })).toBeVisible();
      await expect(sectionAfterReload.getByText('Complete', { exact: true })).not.toBeVisible();

      await page.reload();
      const sectionAfterRetract = daySection(page, mealName);
      await expect(sectionAfterRetract.getByRole('button', { name: 'Mark day complete' })).toBeVisible({ timeout: 10_000 });
      await expect(sectionAfterRetract.getByText('Complete', { exact: true })).not.toBeVisible();

      // 8.4: emptying the day (deleting its only meal) must drop the section
      // entirely, not just the completeness control within it.
      await deleteMeal(request, cookies, meal.id);
      await page.reload();
      await expect(page.getByText(initialLabel, { exact: true })).not.toBeVisible();
      await expect(page.getByText(mealName)).not.toBeVisible();
    } finally {
      await deleteMeal(request, cookies, meal.id);
      await unconfirmDate(request, cookies, date);
      await putSettings(request, cookies, original);
    }
  });

  // 12.2: a day already meeting the threshold shows no completeness control
  // at all — neither the "mark complete" button nor a badge.
  test('a day meeting the threshold shows no completeness control', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC', usual_meals_per_day: 3 });

    const mealName = `E2E Completeness At-Threshold Day ${RUN_TAG}`;
    // Three occasions at least 10 minutes apart (the Eating Occasion
    // collapsing window), so this day sits at or above any threshold this
    // test goes on to choose.
    const created = [
      await createMealAt(request, cookies, mealName, isoAtUTC(4, 8, 0)),
      await createMealAt(request, cookies, `${mealName} 2`, isoAtUTC(4, 8, 15)),
      await createMealAt(request, cookies, `${mealName} 3`, isoAtUTC(4, 8, 30)),
    ];
    const date = isoAtUTC(4, 8, 0).slice(0, 10);

    // A second day, deliberately left below threshold, purely so the page has
    // something completeness-driven to render. `complete` and "not fetched
    // yet" both render nothing (DayCompletenessControl returns null for
    // either), so the at-threshold day offers no signal of its own that the
    // state ever arrived — every assertion below would pass on an empty page.
    // This day's "Mark day complete" button is that signal: it appears only
    // after every window's response has resolved (the page setStates once,
    // after Promise.all) and React has committed the result.
    const sentinelName = `E2E Completeness Sentinel Day ${RUN_TAG}`;
    const sentinel = await createMealAt(request, cookies, sentinelName, isoAtUTC(3, 8, 0));
    const sentinelDate = isoAtUTC(3, 8, 0).slice(0, 10);

    try {
      await unconfirmDate(request, cookies, sentinelDate);
      // Derived, not hardcoded: this account is shared across spec files, so
      // either day may already hold occasions this test did not create.
      const atCount = await occasionCount(request, cookies, date);
      const threshold = await thresholdLeavingDayBelow(request, cookies, sentinelDate);
      expect(
        threshold,
        `the sentinel day ${sentinelDate} holds at least as many occasions as the at-threshold day ` +
        `${date}; leftovers from an earlier run make this test unable to tell the two states apart`
      ).toBeLessThanOrEqual(atCount);
      await putSettings(request, cookies, { ...original, timezone: 'UTC', usual_meals_per_day: threshold });

      await page.goto('/food/history/');
      // exact: true — the other two seeded meals' names both contain this
      // one as a substring ("... 2", "... 3"), which getByText's default
      // substring match would otherwise resolve to all three.
      await expect(page.getByText(mealName, { exact: true })).toBeVisible();
      await expect(
        daySection(page, sentinelName).getByRole('button', { name: 'Mark day complete' })
      ).toBeVisible({ timeout: 15_000 });

      const section = daySection(page, mealName);
      await expect(section.getByRole('button', { name: 'Mark day complete' })).toHaveCount(0);
      await expect(section.getByRole('button', { name: 'Unconfirm' })).toHaveCount(0);
      await expect(section.getByText('Complete', { exact: true })).toHaveCount(0);
    } finally {
      await deleteMeals(request, cookies, [...created.map(m => m.id), sentinel.id]);
      await unconfirmDate(request, cookies, date);
      await unconfirmDate(request, cookies, sentinelDate);
      await putSettings(request, cookies, original);
    }
  });

  // 12.3: switching the account's timezone via the settings panel reshuffles
  // which Logged Day a meal near a UTC/zone boundary groups under.
  //
  // The day heading text (`day.label` in page.tsx) is formatted via
  // `loggedDayLabel`, the same account-`timezone`-aware helper the grouping
  // *key* (`loggedDayKey`, tasks.md 7.2) uses, so both move together when the
  // account timezone changes. This test still asserts on section membership
  // rather than label text: it seeds two meals that fall on different UTC
  // calendar days but the *same* America/Los_Angeles calendar day, and
  // asserts they move from two separate day sections into one merged
  // section — a more direct proof of the regrouping than comparing label
  // strings would be.
  test('a non-UTC timezone regroups meals straddling the UTC boundary under the shifted day', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC', usual_meals_per_day: 3 });

    // earlyName: 01:00 UTC on dayX -> UTC calendar date dayX; in
    // America/Los_Angeles (UTC-7/-8 year-round) that's 17:00-18:00 the
    // *previous* day, i.e. dayX-1.
    // lateName: 23:00 UTC on dayX-1 -> UTC calendar date dayX-1; in
    // America/Los_Angeles that's 15:00-16:00 the *same* dayX-1.
    // So under UTC the two are on different calendar days; under
    // America/Los_Angeles both land on dayX-1.
    const earlyName = `E2E Completeness TZ Merge Early ${RUN_TAG}`;
    const lateName = `E2E Completeness TZ Merge Late ${RUN_TAG}`;
    const early = await createMealAt(request, cookies, earlyName, isoAtUTC(5, 1, 0));
    const late = await createMealAt(request, cookies, lateName, isoAtUTC(6, 23, 0));

    try {
      await page.goto('/food/history/');
      await expect(page.getByText(earlyName, { exact: true })).toBeVisible();
      await expect(page.getByText(lateName, { exact: true })).toBeVisible();
      // Under UTC, the two must be in separate sections.
      await expect(daySection(page, earlyName)).not.toContainText(lateName);

      await page.getByText('Day grouping settings').click();
      await page.getByLabel('Timezone').selectOption('America/Los_Angeles');
      await withSettingsSave(page, async () => {
        await page.getByRole('button', { name: 'Save', exact: true }).click();
      });

      // No reload — the page's own onSaved handler must repaint the
      // grouping immediately (tasks.md 9.3/7). Under America/Los_Angeles,
      // both meals now fall in the same section.
      await expect(daySection(page, earlyName)).toContainText(lateName);
    } finally {
      await deleteMeals(request, cookies, [early.id, late.id]);
      await putSettings(request, cookies, original);
    }
  });

  // 12.4: the settings panel end to end — saving timezone and
  // usual_meals_per_day (a) never clobbers dashboard_order/display_language
  // and (b) updates both day grouping and completeness badges without a
  // page reload. Reuses 12.3's UTC-day-split / America/Los_Angeles-merge
  // pair so the same save also changes the merged day's occasion count from
  // 1 (each side alone) to 2 — combined with lowering the threshold to 2 in
  // the same save, that merged day now meets threshold and its control
  // disappears entirely, proving both the grouping and the completeness
  // state picked up the new settings live.
  test('the settings panel saves both fields without clobbering unrelated settings, and updates the page live', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, {
      ...original,
      timezone: 'UTC',
      usual_meals_per_day: 3,
      dashboard_order: ['weight', 'steps'],
      display_language: 'en',
    });

    const earlyName = `E2E Completeness Settings Panel Early ${RUN_TAG}`;
    const lateName = `E2E Completeness Settings Panel Late ${RUN_TAG}`;
    const early = await createMealAt(request, cookies, earlyName, isoAtUTC(7, 1, 0));
    const late = await createMealAt(request, cookies, lateName, isoAtUTC(8, 23, 0));
    const earlyDate = isoAtUTC(7, 1, 0).slice(0, 10);
    const lateDate = isoAtUTC(8, 23, 0).slice(0, 10);

    try {
      // Both days must start Unconfirmed for the "before" half to mean
      // anything: below threshold, and with no confirmation row left over.
      // The threshold is derived from what the early day actually holds
      // rather than fixed at 3 — this shared account had already accumulated
      // 3 occasions on that date from other specs, which is exactly what made
      // this test fail while asserting something true about the app.
      await unconfirmDate(request, cookies, earlyDate);
      await unconfirmDate(request, cookies, lateDate);
      await putSettings(request, cookies, {
        ...original,
        timezone: 'UTC',
        usual_meals_per_day: await thresholdLeavingDayBelow(request, cookies, earlyDate),
        dashboard_order: ['weight', 'steps'],
        display_language: 'en',
      });

      await page.goto('/food/history/');
      await expect(page.getByText(earlyName, { exact: true })).toBeVisible();
      await expect(page.getByText(lateName, { exact: true })).toBeVisible();
      const earlySection = daySection(page, earlyName);
      // Under UTC the two meals fall on separate days, and the threshold set
      // above leaves the early one exactly one occasion short — so it is
      // Unconfirmed and shows its own "Mark day complete" button.
      await expect(earlySection.getByRole('button', { name: 'Mark day complete' })).toBeVisible({ timeout: 10_000 });
      await expect(earlySection).not.toContainText(lateName);

      await page.getByText('Day grouping settings').click();
      await page.getByLabel('Timezone').selectOption('America/Los_Angeles');
      // 1, not a fixed 2: any day with at least one occasion meets a
      // threshold of 1, so "the merged day now meets threshold" holds
      // whatever else this shared account happens to have logged that day.
      // That the merge itself happened is asserted separately below, by the
      // merged section containing the late meal.
      await page.getByLabel('Usual meals per day').fill('1');
      await withSettingsSave(page, async () => {
        await page.getByRole('button', { name: 'Save', exact: true }).click();
      });

      // (a) dashboard_order/display_language must be exactly as seeded above.
      const afterSave = await getSettings(request, cookies);
      expect(afterSave.dashboard_order).toEqual(['weight', 'steps']);
      expect(afterSave.display_language).toBe('en');
      expect(afterSave.timezone).toBe('America/Los_Angeles');
      expect(afterSave.usual_meals_per_day).toBe(1);

      // (b) grouping merged, live, no reload...
      const mergedSection = daySection(page, earlyName);
      await expect(mergedSection).toContainText(lateName);

      // ...and with the merged day meeting the new threshold, its
      // completeness control disappeared entirely — neither the button nor
      // the confirmed badge, since a day at or above threshold is Complete
      // outright and offers nothing to confirm.
      await expect(mergedSection.getByRole('button', { name: 'Mark day complete' })).toHaveCount(0);
      await expect(mergedSection.getByText('Complete', { exact: true })).toHaveCount(0);
    } finally {
      await deleteMeals(request, cookies, [early.id, late.id]);
      await putSettings(request, cookies, original);
    }
  });
});
