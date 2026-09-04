import { expect, type Page, type APIRequestContext, type Locator } from '@playwright/test';

/**
 * Helpers for tests that seed meals on a specific local day and assert on how
 * the history page groups them.
 *
 * These lived twice — in `completeness.spec.ts` and `meal-history-layout.spec.ts`
 * — and the duplication cost something concrete: `completeness.spec.ts` had been
 * hardened to write a UTC baseline *before* reading any occasion count, and the
 * second copy did not inherit that, so its Russian iteration could derive a
 * threshold for the wrong local day. Anything both files need belongs here, so
 * hardening one hardens both.
 *
 * `getSettings`/`putSettings`/`cookieHeader` are duplicated more widely still
 * (five spec files). They are re-exported from here for the two files this
 * module already serves, but the other three keep their own copies — pulling
 * them in would mean editing specs unrelated to the change this module arrived
 * with.
 */

export const USER = process.env.HCW_USER || 'alice';
export const PASS = process.env.HCW_PASS || 'pass1';

// Tests using this module PUT settings that change `timezone` — which cascades
// to hard-deleting the caller's FoodDayCompletion rows (design.md §4 "Storage")
// — and `display_language`, which every other spec file reads as English. Which
// deployment that lands on is decided in one place for the whole suite, and prod
// is refused there outright.
import { BASE_URL } from './target';

/** Meals per page on /food/history/ — `frontend/app/food/history/page.tsx`. */
export const HISTORY_PAGE_SIZE = 50;

/**
 * Fails unless the browser and this module's API calls target the same stack.
 *
 * Both now come from `target.ts` — the config's `baseURL` included — so the
 * defaults can no longer disagree, which is what used to log the browser into
 * one stack while every API call went to the other. What remains is a spec
 * overriding `baseURL` through `test.use({ baseURL })`: that moves the browser
 * without moving these calls, and the symptom is still `unauthorized` from
 * whichever read happens first, saying nothing about the cause. Cheap to keep.
 */
function assertSameStack(page: Page): void {
  const browserHost = new URL(page.url()).host;
  if (browserHost !== new URL(BASE_URL).host) {
    throw new Error(
      `the browser is on ${browserHost} but this module's API calls target ${BASE_URL}. ` +
      `Both derive from tests/helpers/target.ts, so something moved the browser on its own — ` +
      `look for a \`test.use({ baseURL })\` in the running spec.`
    );
  }
}

export async function login(page: Page) {
  await page.goto('/login/');
  // After the first navigation, so page.url() is resolved against the browser's
  // own baseURL rather than read off a private field.
  assertSameStack(page);
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

export async function cookieHeader(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  return cookies.map(c => `${c.name}=${c.value}`).join('; ');
}

// Seeds a confirmed meal at a specific instant via the manual-entry API (no
// vision call, deterministic), so Day Completeness state (which is driven
// entirely by logged_at) is fully under the test's control.
export async function createMealAt(
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

export async function deleteMeal(request: APIRequestContext, cookies: string, id: string): Promise<void> {
  const res = await request.delete(`${BASE_URL}/api/data/food_meal/${id}`, { headers: { Cookie: cookies } });
  if (!res.ok() && res.status() !== 404) {
    throw new Error(`failed to delete meal ${id}: ${res.status()} ${await res.text()}`);
  }
}

export async function deleteMeals(request: APIRequestContext, cookies: string, ids: string[]): Promise<void> {
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

export async function getSettings(request: APIRequestContext, cookies: string): Promise<Record<string, unknown>> {
  const res = await request.get(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies } });
  return res.json();
}

// Full (non-merging) write, mirroring PUT /users/me/settings' whole-document
// semantics — used to force a known baseline before a test and to restore
// the account's prior settings afterward, regardless of what the test itself
// changed via the UI.
export async function putSettings(
  request: APIRequestContext,
  cookies: string,
  settings: Record<string, unknown>
) {
  const res = await request.put(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies }, data: settings });
  if (!res.ok()) throw new Error(`failed to write settings: ${res.status()} ${await res.text()}`);
}

