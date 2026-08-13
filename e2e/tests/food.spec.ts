import { test, expect, type Page, type APIRequestContext } from '@playwright/test';
import path from 'path';

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

// This is a dogfood deployment (the account's only instance, real data) —
// every test cleans up whatever it creates via the same DELETE endpoints the
// app itself uses, so a test run leaves no residue in the real account.
// deleteMeal verifies the delete actually succeeded (2xx, or 404 treated as
// already-clean) rather than firing the request and trusting it worked —
// Playwright's request.delete() only throws on a transport-level failure,
// not an HTTP error status, so a silently-swallowed 500 or an expired
// session would otherwise leave the meal behind with the test still
// reporting green.
async function deleteMeal(request: APIRequestContext, cookies: string, id: string): Promise<void> {
  const res = await request.delete(`${BASE_URL}/api/data/food_meal/${id}`, { headers: { Cookie: cookies } });
  if (!res.ok() && res.status() !== 404) {
    throw new Error(`failed to delete meal ${id}: ${res.status()} ${await res.text()}`);
  }
}

// deleteMeals attempts every ID via Promise.allSettled — one failure can't
// prevent delete attempts for the rest, unlike a sequential for-loop, where
// a thrown error would stop the loop and abandon every ID after it. Reports
// (throws) with the full list of failed IDs if any remain undeleted, so a
// cleanup gap surfaces loudly in test output instead of silently leaking
// into the shared account.
async function deleteMeals(request: APIRequestContext, cookies: string, ids: string[]): Promise<void> {
  const results = await Promise.allSettled(ids.map(id => deleteMeal(request, cookies, id)));
  const failedIds = results.flatMap((r, i) => (r.status === 'rejected' ? [ids[i]] : []));
  if (failedIds.length > 0) {
    throw new Error(`failed to clean up ${failedIds.length}/${ids.length} meals: ${failedIds.join(', ')}`);
  }
}

async function deleteCustomFood(request: APIRequestContext, cookies: string, id: string) {
  await request.delete(`${BASE_URL}/api/food/custom/${id}`, { headers: { Cookie: cookies } });
}

async function cookieHeader(page: Page): Promise<string> {
  const cookies = await page.context().cookies();
  return cookies.map(c => `${c.name}=${c.value}`).join('; ');
}

test.describe('Manual meal entry', () => {
  test('creates a confirmed meal with direct macros and shows it on review', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);

    await page.goto('/food/manual/');
    await page.getByPlaceholder('Food name').fill('E2E Test Snack');
    await page.getByRole('button', { name: 'Enter macros' }).click();
    await page.locator('label:has-text("Calories") input').fill('250');
    await page.locator('label:has-text("Protein (g)") input').fill('10');

    await page.getByRole('button', { name: 'Save Meal' }).click();
    await page.waitForURL(/\/food\/review\/\?meal=/);

    const url = new URL(page.url());
    const mealId = url.searchParams.get('meal')!;
    expect(mealId).toBeTruthy();

    await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
    await expect(page.getByText('E2E Test Snack')).toBeVisible();
    await expect(page.getByText('250', { exact: true })).toBeVisible();

    await deleteMeal(request, cookies, mealId);
  });
});

test.describe('Custom foods', () => {
  test('add, list, and delete a custom food', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);

    await page.goto('/food/custom/');
    await page.getByRole('button', { name: '+ Add Custom Food' }).click();
    await page.getByLabel('Name').fill('E2E Test Custom Food');
    await page.locator('label:has-text("Calories") input').fill('123');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect(page.getByText('E2E Test Custom Food')).toBeVisible();
    await expect(page.getByText('123 kcal/100g')).toBeVisible();

    // Clean up via the UI's own delete flow (confirms the delete affordance works).
    await page.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('button', { name: 'Confirm' }).click();
    await expect(page.getByText('E2E Test Custom Food')).not.toBeVisible();

    // Best-effort cleanup in case the UI delete assertion above ran before
    // the request settled.
    const listResp = await request.get(`${BASE_URL}/api/food/custom`, { headers: { Cookie: cookies } });
    const foods = await listResp.json();
    for (const f of foods) {
      if (f.name === 'E2E Test Custom Food') await deleteCustomFood(request, cookies, f.id);
    }
  });
});

