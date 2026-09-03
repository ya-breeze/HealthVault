import { test, expect, type Locator } from '@playwright/test';
import {
  cookieHeader,
  createMealAt,
  daySection,
  deleteMeal,
  expectOnFirstHistoryPage,
  getSettings,
  isoAtUTC,
  login,
  putSettings,
  thresholdLeavingDayBelow,
  unconfirmDate,
  waitForLanguage,
} from './helpers/food-day';

// The two widths the mobile audit measured this page at. 390x844 is a current
// phone; 320x568 is the narrowest viewport the project supports, and it is
// where the header failed even on days carrying no completeness control.
const VIEWPORTS = [
  { name: '390x844', width: 390, height: 844 },
  { name: '320x568', width: 320, height: 568 },
] as const;

// Russian is the binding case, not a secondary one: "Mark day complete"
// translates to "Отметить день заполненным", which is the widest single unit
// the day header has to hold. An English-only test passes against the layout
// that is broken today.
const LANGUAGES = ['en', 'ru'] as const;

// Distinguishes this run's seeded meals from an earlier run's leftovers.
const RUN_TAG = Math.random().toString(36).slice(2, 8);

// Seeded one day back, not three. The account is shared and holds ~17 meals on
// some days, so three days back can already sit past the history page's first
// page of 50 — which surfaces as an opaque visibility timeout rather than as
// anything about layout. expectOnFirstHistoryPage checks this rather than
// trusting it.
const DAYS_AGO = 1;

/** True when the two boxes share any area at all — as mobile-nav.spec.ts. */
function intersects(a: NonNullable<Awaited<ReturnType<Locator['boundingBox']>>>,
                    b: NonNullable<Awaited<ReturnType<Locator['boundingBox']>>>) {
  return a.x < b.x + b.width && b.x < a.x + a.width
    && a.y < b.y + b.height && b.y < a.y + a.height;
}

