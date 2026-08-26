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
    // Display Language moved off the header into /settings (see
    // user-profile-and-nutrition-target's design.md); the header control in
    // its place is this icon-only link to /settings, so it's what needs
    // covering here now — the existing assertions above enumerate header
    // controls by name, so a newly added one is not covered until it is named
    // here. See openspec/specs/mobile-touch-targets "Header and toast
    // controls meet the minimum".
    await assertMinTapTarget(page.getByTitle('Settings'), 'header Settings link');
  });

  test('the relocated Display Language control on /settings meets the 48px minimum', async ({ page }) => {
    await login(page);
    await page.goto('/settings');
    // This one shipped at 44px in its original header location — kept as its
    // own assertion after the move so that regression coverage follows the
    // control rather than being lost along with its old spot in the header.
    await assertMinTapTarget(page.locator('#display-language'), 'settings language select');
  });

  test('the Profile form controls on /settings meet the 48px minimum', async ({ page }) => {
    await login(page);
    await page.goto('/settings');
    // mobile-touch-targets' "Settings page controls meet the minimum"
    // scenario covers the whole Profile form, not just Display Language —
    // these three share the same TapTarget wrapper, so this is the
    // regression guard for the rest of it.
    await assertMinTapTarget(page.getByLabel('Birthdate'), 'settings birthdate input');
    await assertMinTapTarget(page.getByLabel('Sex'), 'settings sex select');
    await assertMinTapTarget(page.getByLabel('Activity level'), 'settings activity level select');
  });

  test('the Expert Mode toggle meets the 48px minimum', async ({ page }) => {
    await login(page);
    await page.goto('/food/custom/');
    // The checkbox itself stays visually small; the label around it is the
    // clickable surface and is what has to meet the minimum.
    await assertMinTapTarget(
      page.locator('label:has([data-testid="expert-mode-toggle"])'),
      'Expert Mode toggle'
    );
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

// Mocks every /api/data/<type> read the data detail page makes, so the record
// table always renders a fixed set of rows. The page defaults to a `week`
// zoom whose range is only the last 7 days, so a case relying on real records
// would start skipping — silently, and permanently — as soon as the chosen
// type stopped receiving recent data. See data-page-tap-targets/design.md.
async function mockDataRecords(page: Page, type: string, rows: Record<string, unknown>[]) {
  await page.route('**/api/data/**', route => {
    const url = new URL(route.request().url());
    // Bucketed reads feed the chart only; an empty result there is explicitly
    // non-fatal in DataTypeClient, and the chart is not what's being measured.
    if (url.searchParams.has('bucket')) return route.fulfill({ json: [] });
    return route.fulfill({ json: url.pathname.endsWith(`/data/${type}`) ? rows : [] });
  });
}

function weightRow(id: string, kg: number, daysAgo: number) {
  const at = new Date(Date.now() - daysAgo * 86_400_000).toISOString();
  return { id, kilograms: kg, time: at, created_at: at, updated_at: at };
}

// Covers the data detail route, which the original mobile-tap-targets change
// scoped out entirely — see openspec/specs/mobile-touch-targets "Data detail
// record delete control meets the minimum" and its three sibling scenarios.
test.describe('Mobile tap targets — data detail page', () => {
  test.use({ viewport: MOBILE_VIEWPORT });

  test('record delete, its confirmation, and the zoom tabs meet the 48px minimum', async ({ page }) => {
    await login(page);
    await mockDataRecords(page, 'weight', [
      weightRow('rec-1', 80.5, 1),
      weightRow('rec-2', 80.1, 2),
    ]);

    await page.goto('/data/weight/');

    const deleteControls = page.getByRole('button', { name: 'Delete record' });
    await expect(deleteControls.first()).toBeVisible();
    await expect(deleteControls).toHaveCount(2);
    await assertMinTapTarget(deleteControls.first(), 'record delete control');

    for (const z of ['Day', 'Week', 'Month', 'Year']) {
      await assertMinTapTarget(page.getByRole('button', { name: z, exact: true }), `${z} zoom tab`);
    }

    // Activating one row's delete swaps that cell into its inline
    // confirm/cancel state; both replacements need the minimum too.
    await deleteControls.first().click();
    const confirm = page.getByRole('button', { name: 'Confirm', exact: true });
    const cancel = page.getByRole('button', { name: 'Cancel', exact: true });
    await expect(confirm).toBeVisible();
    await assertMinTapTarget(confirm, 'delete confirm control');
    await assertMinTapTarget(cancel, 'delete cancel control');
  });

  test('nutrition macro tabs meet the 48px minimum', async ({ page }) => {
    await login(page);
    await mockDataRecords(page, 'nutrition', []);

    await page.goto('/data/nutrition/');

    for (const m of ['Calories', 'Protein', 'Carbs', 'Fat', 'Sugar', 'Sodium', 'Fiber']) {
      await assertMinTapTarget(page.getByRole('button', { name: m, exact: true }), `${m} macro tab`);
    }
  });
});
