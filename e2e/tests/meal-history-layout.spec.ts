import { test, expect, type Page, type APIRequestContext, type Locator } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
// Like completeness.spec.ts, these tests PUT settings — `timezone`, which
// cascades to hard-deleting FoodDayCompletion rows, and `display_language`,
// which every other spec file reads as English. A no-env run must never land
// on the prod stack (8888) by accident, so it defaults to WIP.
const BASE_URL = process.env.BASE_URL || 'http://192.168.1.54:8892';

// The two widths the mobile audit measured this page at. 390x844 is a current
// phone; 320x568 is the narrowest viewport the project supports, and it is
// where the header failed even on days carrying no completeness control.
const VIEWPORTS = [
  { name: '390x844', width: 390, height: 844 },
  { name: '320x568', width: 320, height: 568 },
] as const;

// Russian is the binding case, not a secondary one: "Mark day complete"
// translates to "Отметить день заполненным", which is the widest single unit
// the day header has to hold. An English-only test passes against the layout
// that is broken today.
const LANGUAGES = ['en', 'ru'] as const;

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

async function unconfirmDate(request: APIRequestContext, cookies: string, date: string) {
  await request.delete(`${BASE_URL}/api/food/completeness/${date}/confirm`, { headers: { Cookie: cookies } }).catch(() => {});
}

async function getSettings(request: APIRequestContext, cookies: string): Promise<Record<string, unknown>> {
  const res = await request.get(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies } });
  return res.json();
}

async function putSettings(request: APIRequestContext, cookies: string, settings: Record<string, unknown>) {
  const res = await request.put(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies }, data: settings });
  if (!res.ok()) throw new Error(`failed to write settings: ${res.status()} ${await res.text()}`);
}

// The account is shared with every other spec file, so a date this test picks
// may already hold occasions it did not create. Derive the threshold from what
// the day actually holds rather than hardcoding one — see completeness.spec.ts,
// where hardcoding it made failures a matter of which dates happened to be
// clean.
async function occasionCount(request: APIRequestContext, cookies: string, date: string): Promise<number> {
  const res = await request.get(`${BASE_URL}/api/food/completeness?from=${date}&to=${date}`, {
    headers: { Cookie: cookies },
  });
  if (!res.ok()) throw new Error(`failed to read completeness for ${date}: ${res.status()} ${await res.text()}`);
  const days = (await res.json()) as Array<{ date: string; occasion_count: number }>;
  return days.find(d => d.date === date)?.occasion_count ?? 0;
}

async function thresholdLeavingDayBelow(
  request: APIRequestContext,
  cookies: string,
  date: string
): Promise<number> {
  return (await occasionCount(request, cookies, date)) + 1;
}

function daySection(page: Page, mealName: string): Locator {
  return page.locator('div.mb-5').filter({ hasText: mealName });
}

const RUN_TAG = Math.random().toString(36).slice(2, 8);

function isoAtUTC(daysAgo: number, hour: number, minute = 0): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - daysAgo);
  d.setUTCHours(hour, minute, 0, 0);
  return d.toISOString();
}

/** True when the two boxes share any area at all — as mobile-nav.spec.ts. */
function intersects(a: NonNullable<Awaited<ReturnType<Locator['boundingBox']>>>,
                    b: NonNullable<Awaited<ReturnType<Locator['boundingBox']>>>) {
  return a.x < b.x + b.width && b.x < a.x + a.width
    && a.y < b.y + b.height && b.y < a.y + a.height;
}

