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
async function deleteMeal(request: APIRequestContext, cookies: string, id: string) {
  await request.delete(`${BASE_URL}/api/data/food_meal/${id}`, { headers: { Cookie: cookies } });
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

    await expect(page.getByText('Confirmed')).toBeVisible();
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