test.describe('Photo upload', () => {
  test('includes an optional initial hint in the multipart upload', async ({ page }) => {
    await login(page);
    let uploadBody = '';
    await page.route('**/api/food/meals', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      uploadBody = route.request().postData() ?? '';
      return route.fulfill({
        status: 201,
        json: mockFoodMeal({ id: 'hinted-upload', status: 'pending_review' }),
      });
    });
    await page.goto('/food/upload/');
    await page.getByRole('button', { name: 'Add a hint (optional)' }).click();
    await page.getByLabel('Photo hint (optional)').fill('grilled chicken with red beans');
    await page.locator('input[type="file"]').setInputFiles(path.join(__dirname, 'fixtures', 'meal.jpg'));
    await page.waitForURL(/meal=hinted-upload/);
    expect(uploadBody).toContain('name="hint"');
    expect(uploadBody).toContain('grilled chicken with red beans');
  });

  test('rejects an over-limit initial hint before upload', async ({ page }) => {
    await login(page);
    let uploadCalls = 0;
    await page.route('**/api/food/meals', route => {
      if (route.request().method() === 'POST') uploadCalls++;
      return route.continue();
    });
    await page.goto('/food/upload/');
    await page.getByRole('button', { name: 'Add a hint (optional)' }).click();
    await page.getByLabel('Photo hint (optional)').fill(` ${'🙂'.repeat(500)} `);
    await expect(page.getByText('500/500')).toBeVisible();
    await page.getByLabel('Photo hint (optional)').fill('🙂'.repeat(501));
    await page.locator('input[type="file"]').setInputFiles(path.join(__dirname, 'fixtures', 'meal.jpg'));
    await expect(page.getByText('Hint must be at most 500 characters')).toBeVisible();
    expect(uploadCalls).toBe(0);
  });

  test('camera capture uses the same hinted upload path', async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'mediaDevices', {
        configurable: true,
        value: { getUserMedia: async () => ({ getTracks: () => [{ stop: () => {} }] }) },
      });
      Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { configurable: true, get: () => 1 });
      Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { configurable: true, get: () => 1 });
      Object.defineProperty(HTMLMediaElement.prototype, 'srcObject', {
        configurable: true,
        get: () => null,
        set: () => {},
      });
      HTMLCanvasElement.prototype.getContext = (() => ({ drawImage: () => {} })) as typeof HTMLCanvasElement.prototype.getContext;
      HTMLCanvasElement.prototype.toBlob = function (callback) {
        callback(new Blob(['camera'], { type: 'image/jpeg' }));
      };
    });
    await login(page);
    let uploadBody = '';
    await page.route('**/api/food/meals', route => {
      if (route.request().method() !== 'POST') return route.continue();
      uploadBody = route.request().postData() ?? '';
      return route.fulfill({ status: 201, json: mockFoodMeal({ id: 'camera-upload' }) });
    });
    await page.goto('/food/upload/');
    await page.getByRole('button', { name: 'Add a hint (optional)' }).click();
    await page.getByLabel('Photo hint (optional)').fill('red beans');
    await page.getByRole('button', { name: 'Take Photo' }).click();
    await page.getByRole('button', { name: 'Capture' }).click();
    await page.waitForURL(/meal=camera-upload/);
    expect(uploadBody).toContain('name="photo"');
    expect(uploadBody).toContain('red beans');
  });

  test('uploads a photo and reaches a terminal or actionable review state', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);

    await page.goto('/food/upload/');
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles(path.join(__dirname, 'fixtures', 'meal.jpg'));

    // The upload request blocks on the synchronous vision call, so this can
    // take a while; the page navigates to the review route once it returns.
    await page.waitForURL(/\/food\/review\/\?meal=/, { timeout: 90_000 });

    const url = new URL(page.url());
    const mealId = url.searchParams.get('meal')!;
    expect(mealId).toBeTruthy();

    // Recognition on a synthetic (non-food) test image is non-deterministic —
    // assert only that analysis reached a real outcome, not a specific one.
    await expect(
      page.getByText(/Review needed|Needs clarification|Analysis failed/)
    ).toBeVisible({ timeout: 15_000 });

    await deleteMeal(request, cookies, mealId);
  });

  test('API: a photo above nginx default 1MB but within the app limit is accepted, not 413', async ({
    page,
    request,
  }) => {
    // Regression for a real bug: the nginx /api/ location had no
    // client_max_body_size override, so it fell back to nginx's 1MB default
    // and rejected any phone-camera-sized photo before it reached the app,
    // even though the backend itself allows up to 10MB. Pad a valid JPEG
    // (decoders stop at the EOI marker, so trailing bytes don't affect
    // decoding) well past 1MB but under the app's limit.
    await login(page);
    const cookies = await cookieHeader(page);

    const fs = await import('fs');
    const base = fs.readFileSync(path.join(__dirname, 'fixtures', 'meal.jpg'));
    const padded = Buffer.concat([base, Buffer.alloc(3 * 1024 * 1024)]);

    const res = await request.post(`${BASE_URL}/api/food/meals`, {
      headers: { Cookie: cookies },
      multipart: { photo: { name: 'meal.jpg', mimeType: 'image/jpeg', buffer: padded } },
      timeout: 90_000,
    });

    expect(res.status()).not.toBe(413);
    expect(res.status()).toBe(201);
    const meal = await res.json();
    await deleteMeal(request, cookies, meal.id);
  });
});

test.describe('In-app camera capture', () => {
  test('shows a graceful in-page error instead of crashing when no secure-context camera is available', async ({
    page,
  }) => {
    // Regression for a real bug: CameraCapture called
    // navigator.mediaDevices.getUserMedia(...) unconditionally.
    // navigator.mediaDevices is undefined outside a secure context (HTTPS or
    // localhost) — this deployment's plain-HTTP IP origin is exactly that —
    // so the call threw synchronously, before any promise existed for
    // .catch() to handle, crashing the whole page to Next.js's error
    // boundary instead of showing the component's own error state.
    await login(page);
    await page.goto('/food/upload/');
    await page.getByRole('button', { name: 'Take Photo' }).click();

    await expect(page.getByText(/secure.*HTTPS|HTTPS.*secure/i)).toBeVisible({ timeout: 5_000 });
    // The page itself must still be alive and showing the app, not a crash screen.
    await expect(page.getByRole('heading', { name: /log a meal/i })).toBeVisible();
  });
});

// Seeds a confirmed meal directly via the manual-entry API (no vision call,
// deterministic) so these tests can exercise the review-page UI against a
// known starting state without depending on photo recognition.
async function createConfirmedMeal(
  request: APIRequestContext,
  cookies: string,
  name: string,
  itemName: string,
  calories: number
) {
  const res = await request.post(`${BASE_URL}/api/food/meals/manual`, {
    headers: { Cookie: cookies },
    data: { name, items: [{ name: itemName, source: 'manual', weight_grams: 100, calories }] },
  });
  expect(res.status()).toBe(201);
  return res.json();
}

