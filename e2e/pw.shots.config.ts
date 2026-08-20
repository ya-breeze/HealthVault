import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
  testDir: './.shots-tmp',
  testMatch: /ru-shots\.spec\.ts/,
  timeout: 90_000,
  retries: 0,
  workers: 1,
  reporter: 'line',
  use: { baseURL: process.env.BASE_URL, headless: true, viewport: { width: 1280, height: 900 } },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
