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

  test('returns what it checked, not what was typed', () => {
    // Callers concatenate `${BASE_URL}/api/...`, so a trailing slash would give
    // `//api/...`. And `new URL()` tolerates surrounding whitespace — returning
    // the raw string would hand callers something the deny-list never saw.
    expect(resolveTarget({ BASE_URL: `${WIP}/` })).toBe(WIP);
    expect(resolveTarget({ BASE_URL: ` ${WIP} ` })).toBe(WIP);
    expect(resolveTarget({ BASE_URL: 'http://192.168.1.54:8892/app/' })).toBe(`${WIP}/app`);
  });

  test('refuses the prod stack on every port it publishes', () => {
    // nginx maps :80 and :443 to 8888 and 9888 — two doors, one database.
    expect(() => resolveTarget({ BASE_URL: 'http://192.168.1.54:8888' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'https://192.168.1.54:9888' })).toThrow(/hcw-prod/);
    // A path or trailing slash must not smuggle it past the check.
    expect(() => resolveTarget({ BASE_URL: 'http://192.168.1.54:8888/food/history/' })).toThrow(/hcw-prod/);
  });

  test('refuses the loopback spellings of the same container', () => {
    // How prod is addressed from the TrueNAS host itself, or through an
    // `ssh -L 8888:localhost:8888` forward from off-network.
    expect(() => resolveTarget({ BASE_URL: 'http://localhost:8888' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'http://127.0.0.1:8888' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'https://127.0.0.1:9888' })).toThrow(/hcw-prod/);
  });

  test('refuses the prod stack by its Cloudflare hostname, on any port', () => {
    // Same container as :8888. The hostname is matched without its port, so a
    // stray one cannot walk past the check.
    expect(() => resolveTarget({ BASE_URL: 'https://healthvault.ikoro.in' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'https://healthvault.ikoro.in:443/' })).toThrow(/hcw-prod/);
    expect(() => resolveTarget({ BASE_URL: 'http://healthvault.ikoro.in:8888' })).toThrow(/hcw-prod/);
    // The trailing-dot FQDN is the one spelling `new URL()` does not normalise.
    expect(() => resolveTarget({ BASE_URL: 'https://healthvault.ikoro.in./' })).toThrow(/hcw-prod/);
  });

  test('leaves the neighbouring wip ports alone', () => {
    // The addresses are only refused in combination with a prod port: hcw-wip is
    // 8892/9892 on the same IP, and `make run-backend` serves from loopback.
    expect(resolveTarget({ BASE_URL: 'http://192.168.1.54:8892' })).toBe(WIP);
    expect(resolveTarget({ BASE_URL: 'https://192.168.1.54:9892' })).toBe('https://192.168.1.54:9892');
    expect(resolveTarget({ BASE_URL: 'http://localhost:8080' })).toBe('http://localhost:8080');
  });

  test('rejects a BASE_URL that is not an http(s) URL', () => {
    // Otherwise `new URL()` throws deep inside an unrelated request call...
    expect(() => resolveTarget({ BASE_URL: '192.168.1.54:8892' })).toThrow(/not a valid URL/);
    // ...or parses into something whose `origin` is the string "null", which
    // would pass the deny-list and then be concatenated into every request.
    expect(() => resolveTarget({ BASE_URL: 'file:///etc/hosts' })).toThrow(/must be http or https/);
  });
});