test.describe('Meal history', () => {
  test('a confirmed meal appears in the history list and opens its review page', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const meal = await createConfirmedMeal(request, cookies, 'E2E History Meal', 'Snack', 120);
    try {
      await page.goto('/food/history/');
      await expect(page.getByText('E2E History Meal')).toBeVisible();

      await page.getByText('E2E History Meal').click();
      await page.waitForURL(new RegExp(`meal=${meal.id}`));
      await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
    } finally {
      await deleteMeal(request, cookies, meal.id);
    }
  });

  test('"Load older" fetches and appends a real second page', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);

    // The frontend's history page always requests PAGE_SIZE=50 per fetch, so
    // proving "Load older" appends a *real* second page (not just an empty
    // one) needs more than 50 meals. All manual-entry, no vision calls — 51
    // plain API POSTs is fast. Sequential creation gives each a distinct
    // logged_at (server time at insert), so ordering is deterministic:
    // newest-created meal is 'E2E LoadOlder 50', oldest is 'E2E LoadOlder 0'.
    //
    // This deliberately does NOT assert that "Load older" hides itself
    // after this — this account is shared and reused, and if it already
    // holds ~49+ older meals of its own, page 2 (this test's remaining
    // meal plus however many of the account's own older meals fit) would
    // legitimately still be a full page, keeping the button visible. The
    // short-page-hides-the-button behavior itself is covered deterministically,
    // with a fully isolated mocked dataset, by the next test.
    //
    // Every meal ID goes on this list the instant its creation succeeds, and
    // cleanup runs in `finally` regardless of what happens afterward — a
    // failed assertion, a failed navigation, or a failed creation partway
    // through must not leak meals into this shared, reused account.
    // deleteMeals verifies every delete actually succeeded and attempts all
    // of them even if some fail (Promise.allSettled), rather than a
    // sequential loop that could abandon the rest after one throws.
    const createdIds: string[] = [];
    try {
      for (let i = 0; i < 51; i++) {
        const m = await createConfirmedMeal(request, cookies, `E2E LoadOlder ${i}`, 'Item', 10);
        createdIds.push(m.id);
      }

      await page.goto('/food/history/');
      // Newest (index 50) is on page 1; oldest (index 0) is exactly the
      // 51st meal, so it's the one meal page 1 (limit 50) can't include yet.
      await expect(page.getByText('E2E LoadOlder 50')).toBeVisible();
      await expect(page.getByText('E2E LoadOlder 0')).not.toBeVisible();

      const loadOlder = page.getByRole('button', { name: 'Load older' });
      await expect(loadOlder).toBeVisible();
      await loadOlder.click();

      // The real second page (containing at least the 51st meal) arrives
      // and is appended, not swapped in.
      await expect(page.getByText('E2E LoadOlder 0')).toBeVisible({ timeout: 10_000 });
      await expect(page.getByText('E2E LoadOlder 50')).toBeVisible();
    } finally {
      await deleteMeals(request, cookies, createdIds);
    }
  });

  test('a short page hides "Load older" immediately (mocked, deterministic)', async ({ page }) => {
    await login(page);
    // Isolated from any real account state: a first page far shorter than
    // PAGE_SIZE (50) proves the keyset cursor's own guarantee — it never
    // defers rows, so a short page reliably means nothing more remains —
    // without depending on how much real history this shared account holds.
    const shortPage = Array.from({ length: 3 }, (_, i) => ({
      id: `mock-history-${i}`,
      name: `Mock History Meal ${i}`,
      logged_at: new Date(Date.now() - i * 1000).toISOString(),
      status: 'confirmed',
      calories: 100,
      protein_grams: 10,
      carbs_grams: 10,
      fat_grams: 5,
    }));

    await page.route('**/api/food/meals?*', route => route.fulfill({ json: shortPage }));

    await page.goto('/food/history/');
    await expect(page.getByText('Mock History Meal 0')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Load older' })).not.toBeVisible();
  });

  test('meals from two different days render under separate day headers with correct per-day totals', async ({ page }) => {
    await login(page);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);

    // Two confirmed meals "today" (300 + 150 kcal, 20+10g protein) and one
    // confirmed meal "yesterday" (80 kcal, 5g protein) — chosen so each
    // day's total is distinct from any single meal's own calorie badge,
    // so a test failure that mixed up per-meal vs per-day totals would
    // show up as a wrong number rather than an accidental match.
    const mocked = [
      {
        id: 'mock-day-today-1', name: 'Today Meal 1', logged_at: today.toISOString(), status: 'confirmed',
        calories: 300, protein_grams: 20, carbs_grams: 30, fat_grams: 10,
      },
      {
        id: 'mock-day-today-2', name: 'Today Meal 2', logged_at: today.toISOString(), status: 'confirmed',
        calories: 150, protein_grams: 10, carbs_grams: 15, fat_grams: 5,
      },
      {
        id: 'mock-day-yesterday-1', name: 'Yesterday Meal 1', logged_at: yesterday.toISOString(), status: 'confirmed',
        calories: 80, protein_grams: 5, carbs_grams: 8, fat_grams: 2,
      },
    ];

    await page.route('**/api/food/meals?*', route => route.fulfill({ json: mocked }));

    await page.goto('/food/history/');
    await expect(page.getByText('Today Meal 1')).toBeVisible();
    await expect(page.getByText('Yesterday Meal 1')).toBeVisible();

    // Today's total: 300+150=450 kcal, 20+10=30g protein.
    await expect(page.getByText('450 kcal · P 30g · C 45g · F 15g')).toBeVisible();
    // Yesterday's total: just the one meal.
    await expect(page.getByText('80 kcal · P 5g · C 8g · F 2g')).toBeVisible();
  });

  test('a day with only a non-confirmed meal shows a zero total', async ({ page }) => {
    await login(page);
    const mocked = [
      {
        id: 'mock-pending-1', name: 'Pending Meal', logged_at: new Date().toISOString(), status: 'pending_review',
        calories: 999, protein_grams: 99, carbs_grams: 99, fat_grams: 99,
      },
    ];

    await page.route('**/api/food/meals?*', route => route.fulfill({ json: mocked }));

    await page.goto('/food/history/');
    await expect(page.getByText('Pending Meal')).toBeVisible();
    // The day total must exclude the pending meal's numbers entirely (it
    // has no final nutrition yet), showing zero rather than omitting the
    // total line or leaking the pending meal's provisional values into it.
    await expect(page.getByText('0 kcal · P 0g · C 0g · F 0g')).toBeVisible();
  });

  test('"Load older" merges into an existing day section and adds a new one', async ({ page }) => {
    await login(page);
    const today = new Date();
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);

    // Page 1: 50 confirmed meals, all "today", 10 kcal each (500 kcal total)
    // — a full PAGE_SIZE page so "Load older" is offered. Page 2 (fetched
    // via before/before_id): one more "today" meal (continuing that day's
    // section) plus one "yesterday" meal (a new section).
    const page1 = Array.from({ length: 50 }, (_, i) => ({
      id: `mock-merge-today-${i}`,
      name: `Merge Today ${i}`,
      logged_at: new Date(today.getTime() - i * 1000).toISOString(),
      status: 'confirmed',
      calories: 10, protein_grams: 1, carbs_grams: 1, fat_grams: 1,
    }));
    const page2 = [
      {
        id: 'mock-merge-today-extra', name: 'Merge Today Extra',
        logged_at: new Date(today.getTime() - 51_000).toISOString(), status: 'confirmed',
        calories: 10, protein_grams: 1, carbs_grams: 1, fat_grams: 1,
      },
      {
        id: 'mock-merge-yesterday', name: 'Merge Yesterday',
        logged_at: yesterday.toISOString(), status: 'confirmed',
        calories: 20, protein_grams: 2, carbs_grams: 2, fat_grams: 2,
      },
    ];

    await page.route('**/api/food/meals?*', route => {
      const url = new URL(route.request().url());
      route.fulfill({ json: url.searchParams.has('before') ? page2 : page1 });
    });

    await page.goto('/food/history/');
    await expect(page.getByText('Merge Today 0')).toBeVisible();
    // 50 meals * 10 kcal = 500 kcal for today's section before loading more.
    await expect(page.getByText('500 kcal · P 50g · C 50g · F 50g')).toBeVisible();

    await page.getByRole('button', { name: 'Load older' }).click();

    await expect(page.getByText('Merge Today Extra')).toBeVisible();
    await expect(page.getByText('Merge Yesterday')).toBeVisible();
    // Today's total grows by the extra meal: 500+10=510 kcal.
    await expect(page.getByText('510 kcal · P 51g · C 51g · F 51g')).toBeVisible();
    // A new, separate section for yesterday.
    await expect(page.getByText('20 kcal · P 2g · C 2g · F 2g')).toBeVisible();
  });
});

