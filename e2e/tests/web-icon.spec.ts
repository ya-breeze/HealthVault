import { test, expect } from '@playwright/test';
import { BASE_URL } from './helpers/target';

// The browser identity is emitted by the App Router's file conventions and by `app/manifest.ts`,
// none of which any unit test observes: `icon.png`, `apple-icon.png` and the manifest route only
// become <link> tags in a real build. A renamed file or a dropped convention would leave the app
// working perfectly and silently back on a placeholder icon, which is exactly the state this
// change exists to end.
//
// Only the login page is visited, so this spec needs no session and writes nothing.

test.describe('web icon and manifest', () => {
  test('the document advertises an icon, an Apple touch icon and a manifest', async ({ page }) => {
    await page.goto('/login/');

    const icon = page.locator('link[rel="icon"]');
    await expect(icon.first()).toHaveCount(1);

    const appleHref = await page.locator('link[rel="apple-touch-icon"]').first().getAttribute('href');
    expect(appleHref, 'no apple-touch-icon link').toBeTruthy();

    const manifestHref = await page.locator('link[rel="manifest"]').first().getAttribute('href');
    expect(manifestHref, 'no manifest link').toBeTruthy();

    // The Apple icon must be opaque, but a test cannot see pixels through a <link>; what it can
    // prove is that the URL the document advertises actually serves an image.
    const apple = await page.request.get(new URL(appleHref!, BASE_URL).toString());
    expect(apple.status()).toBe(200);
    expect(apple.headers()['content-type']).toContain('image');
  });

  test('the manifest names icons that are themselves served', async ({ page }) => {
    await page.goto('/login/');
    const manifestHref = await page.locator('link[rel="manifest"]').first().getAttribute('href');
    const manifestURL = new URL(manifestHref!, BASE_URL).toString();

    const response = await page.request.get(manifestURL);
    expect(response.status()).toBe(200);

    const manifest = await response.json();
    expect(manifest.name).toBe('HealthVault');
    expect(manifest.start_url).toBe('/');
    expect(manifest.display).toBe('standalone');
    expect(manifest.theme_color).toBeTruthy();

    // Android crops an installed icon to the launcher's shape, so a maskable entry is not
    // optional decoration: without it the plain icon is cropped and loses its outer edge.
    const maskable = manifest.icons.filter((i: { purpose?: string }) => i.purpose === 'maskable');
    expect(maskable, 'manifest declares no maskable icon').toHaveLength(1);

    for (const icon of manifest.icons as Array<{ src: string; sizes: string; type: string }>) {
      expect(icon.sizes).toMatch(/^\d+x\d+$/);
      expect(icon.type).toBe('image/png');
      const asset = await page.request.get(new URL(icon.src, manifestURL).toString());
      expect(asset.status(), `manifest names ${icon.src}, which is not served`).toBe(200);
    }
  });

  test('the retired scaffold assets are gone', async ({ page }) => {
    // These shipped with create-next-app and nothing referenced them. Asserting their absence
    // keeps a future copy-paste from quietly restoring the "this is still a scaffold" look.
    for (const name of ['next.svg', 'vercel.svg', 'window.svg', 'globe.svg', 'file.svg']) {
      const asset = await page.request.get(new URL(`/${name}`, BASE_URL).toString());
      expect(asset.status(), `/${name} is still served`).toBe(404);
    }
  });
});
