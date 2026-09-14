import { test, expect, type Page } from '@playwright/test';

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

function expectKotlinInt(value: unknown) {
  expect(Number.isInteger(value)).toBe(true);
  expect(value as number).toBeGreaterThanOrEqual(-2_147_483_648);
  expect(value as number).toBeLessThanOrEqual(2_147_483_647);
}

// This test exists to pin the one contract android/ compiles against without
// ever compiling it: android/app/src/main/kotlin/net/ikoro/healthvault/api/TodaySummary.kt
// mirrors backend/pkg/server/summary_today.go's summaryTodayResponse field
// for field, by hand, with no shared schema between them. A change here that
// drops or retypes a field the Kotlin model parses fails this spec file in
// this repository — the only automated signal a breaking change to
// summaryTodayResponse gets, on a machine with no Android SDK to compile the
// client itself (see the spec's "Gate wiring, and the gap in it").
test.describe('GET /api/summary/today — Android widget contract', () => {
  test('every field the Android client parses is present with the expected type', async ({ page }) => {
    await login(page);
    const cookies = await cookieHeader(page);

    const res = await page.request.get('/api/summary/today', { headers: { Cookie: cookies } });
    expect(res.status()).toBe(200);
    const body = await res.json();

    expect(typeof body.date).toBe('string');
    expect(typeof body.calories_consumed).toBe('number');
    expect(typeof body.protein_grams_consumed).toBe('number');
    expect(typeof body.carbs_grams_consumed).toBe('number');
    expect(typeof body.fat_grams_consumed).toBe('number');
    expectKotlinInt(body.meal_count);
    expect(typeof body.display_language).toBe('string');

    // last_logged_at: null, or a string TodaySummary.kt's lastLoggedAt (a
    // nullable String, not parsed on the Kotlin side) can hold as-is —
    // still checked for being a genuinely parseable timestamp, since the
    // today screen and widget both format it.
    if (body.last_logged_at !== null) {
      expect(typeof body.last_logged_at).toBe('string');
      expect(body.last_logged_at).toMatch(
        /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/
      );
      expect(Number.isNaN(Date.parse(body.last_logged_at))).toBe(false);
    }

    // target.available discriminates the payload the same way
    // TodaySummaryTarget.kt reads it: the four numeric fields are always
    // present (never omitted — see summaryTargetPayload's own comment on
    // why omitempty would make a *present* zero-valued target look
    // partial), and `reason` is present only when unavailable.
    expect(typeof body.target).toBe('object');
    expect(typeof body.target.available).toBe('boolean');
    expectKotlinInt(body.target.calories);
    expectKotlinInt(body.target.protein_grams);
    expectKotlinInt(body.target.carbs_grams);
    expectKotlinInt(body.target.fat_grams);
    if (!body.target.available) {
      expect(typeof body.target.reason).toBe('string');
    }
  });

  test('stays self-only: ?user= is ignored, the caller always gets their own data', async ({ page }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const otherUser = USER.toLowerCase() === 'bob' ? 'alice' : 'bob';

    // Fetch concurrently so a local-midnight boundary cannot make two
    // otherwise identical self-only responses differ just because their
    // `date` fields were computed on opposite sides of the boundary.
    const [own, withUserParam] = await Promise.all([
      page.request.get('/api/summary/today', { headers: { Cookie: cookies } }),
      page.request.get(`/api/summary/today?user=${encodeURIComponent(otherUser)}`, {
        headers: { Cookie: cookies },
      }),
    ]);
    expect(own.status()).toBe(200);
    expect(withUserParam.status()).toBe(200);
    const ownBody = await own.json();
    const withUserParamBody = await withUserParam.json();
    expect(withUserParamBody).toEqual(ownBody);
  });
});