test.describe('Editing a confirmed meal', () => {
  test('editing, adding, and deleting items keeps the visible total in sync', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const meal = await createConfirmedMeal(request, cookies, 'E2E Edit Meal', 'Base item', 100);
    try {
      await page.goto(`/food/review/?meal=${meal.id}`);
      await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
      await expect(page.getByText('100', { exact: true })).toBeVisible();

      // Editing on a confirmed meal used to be locked (409); it must now
      // work and refresh the visible total immediately. A weight-only
      // change on a manual-source item deliberately leaves calories
      // untouched (it just rescales a reference binding, which manual
      // items don't have), so this exercises a real macro edit via "Change
      // match" -> manual entry instead — the case that actually proves the
      // total updates from an edit.
      await page.getByRole('button', { name: 'Change match' }).click();
      await page.getByRole('button', { name: 'Enter macros' }).click();
      await page.locator('label:has-text("Calories") input').fill('250');
      await page.getByRole('button', { name: 'Save' }).click();
      await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
      await expect(page.getByText('250', { exact: true })).toBeVisible();

      // Add a new item — the total must include it immediately, no reload,
      // and a success toast must confirm the write (ui-notifications spec:
      // "Food Review Mutation Feedback").
      await page.getByRole('button', { name: '+ Add item' }).click();
      await page.getByRole('button', { name: 'Enter macros' }).click();
      await page.locator('label:has-text("Name") input').fill('Extra snack');
      await page.locator('label:has-text("Calories") input').fill('50');
      await page.getByRole('button', { name: 'Save' }).click();

      await expect(page.getByText('Extra snack')).toBeVisible();
      await expect(page.getByText('300', { exact: true })).toBeVisible();
      await expect(page.getByRole('status').filter({ hasText: 'Item added' })).toBeVisible();

      // Delete it again — the total must drop back immediately, with its
      // own distinct toast.
      await page.locator('button[title="Delete item"]').last().click();
      await expect(page.getByText('Extra snack')).not.toBeVisible();
      await expect(page.getByText('250', { exact: true })).toBeVisible();
      await expect(page.getByRole('status').filter({ hasText: 'Item removed' })).toBeVisible();
    } finally {
      await deleteMeal(request, cookies, meal.id);
    }
  });
});

