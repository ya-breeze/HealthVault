import { test, expect, Page } from '@playwright/test';
const OUT = process.env.SHOT_DIR!;
async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill('alice');
  await page.getByPlaceholder(/password/i).fill('pass1');
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}
test('dashboard+history ru', async ({ page }) => {
  await login(page);
  await page.selectOption('#display-language', 'ru');
  await page.waitForTimeout(1500);
  await page.goto('/');
  await page.waitForTimeout(3000);
  await page.screenshot({ path: `${OUT}/10-dashboard-ru.png`, fullPage: true });
  await page.getByRole('button', { name: /Изменить порядок/i }).click();
  await page.waitForTimeout(800);
  await page.screenshot({ path: `${OUT}/11-dashboard-ru-edit.png`, fullPage: true });
  await page.goto('/food/history/');
  await page.waitForTimeout(2500);
  await page.screenshot({ path: `${OUT}/12-history-ru.png`, fullPage: true });
  await page.goto('/');
  await page.selectOption('#display-language', 'en');
  await page.waitForTimeout(1500);
});
