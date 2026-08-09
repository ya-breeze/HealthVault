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
    }));

    await page.route('**/api/food/meals?*', route => route.fulfill({ json: shortPage }));

    await page.goto('/food/history/');
    await expect(page.getByText('Mock History Meal 0')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Load older' })).not.toBeVisible();
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

      // Add a new item — the total must include it immediately, no reload.
      await page.getByRole('button', { name: '+ Add item' }).click();
      await page.getByRole('button', { name: 'Enter macros' }).click();
      await page.locator('label:has-text("Name") input').fill('Extra snack');
      await page.locator('label:has-text("Calories") input').fill('50');
      await page.getByRole('button', { name: 'Save' }).click();

      await expect(page.getByText('Extra snack')).toBeVisible();
      await expect(page.getByText('300', { exact: true })).toBeVisible();

      // Delete it again — the total must drop back immediately.
      await page.locator('button[title="Delete item"]').last().click();
      await expect(page.getByText('Extra snack')).not.toBeVisible();
      await expect(page.getByText('250', { exact: true })).toBeVisible();
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

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: initial }) : route.continue()
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
    await page.getByRole('button', { name: 'Search', exact: true }).click();
    await page.getByRole('button', { name: /New Food/ }).click();

    await expect(page.getByText('New Food')).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toHaveValue('200');
    // The delayed weight PATCH's 409 lands after the bind — confirm it
    // doesn't revert the winning value once it does.
    await expect(page.getByText('This item was just changed by another edit')).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toHaveValue('200');
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
    await page.getByRole('button', { name: 'Search', exact: true }).click();

    const result = page.getByRole('button', { name: /New Item/ });
    await result.click();
    await expect(result).toBeDisabled();

    await expect(page.getByText('New Item')).toBeVisible();
    expect(createCalls).toBe(1);
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

    await page.getByRole('button', { name: 'Reanalyze with a hint' }).click();
    await page.locator('textarea').fill('this is a different dish entirely');
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
    await page.getByRole('button', { name: 'Reanalyze with a hint' }).click();
    await page.locator('textarea').fill('a hint that will be rejected by the mocked backend');
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

    await page.getByRole('button', { name: 'Reanalyze with a hint' }).click();
    await page.locator('textarea').fill('a hint that will be superseded by the mocked backend');
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
    await page.getByRole('button', { name: 'Reanalyze with a hint' }).click();
    // Leave the textarea blank and submit directly.
    await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();

    await expect(page.getByText('A hint is required')).toBeVisible();
    expect(reanalyzeCalls).toBe(0);
  });
});

test.describe('Reanalyze with a hint', () => {
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
      await expect(page.getByRole('button', { name: 'Reanalyze with a hint' })).not.toBeVisible();
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
      await page.getByRole('button', { name: 'Reanalyze with a hint' }).click();

      // Blank-hint validation, through the real UI this time (a manual
      // photo-less meal can't reach this control at all — see the earlier
      // test — so this is the only place it's exercised against the real
      // control rather than a mock).
      await page.getByRole('button', { name: 'Reanalyze', exact: true }).click();
      await expect(page.getByText('A hint is required')).toBeVisible();

      await page.locator('textarea').fill('this is a bowl of rice with grilled chicken');
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