test.describe('Editing a confirmed meal — mocked UI behavior (deterministic)', () => {
  // Regression (round 8): MealMetaEditor always sent logged_at back on save,
  // even for a name-only edit — and the datetime-local input truncates to
  // minute granularity, so that silently dropped the meal's real
  // seconds/fractional-seconds on every save. A meal logged with non-zero
  // seconds is what actually exposes this, hence the specific timestamp
  // below rather than a round one.
  test('a name-only edit does not overwrite logged_at', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal({ logged_at: '2026-01-15T12:34:56.789Z' });
    let patchBody: Record<string, unknown> | null = null;

    await page.route('**/api/food/meals/mock-meal-id', route => {
      if (route.request().method() === 'GET') return route.fulfill({ json: initial });
      if (route.request().method() === 'PATCH') {
        patchBody = route.request().postDataJSON();
        return route.fulfill({ json: { ...initial, name: 'Renamed' } });
      }
      return route.continue();
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Edit name/time' }).click();
    await page.locator('input[type="text"]').fill('Renamed');
    await page.getByRole('button', { name: 'Save', exact: true }).click();

    await expect(page.getByText('Renamed')).toBeVisible();
    expect(patchBody).not.toBeNull();
    expect(patchBody).toEqual({ name: 'Renamed' });
  });

  // Regression (round 8): Cancel only hid the editor; name/loggedAt/error
  // stayed in component state, so reopening showed the canceled draft
  // instead of the meal's actual current values, and a later Save would
  // apply the never-actually-wanted edit.
  test('Cancel discards the draft, even after reopening', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal({ name: 'Original Name' });
    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Edit name/time' }).click();
    await page.locator('input[type="text"]').fill('Discarded Edit');
    await page.getByRole('button', { name: 'Cancel' }).click();

    await page.getByRole('button', { name: 'Edit name/time' }).click();
    await expect(page.locator('input[type="text"]')).toHaveValue('Original Name');
  });

  // Regression (round 8): MealItemRow's weight `useState` only reads its
  // initial value from props once, so a sibling PATCH winning a race against
  // this row's own in-flight weight PATCH (the shipped UI can fire both: a
  // weight change's onBlur, then a bind click, before the first response
  // lands) left the input showing neither the old nor the new value once the
  // losing request's 409 handler ran. The weight PATCH is deliberately
  // delayed here so it resolves *after* the bind PATCH, reproducing the
  // exact interleaving this regression depends on.
  test('a weight edit that loses a race against a bind shows the winner, not a stale value', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const bound = mockFoodMeal({
      items: [{ ...mockFoodMeal().items[0], name: 'New Food', macro_source: 'reference', weight_grams: 200, fdc_id: 99 }],
    });
    // MealItemRow now explicitly refetches on a 409 (round 9 — see the test
    // below for why) rather than trusting the item prop already reflects
    // the winner, so GET must also start returning the bind's result once
    // it's applied, the same way a real backend would.
    let bindApplied = false;

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: bindApplied ? bound : initial }) : route.continue()
    );
    await page.route('**/api/food/search**', route =>
      route.fulfill({
        json: { results: [{ source: 'usda', fdc_id: 99, name: 'New Food', profile: { calories_per_100g: 100, protein_per_100g: 0, carbs_per_100g: 0, fat_per_100g: 0, sugar_per_100g: 0, sodium_per_100g: 0, dietary_fiber_per_100g: 0 } }] },
      })
    );
    await page.route('**/api/food/meals/mock-meal-id/items/item-1', async route => {
      const body = route.request().postDataJSON() as { fdc_id?: number };
      if (body.fdc_id !== undefined) {
        // The bind request: resolves immediately.
        bindApplied = true;
        return route.fulfill({ json: bound });
      }
      // The weight-only request: deliberately delayed so it resolves after
      // the bind above, then rejected — see PatchMealItem's round-7
      // optimistic-concurrency check.
      await new Promise(resolve => setTimeout(resolve, 400));
      return route.fulfill({
        status: 409,
        contentType: 'text/plain',
        body: 'item was modified by another request; reload and try again',
      });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await expect(page.getByText('Old Item')).toBeVisible();

    await page.locator('input[type="number"]').first().fill('150');
    await page.locator('input[type="number"]').first().blur();
    await page.getByRole('button', { name: 'Change match' }).click();
    await page.getByRole('button', { name: 'Search', exact: true }).last().click();
    await page.getByRole('button', { name: /New Food/ }).click();

    await expect(page.getByText('New Food')).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toHaveValue('200');
    // The delayed weight PATCH's 409 lands after the bind — confirm it
    // doesn't revert the winning value once it does.
    await expect(page.getByText('This item was just changed by another edit')).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toHaveValue('200');
  });

  // Regression (round 9): the round-8 fix assumed a 409's winning edit would
  // always come through this same page (via a sibling control's onUpdated
  // call), so the item prop-sync effect would already show the current
  // value by the time the message says so. But the winner might come from
  // another tab, another device, or a direct API call — none of which touch
  // this page's state at all. This test's 409 has no sibling bind response
  // in play; the only way the displayed value can become correct is if the
  // component actively refetches.
  test('a 409 with no local sibling response still refetches to show the current value', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const changedElsewhere = mockFoodMeal({
      items: [{ ...mockFoodMeal().items[0], name: 'Changed In Another Tab', weight_grams: 175 }],
    });
    let conflictReturned = false;

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET'
        ? route.fulfill({ json: conflictReturned ? changedElsewhere : initial })
        : route.continue()
    );
    await page.route('**/api/food/meals/mock-meal-id/items/item-1', route => {
      conflictReturned = true;
      return route.fulfill({
        status: 409,
        contentType: 'text/plain',
        body: 'item was modified by another request; reload and try again',
      });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await expect(page.getByText('Old Item')).toBeVisible();

    await page.locator('input[type="number"]').first().fill('150');
    await page.locator('input[type="number"]').first().blur();

    await expect(page.getByText('This item was just changed by another edit')).toBeVisible();
    // The only source for this is an explicit refetch — nothing else in
    // this test ever supplied the changed-elsewhere state.
    await expect(page.getByText('Changed In Another Tab')).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toHaveValue('175');
  });

  // Regression (round 8): ItemResolver had no in-flight guard at all, so
  // its search-result buttons and manual Save stayed enabled for the entire
  // awaited onBind/onManual call — a double-click sent two POSTs before the
  // first response closed the form, and since each POST allocates its own
  // new item ID, both succeeded and the meal ended up with the item twice.
  // A real double-click can't be replayed faithfully here once the fix is
  // in place — a disabled <button> doesn't dispatch click events in a real
  // browser at all, which is exactly the structural protection the fix
  // relies on — so this asserts the guard directly: the button disables
  // itself synchronously once clicked (before the POST resolves), and
  // exactly one request/item results from the one click.
  test('the search-result button disables itself in flight so it cannot double-submit', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const created = mockFoodMeal({
      items: [...mockFoodMeal().items, { ...mockFoodMeal().items[0], id: 'item-2', name: 'New Item', macro_source: 'reference' }],
    });
    let createCalls = 0;

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/search**', route =>
      route.fulfill({
        json: { results: [{ source: 'usda', fdc_id: 99, name: 'New Item', profile: { calories_per_100g: 100, protein_per_100g: 0, carbs_per_100g: 0, fat_per_100g: 0, sugar_per_100g: 0, sodium_per_100g: 0, dietary_fiber_per_100g: 0 } }] },
      })
    );
    await page.route('**/api/food/meals/mock-meal-id/items', async route => {
      if (route.request().method() !== 'POST') return route.continue();
      createCalls++;
      // Delayed so there's a real window in which a second click, if it
      // could reach the handler at all, would fire a second POST.
      await new Promise(resolve => setTimeout(resolve, 300));
      return route.fulfill({ status: 201, json: created });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: '+ Add item' }).click();
    await page.getByRole('button', { name: 'Search', exact: true }).last().click();

    const result = page.getByRole('button', { name: /New Item/ });
    await result.click();
    await expect(result).toBeDisabled();

    await expect(page.getByText('New Item')).toBeVisible();
    expect(createCalls).toBe(1);
  });

  // Regression (round 9, revised round 10): every mutation control (item
  // rows, add-item, meal meta, reanalyze) independently handed its own
  // response straight to setMeal, with no ordering between them — two
  // edits to different items could both succeed but arrive at the browser
  // out of the order they were issued in, and applying an older-issued,
  // later-arriving response naively moved the UI backward (here:
  // resurrecting an item a second edit already deleted). Round 9's fix
  // compared a ticket assigned at issue time, but a round-10 review found
  // that's not actually safe either: issue order isn't guaranteed to be
  // commit order (a slow first request can still be outstanding when a
  // fast second one lands), so comparing tickets can just as easily
  // discard the response that's genuinely newer. ReviewClient's
  // applyMealUpdate now serializes *issuing* each request instead — the
  // delete below is only actually sent once the weight edit's (slow)
  // request has resolved and applied, so this test's "delay" now models
  // the time a second mutation spends queued rather than a race between
  // two in-flight responses. The delete's mocked response reflects the
  // weight edit that, by the time it fires, has already been applied
  // (weight 175) — the same as what a real backend recomputing from
  // current state would return.
  test('a slow mutation is not overtaken by one requested while it is still in flight', async ({
    page,
  }) => {
    await login(page);
    const twoItems = mockFoodMeal({
      items: [
        { ...mockFoodMeal().items[0], id: 'item-1', name: 'Keep Me', weight_grams: 100 },
        { ...mockFoodMeal().items[0], id: 'item-2', name: 'Delete Me', weight_grams: 50 },
      ],
    });
    const afterWeightEditOnly = mockFoodMeal({
      items: [
        { ...mockFoodMeal().items[0], id: 'item-1', name: 'Keep Me', weight_grams: 175 },
        { ...mockFoodMeal().items[0], id: 'item-2', name: 'Delete Me', weight_grams: 50 },
      ],
    });
    // The delete's response, issued only after the weight edit above has
    // already been applied — so it reflects both changes, matching what a
    // real backend recomputing from current state would return.
    const afterWeightEditAndDelete = mockFoodMeal({
      items: [{ ...mockFoodMeal().items[0], id: 'item-1', name: 'Keep Me', weight_grams: 175 }],
    });

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: twoItems }) : route.continue()
    );
    await page.route('**/api/food/meals/mock-meal-id/items/item-1', async route => {
      await new Promise(resolve => setTimeout(resolve, 400));
      return route.fulfill({ json: afterWeightEditOnly });
    });
    await page.route('**/api/food/meals/mock-meal-id/items/item-2', route =>
      route.request().method() === 'DELETE' ? route.fulfill({ json: afterWeightEditAndDelete }) : route.continue()
    );

    await page.goto('/food/review/?meal=mock-meal-id');
    await expect(page.getByText('Keep Me')).toBeVisible();
    await expect(page.getByText('Delete Me')).toBeVisible();

    const weightInputs = page.locator('input[type="number"]');
    const start = Date.now();
    await weightInputs.first().fill('175');
    await weightInputs.first().blur(); // queues the slow weight PATCH (in flight for 400ms)
    await page.locator('button[title="Delete item"]').last().click(); // queued behind it, not sent yet

    // The delete's own network request must not go out until the weight
    // edit ahead of it in the queue has resolved — asserting on elapsed
    // time (rather than DOM visibility, which could pass either way
    // depending on render timing) is what actually proves the two
    // mutations were serialized rather than raced.
    await page.waitForRequest('**/api/food/meals/mock-meal-id/items/item-2');
    expect(Date.now() - start).toBeGreaterThanOrEqual(350);

    await expect(page.getByText('Delete Me')).not.toBeVisible();
    await expect(page.getByText('Keep Me')).toBeVisible();
    await expect(weightInputs.first()).toHaveValue('175');
  });

  // multilingual-food-search (tasks.md 6.1): a translated search shows what
  // was actually searched for, plus a control to redo the translation.
  test('a translated search shows the translated term and a refresh control', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/search**', route =>
      route.fulfill({
        json: {
          results: [{ source: 'usda', fdc_id: 1, name: 'Oatmeal, raw', profile: { calories_per_100g: 389, protein_per_100g: 17, carbs_per_100g: 66, fat_per_100g: 7, sugar_per_100g: 0, sodium_per_100g: 0, dietary_fiber_per_100g: 10 } }],
          translated_query: 'oatmeal',
        },
      })
    );

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Change match' }).click();
    await page.getByRole('button', { name: 'Search', exact: true }).last().click();

    await expect(page.getByText('Searched as:')).toBeVisible();
    // Scoped to <strong> — the result list below also contains "Oatmeal,
    // raw", and getByText's default substring match is case-insensitive.
    await expect(page.locator('strong', { hasText: 'oatmeal' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Refresh translation' })).toBeVisible();
  });

  // multilingual-food-search (tasks.md 6.2): a refresh that comes back
  // without a translation (the server's fail-open path) must not blank out
  // the last known-good "Searched as" text — it should keep showing it
  // alongside an indication that the refresh itself didn't produce a new one.
  test('a failed refresh keeps the previous translated term visible with an error indication', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    let refreshed = false;

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/search**', route => {
      const isRefresh = route.request().url().includes('refresh=true');
      if (isRefresh) refreshed = true;
      return route.fulfill({
        json: {
          results: [{ source: 'usda', fdc_id: 1, name: 'Oatmeal, raw', profile: { calories_per_100g: 389, protein_per_100g: 17, carbs_per_100g: 66, fat_per_100g: 7, sugar_per_100g: 0, sodium_per_100g: 0, dietary_fiber_per_100g: 10 } }],
          // The first (non-refresh) call reports a translation; the refresh
          // call mimics the server's fail-open path and omits it.
          ...(isRefresh ? {} : { translated_query: 'oatmeal' }),
        },
      });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Change match' }).click();
    await page.getByRole('button', { name: 'Search', exact: true }).last().click();
    await expect(page.getByText('Searched as:')).toBeVisible();

    await page.getByRole('button', { name: 'Refresh translation' }).click();

    expect(refreshed).toBe(true);
    // Still showing the previous term, not blanked out.
    await expect(page.getByText('Searched as:')).toBeVisible();
    await expect(page.locator('strong', { hasText: 'oatmeal' })).toBeVisible();
    await expect(page.getByText(/Could not refresh the translation/)).toBeVisible();
  });
});

