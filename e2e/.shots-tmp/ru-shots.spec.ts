import { test, expect, Page } from '@playwright/test';

const BASE_URL = process.env.BASE_URL!;
const OUT = process.env.SHOT_DIR!;
const USER = process.env.HV_USER || 'alice';
const PASS = process.env.HV_PASS || 'pass1';

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

async function setLang(page: Page, code: 'en' | 'ru') {
  await page.selectOption('#display-language', code);
  // The switch persists via PUT and then re-renders; wait for the header's own
  // label to settle rather than racing the network.
  await page.waitForTimeout(1500);
}

// A confirmed meal with Cyrillic Display Names and English Canonical Names —
// the exact shape a Russian recognition produces. Mocked so the capture does
// not depend on what happens to be in the seeded account.
const RU_MEAL = {
  id: 'shot-meal', photo_path: 'fake.jpg', status: 'confirmed',
  logged_at: new Date().toISOString(), name: 'Завтрак',
  clarify_round: 0, clarify_log: '',
  calories: 640, protein_grams: 28.4, carbs_grams: 71.2, fat_grams: 24.6,
  sugar_grams: 12.1, sodium_grams: 1.2, dietary_fiber_grams: 4.3,
  items: [
    { id: 'i1', meal_id: 'shot-meal', name: 'вареники с картошкой', canonical_name: 'dumplings with potato',
      preparation: '', state: '', weight_grams: 220, calories: 396, protein_grams: 11.2,
      carbs_grams: 55.4, fat_grams: 13.8, sugar_grams: 2.1, sodium_grams: 0.6,
      dietary_fiber_grams: 2.8, macro_source: 'estimated', fdc_id: null,
      custom_food_id: null, off_code: null },
    { id: 'i2', meal_id: 'shot-meal', name: 'сметана', canonical_name: 'sour cream',
      preparation: '', state: '', weight_grams: 60, calories: 122, protein_grams: 1.7,
      carbs_grams: 2.4, fat_grams: 11.9, sugar_grams: 2.4, sodium_grams: 0.1,
      dietary_fiber_grams: 0, macro_source: 'estimated', fdc_id: null,
      custom_food_id: null, off_code: null },
    { id: 'i3', meal_id: 'shot-meal', name: 'чёрный хлеб', canonical_name: 'rye bread',
      preparation: '', state: '', weight_grams: 55, calories: 122, protein_grams: 15.5,
      carbs_grams: 13.4, fat_grams: 0.9, sugar_grams: 7.6, sodium_grams: 0.5,
      dietary_fiber_grams: 1.5, macro_source: 'estimated', fdc_id: null,
      custom_food_id: null, off_code: null },
  ],
};

async function mockMeal(page: Page) {
  await page.route('**/api/food/meals/shot-meal', route =>
    route.request().method() === 'GET'
      ? route.fulfill({ json: RU_MEAL })
      : route.continue());
  await page.route('**/api/food/search?**', route =>
    route.fulfill({ json: { results: [
      { source: 'custom', custom_food_id: 'c1', fdc_id: null, off_code: null,
        name: 'блины', canonical_name: 'pancakes',
        profile: { calories_per_100g: 227, protein_per_100g: 6.1, carbs_per_100g: 28,
          fat_per_100g: 9.6, sugar_per_100g: 5, sodium_per_100g: 0.4, dietary_fiber_per_100g: 1 } },
      { source: 'custom', custom_food_id: 'c2', fdc_id: null, off_code: null,
        name: 'борщ', canonical_name: 'borscht',
        profile: { calories_per_100g: 58, protein_per_100g: 1.6, carbs_per_100g: 6.7,
          fat_per_100g: 2.8, sugar_per_100g: 2, sodium_per_100g: 0.3, dietary_fiber_per_100g: 1.4 } },
    ] } }));
}

test('capture', async ({ page }) => {
  await login(page);

  // English first, same screen, for an honest before/after on the one screen
  // where the font question is decidable.
  await setLang(page, 'en');
  await mockMeal(page);
  await page.goto('/food/review/?meal=shot-meal');
  await expect(page.getByText('вареники с картошкой')).toBeVisible();
  await page.screenshot({ path: `${OUT}/01-review-en.png`, fullPage: true });

  await setLang(page, 'ru');

  await page.goto('/');
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${OUT}/02-dashboard-ru.png`, fullPage: true });

  await page.goto('/food/review/?meal=shot-meal');
  await expect(page.getByText('вареники с картошкой')).toBeVisible();
  await page.screenshot({ path: `${OUT}/03-review-ru.png`, fullPage: true });

  // Expert Mode on: Canonical Names revealed under each Display Name.
  await page.getByTestId('expert-mode-toggle').check();
  await expect(page.getByText('dumplings with potato')).toBeVisible();
  await page.screenshot({ path: `${OUT}/04-review-ru-expert.png`, fullPage: true });

  // The resolver panel, which is where round 10's unrendered canonical_name lived.
  await page.getByRole('button', { name: /change match|сменить/i }).first().click();
  await page.getByRole('button', { name: /^(search|найти)$/i }).first().click();
  await expect(page.getByText('блины')).toBeVisible();
  await page.screenshot({ path: `${OUT}/05-resolver-ru-expert.png`, fullPage: true });

  await page.goto('/food/history/');
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${OUT}/06-history-ru.png`, fullPage: true });

  await page.goto('/food/custom/');
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${OUT}/07-custom-ru.png`, fullPage: true });

  await page.goto('/food/upload/');
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `${OUT}/08-upload-ru.png`, fullPage: true });

  // Leave the account as we found it.
  await page.goto('/');
  await setLang(page, 'en');
});
