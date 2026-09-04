import { test, expect } from '@playwright/test';
import { resolveTarget } from './helpers/target';

// `resolveTarget` decides which deployment the whole suite talks to, and its
// job is to refuse hcw-prod. That refusal is the one behaviour here no other
// test can exercise: every other spec proves it *ran* somewhere, never that a
// wrong somewhere would have been rejected.
//
// These cases pass an explicit env object rather than touching process.env, so
// they neither depend on how this run was invoked nor leak a target into the
// files that run after them.

const WIP = 'http://192.168.1.54:8892';

test.describe('e2e target resolution', () => {
  test('defaults to the wip stack when BASE_URL is unset', () => {
    expect(resolveTarget({})).toBe(WIP);
  });

  test('accepts an explicit non-prod URL', () => {
    expect(resolveTarget({ BASE_URL: WIP })).toBe(WIP);
    expect(resolveTarget({ BASE_URL: 'http://localhost:3000' })).toBe('http://localhost:3000');
  });

  test('strips a trailing slash', () => {
    // Callers concatenate: `${BASE_URL}/api/...` would otherwise be `//api/...`.
    expect(resolveTarget({ BASE_URL: `${WIP}/` })).toBe(WIP);
  });

  test('refuses the prod stack by port', () => {
    expect(() => resolveTarget({ BASE_URL: 'http://192.168.1.54:8888' })).toThrow(/hcw-prod/);
    // A path or trailing slash must not smuggle it past the check.
    expect(() => resolveTarget({ BASE_URL: 'http://192.168.1.54:8888/food/history/' })).toThrow(/hcw-prod/);
  });

  test('refuses the prod stack by its Cloudflare hostname, on any port', () => {
    // Same container as :8888. The hostname is matched without its port, so a
    // stray one cannot walk past the check.
    expect(() => resolveTarget({ BASE_URL: 'https://healthvault.ikoro.in' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'https://healthvault.ikoro.in:443/' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'http://healthvault.ikoro.in:8888' })).toThrow(/hcw-prod/);
  });

  test('leaves the neighbouring wip port alone', () => {
    // The IP is matched with its port, not on its own: :8892 lives there too.
    expect(resolveTarget({ BASE_URL: 'http://192.168.1.54:8892' })).toBe(WIP);
  });

  test('rejects a BASE_URL that is not a URL', () => {
    // Otherwise `new URL()` throws deep inside an unrelated request call.
    expect(() => resolveTarget({ BASE_URL: '192.168.1.54:8892' })).toThrow(/not a valid URL/);
  });
});