// A minimal mocked meal shape ReviewClient/ReanalyzeControl need to render
// and to make canReanalyze true (Boolean(meal.photo_path)). Used by the
// route-mocked tests below, which verify the UI's *own* success/failure
// handling deterministically — no real backend or vision call involved.
function mockFoodMeal(overrides: Record<string, unknown> = {}) {
  return {
    id: 'mock-meal-id',
    photo_path: 'fake/path.jpg',
    status: 'confirmed',
    logged_at: new Date().toISOString(),
    name: 'Mock Meal',
    clarify_round: 0,
    clarify_log: '',
    calories: 300,
    protein_grams: 10,
    carbs_grams: 20,
    fat_grams: 5,
    sugar_grams: 1,
    sodium_grams: 1,
    dietary_fiber_grams: 1,
    items: [
      {
        id: 'item-1',
        meal_id: 'mock-meal-id',
        name: 'Old Item',
        preparation: '',
        state: '',
        macro_source: 'manual',
        weight_grams: 100,
        confidence: 1,
        calories: 300,
        protein_grams: 10,
        carbs_grams: 20,
        fat_grams: 5,
        sugar_grams: 1,
        sodium_grams: 1,
        dietary_fiber_grams: 1,
      },
    ],
    ...overrides,
  };
}

// Open Food Facts source badge (tasks.md 6.6): a real OFF-sourced match
// requires an operator-imported OFF database, which this E2E environment
// does not provision (no import-off has run against the WIP stack). Mocking
// the meal GET response covers the same review-UI behavior deterministically
// instead — MealItemRow's badge is purely a function of which reference
// field the item carries, so a mocked off_code exercises the same rendering
// path a real OFF match would. The brand-gated resolution logic itself
// (which source gets queried, and the fallback rule) is covered at the unit
// level in backend/pkg/server/food_upload_test.go, since it depends on the
// vision Select/Recognize flow this E2E suite does not drive end-to-end.
test.describe('Open Food Facts source badge — mocked UI behavior (deterministic)', () => {
  test('an item bound via off_code shows an Open Food Facts badge, not USDA', async ({ page }) => {
    await login(page);
    const meal = mockFoodMeal({
      items: [
        { ...mockFoodMeal().items[0], macro_source: 'reference', off_code: '8594001222227', brand: 'Olma' },
      ],
    });
    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: meal }) : route.continue()
    );

    await page.goto('/food/review/?meal=mock-meal-id');

    // "Open Food Facts" now appears twice: the item's source badge (a
    // <span>) and the attribution note's link to openfoodfacts.org. Scope to
    // the badge specifically so this doesn't hit a strict-mode violation.
    await expect(page.locator('span', { hasText: 'Open Food Facts' })).toBeVisible();
    await expect(page.getByText('USDA', { exact: true })).not.toBeVisible();
    // The ODbL attribution note only appears when a meal actually has an
    // OFF-sourced item, and per Open Food Facts' reuse terms it must link to
    // Open Food Facts itself, not only to the ODbL license text.
    await expect(page.getByRole('link', { name: 'Open Food Facts' })).toBeVisible();
    await expect(page.getByText('Open Database License (ODbL)')).toBeVisible();
  });

  test('a USDA-bound item shows a USDA badge and no attribution note', async ({ page }) => {
    await login(page);
    const meal = mockFoodMeal({
      items: [{ ...mockFoodMeal().items[0], macro_source: 'reference', fdc_id: 42 }],
    });
    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: meal }) : route.continue()
    );

    await page.goto('/food/review/?meal=mock-meal-id');

    await expect(page.getByText('USDA', { exact: true })).toBeVisible();
    await expect(page.locator('span', { hasText: 'Open Food Facts' })).not.toBeVisible();
    await expect(page.getByRole('link', { name: 'Open Food Facts' })).not.toBeVisible();
    await expect(page.getByText('Open Database License (ODbL)')).not.toBeVisible();
  });
});

