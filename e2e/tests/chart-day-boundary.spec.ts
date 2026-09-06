import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
// This spec PUTs settings that change `timezone`, which cascades to
// hard-deleting the caller's FoodDayCompletion rows (design.md §4 "Storage") —
// one of the reasons `target.ts` refuses to resolve a prod URL at all.
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

// Full (non-merging) write, mirroring PUT /users/me/settings' whole-document
// semantics — used to force a known baseline before this test and to
// restore the account's prior settings afterward.
async function putSettings(request: APIRequestContext, cookies: string, settings: Record<string, unknown>) {
  const res = await request.put(`${BASE_URL}/api/users/me/settings`, { headers: { Cookie: cookies }, data: settings });
  if (!res.ok()) throw new Error(`failed to write settings: ${res.status()} ${await res.text()}`);
}

async function createWeight(request: APIRequestContext, cookies: string, isoTime: string, kilograms: number) {
  const res = await request.post(`${BASE_URL}/api/data/weight`, {
    headers: { Cookie: cookies },
    data: { value: kilograms, time: isoTime },
  });
  if (!res.ok()) throw new Error(`failed to create weight: ${res.status()} ${await res.text()}`);
  return res.json();
}

async function deleteWeight(request: APIRequestContext, cookies: string, id: string): Promise<void> {
  const res = await request.delete(`${BASE_URL}/api/data/weight/${id}`, { headers: { Cookie: cookies } });
  if (!res.ok() && res.status() !== 404) {
    throw new Error(`failed to delete weight ${id}: ${res.status()} ${await res.text()}`);
  }
}

// The bucket_start the API reports for the record at isoTime, under
// whatever timezone the account is currently set to — narrowed to a 2-hour
// window around isoTime so an account already holding other weight history
// (this suite runs against a shared account) can't land in a different
// bucket than the one this test is asserting about.
async function bucketStartFor(request: APIRequestContext, cookies: string, isoTime: string): Promise<string> {
  const center = new Date(isoTime);
  const from = new Date(center.getTime() - 60 * 60 * 1000).toISOString();
  const to = new Date(center.getTime() + 60 * 60 * 1000).toISOString();
  const res = await request.get(
    `${BASE_URL}/api/data/weight?bucket=day&from=${from}&to=${to}`,
    { headers: { Cookie: cookies } }
  );
  if (!res.ok()) throw new Error(`failed to read bucketed weight: ${res.status()} ${await res.text()}`);
  const rows = (await res.json()) as Array<{ bucket_start: string }>;
  expect(rows.length, `expected exactly one bucket around ${isoTime}, got ${JSON.stringify(rows)}`).toBe(1);
  return rows[0].bucket_start;
}

// A UTC timestamp late in the day (23:45), on a date chosen fresh (randomly,
// far in the past) each run so re-running this suite never collides with a
// previous run's record under the (user_id, time) unique index.
function lateUTCInstant(): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - 300 - Math.floor(Math.random() * 1000));
  d.setUTCHours(23, 45, 0, 0);
  return d.toISOString();
}

test.describe('Chart day boundary', () => {
  // 8.9/8.10: a record logged late in the UTC day buckets under today's UTC
  // date when the account is UTC, and under *tomorrow's* date once the
  // account is switched to a zone ahead of UTC — 23:45 UTC plus a positive
  // offset crosses local midnight. This is the same boundary shift the
  // whole feature exists to fix, exercised end to end through the deployed
  // API rather than against the storage layer directly.
  test('a late-UTC-day record moves to the next bucket under a zone ahead of UTC', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    const isoTime = lateUTCInstant();
    const record = await createWeight(request, cookies, isoTime, 71.5);

    try {
      const utcBucket = await bucketStartFor(request, cookies, isoTime);
      expect(utcBucket).toBe(`${isoTime.slice(0, 10)}T00:00:00Z`);

      await putSettings(request, cookies, { ...original, timezone: 'Asia/Tokyo' });
      const tokyoBucket = await bucketStartFor(request, cookies, isoTime);

      const utcDate = new Date(utcBucket);
      const tokyoDate = new Date(tokyoBucket);
      const dayMs = 24 * 60 * 60 * 1000;
      expect(
        (tokyoDate.getTime() - utcDate.getTime()) / dayMs,
        `UTC bucket ${utcBucket} and Asia/Tokyo bucket ${tokyoBucket} for the same record ` +
        `must differ by exactly one day (23:45 UTC + 9h crosses local midnight)`
      ).toBe(1);
    } finally {
      await deleteWeight(request, cookies, record.id);
      await putSettings(request, cookies, original);
    }
  });
});
