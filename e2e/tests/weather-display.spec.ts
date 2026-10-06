import { test, expect, type Page } from '@playwright/test';

const USER = process.env.HCW_USER || 'alice';
const PASS = process.env.HCW_PASS || 'pass1';
const units = {
  temperature_c: '°C', apparent_temperature_c: '°C', relative_humidity_percent: '%',
  surface_pressure_hpa: 'hPa', mean_sea_level_pressure_hpa: 'hPa',
  precipitation_mm: 'mm', wind_speed_kmh: 'km/h',
};

function recentDate(daysAgo = 1): string {
  return new Date(Date.now() - daysAgo * 86_400_000).toISOString().slice(0, 10);
}

function hour(date: string, time: string, temperature: number) {
  return {
    hour: `${date}T${time}:00:00Z`, temperature_c: temperature,
    apparent_temperature_c: temperature - 1, relative_humidity_percent: 62,
    surface_pressure_hpa: 1008.2, mean_sea_level_pressure_hpa: 1012.4,
    precipitation_mm: 0.3, wind_speed_kmh: 12.7,
    latitude: 50.1, longitude: 14.4, source: 'open-meteo historical forecast',
    model: 'ecmwf_ifs025', fetched_at: `${date}T23:00:00Z`,
  };
}

function fixture(date = recentDate()) {
  // Deliberately unsorted. The latest saved hour must be selected by its
  // timestamp, not by whichever row happens to be last in the response.
  return {
    hours: [hour(date, '08', -9), hour(date, '13', 23.5), hour(date, '09', 11), hour(date, '12', 18)],
    gaps: [
      { from: `${date}T10:00:00Z`, to: `${date}T10:15:00Z`, reason: 'missing_location' },
      { from: `${date}T10:15:00Z`, to: `${date}T11:45:00Z`, reason: 'moving' },
      { from: `${date}T11:45:00Z`, to: `${date}T12:00:00Z`, reason: 'missing_location' },
    ],
    units,
  };
}

async function mockSettings(page: Page) {
  let settings: Record<string, unknown> = { display_language: 'en', timezone: 'UTC', dashboard_order: [] };
  await page.route('**/api/users/me/settings', async route => {
    if (route.request().method() === 'PUT') settings = route.request().postDataJSON();
    await route.fulfill({ json: settings });
  });
  // The dashboard can use its combined read model. Keep nutrition/presence
  // from the deployed backend, but isolate this test's preferences as well.
  await page.route('**/api/dashboard**', async route => {
    const response = await route.fetch();
    if (!response.ok()) { await route.fulfill({ response }); return; }
    const body = await response.json();
    await route.fulfill({ response, json: { ...body, settings: { status: 'ok', value: settings } } });
  });
}

async function login(page: Page) {
  await page.goto('/login/');
  await page.getByPlaceholder(/username/i).fill(USER);
  await page.getByPlaceholder(/password/i).fill(PASS);
  await page.getByRole('button', { name: /sign in|login/i }).click();
  await page.waitForURL('/');
}