// Best-effort: a confirmation that was never created (or already retracted)
// 404s/204s either way, and cleanup must not fail the test over that.
//
// Call this *after* any timezone change, never before: switching `timezone`
// hard-deletes the caller's FoodDayCompletion rows, so unconfirming first is a
// no-op that reads like a precaution.
export async function unconfirmDate(request: APIRequestContext, cookies: string, date: string) {
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
//
// The backend resolves this endpoint's `from`/`to` in the caller's *stored*
// timezone (`backend/pkg/server/food_completeness.go` → `database.ResolveTimezone`),
// so write the UTC baseline before calling this, or the count describes a
// different local day than the one the caller has in mind.
export async function occasionCount(
  request: APIRequestContext,
  cookies: string,
  date: string
): Promise<number> {
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
export async function thresholdLeavingDayBelow(
  request: APIRequestContext,
  cookies: string,
  date: string
): Promise<number> {
  return (await occasionCount(request, cookies, date)) + 1;
}

/**
 * How many of the account's meals are logged on a day *newer* than `date`.
 *
 * The history page loads meals logged_at-DESC in pages of HISTORY_PAGE_SIZE, so
 * this is the index at which a meal seeded on `date` will appear. Above the page
 * size it is simply not on screen, and a test waiting for it fails as a
 * visibility timeout that says nothing about why.
 *
 * The `limit` is the API's own maximum. Capping there is harmless: any answer
 * large enough to be capped is already far past the page size.
 */
export async function mealsNewerThan(
  request: APIRequestContext,
  cookies: string,
  date: string
): Promise<number> {
  const res = await request.get(`${BASE_URL}/api/food/meals?limit=200`, { headers: { Cookie: cookies } });
  if (!res.ok()) throw new Error(`failed to list meals: ${res.status()} ${await res.text()}`);
  const meals = (await res.json()) as Array<{ logged_at: string }>;
  return meals.filter(m => m.logged_at.slice(0, 10) > date).length;
}

/**
 * Throws unless a meal seeded on `date` would land on the history page's first
 * page.
 *
 * Only meaningful under a UTC baseline, since `date` is derived with isoAtUTC
 * and `logged_at` comes back UTC-normalized.
 */
export async function expectOnFirstHistoryPage(
  request: APIRequestContext,
  cookies: string,
  date: string
): Promise<void> {
  const ahead = await mealsNewerThan(request, cookies, date);
  if (ahead + 1 > HISTORY_PAGE_SIZE) {
    throw new Error(
      `a meal seeded on ${date} would sit behind ${ahead} newer meals, past the history page's ` +
      `first page of ${HISTORY_PAGE_SIZE}, so it never renders without "Load older". Seed a more ` +
      `recent day, or clean up the shared account's leftover E2E meals.`
    );
  }
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
//
// Every day it skips holds at least one occasion by construction, so walking
// back far enough eventually pushes the day past the history page's first page.
// That is checked per candidate rather than left to surface as a visibility
// timeout in the caller.
export async function findEmptyDaysAgo(
  request: APIRequestContext,
  cookies: string,
  startDaysAgo: number,
  maxLookback = 60
): Promise<number> {
  for (let d = startDaysAgo; d < startDaysAgo + maxLookback; d++) {
    const date = isoAtUTC(d, 12).slice(0, 10);
    await expectOnFirstHistoryPage(request, cookies, date);
    if ((await occasionCount(request, cookies, date)) === 0) return d;
  }
  throw new Error(
    `no empty day found between ${startDaysAgo} and ${startDaysAgo + maxLookback} days ago; ` +
    `the shared account may need its leftover E2E meals cleaned up`
  );
}

// Locates the day-group section (page.tsx's `<div className="mb-5">` per
// day) containing the given meal name, so assertions about that day's
// completeness control don't accidentally match another day's section.
export function daySection(page: Page, mealName: string): Locator {
  return page.locator('div.mb-5').filter({ hasText: mealName });
}

export function isoAtUTC(daysAgo: number, hour: number, minute = 0): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - daysAgo);
  d.setUTCHours(hour, minute, 0, 0);
  return d.toISOString();
}

/**
 * Waits until the page is actually rendering in `lang`.
 *
 * LanguageProvider initializes to 'en' and switches only once its settings GET
 * resolves, setting `document.documentElement.lang` as it does
 * (`frontend/components/LanguageContext.tsx`). Without this wait, a test that
 * addresses elements language-agnostically — by test id, by role, by an ASCII
 * meal name — will happily measure the English render and report the Russian
 * case as passing. That is a false pass, not flake: it is the one case the
 * layout assertions exist to cover.
 */
export async function waitForLanguage(page: Page, lang: string): Promise<void> {
  await expect(page.locator('html')).toHaveAttribute('lang', lang, { timeout: 15_000 });
}
