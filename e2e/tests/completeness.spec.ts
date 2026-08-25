import { test, expect, type Page, type APIRequestContext, type Locator } from '@playwright/test';

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
  const failedIds = results.flatMap((r, i) => (r.status === 'rejected' ? [ids[i]] : []));
  if (failedIds.length > 0) {
    throw new Error(`failed to clean up ${failedIds.length}/${ids.length} meals: ${failedIds.join(', ')}`);
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
    // Force a known baseline: UTC + the default threshold, so a single meal
    // reliably lands below threshold regardless of whatever this shared
    // account's settings happened to hold from an earlier run.
    await putSettings(request, cookies, { ...original, timezone: 'UTC', usual_meals_per_day: 3 });

    const mealName = 'E2E Completeness Unconfirmed Day';
    const meal = await createMealAt(request, cookies, mealName, isoAtUTC(2, 12));
    const date = isoAtUTC(2, 12).slice(0, 10);

    try {
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

    const mealName = 'E2E Completeness At-Threshold Day';
    // Three occasions at least 10 minutes apart (the Eating Occasion
    // collapsing window), meeting the default threshold of 3.
    const created = [
      await createMealAt(request, cookies, mealName, isoAtUTC(4, 8, 0)),
      await createMealAt(request, cookies, `${mealName} 2`, isoAtUTC(4, 8, 15)),
      await createMealAt(request, cookies, `${mealName} 3`, isoAtUTC(4, 8, 30)),
    ];
    const date = isoAtUTC(4, 8, 0).slice(0, 10);

    try {
      await page.goto('/food/history/');
      // exact: true — the other two seeded meals' names both contain this
      // one as a substring ("... 2", "... 3"), which getByText's default
      // substring match would otherwise resolve to all three.
      await expect(page.getByText(mealName, { exact: true })).toBeVisible();
      // Wait for the completeness fetch covering this range to actually
      // land before asserting absence — otherwise "no control" would pass
      // trivially just because the fetch hasn't resolved yet.
      await page.waitForResponse(r => r.url().includes('/api/food/completeness'));

      const section = daySection(page, mealName);
      await expect(section.getByRole('button', { name: 'Mark day complete' })).toHaveCount(0);
      await expect(section.getByRole('button', { name: 'Unconfirm' })).toHaveCount(0);
      await expect(section.getByText('Complete', { exact: true })).toHaveCount(0);
    } finally {
      await deleteMeals(request, cookies, created.map(m => m.id));
      await unconfirmDate(request, cookies, date);
      await putSettings(request, cookies, original);
    }
  });

  // 12.3: switching the account's timezone via the settings panel reshuffles
  // which Logged Day a meal near a UTC/zone boundary groups under.
  //
  // The day heading text itself (`day.label` in page.tsx) is formatted via
  // `Date.toLocaleDateString` with no explicit `timeZone`, so it always
  // reflects the *browser's* local zone, not the account's chosen one — only
  // the grouping *key* (`loggedDayKey`, tasks.md 7.2) uses the account
  // timezone. Comparing label text before/after switching would therefore
  // prove nothing. Instead this seeds two meals that fall on different UTC
  // calendar days but the *same* America/Los_Angeles calendar day, and
  // asserts they move from two separate day sections into one merged
  // section — a change only the grouping key, not the label, can produce.
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
    const earlyName = 'E2E Completeness TZ Merge Early';
    const lateName = 'E2E Completeness TZ Merge Late';
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

    const earlyName = 'E2E Completeness Settings Panel Early';
    const lateName = 'E2E Completeness Settings Panel Late';
    const early = await createMealAt(request, cookies, earlyName, isoAtUTC(7, 1, 0));
    const late = await createMealAt(request, cookies, lateName, isoAtUTC(8, 23, 0));

    try {
      await page.goto('/food/history/');
      await expect(page.getByText(earlyName, { exact: true })).toBeVisible();
      await expect(page.getByText(lateName, { exact: true })).toBeVisible();
      const earlySection = daySection(page, earlyName);
      // Under UTC, each is its own single-occasion day: below threshold 3,
      // unconfirmed, so each shows its own "Mark day complete" button.
      await expect(earlySection.getByRole('button', { name: 'Mark day complete' })).toBeVisible({ timeout: 10_000 });
      await expect(earlySection).not.toContainText(lateName);

      await page.getByText('Day grouping settings').click();
      await page.getByLabel('Timezone').selectOption('America/Los_Angeles');
      await page.getByLabel('Usual meals per day').fill('2');
      await withSettingsSave(page, async () => {
        await page.getByRole('button', { name: 'Save', exact: true }).click();
      });

      // (a) dashboard_order/display_language must be exactly as seeded above.
      const afterSave = await getSettings(request, cookies);
      expect(afterSave.dashboard_order).toEqual(['weight', 'steps']);
      expect(afterSave.display_language).toBe('en');
      expect(afterSave.timezone).toBe('America/Los_Angeles');
      expect(afterSave.usual_meals_per_day).toBe(2);

      // (b) grouping merged, live, no reload...
      const mergedSection = daySection(page, earlyName);
      await expect(mergedSection).toContainText(lateName);

      // ...and with the merged day now at 2 occasions meeting the new
      // threshold of 2, its completeness control disappeared entirely.
      await expect(mergedSection.getByRole('button', { name: 'Mark day complete' })).toHaveCount(0);
      await expect(mergedSection.getByText('Complete', { exact: true })).toHaveCount(0);
    } finally {
      await deleteMeals(request, cookies, [early.id, late.id]);
      await putSettings(request, cookies, original);
    }
  });
});