async function boxOf(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} should have a bounding box`).not.toBeNull();
  return box!;
}

/**
 * How many lines an element's own text renders on.
 *
 * This is the measurement that separates a header that fits from the one on
 * `main`: at 390px the date rendered as `среда,` / `26 авг.` — 2 — and the
 * macro summary wrapped around the completeness pill, splitting `У` from `0г`.
 * Bounding-box intersection alone does not catch that: the wrapped fragments
 * sit beside the pill rather than under it, so their boxes stay disjoint while
 * the header still reads as three interleaved pieces.
 *
 * Counts distinct rendered line boxes via a Range rather than dividing box
 * height by line-height. The height-based version could not tell padding from a
 * second line: the `День заполнен` badge carries `py-0.5`, so its 16px line box
 * inside a 20px border box measured 1.3 on a badge that renders on one line.
 * The same rounding could equally have swallowed a real second line on a padded
 * element, so this is about a wrong answer, not just a noisy one.
 */
async function lineCount(locator: Locator): Promise<number> {
  return locator.evaluate(el => {
    const range = document.createRange();
    range.selectNodeContents(el);
    const rects = Array.from(range.getClientRects()).filter(r => r.width > 0 && r.height > 0);
    // Distinct top edges, not raw rect count: one line holding runs of
    // different sizes (a bold number beside muted units) yields several rects
    // that all share a line box. Rounded, because those runs can sit at
    // sub-pixel offsets from each other.
    return new Set(rects.map(r => Math.round(r.top))).size;
  });
}

test.describe('Meal history layout', () => {
  // The header case walks four viewport x language combinations plus a
  // confirmed-branch pass, each a settings write and a full page load, and the
  // row case walks two. Both exceed the config's 30s default without being slow
  // in any way worth fixing.
  test.describe.configure({ timeout: 120_000 });

  test('the day header holds date, completeness control and totals without collision', async ({
    page,
    request,
  }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);

    // The UTC baseline goes first, before anything derived from it. The
    // completeness endpoint resolves its from/to in the caller's *stored* zone,
    // so an occasion count read under whatever zone another spec happened to
    // leave behind describes a different local day — the threshold then lands
    // wrong and the "Mark day complete" control this test needs is legitimately
    // absent. completeness.spec.ts orders it this way for the same reason.
    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    const date = isoAtUTC(DAYS_AGO, 12).slice(0, 10);
    await expectOnFirstHistoryPage(request, cookies, date);

    const mealName = `E2E History Header ${RUN_TAG}`;
    const meal = await createMealAt(request, cookies, mealName, isoAtUTC(DAYS_AGO, 12));

    try {
      // The day must render its "Mark day complete" control: that control is
      // the widest unit in the row and the one that cannot shrink, since it is
      // a TapTarget bound by the 48x48 minimum PR #36 established. Unconfirmed
      // after the timezone write, not before — changing the zone hard-deletes
      // FoodDayCompletion rows, so the earlier ordering was a no-op.
      await unconfirmDate(request, cookies, date);
      const threshold = await thresholdLeavingDayBelow(request, cookies, date);

      for (const lang of LANGUAGES) {
        await putSettings(request, cookies, {
          ...original,
          timezone: 'UTC',
          display_language: lang,
          usual_meals_per_day: threshold,
        });

        for (const vp of VIEWPORTS) {
          await test.step(`${vp.name} / ${lang}`, async () => {
            await page.setViewportSize({ width: vp.width, height: vp.height });
            await page.goto('/food/history/');
            // Before any measurement. LanguageProvider renders English first and
            // swaps only once its settings GET resolves, and every locator below
            // is language-agnostic — the meal name is ASCII and the control is
            // addressed by role — so without this the Russian pass would measure
            // the English render and report a false pass on the binding case.
            await waitForLanguage(page, lang);

            const section = daySection(page, mealName);
            await expect(section).toBeVisible({ timeout: 15_000 });

            const header = section.getByTestId('day-header');
            // Addressed by role rather than by name: the accessible name is
            // "Mark day complete" in English and "Отметить день заполненным"
            // in Russian, and the unconfirmed day renders exactly one button.
            const control = header.getByRole('button');
            await expect(control).toBeVisible({ timeout: 10_000 });
            // The binding case must actually render in Russian. waitForLanguage
            // makes that true; this asserts it, so if that wait ever stops
            // working the run fails here instead of quietly degrading the ru
            // pass into a second English one that measures a shorter label.
            await expect(control).toHaveText(
              lang === 'ru' ? 'Отметить день заполненным' : 'Mark day complete'
            );

            const label = section.locator('h2');
            const calories = section.getByTestId('day-total-calories');
            const macros = section.getByTestId('day-total-macros');

            // Nothing in the header may overlap anything else in it.
            const [lb, cb, calb, macb] = [
              await boxOf(label, 'day label'),
              await boxOf(control, 'completeness control'),
              await boxOf(calories, 'day calories'),
              await boxOf(macros, 'day macros'),
            ];
            expect(intersects(lb, cb), 'date must not overlap the completeness control').toBe(false);
            expect(intersects(lb, calb), 'date must not overlap the calories total').toBe(false);
            expect(intersects(lb, macb), 'date must not overlap the macro totals').toBe(false);
            expect(intersects(cb, calb), 'completeness control must not overlap the calories total').toBe(false);
            expect(intersects(cb, macb), 'completeness control must not overlap the macro totals').toBe(false);

            // Each unit renders whole, on one line of its own.
            expect(await lineCount(label), 'the date must not break across lines').toBe(1);
            expect(await lineCount(calories), 'the calories total must not wrap').toBe(1);
            expect(await lineCount(macros), 'the macro totals must not wrap').toBe(1);

            // The control is asserted by its computed white-space rather than
            // by line count: it is a TapTarget, so its box is 48px tall by
            // guarantee and a height-based measure would read three lines of
            // 16px text however the label actually renders. `nowrap` is the
            // property that makes the control drop whole onto its own line
            // instead of re-wrapping inside its pill.
            await expect(control).toHaveCSS('white-space', 'nowrap');

            // The header must not scroll the page sideways at any width.
            const overflow = await header.evaluate(el => el.scrollWidth - el.clientWidth);
            expect(overflow, 'the header must not overflow its own width').toBeLessThanOrEqual(1);
          });
        }
      }

      // The control has a second branch the loop above can never reach, because
      // it unconfirms before every pass: a day already marked complete renders a
      // `День заполнен` badge beside an `Отменить` TapTarget. That is what a user
      // sees for every day they have confirmed, and `whitespace-nowrap` was added
      // to it too. Asserted where it is tightest — 320px, Russian.
      await test.step('320x568 / ru, a day already marked complete', async () => {
        await page.setViewportSize({ width: 320, height: 568 });
        await page.goto('/food/history/');
        await waitForLanguage(page, 'ru');

        const section = daySection(page, mealName);
        await expect(section).toBeVisible({ timeout: 15_000 });
        const header = section.getByTestId('day-header');
        await header.getByRole('button').click();

        const badge = header.getByText('День заполнен', { exact: true });
        await expect(badge).toBeVisible({ timeout: 10_000 });
        const undo = header.getByRole('button');
        await expect(undo).toHaveText('Отменить');

        const label = section.locator('h2');
        const [lb, bb, ub] = [
          await boxOf(label, 'day label'),
          await boxOf(badge, 'complete badge'),
          await boxOf(undo, 'unconfirm control'),
        ];
        expect(intersects(lb, bb), 'date must not overlap the complete badge').toBe(false);
        expect(intersects(lb, ub), 'date must not overlap the unconfirm control').toBe(false);
        expect(intersects(bb, ub), 'the badge must not overlap the unconfirm control').toBe(false);

        expect(await lineCount(label), 'the date must not break across lines').toBe(1);
        expect(await lineCount(badge), 'the complete badge must not wrap').toBe(1);
        await expect(badge).toHaveCSS('white-space', 'nowrap');
        await expect(undo).toHaveCSS('white-space', 'nowrap');

        const overflow = await header.evaluate(el => el.scrollWidth - el.clientWidth);
        expect(overflow, 'the header must not overflow its own width').toBeLessThanOrEqual(1);
      });
    } finally {
      await deleteMeal(request, cookies, meal.id);
      await unconfirmDate(request, cookies, date);
      await putSettings(request, cookies, original);
    }
  });

  test('a meal row is identified by its time alone, in the account timezone', async ({ page, request }) => {
    await login(page);
    const cookies = await cookieHeader(page);
    const original = await getSettings(request, cookies);

    await putSettings(request, cookies, { ...original, timezone: 'UTC' });

    const date = isoAtUTC(DAYS_AGO, 23).slice(0, 10);
    await expectOnFirstHistoryPage(request, cookies, date);

    const mealName = `E2E History Row ${RUN_TAG}`;
    // 23:00 UTC: the hour most likely to expose a timestamp rendered in the
    // browser's zone instead of the account's, since any positive offset moves
    // it into the following calendar day and away from the header above it.
    const meal = await createMealAt(request, cookies, mealName, isoAtUTC(DAYS_AGO, 23));

    try {
      await page.setViewportSize({ width: 390, height: 844 });

      for (const lang of LANGUAGES) {
        await putSettings(request, cookies, { ...original, timezone: 'UTC', display_language: lang });
        await page.goto('/food/history/');
        await waitForLanguage(page, lang);

        await test.step(lang, async () => {
          const section = daySection(page, mealName);
          await expect(section).toBeVisible({ timeout: 15_000 });

          const meta = section.locator('a').filter({ hasText: mealName }).getByTestId('meal-meta');
          const text = (await meta.innerText()).trim();

          // Time only: no calendar date, no seconds. The day header above the
          // row already names the day.
          expect(text, 'the row must lead with a time').toMatch(/^\d{1,2}:\d{2}\b/);
          expect(text, 'the row must not repeat the year').not.toMatch(/\d{4}/);
          expect(text, 'the row must not carry seconds').not.toMatch(/\d{1,2}:\d{2}:\d{2}/);

          // The time must be the account's zone — the zone the day grouping
          // used — not the browser's. Asserted in Russian only: English falls
          // back to the *runner's* default locale (dateLocaleFor returns
          // undefined for 'en'), so its 12-hour rendering of 23:00 UTC depends
          // on where the test runs, while 'ru' is a fixed 24-hour locale and
          // gives exactly "23:00" for a UTC account anywhere.
          if (lang === 'ru') {
            expect(text, 'the time must be rendered in the account timezone').toMatch(/^23:00\b/);
          }
        });
      }
    } finally {
      await deleteMeal(request, cookies, meal.id);
      await putSettings(request, cookies, original);
    }
  });
});