test.describe('Reanalyze with a hint — mocked UI behavior (deterministic)', () => {
  // These mock the network response for POST .../reanalyze via
  // page.route(), so they exercise ReanalyzeControl's own success/failure
  // handling deterministically — no real backend state, no vision call, no
  // cost, no dependence on what a synthetic test image happens to produce.
  // The GetMeal response is mocked too, so the control renders regardless
  // of what real meal (if any) 'mock-meal-id' would resolve to.
  test('a successful response updates the displayed meal', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const reanalyzed = mockFoodMeal({
      status: 'pending_review',
      calories: 0,
      protein_grams: 0,
      carbs_grams: 0,
      fat_grams: 0,
      sugar_grams: 0,
      sodium_grams: 0,
      dietary_fiber_grams: 0,
      items: [{ ...mockFoodMeal().items[0], id: 'item-2', name: 'New Item', macro_source: 'none', calories: 0 }],
    });

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/meals/mock-meal-id/reanalyze', route => route.fulfill({ json: reanalyzed }));

    await page.goto('/food/review/?meal=mock-meal-id');
    await expect(page.getByText('Old Item')).toBeVisible();

    await page.getByRole('button', { name: 'Improve analysis' }).click();
    await page.getByLabel('What did the model miss?').fill('this is a different dish entirely');
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

    await expect(page.getByText('New Item')).toBeVisible();
    await expect(page.getByText('Old Item')).not.toBeVisible();
    await expect(page.getByText('Review needed')).toBeVisible();
  });

  test('a 502 response shows the failure message and leaves the displayed meal unchanged', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/meals/mock-meal-id/reanalyze', route =>
      route.fulfill({ status: 502, contentType: 'text/plain', body: 'reanalysis failed; the meal is unchanged' })
    );

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Improve analysis' }).click();
    await page.getByLabel('What did the model miss?').fill('a hint that will be rejected by the mocked backend');
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

    // Asserts on text unique to ReanalyzeControl's custom 502 copy ("You can
    // try again"), not just "Reanalysis failed" — that substring also
    // appears in the raw backend error body, so it would pass even if the
    // instanceof/name check misfired and the code fell through to the
    // generic fallback message. This is what let a real instanceof-under-
    // transpilation bug through undetected until the sibling 412 test (whose
    // assertion was already unique) caught it.
    await expect(page.getByText('You can try again')).toBeVisible();
    // Unchanged: still the original item, still confirmed.
    await expect(page.getByText('Old Item')).toBeVisible();
    await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
  });

  test('a 412 response refetches and shows the current (superseded) state', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const superseded = mockFoodMeal({
      status: 'pending_review',
      items: [{ ...mockFoodMeal().items[0], id: 'item-3', name: 'Superseding Item' }],
    });

    // Any GET before the reanalyze POST fires returns the original meal;
    // any GET after it (the refetch ReanalyzeControl issues on 412) returns
    // what a newer operation — e.g. a concurrent Retry — left behind. Keyed
    // on whether reanalyze has been attempted, not a raw call count, since
    // the exact number of incidental GETs before that point isn't this
    // test's concern. Distinct from the 502 case above: there, no refetch
    // happens at all, since the backend guarantees nothing changed.
    let reanalyzeAttempted = false;
    await page.route('**/api/food/meals/mock-meal-id', route => {
      if (route.request().method() !== 'GET') return route.continue();
      return route.fulfill({ json: reanalyzeAttempted ? superseded : initial });
    });
    await page.route('**/api/food/meals/mock-meal-id/reanalyze', route => {
      reanalyzeAttempted = true;
      return route.fulfill({
        status: 412,
        contentType: 'text/plain',
        body: 'reanalysis failed; the meal was claimed by another operation',
      });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await expect(page.getByText('Old Item')).toBeVisible();

    await page.getByRole('button', { name: 'Improve analysis' }).click();
    await page.getByLabel('What did the model miss?').fill('a hint that will be superseded by the mocked backend');
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

    await expect(page.getByText('Another operation')).toBeVisible();
    // Refetched: now shows the current (superseded) state, not the stale one.
    await expect(page.getByText('Superseding Item')).toBeVisible();
    await expect(page.getByText('Old Item')).not.toBeVisible();
  });

  // The blank-hint validation itself (ReanalyzeControl rejects an empty
  // submission before any network call) needs a meal the control actually
  // renders for — a photo-backed mock, same as the tests above — so it's
  // deterministic and doesn't depend on reaching this point inside the
  // provider-dependent smoke test below, which can skip beforehand.
  test('a blank hint is rejected client-side with no reanalyze request', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    let reanalyzeCalls = 0;

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/meals/mock-meal-id/reanalyze', route => {
      reanalyzeCalls++;
      return route.fulfill({ json: initial });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Improve analysis' }).click();
    // Leave the textarea blank and submit directly.
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

    await expect(page.getByText('A hint is required')).toBeVisible();
    expect(reanalyzeCalls).toBe(0);
  });

  test('expert mode submits names and optional weights separately', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const expertResult = mockFoodMeal({
      status: 'pending_review',
      items: [
        { ...mockFoodMeal().items[0], id: 'expert-1', name: 'Grilled chicken', weight_grams: 180 },
        { ...mockFoodMeal().items[0], id: 'expert-2', name: 'Red beans', weight_grams: 73 },
      ],
    });
    let submitted: unknown;
    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
    );
    await page.route('**/api/food/meals/mock-meal-id/reanalyze', async route => {
      submitted = route.request().postDataJSON();
      return route.fulfill({ json: expertResult });
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Improve analysis' }).click();
    await page.getByRole('tab', { name: 'Expert' }).click();
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();
    await expect(page.getByText('Ingredient 1 needs a name')).toBeVisible();
    expect(submitted).toBeUndefined();

    await page.getByLabel('Ingredient 1').fill('Grilled chicken');
    await page.getByLabel('Grams').fill('0');
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();
    await expect(page.getByText('Ingredient 1 weight must be greater than zero')).toBeVisible();
    expect(submitted).toBeUndefined();

    await page.getByLabel('Grams').fill('180');
    await page.getByRole('button', { name: 'Add ingredient' }).click();
    await page.getByRole('button', { name: 'Remove ingredient' }).last().click();
    await page.getByRole('button', { name: 'Add ingredient' }).click();
    await page.getByLabel('Ingredient 2').fill('Red beans');
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

    expect(submitted).toEqual({
      components: [{ name: 'Grilled chicken', weight_grams: 180 }, { name: 'Red beans' }],
    });
    await expect(page.getByText('Grilled chicken')).toBeVisible();
    await expect(page.getByText('Red beans')).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toHaveValue('180');
  });
});

