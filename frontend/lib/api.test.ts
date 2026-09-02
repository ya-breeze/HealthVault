import { describe, it, expect, beforeEach, vi } from 'vitest';

// The refresh coordination is module-level state (`refreshPromise`, the
// last-refresh timestamp), so every case imports a fresh copy of the module
// rather than trying to reset internals it does not export.
async function freshApi() {
  vi.resetModules();
  return (await import('./api')).api;
}

type Handler = (path: string, init?: RequestInit) => Promise<Response> | Response;

// Records every request the module makes and answers each from `handler`, so a
// case can assert on how many times /auth/refresh was called rather than on
// what any single call returned.
function installFetch(handler: Handler) {
  const calls: string[] = [];
  const spy = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    calls.push(`${init?.method ?? 'GET'} ${path}`);
    return handler(path, init);
  });
  vi.stubGlobal('fetch', spy);
  return calls;
}

const json = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
const unauthorized = () => new Response('unauthorized', { status: 401 });
const noContent = () => new Response(null, { status: 204 });

describe('transparent refresh on 401', () => {
  // Vitest runs these in the `node` environment, which has neither
  // `localStorage` nor `navigator.locks` — the same shape as a browser on a
  // non-secure origin, which is precisely the path these cases are about.
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('refreshes once when a second 401 arrives after the first refresh completed', async () => {
    // The dashboard's shape: several calls dispatched together, all 401. The
    // first one's 401 comes back promptly and drives a refresh; the second's
    // is released only once that refresh has finished, so it can no longer
    // join the in-flight `refreshPromise` and — before the fix — started a
    // second refresh with an already-rotated token.
    let refreshes = 0;
    let refreshDone = false;
    let releaseSecond = () => {};
    const secondUnauthorized = new Promise<void>(r => { releaseSecond = r; });

    const calls = installFetch(async (path, init) => {
      if (path.endsWith('/auth/refresh')) {
        refreshes++;
        // Only the first refresh can succeed: RotateRefreshToken consumes the
        // token it is given, so a duplicate presents a spent one.
        const ok = refreshes === 1;
        refreshDone = true;
        return ok ? noContent() : unauthorized();
      }
      if (path.includes('/summary/today')) {
        if (refreshDone) return json({ date: '2026-08-31' });
        return unauthorized();
      }
      if (path.includes('/data/weight')) {
        if (refreshDone) return json([]);
        await secondUnauthorized;
        return unauthorized();
      }
      // No catch-all: a path this stub does not recognize means the case is
      // no longer exercising what it claims to. An earlier draft returned 200
      // here, and a typo'd matcher made the whole case pass vacuously against
      // the unfixed code.
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    const first = api.getTodaySummary();
    const second = api.data('weight');

    // Await the first call in full, not merely the refresh: `refreshPromise`
    // is cleared in a `.finally` that runs before this await resumes. Release
    // the second 401 any earlier and it simply joins the in-flight refresh —
    // dedup working as intended, and not the case under test.
    await expect(first).resolves.toBeTruthy();
    releaseSecond();
    await expect(second).resolves.toBeTruthy();
    expect(refreshes, 'a duplicate refresh spends an already-rotated token').toBe(1);
    expect(calls.filter(c => c.includes('/auth/refresh'))).toHaveLength(1);
  });

  it('still refreshes for a 401 on a request dispatched after the last refresh', async () => {
    // The guard must not swallow a genuine later expiry: this request is sent
    // strictly after the previous refresh completed, so its 401 is its own.
    let refreshes = 0;
    let authorized = false;

    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) {
        refreshes++;
        authorized = true;
        return noContent();
      }
      if (authorized) {
        authorized = false; // each access token is good for exactly one call here
        return json({ date: '2026-08-31' });
      }
      return unauthorized();
    });

    const api = await freshApi();
    await expect(api.getTodaySummary()).resolves.toBeTruthy();
    expect(refreshes).toBe(1);

    // A later call, dispatched after that refresh, 401s again and must get its
    // own refresh rather than being told a fresh one already happened.
    await new Promise(r => setTimeout(r, 2));
    await expect(api.getTodaySummary()).resolves.toBeTruthy();
    expect(refreshes, 'a 401 dispatched after the last refresh needs a new one').toBe(2);
  });

  it('gives up and returns the 401 when the refresh itself fails', async () => {
    let refreshes = 0;
    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) {
        refreshes++;
        return unauthorized();
      }
      return unauthorized();
    });

    const api = await freshApi();
    await expect(api.getTodaySummary()).rejects.toMatchObject({ status: 401 });
    expect(refreshes).toBe(1);
  });
});

describe('Cf-Access exchange as a second recovery step', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('tries the exchange once when refresh fails, and retries the original request on success', async () => {
    let refreshes = 0;
    let exchanges = 0;
    let exchanged = false;

    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) {
        refreshes++;
        return unauthorized();
      }
      if (path.endsWith('/auth/cf-access')) {
        exchanges++;
        exchanged = true;
        return noContent();
      }
      if (path.includes('/summary/today')) {
        if (exchanged) return json({ date: '2026-08-31' });
        return unauthorized();
      }
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    await expect(api.getTodaySummary()).resolves.toBeTruthy();
    expect(refreshes, 'refresh is tried first, same as any other 401').toBe(1);
    expect(exchanges, 'the exchange is the fallback once refresh has failed').toBe(1);
  });

  it('gives up and returns the original 401 when both refresh and the exchange fail', async () => {
    let refreshes = 0;
    let exchanges = 0;

    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) {
        refreshes++;
        return unauthorized();
      }
      if (path.endsWith('/auth/cf-access')) {
        exchanges++;
        return unauthorized();
      }
      return unauthorized();
    });

    const api = await freshApi();
    await expect(api.getTodaySummary()).rejects.toMatchObject({ status: 401 });
    expect(refreshes).toBe(1);
    expect(exchanges, 'the exchange is tried exactly once, never looped').toBe(1);
  });
});