async function boxOf(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} should have a bounding box`).not.toBeNull();
  return box!;
}

/**
 * How many lines of its own text an element renders, to one decimal.
 *
 * This is the measurement that separates a header that fits from the one on
 * `main`: at 390px the date rendered as `среда,` / `26 авг.` — 2.0 — and the
 * macro summary wrapped around the completeness pill, splitting `У` from `0г`.
 * Bounding-box intersection alone does not catch that: the wrapped fragments
 * sit beside the pill rather than under it, so their boxes stay disjoint while
 * the header still reads as three interleaved pieces.
 */
async function lineCount(locator: Locator): Promise<number> {
  return locator.evaluate(el => {
    const cs = getComputedStyle(el);
    const lh = parseFloat(cs.lineHeight) || parseFloat(cs.fontSize) * 1.2;
    return Math.round((el.getBoundingClientRect().height / lh) * 10) / 10;
  });
}

test.describe('Meal history layout', () => {
  // The header case walks four viewport x language combinations, each a
  // settings write plus a full page load, and the row case walks two. Both
  // exceed the config's 30s default without being slow in any way worth
  // fixing.
  test.describe.configure({ timeout: 120_000 });

  test('the day header holds date, completeness control and totals without collision', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);

    const mealName = `E2E History Header ${RUN_TAG}`;
    const meal = await createMealAt(request, cookies, mealName, isoAtUTC(3, 12));
    const date = isoAtUTC(3, 12).slice(0, 10);

    try {
      // The day must render its "Mark day complete" control: that control is
      // the widest unit in the row and the one that cannot shrink, since it is
      // a TapTarget bound by the 48x48 minimum PR #36 established.
      await unconfirmDate(request, cookies, date);

      for (const lang of LANGUAGES) {
        await putSettings(request, cookies, {
          ...original,
          timezone: 'UTC',
          display_language: lang,
          usual_meals_per_day: await thresholdLeavingDayBelow(request, cookies, date),
        });

        for (const vp of VIEWPORTS) {
          await test.step(`${vp.name} / ${lang}`, async () => {
            await page.setViewportSize({ width: vp.width, height: vp.height });
            await page.goto('/food/history/');

            const section = daySection(page, mealName);
            await expect(section).toBeVisible({ timeout: 15_000 });

            const header = section.getByTestId('day-header');
            // Addressed by role rather than by name: the accessible name is
            // "Mark day complete" in English and "Отметить день заполненным"
            // in Russian, and the unconfirmed day renders exactly one button.
            const control = header.getByRole('button');
            await expect(control).toBeVisible({ timeout: 10_000 });

            const label = section.locator('h2');
            const calories = section.getByTestId('day-total-calories');
            const macros = section.getByTestId('day-total-macros');

            // Nothing in the header may overlap anything else in it.
            const [lb, cb, calb, macb] = [
              await boxOf(label, 'day label'),
              await boxOf(control, 'completeness control'),
              await boxOf(calories, 'day calories'),
              await boxOf(macros, 'day macros'),
            ];
            expect(intersects(lb, cb), 'date must not overlap the completeness control').toBe(false);
            expect(intersects(lb, calb), 'date must not overlap the calories total').toBe(false);
            expect(intersects(lb, macb), 'date must not overlap the macro totals').toBe(false);
            expect(intersects(cb, calb), 'completeness control must not overlap the calories total').toBe(false);
            expect(intersects(cb, macb), 'completeness control must not overlap the macro totals').toBe(false);

            // Each unit renders whole, on one line of its own.
            expect(await lineCount(label), 'the date must not break across lines').toBe(1);
            expect(await lineCount(calories), 'the calories total must not wrap').toBe(1);
            expect(await lineCount(macros), 'the macro totals must not wrap').toBe(1);

            // The control is asserted by its computed white-space rather than
            // by line count: it is a TapTarget, so its box is 48px tall by
            // guarantee and a height-based measure would read three lines of
            // 16px text however the label actually renders. `nowrap` is the
            // property that makes the control drop whole onto its own line
            // instead of re-wrapping inside its pill.
            await expect(control).toHaveCSS('white-space', 'nowrap');

            // The header must not scroll the page sideways at any width.
            const overflow = await header.evaluate(el => el.scrollWidth - el.clientWidth);
            expect(overflow, 'the header must not overflow its own width').toBeLessThanOrEqual(1);
          });
        }
      }
    } finally {
      await deleteMeal(request, cookies, meal.id);
      await unconfirmDate(request, cookies, date);
      await putSettings(request, cookies, original);
    }
  });

  test('a meal row is identified by its time alone, in the account timezone', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);

    const mealName = `E2E History Row ${RUN_TAG}`;
    // 23:00 UTC: the hour most likely to expose a timestamp rendered in the
    // browser's zone instead of the account's, since any positive offset moves
    // it into the following calendar day and away from the header above it.
    const meal = await createMealAt(request, cookies, mealName, isoAtUTC(3, 23));

    try {
      await page.setViewportSize({ width: 390, height: 844 });

      for (const lang of LANGUAGES) {
        await putSettings(request, cookies, { ...original, timezone: 'UTC', display_language: lang });
        await page.goto('/food/history/');

        await test.step(lang, async () => {
          const section = daySection(page, mealName);
          await expect(section).toBeVisible({ timeout: 15_000 });

          const meta = section.locator('a').filter({ hasText: mealName }).getByTestId('meal-meta');
          const text = (await meta.innerText()).trim();

          // Time only: no calendar date, no seconds. The day header above the
          // row already names the day.
          expect(text, 'the row must lead with a time').toMatch(/^\d{1,2}:\d{2}\b/);
          expect(text, 'the row must not repeat the year').not.toMatch(/\d{4}/);
          expect(text, 'the row must not carry seconds').not.toMatch(/\d{1,2}:\d{2}:\d{2}/);

          // The time must be the account's zone — the zone the day grouping
          // used — not the browser's. Asserted in Russian only: English falls
          // back to the *runner's* default locale (dateLocaleFor returns
          // undefined for 'en'), so its 12-hour rendering of 23:00 UTC depends
          // on where the test runs, while 'ru' is a fixed 24-hour locale and
          // gives exactly "23:00" for a UTC account anywhere.
          if (lang === 'ru') {
            expect(text, 'the time must be rendered in the account timezone').toMatch(/^23:00\b/);
          }
        });
      }
    } finally {
      await deleteMeal(request, cookies, meal.id);
      await putSettings(request, cookies, original);
    }
  });
});