test.describe('Reanalyze with a hint', () => {
  test('API rejects both correction modes before checking for a photo', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const meal = await createConfirmedMeal(request, cookies, 'E2E Guidance Validation', 'Item', 100);
    try {
      const response = await request.post(`${BASE_URL}/api/food/meals/${meal.id}/reanalyze`, {
        headers: { Cookie: cookies },
        data: { hint: 'rice', components: null },
      });
      expect(response.status()).toBe(400);
      expect(await response.text()).toContain('exactly one');
    } finally {
      await deleteMeal(request, cookies, meal.id);
    }
  });

  // Manual meals have no stored photo, so the control never even renders for
  // them (see canReanalyze in ReviewClient.tsx). Distinct from the blank-hint
  // validation test above: this covers the control being hidden entirely,
  // not what happens once it's open.
  test('the control is hidden entirely for a photo-less manual meal', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const meal = await createConfirmedMeal(request, cookies, 'E2E Reanalyze Validation Meal', 'Item', 100);
    try {
      await page.goto(`/food/review/?meal=${meal.id}`);
      // No photo on a manual meal, so the control isn't offered at all.
      await expect(page.getByRole('button', { name: 'Improve analysis' })).not.toBeVisible();
    } finally {
      await deleteMeal(request, cookies, meal.id);
    }
  });

  // Supplementary live-provider smoke coverage — the deterministic UI
  // success/failure cases are the mocked tests above. This drives the
  // actual ReanalyzeControl component against a real, billed vision call
  // (same cost class as the "Photo upload" tests above) to prove the whole
  // stack still works end to end. Recognition on the synthetic test image
  // is non-deterministic, so this handles both outcomes it can actually
  // produce: a successful reanalysis (items replaced, status left
  // non-confirmed) or a genuine vision-provider hiccup (502, meal left
  // byte-for-byte untouched — checked against items and aggregate, not just
  // status). A forced/simulated vision failure isn't reachable through the
  // live HTTP API; that path has its own deterministic mocked test above,
  // and is also covered at the Go level by
  // TestReanalyze_FailedVisionCallLeavesMealUnchanged.
  test('reanalyzing via the UI either replaces the meal\'s items or leaves it fully untouched', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const fs = await import('fs');
    const photo = fs.readFileSync(path.join(__dirname, 'fixtures', 'meal.jpg'));

    // Reanalyze is eligible from failed/pending_review/confirmed, not
    // pending_clarification. Retry the upload (bounded) rather than
    // implementing a full clarification-answering flow just to seed this.
    // Every candidate this loop creates — even a discarded
    // pending_clarification one — is either deleted immediately or tracked
    // for the outer finally, so nothing leaks regardless of how this ends.
    let meal: { id: string; status: string; items: { name: string }[]; calories: number } | null = null;
    try {
      for (let attempt = 0; attempt < 3 && !meal; attempt++) {
        const uploadRes = await request.post(`${BASE_URL}/api/food/meals`, {
          headers: { Cookie: cookies },
          multipart: { photo: { name: 'meal.jpg', mimeType: 'image/jpeg', buffer: photo } },
          timeout: 90_000,
        });
        expect(uploadRes.status()).toBe(201);
        const candidate = await uploadRes.json();
        if (candidate.status === 'pending_review' || candidate.status === 'failed') {
          meal = candidate;
        } else {
          await deleteMeal(request, cookies, candidate.id);
        }
      }
      test.skip(!meal, 'could not reach a reanalyze-eligible status from the synthetic test image in 3 attempts');
      if (!meal) return;
      const before = meal;

      await page.goto(`/food/review/?meal=${before.id}`);
      await page.getByRole('button', { name: 'Improve analysis' }).click();

      // Blank-hint validation, through the real UI this time (a manual
      // photo-less meal can't reach this control at all — see the earlier
      // test — so this is the only place it's exercised against the real
      // control rather than a mock).
      await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();
      await expect(page.getByText('A hint is required')).toBeVisible();

      await page.getByLabel('What did the model miss?').fill('this is a bowl of rice with grilled chicken');
      await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

      const outcome = page.getByText(/Review needed|Needs clarification|Reanalysis failed/);
      await expect(outcome).toBeVisible({ timeout: 90_000 });

      const afterRes = await request.get(`${BASE_URL}/api/food/meals/${before.id}`, { headers: { Cookie: cookies } });
      const after = await afterRes.json();

      if (await page.getByText('Reanalysis failed').isVisible()) {
        // Non-destructive failure: the meal must be exactly as it was
        // found — not just the same status, but the same items and the
        // same aggregate.
        expect(after.status).toBe(before.status);
        expect((after.items ?? []).map((i: { name: string }) => i.name)).toEqual(
          (before.items ?? []).map(i => i.name)
        );
        expect(after.calories).toBe(before.calories);
      } else {
        expect(after.status).not.toBe('confirmed');
        expect(['pending_review', 'pending_clarification']).toContain(after.status);
      }
    } finally {
      if (meal) await deleteMeal(request, cookies, meal.id);
    }
  });
});
