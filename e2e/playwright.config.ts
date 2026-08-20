import { defineConfig, devices } from '@playwright/test';

const BASE_URL = process.env.BASE_URL || 'http://192.168.1.54:8888';

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  retries: 1,
  // Every spec file logs in as the same seeded account against one shared
  // backend, so "parallel" here means several workers mutating one user's
  // state at once. That was tolerable while tests only created their own
  // food records, but the Display Language switcher is per-user UI state:
  // while the settings-race test in dashboard.spec.ts holds display_language
  // = 'ru', any concurrently running file asserting on English label text
  // ('Change match', 'Custom Foods', 'Load older', ...) fails, and retries:1
  // would turn that into intermittent flake rather than a real signal.
  // Serializing removes the whole class of cross-file interference rather
  // than just this instance; the alternative — a second seeded account for
  // the language test — would make the suite depend on seeding that any
  // other target stack may not have. Found in code review.
  workers: 1,
  reporter: 'line',
  use: {
    baseURL: BASE_URL,
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
