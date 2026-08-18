import { test, expect, type Page, type Locator } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';

const MOBILE_VIEWPORT = { width: 375, height: 667 };
const MIN_TAP_TARGET = 48;

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

// Single item by default, with macro_source not 'none' — MealItemRow only
// auto-opens its own ItemResolver when macro_source is 'none', so this keeps
// exactly one ItemResolver mounted at a time. The test below relies on that
// to address its exact-'Search' submit button unambiguously.
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

async function assertMinTapTarget(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} should have a bounding box`).not.toBeNull();
  expect(box!.width, `${label} width`).toBeGreaterThanOrEqual(MIN_TAP_TARGET);
  expect(box!.height, `${label} height`).toBeGreaterThanOrEqual(MIN_TAP_TARGET);
}

// Spot-checks a representative sample of controls from the mobile-tap-targets
// change (openspec/changes/mobile-tap-targets) against the 48x48 CSS px
// minimum (Android Material Design, matching the maintainer's test device).
// This is a regression guard on the controls that were the actual motivating
// examples (item delete "x", search-result rows, meal-meta edit, header nav,
// toast dismiss) — NOT exhaustive coverage of every interactive control the
// change's spec requirement names "every interactive control" for. Full-flow
// coverage is manual, on a real Android device, per tasks.md 6.4.
test.describe('Mobile tap targets — review page', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('delete control, search-result row, and meal-meta edit meet the 48px minimum', async ({ page }) => {
    await login(page);
    const meal = mockFoodMeal();

    await page.route('**/api/food/meals/mock-meal-id', route =>
      route.request().method() === 'GET' ? route.fulfill({ json: meal }) : route.continue()
    );
    await page.route('**/api/food/search**', route =>
      route.fulfill({
        json: {
          results: [
            {
              source: 'usda',
              fdc_id: 99,
              name: 'Search Result Food',
              profile: {
                calories_per_100g: 100, protein_per_100g: 0, carbs_per_100g: 0, fat_per_100g: 0,
                sugar_per_100g: 0, sodium_per_100g: 0, dietary_fiber_per_100g: 0,
              },
            },
          ],
        },
      })
    );

    await page.goto('/food/review/?meal=mock-meal-id');
    await expect(page.getByText('Old Item')).toBeVisible();

    await assertMinTapTarget(page.locator('button[title="Delete item"]'), 'delete control');
    await assertMinTapTarget(page.getByRole('button', { name: 'Edit name/time' }), 'meal-meta edit control');

    await page.getByRole('button', { name: 'Change match' }).click();
    await page.getByRole('button', { name: 'Search', exact: true }).click();
    const result = page.getByRole('button', { name: /Search Result Food/ });
    await expect(result).toBeVisible();
    await assertMinTapTarget(result, 'search-result picker row');
  });
});

test.describe('Mobile tap targets — header and toast', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('header nav controls meet the 48px minimum', async ({ page }) => {
    await login(page);
    await page.goto('/food/history/');

    await assertMinTapTarget(page.getByRole('link', { name: 'Custom Foods' }), 'header Custom Foods link');
    await assertMinTapTarget(page.getByRole('link', { name: 'Import' }), 'header Import link');
    await assertMinTapTarget(page.getByRole('button', { name: 'Logout' }), 'header Logout button');
  });

  test('toast dismiss control meets the 48px minimum', async ({ page }) => {
    await login(page);
    const initial = mockFoodMeal();
    const updated = mockFoodMeal({ name: 'Renamed Meal' });
    let patched = false;

    await page.route('**/api/food/meals/mock-meal-id', route => {
      if (route.request().method() === 'GET') return route.fulfill({ json: patched ? updated : initial });
      if (route.request().method() === 'PATCH') {
        patched = true;
        return route.fulfill({ json: updated });
      }
      return route.continue();
    });

    await page.goto('/food/review/?meal=mock-meal-id');
    await page.getByRole('button', { name: 'Edit name/time' }).click();
    await page.locator('input[type="text"]').fill('Renamed Meal');
    await page.getByRole('button', { name: 'Save', exact: true }).click();

    const toast = page.getByRole('status').filter({ hasText: 'Meal updated' });
    await expect(toast).toBeVisible();
    await assertMinTapTarget(toast.getByRole('button', { name: 'Dismiss notification' }), 'toast dismiss control');
  });
});
