import { test, expect, type Page } from '@playwright/test';

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

// Runs `action` and waits for the settings PUT it triggers to actually come
// back from the server, rather than only for the click/selectOption that
// starts it. Every settings writer in the app (the language switcher, the
// dashboard-order editor, and the Profile form) does a GET-then-PUT via
// api.updateSettings, so the UI-level interaction resolves well before the
// write lands; a test that ends there has its context torn down with the
// request still in flight, and the change it thought it had made — or, in a
// `finally`, un-made — may never reach the shared seeded account. Best-effort
// by design: this is only ever used for cleanup or to serialize test steps,
// so a missing response times out quietly instead of masking the assertion
// failure that triggered it. Duplicated from dashboard.spec.ts, following
// this suite's existing per-file convention (see also `login` above).
async function withSettingsSave(page: Page, action: () => Promise<unknown>): Promise<boolean> {
  const saved = page
    .waitForResponse(
      r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
      { timeout: 15_000 }
    )
    .then(r => r.ok())
    .catch(() => false);
  await action().catch(() => {});
  return saved;
}

// Shared cleanup for tests that reorder the vitals grid: puts Weight back
// where PRIMARY_METRICS has it. Duplicated from dashboard.spec.ts (see that
// file's copy for the full history of why each step is shaped this way);
// still needed here because this file's own reorder tests move Weight to the
// front the same way dashboard.spec.ts's do.
async function restoreDefaultOrder(page: Page) {
  const customizeBtn = page.getByRole('button', { name: 'Customize' });
  await customizeBtn.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => null);
  if (!(await customizeBtn.isVisible().catch(() => false))) return;
  await customizeBtn.click().catch(() => {});
  const moveWeightDown = page.getByRole('button', { name: /move weight down/i });
  for (let i = 0; i < 8; i++) {
    if (await moveWeightDown.isDisabled().catch(() => true)) break;
    await moveWeightDown.click().catch(() => {});
  }
  const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
  for (let i = 0; i < 2; i++) {
    if (await moveWeightUp.isDisabled().catch(() => true)) break;
    await moveWeightUp.click().catch(() => {});
  }
  await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
}

// Shared cleanup for tests that hide/show cards: un-hides every card that is
// currently hidden and saves. Duplicated from dashboard.spec.ts; still needed
// here because this file's beforeEach preserves that file's original
// "repair a leaked hidden card before asserting on grid contents" guard for
// the moved 'Settings lost-update race' describe block.
async function restoreAllVisible(page: Page) {
  const customizeBtn = page.getByRole('button', { name: 'Customize' });
  const doneBtn = page.getByRole('button', { name: 'Done' });

  await expect(customizeBtn.or(doneBtn)).toBeEnabled({ timeout: 10_000 }).catch(() => {});

  const enteredEditMode = !(await doneBtn.isVisible().catch(() => false));
  if (enteredEditMode) {
    if (!(await customizeBtn.isEnabled().catch(() => false))) return;
    await customizeBtn.click({ timeout: 5_000 }).catch(() => {});
    await expect(doneBtn).toBeVisible({ timeout: 5_000 }).catch(() => {});
  }

  const hiddenToggles = page.locator('[data-hidden="true"] [data-testid$="-visibility"]');
  let restored = 0;
  for (let i = 0; i < 20; i++) {
    if ((await hiddenToggles.count().catch(() => 0)) === 0) break;
    await hiddenToggles.first().click().catch(() => {});
    restored++;
  }

  if (restored === 0 && enteredEditMode) {
    await page.goto('/').catch(() => {});
    return;
  }
  if (await doneBtn.isVisible().catch(() => false)) {
    await withSettingsSave(page, () => doneBtn.click());
  }
}

