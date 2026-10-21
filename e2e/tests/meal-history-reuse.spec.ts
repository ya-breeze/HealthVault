import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { BASE_URL } from './helpers/target';
import { login, cookieHeader, getSettings, putSettings, deleteMeals } from './helpers/food-day';

interface Nutrients {
  calories: number; protein_grams: number; fat_grams: number; carbs_grams: number;
  sugar_grams: number; sodium_grams: number; dietary_fiber_grams: number; saturated_fat_grams: number;
}
interface FoodItem extends Nutrients {
  id: string; meal_id: string; name: string; preparation: string; state: string;
  macro_source: string; weight_grams: number; confidence: number;
  history_original_source?: string; history_source_meal_id?: string; history_source_item_id?: string;
}
interface FoodMeal extends Nutrients {
  id: string; name: string; status: string; logged_at: string; clarify_round: number;
  clarify_log: string; photo_path?: string; history_version?: string; items: FoodItem[] | null;
  history_suggestion?: {
    source_meal_id: string; source_name: string; source_logged_at: string;
    mappings: { item_id: string; source_item_id: string; source_name: string }[];
    source_items: { name: string; weight_grams: number; macro_source: string }[];
  };
}

// Repeat uses the real WIP API. Suggestion presentation uses deterministic model
// responses; the backend handler/transport tests exercise real acceptance rules.
test.describe('Recent meal reuse', () => {
  test('repeats a whole meal, scales corrected portions, and confirms independent intake', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const settings = await getSettings(request, cookies);
    const ids: string[] = [];
    try {
      await putSettings(request, cookies, { ...settings, display_language: 'en' });
      const response = await request.post(`${BASE_URL}/api/food/meals/manual`, {
        headers: { Cookie: cookies },
        data: {
          name: `E2E recent meal ${Date.now()}`,
          logged_at: new Date(Date.now() - 60_000).toISOString(),
          items: [
            { name: 'Meat cutlet without bread', source: 'manual', weight_grams: 100,
              calories: 231, protein_grams: 22, fat_grams: 15, carbs_grams: 2,
              sugar_grams: 1, sodium_grams: 0.1, dietary_fiber_grams: 0.5, saturated_fat_grams: 4 },
            { name: 'Vegetables without oil', source: 'manual', weight_grams: 200,
              calories: 77.4, protein_grams: 4, fat_grams: 0.6, carbs_grams: 14,
              sugar_grams: 6, sodium_grams: 0.05, dietary_fiber_grams: 5, saturated_fat_grams: 0.1 },
          ],
        },
      });
      expect(response.status()).toBe(201);
      const source: FoodMeal = await response.json();
      ids.push(source.id);

      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto('/food/history/');
      const repeat = page.getByTestId(`repeat-meal-${source.id}`);
      await expect(repeat).toBeVisible();
      const bounds = await repeat.boundingBox();
      expect(bounds?.height).toBeGreaterThanOrEqual(48);
      const created = page.waitForResponse(r => r.url().endsWith(`/api/food/meals/${source.id}/repeat`) && r.request().method() === 'POST');
      await repeat.click();
      const repeatResponse = await created;
      expect(repeatResponse.ok()).toBeTruthy();
      const draft: FoodMeal = await repeatResponse.json();
      ids.push(draft.id);
      const replay = await request.post(`${BASE_URL}/api/food/meals/${source.id}/repeat`, {
        headers: { Cookie: cookies }, data: repeatResponse.request().postDataJSON(),
      });
      expect(replay.ok()).toBeTruthy();
      expect((await replay.json()).id).toBe(draft.id);
      expect(draft.id).not.toBe(source.id);
      expect(draft.status).toBe('pending_review');
      expect(draft.calories).toBe(0);
      expect(draft.photo_path).toBeFalsy();
      expect(draft.items).toHaveLength(2);
      expect(draft.items?.every(i => i.macro_source === 'history' && i.history_original_source === 'manual')).toBe(true);
      await page.waitForURL(`**/food/review/?meal=${draft.id}`);
      const cutlet = draft.items!.find(i => i.name === 'Meat cutlet without bread')!;
      const nutrients = page.getByTestId(`item-nutrients-${cutlet.id}`);
      const row = nutrients.locator('..');
      const weight = row.locator('input[type="number"]').first();
      await weight.fill('150');
      const scaled = page.waitForResponse(r => r.url().endsWith(`/items/${cutlet.id}`) && r.request().method() === 'PATCH');
      await weight.blur();
      const scaledMeal: FoodMeal = await (await scaled).json();
      const scaledCutlet = scaledMeal.items!.find(i => i.id === cutlet.id)!;
      expect(scaledCutlet.protein_grams).toBeCloseTo(33);
      expect(scaledCutlet.fat_grams).toBeCloseTo(22.5);
      expect(scaledCutlet.carbs_grams).toBeCloseTo(3);
      expect(scaledCutlet.sodium_grams).toBeCloseTo(0.15);
      expect(scaledCutlet.dietary_fiber_grams).toBeCloseTo(0.75);
      await expect(weight).toHaveValue('150');
      await page.getByRole('button', { name: 'Confirm Meal', exact: true }).click();
      await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
      const finalResponse = await request.get(`${BASE_URL}/api/food/meals/${draft.id}`, { headers: { Cookie: cookies } });
      expect(finalResponse.ok()).toBeTruthy();
      const confirmed: FoodMeal = await finalResponse.json();
      expect(confirmed.status).toBe('confirmed');
      expect(confirmed.calories).toBeCloseTo(423.9);
      expect(confirmed.protein_grams).toBeCloseTo(37);
      expect(confirmed.carbs_grams).toBeCloseTo(17);
      const originalResponse = await request.get(`${BASE_URL}/api/food/meals/${source.id}`, { headers: { Cookie: cookies } });
      const original: FoodMeal = await originalResponse.json();
      expect(original.items).toEqual(source.items);
      expect(original.calories).toBeCloseTo(308.4);
    } finally {
      await deleteMeals(request, cookies, ids);
      await putSettings(request, cookies, settings);
    }
  });

  for (const action of ['apply', 'dismiss'] as const) {
    test(`photo suggestion requires explicit ${action} and preserves fresh portion weights`, async ({ page, request }) => {
      await login(page);
      const cookies = await cookieHeader(page);
      const settings = await getSettings(request, cookies);
      await putSettings(request, cookies, { ...settings, display_language: 'en' });
      try {
        const item: FoodItem = {
          id: 'suggestion-target-item', meal_id: 'suggestion-target', name: 'Meat cutlet',
          preparation: 'fried', state: 'cooked', macro_source: 'estimated', weight_grams: 150,
          confidence: 0.8, calories: 375, protein_grams: 22.5, fat_grams: 22.5,
          carbs_grams: 30, sugar_grams: 1.5, sodium_grams: 0.15,
          dietary_fiber_grams: 1.5, saturated_fat_grams: 6,
        };
        const initial: FoodMeal = {
          id: 'suggestion-target', name: 'New photo', status: 'pending_review',
          logged_at: new Date().toISOString(), clarify_round: 0, clarify_log: '',
          calories: 0, protein_grams: 0, carbs_grams: 0, fat_grams: 0, sugar_grams: 0,
          sodium_grams: 0, dietary_fiber_grams: 0, saturated_fat_grams: 0,
          history_version: 'initial-history-version', items: [item],
          history_suggestion: {
            source_meal_id: 'suggestion-source', source_name: 'Yesterday homemade lunch',
            source_logged_at: new Date(Date.now() - 86_400_000).toISOString(),
            mappings: [{ item_id: item.id, source_item_id: 'source-cutlet', source_name: 'Meat cutlet without bread' }],
            source_items: [{ name: 'Meat cutlet without bread', weight_grams: 100, macro_source: 'manual' }],
          },
        };
        let mutationCount = 0;
        await page.route('**/api/food/meals/suggestion-target', route => route.fulfill({ json: initial }));
        await page.route('**/api/food/meals/suggestion-target/history', async route => {
          mutationCount++;
          expect(route.request().postDataJSON()).toEqual({ action, expected_version: initial.history_version });
          const changed: FoodMeal = {
            ...initial, history_suggestion: undefined, history_version: 'resolved-version',
            items: action === 'apply' ? [{ ...item, macro_source: 'history',
              history_original_source: 'manual', history_source_meal_id: 'suggestion-source',
              history_source_item_id: 'source-cutlet', calories: 346.5, protein_grams: 33, carbs_grams: 3 }] : [item],
          };
          await route.fulfill({ json: changed });
        });
        await page.goto('/food/review/?meal=suggestion-target');
        await expect(page.getByTestId('history-suggestion')).toContainText('Yesterday homemade lunch');
        await expect(page.getByTestId('history-suggestion')).toContainText('Meat cutlet without bread');
        expect(mutationCount).toBe(0);
        const row = page.getByTestId(`item-nutrients-${item.id}`).locator('..');
        await expect(row.locator('input[type="number"]').first()).toHaveValue('150');
        await page.getByTestId(`history-suggestion-${action}`).click();
        await expect(page.getByTestId('history-suggestion')).toHaveCount(0);
        expect(mutationCount).toBe(1);
        await expect(row.locator('input[type="number"]').first()).toHaveValue('150');
        await expect(row).toContainText(action === 'apply' ? '33.0' : '22.5');
      } finally {
        await putSettings(request, cookies, settings);
      }
    });
  }

  test('accepting history waits for a portion save and uses its resulting version', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const settings = await getSettings(request, cookies);
    await putSettings(request, cookies, { ...settings, display_language: 'en' });
    let releaseWeight: () => void = () => {};
    const weightGate = new Promise<void>(resolve => { releaseWeight = resolve; });
    try {
      const item: FoodItem = {
        id: 'queued-item', meal_id: 'queued-history', name: 'Cutlet', preparation: 'fried', state: 'cooked',
        macro_source: 'estimated', weight_grams: 100, confidence: 0.8,
        calories: 250, protein_grams: 15, fat_grams: 15, carbs_grams: 20,
        sugar_grams: 1, sodium_grams: 0.1, dietary_fiber_grams: 1, saturated_fat_grams: 4,
      };
      const initial: FoodMeal = {
        id: item.meal_id, name: 'Queued photo', status: 'pending_review', logged_at: new Date().toISOString(),
        clarify_round: 0, clarify_log: '', calories: 0, protein_grams: 0, carbs_grams: 0,
        fat_grams: 0, sugar_grams: 0, sodium_grams: 0, dietary_fiber_grams: 0, saturated_fat_grams: 0,
        history_version: 'before-weight', items: [item],
        history_suggestion: {
          source_meal_id: 'queued-source', source_name: 'Saved lunch', source_logged_at: new Date().toISOString(),
          mappings: [{ item_id: item.id, source_item_id: 'queued-source-item', source_name: 'Meat cutlet without bread' }],
          source_items: [{ name: 'Meat cutlet without bread', weight_grams: 100, macro_source: 'manual' }],
        },
      };
      const weighted: FoodMeal = { ...initial, history_version: 'after-weight', items: [{ ...item, weight_grams: 150 }] };
      const order: string[] = [];
      await page.route('**/api/food/meals/queued-history', route => route.fulfill({ json: initial }));
      await page.route('**/api/food/meals/queued-history/items/queued-item', async route => {
        order.push('weight');
        await weightGate;
        await route.fulfill({ json: weighted });
      });
      await page.route('**/api/food/meals/queued-history/history', async route => {
        order.push('history');
        expect(route.request().postDataJSON()).toEqual({ action: 'apply', expected_version: 'after-weight' });
        await route.fulfill({ json: { ...weighted, history_suggestion: undefined, history_version: 'accepted' } });
      });
      await page.goto('/food/review/?meal=queued-history');
      const weight = page.getByTestId('item-nutrients-queued-item').locator('..').locator('input[type="number"]').first();
      await weight.fill('150');
      // The click blurs the weight input before queueing history acceptance.
      await page.getByTestId('history-suggestion-apply').click();
      await expect(page.getByTestId('history-suggestion-apply')).toBeDisabled();
      await expect.poll(() => order).toEqual(['weight']);
      releaseWeight();
      await expect(page.getByTestId('history-suggestion')).toHaveCount(0);
      expect(order).toEqual(['weight', 'history']);
      await expect(weight).toHaveValue('150');
    } finally {
      releaseWeight();
      await putSettings(request, cookies, settings);
    }
  });
});