test.describe('saved weather display', () => {
  test.beforeEach(async ({ page }) => { await mockSettings(page); });

  test('compact dashboard selects the newest unsorted hour and opens its matching day', async ({ page }) => {
    const date = recentDate();
    await page.route('**/api/weather/history**', route => route.fulfill({ json: fixture(date) }));
    await login(page);
    const card = page.getByTestId('weather-card');
    await expect(card).toBeVisible();
    await expect(card).toContainText('23.5');
    await expect(card).toContainText('°C');
    await expect(card).toContainText(/1,?008\.2/);
    await expect(card).toContainText('hPa');
    await expect(card).toContainText(/saved/i);
    const expectedTime = await page.evaluate(value => new Date(value).toLocaleTimeString(undefined, {
      hour: '2-digit', minute: '2-digit', timeZone: 'UTC',
    }), `${date}T13:00:00Z`);
    await expect(card).toContainText(expectedTime);
    const details = page.getByRole('link', { name: 'Weather details', exact: true });
    const href = await details.getAttribute('href');
    expect(href).toBeTruthy();
    const destination = new URL(href!, page.url());
    expect(destination.pathname).toBe('/weather/day/');
    expect(destination.searchParams.get('date')).toBe(date);
    expect(destination.searchParams.get('timezone')).toBe('UTC');
    await details.click();
    await expect(page.getByTestId('weather-day')).toBeVisible();
    await expect(page.getByTestId('weather-date')).toHaveValue(date);
  });

  test('day charts leave missing hours disconnected and list partial travel coverage', async ({ page }) => {
    const date = recentDate();
    await page.route('**/api/weather/history**', route => route.fulfill({ json: fixture(date) }));
    await login(page);
    await page.goto(`/weather/day/?date=${date}&timezone=UTC`);
    const day = page.getByTestId('weather-day');
    await expect(day).toBeVisible();
    await expect(day.getByTestId('weather-hour')).toHaveCount(4);
    const savedHours = day.getByTestId('weather-hours');
    await expect(savedHours).toHaveJSProperty('open', false);
    await expect(day.getByTestId('weather-hour').first()).toBeHidden();
    await expect(page.getByTestId('weather-gaps')).toContainText(/mov|travel/i);
    await expect(page.getByTestId('weather-gaps')).toContainText('10:15');
    await expect(page.getByTestId('weather-gaps')).toContainText('11:45');
    // Each fixture has two adjacent pairs separated by absent 10/11 UTC
    // points. A single SVG subpath would fabricate continuity across travel.
    for (const testId of ['weather-temperature-chart', 'weather-pressure-chart']) {
      const chart = page.getByTestId(testId);
      await expect(chart).toBeVisible();
      const curve = chart.locator('.recharts-line-curve').first();
      await expect(curve).toHaveAttribute('d', /M.*M/);
    }
    await savedHours.locator(':scope > summary').click();
    await expect(savedHours).toHaveJSProperty('open', true);
    const firstHour = day.getByTestId('weather-hour').first();
    await expect(firstHour).toBeVisible();
    await expect(firstHour).toContainText('62');
    await expect(firstHour).toContainText('12.7');
    await expect(firstHour).toContainText('km/h');
    await expect(firstHour).toContainText('0.3');
    await expect(firstHour).toContainText('mm');
    const provenance = firstHour.locator('details');
    const rawModel = provenance.getByText(/ecmwf_ifs025/);
    await expect(provenance).toHaveJSProperty('open', false);
    await expect(rawModel).toBeHidden();
    await provenance.locator(':scope > summary').click();
    await expect(provenance).toHaveJSProperty('open', true);
    await expect(rawModel).toBeVisible();
  });

  test('day picker requests a different local day and labels Android timezone overrides', async ({ page }) => {
    const requests: URL[] = [];
    await page.route('**/api/weather/history**', route => {
      requests.push(new URL(route.request().url()));
      return route.fulfill({ json: { hours: [], gaps: [], units } });
    });
    await login(page);
    const date = recentDate(2);
    await page.goto(`/weather/day/?date=${date}&timezone=Europe%2FPrague`);
    const day = page.getByTestId('weather-day');
    await expect(day).toContainText('Europe/Prague');
    await expect(day).toContainText(/timezone from link/i);
    await expect(page.getByTestId('weather-date')).toHaveValue(date);
    const nextDate = recentDate(3);
    const countBefore = requests.length;
    await page.getByTestId('weather-date').fill(nextDate);
    await expect.poll(() => requests.length).toBeGreaterThan(countBefore);
    const latest = requests.at(-1)!;
    const from = Date.parse(latest.searchParams.get('from')!);
    const to = Date.parse(latest.searchParams.get('to')!);
    expect(Number.isFinite(from)).toBe(true);
    expect(to - from).toBe(24 * 3_600_000);
    expect(new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Prague', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(from))).toBe(nextDate);
  });

  test('weather remains part of dashboard hide/show customization', async ({ page }) => {
    await page.route('**/api/weather/history**', route => route.fulfill({ json: fixture() }));
    await login(page);
    await expect(page.getByTestId('weather-card')).toBeVisible();
    await page.getByRole('button', { name: 'Customize', exact: true }).click();
    await page.getByTestId('weather-card-visibility').click();
    await expect(page.getByTestId('weather-card')).toHaveAttribute('data-hidden', 'true');
    const hiddenSaved = page.waitForResponse(response => response.url().endsWith('/api/users/me/settings') && response.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Done', exact: true }).click();
    expect((await hiddenSaved).ok()).toBe(true);
    await expect(page.getByTestId('weather-card')).toHaveCount(0);
    await page.reload();
    await expect(page.getByRole('button', { name: 'Customize', exact: true })).toBeEnabled();
    await expect(page.getByTestId('weather-card')).toHaveCount(0);
    await page.getByRole('button', { name: 'Customize', exact: true }).click();
    await page.getByTestId('weather-card-visibility').click();
    const visibleSaved = page.waitForResponse(response => response.url().endsWith('/api/users/me/settings') && response.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Done', exact: true }).click();
    expect((await visibleSaved).ok()).toBe(true);
    await expect(page.getByTestId('weather-card')).toBeVisible();
  });

  test('day API bounds follow DST and never query future hours for today', async ({ page }) => {
    const requests: URL[] = [];
    await page.route('**/api/weather/history**', route => {
      requests.push(new URL(route.request().url()));
      return route.fulfill({ json: { hours: [], gaps: [], units } });
    });
    await login(page);
    let before = requests.length;
    await page.goto('/weather/day/?date=2026-03-29&timezone=Europe%2FPrague');
    await expect.poll(() => requests.length).toBeGreaterThan(before);
    let requested = requests.at(-1)!;
    expect(requested.searchParams.get('from')).toBe('2026-03-28T23:00:00.000Z');
    expect(requested.searchParams.get('to')).toBe('2026-03-29T22:00:00.000Z');
    before = requests.length;
    await page.goto(`/weather/day/?date=${recentDate(0)}&timezone=UTC`);
    await expect.poll(() => requests.length).toBeGreaterThan(before);
    requested = requests.at(-1)!;
    expect(Date.parse(requested.searchParams.get('to')!)).toBeLessThanOrEqual(Date.now());
    expect(Date.parse(requested.searchParams.get('from')!)).toBe(Date.parse(`${recentDate(0)}T00:00:00Z`));
  });

  test('empty saved history stays honest on dashboard and a narrow day view', async ({ page }) => {
    await page.route('**/api/weather/history**', route => route.fulfill({ json: { hours: [], gaps: [], units } }));
    await login(page);
    await expect(page.getByTestId('weather-card')).toContainText(/no saved weather/i);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/weather/day/?date=${recentDate()}&timezone=UTC`);
    await expect(page.getByTestId('weather-day')).toContainText(/no saved weather/i);
    await expect(page.getByTestId('weather-hour')).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });

  test('weather errors do not prevent the rest of the dashboard from loading', async ({ page }) => {
    await page.route('**/api/weather/history**', route => route.fulfill({ status: 503, json: { error: 'fixture unavailable' } }));
    await login(page);
    await expect(page.getByTestId('weather-card')).toContainText(/unavailable/i);
    await expect(page.getByRole('button', { name: 'Customize', exact: true })).toBeEnabled();
    await expect(page.getByTestId('vitals-grid')).toBeVisible();
    await page.goto(`/weather/day/?date=${recentDate()}&timezone=UTC`);
    await expect(page.getByTestId('weather-day')).toContainText(/unavailable/i);
    await expect(page.getByTestId('weather-hour')).toHaveCount(0);
  });

  test('logged-in weather history uses the deployed API without adding evidence', async ({ page }) => {
    await login(page);
    const to = new Date();
    const from = new Date(to.getTime() - 24 * 3_600_000);
    const response = await page.request.get('/api/weather/history', {
      params: { from: from.toISOString(), to: to.toISOString() },
    });
    expect(response.status()).toBe(200);
    const history = await response.json();
    expect(Array.isArray(history.hours)).toBe(true);
    expect(Array.isArray(history.gaps)).toBe(true);
    expect(history.units).toEqual(units);
    // History may include evidence left by the separate collection suite.
    // Do not invent a new observation just to force an empty/ready state.
    for (const saved of history.hours) expect(Number.isFinite(saved.temperature_c)).toBe(true);
  });
});