test.describe('Settings lost-update race', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    // Repair any hidden card leaked by a failed visibility test before
    // asserting on grid contents: every test below assumes the full grid, and
    // a leaked hidden card fails them for reasons unrelated to what they test.
    await restoreAllVisible(page);
  });

  // Regression for a lost-update race fixed in LanguageContext.tsx/page.tsx:
  // the dashboard-order editor and the language switcher each kept an
  // independent cached UserSettings and PUT'd a read-modify-write built from
  // it, with no shared store between them. Saving a reorder and then
  // switching language used to PUT a stale pre-reorder snapshot and silently
  // clobber the just-saved dashboard_order. Display Language now lives on
  // `/settings` rather than the dashboard header, so reaching it after a
  // dashboard reorder requires a navigation — updated from the original
  // same-page version of this test accordingly.
  test('reordering cards then switching language after navigating to /settings persists both', async ({ page }) => {
    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
      for (let i = 0; i < 8; i++) {
        if (await moveWeightUp.isDisabled()) break;
        await moveWeightUp.click();
      }
      await withSettingsSave(page, () => page.getByRole('button', { name: 'Done' }).click());
      await expect(page.getByRole('button', { name: 'Customize' })).toBeVisible();

      await page.getByTitle('Settings').click();
      await expect(page).toHaveURL(/\/settings/);

      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('ru')
      );
      await expect(page.locator('#display-language')).toHaveValue('ru');

      await page.reload();
      await expect(page.locator('#display-language')).toHaveValue('ru');

      await page.goto('/');
      const firstCardAfter = page.getByTestId('vitals-grid').locator('> *').first();
      await expect(firstCardAfter).toHaveAttribute('data-testid', 'vital-card-weight');
    } finally {
      // Restore English + default order so later tests (which assert on
      // English label text and a predictable card order) aren't affected,
      // even if an assertion above failed partway through. Navigates to
      // /settings itself if the failure happened before reaching it.
      if (!page.url().includes('/settings')) {
        await page.goto('/settings').catch(() => {});
      }
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('en')
      );
      await expect(page.locator('#display-language')).toHaveValue('en').catch(() => {});
      await page.goto('/');
      await restoreDefaultOrder(page);
    }
  });

  // Distinct from the test above: this exercises the same race but with the
  // profile form as the second writer instead of the language switcher, and
  // deliberately without any navigation between the two writes — both live on
  // /settings, so "no navigation" is reachable again here even though it no
  // longer is on the dashboard. Regression target: LanguageContext's
  // claim()-based serial queue (see design.md's "third settings writer"
  // decision) is what's supposed to make the profile form's
  // useLanguage().updateSettings() and the language switcher's own
  // GET/PUT safe to interleave in the same session.
  test('saving the profile form and switching language in the same session persists both', async ({ page }) => {
    await page.goto('/settings');
    try {
      await page.getByLabel('Birthdate').fill('1990-05-15');
      await page.getByLabel('Sex').selectOption('female');

      const profileSaved = await withSettingsSave(page, () =>
        page.getByRole('button', { name: 'Save' }).click()
      );
      expect(profileSaved).toBe(true);

      // No navigation here — the point under test.
      const languageSaved = await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('ru')
      );
      expect(languageSaved).toBe(true);
      await expect(page.locator('#display-language')).toHaveValue('ru');

      await page.reload();
      await expect(page.getByLabel('Birthdate')).toHaveValue('1990-05-15');
      await expect(page.getByLabel('Sex')).toHaveValue('female');
      await expect(page.locator('#display-language')).toHaveValue('ru');
    } finally {
      await withSettingsSave(page, () =>
        page.locator('#display-language').selectOption('en')
      );
      await expect(page.locator('#display-language')).toHaveValue('en').catch(() => {});
    }
  });

  // Regression target: page.tsx's dashboard reorder used to be able to lose
  // its PUT if the component that started it unmounted before the response
  // arrived. LanguageProvider's write queue is mounted at the root layout
  // (not per-page), so it should survive the component that enqueued the
  // write going away underneath it — proven here by navigating away via a
  // client-side (not full-page) link click before the write's response has
  // been observed.
  test('a dashboard reorder made just before navigating to /settings survives the trip', async ({ page }) => {
    await page.goto('/');
    await restoreAllVisible(page);
    try {
      await page.getByRole('button', { name: 'Customize' }).click();
      const moveWeightUp = page.getByRole('button', { name: /move weight up/i });
      for (let i = 0; i < 8; i++) {
        if (await moveWeightUp.isDisabled()) break;
        await moveWeightUp.click();
      }
      await expect(page.getByTestId('vitals-grid').locator('> *').first())
        .toHaveAttribute('data-testid', 'vital-card-weight');

      // Start watching for the PUT before triggering it, then navigate away
      // client-side (Link, not page.goto) without waiting for the response —
      // if the write only lived on the dashboard page's own component tree,
      // this navigation would have a chance to lose it.
      const putLanded = page.waitForResponse(
        r => r.url().includes('/api/users/me/settings') && r.request().method() === 'PUT',
        { timeout: 15_000 }
      );
      await page.getByRole('button', { name: 'Done' }).click();
      await page.getByTitle('Settings').click();
      await expect(page).toHaveURL(/\/settings/);
      const response = await putLanded;
      expect(response.ok()).toBe(true);

      await page.goto('/');
      const firstCardAfter = page.getByTestId('vitals-grid').locator('> *').first();
      await expect(firstCardAfter).toHaveAttribute('data-testid', 'vital-card-weight');
    } finally {
      if (!page.url().endsWith('/')) {
        await page.goto('/').catch(() => {});
      }
      await restoreDefaultOrder(page);
    }
  });
});

test.describe('Profile form', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
    await page.goto('/settings');
  });

  test('rejects save with a validation error when birthdate/sex are missing', async ({ page }) => {
    // Explicitly clear both required fields rather than relying on a blank
    // account: an earlier test in this file (or a previous run) may have
    // already saved a profile for the shared seeded account, and the loaded
    // values would otherwise satisfy the very validation this test checks.
    // The Sex <select> has a real '' "Select…" option to return to, unlike a
    // typical required select.
    await page.getByLabel('Birthdate').fill('');
    await page.getByLabel('Sex').selectOption('');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Birthdate and sex are required.')).toBeVisible();
  });

  test('saves birthdate, sex, and an activity override, and reloads with them populated', async ({ page }) => {
    await page.getByLabel('Birthdate').fill('1985-03-20');
    await page.getByLabel('Sex').selectOption('male');
    await page.getByLabel('Activity level').selectOption('active');

    const saved = await withSettingsSave(page, () => page.getByRole('button', { name: 'Save' }).click());
    expect(saved).toBe(true);
    await expect(page.getByText('Profile saved')).toBeVisible();

    await page.reload();
    await expect(page.getByLabel('Birthdate')).toHaveValue('1985-03-20');
    await expect(page.getByLabel('Sex')).toHaveValue('male');
    await expect(page.getByLabel('Activity level')).toHaveValue('active');
  });
});