// Opt in explicitly: this exercises the deployed model adapter and incurs a
// real provider request. It uses the repository's synthetic meal illustration.
test.describe('Recent meal reuse with deployed photo recognition', () => {
  test.skip(process.env.HCW_LIVE_AI_E2E !== '1', 'Set HCW_LIVE_AI_E2E=1 for provider-backed integration');
  for (const action of ['apply', 'dismiss'] as const) {
    test(`real photo suggestion ${action} reaches the deployed history endpoint`, async ({ page, request }) => {
      test.setTimeout(180_000);
      await login(page);
      const cookies = await cookieHeader(page);
      const settings = await getSettings(request, cookies);
      const ids: string[] = [];
      try {
        await putSettings(request, cookies, { ...settings, display_language: 'en' });
        const name = `E2E home patty batch ${Date.now()}`;
        const sourceResponse = await request.post(`${BASE_URL}/api/food/meals/manual`, {
          headers: { Cookie: cookies },
          data: { name, logged_at: new Date(Date.now() - 60_000).toISOString(), items: [
            { name: 'Homemade meat patty without bread', source: 'manual', weight_grams: 100,
              calories: 231, protein_grams: 22, fat_grams: 15, carbs_grams: 2 },
            { name: 'Cucumber without oil', source: 'manual', weight_grams: 50,
              calories: 8, protein_grams: 0.3, fat_grams: 0.05, carbs_grams: 1.8 },
            { name: 'Cherry tomato', source: 'manual', weight_grams: 20,
              calories: 4, protein_grams: 0.2, fat_grams: 0.04, carbs_grams: 0.8 },
          ] },
        });
        expect(sourceResponse.status()).toBe(201);
        const source: FoodMeal = await sourceResponse.json();
        ids.push(source.id);
        const photo = await readFile(path.join(__dirname, 'fixtures/meal.jpg'));
        const uploaded = await request.post(`${BASE_URL}/api/food/meals`, {
          headers: { Cookie: cookies }, timeout: 120_000,
          multipart: {
            photo: { name: 'synthetic-meal.jpg', mimeType: 'image/jpeg', buffer: photo },
            hint: `This illustration represents a homemade meat patty without bread, cucumber without oil and a cherry tomato. These are the same prepared foods as my recent confirmed meal "${name}". Recognize each plated food and estimate its current weight.`,
          },
        });
        expect(uploaded.status()).toBe(201);
        let meal: FoodMeal = await uploaded.json();
        ids.push(meal.id);
        await expect(async () => {
          const response = await request.get(`${BASE_URL}/api/food/meals/${meal.id}`, { headers: { Cookie: cookies } });
          meal = await response.json();
          expect(meal.status).not.toBe('processing');
        }).toPass({ timeout: 90_000 });
        expect(meal.status).toBe('pending_review');
        expect(meal.history_suggestion?.source_meal_id).toBe(source.id);
        expect(meal.history_suggestion!.mappings.length).toBeGreaterThan(0);
        const before = meal;
        await page.goto(`/food/review/?meal=${meal.id}`);
        await expect(page.getByTestId('history-suggestion')).toContainText(name);
        const saved = page.waitForResponse(r => r.url().endsWith(`/api/food/meals/${meal.id}/history`) && r.request().method() === 'POST');
        await page.getByTestId(`history-suggestion-${action}`).click();
        expect((await saved).status()).toBe(200);
        await expect(page.getByTestId('history-suggestion')).toHaveCount(0);
        const afterResponse = await request.get(`${BASE_URL}/api/food/meals/${meal.id}`, { headers: { Cookie: cookies } });
        const after: FoodMeal = await afterResponse.json();
        expect(after.status).toBe('pending_review');
        if (action === 'dismiss') {
          expect(after.items).toEqual(before.items);
        } else {
          for (const mapping of before.history_suggestion!.mappings) {
            const original = source.items!.find(i => i.id === mapping.source_item_id)!;
            const oldTarget = before.items!.find(i => i.id === mapping.item_id)!;
            const reused = after.items!.find(i => i.id === mapping.item_id)!;
            expect(reused.weight_grams).toBe(oldTarget.weight_grams);
            expect(reused.macro_source).toBe('history');
            expect(reused.history_original_source).toBe('manual');
            expect(reused.calories).toBeCloseTo(original.calories * reused.weight_grams / original.weight_grams);
            expect(reused.carbs_grams).toBeCloseTo(original.carbs_grams * reused.weight_grams / original.weight_grams);
          }
        }
      } finally {
        await deleteMeals(request, cookies, ids);
        await putSettings(request, cookies, settings);
      }
    });
  }
});
