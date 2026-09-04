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

// The `node` environment has no sessionStorage, and lib/session.ts reads its
// absence as "not suppressed" — the right default for a browser that denies
// storage, but also the one that would make a suppression case pass without
// testing anything. Cases that care about the flag install a store.
function installSessionStorage(initial: Record<string, string> = {}) {
  const store = new Map(Object.entries(initial));
  vi.stubGlobal('sessionStorage', {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
  });
  return store;
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

  it('runs one exchange for concurrent 401s rather than one per caller', async () => {
    // Every exchange mints its own year-long refresh token row server-side, so
    // the dashboard's ordinary shape — several calls dispatched together, all
    // 401 — used to leave a pile of them behind on one page load.
    let exchanges = 0;
    let exchanged = false;
    let releaseExchange = () => {};
    const exchangeGate = new Promise<void>(r => { releaseExchange = r; });

    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) return unauthorized();
      if (path.endsWith('/auth/cf-access')) {
        exchanges++;
        // Held open so both callers are inside the exchange at the same time.
        // That overlap is the whole case: a shared in-flight promise can only
        // dedup callers that actually overlap, and releasing early would let
        // the second one arrive after the first had already finished.
        await exchangeGate;
        exchanged = true;
        return noContent();
      }
      if (path.includes('/summary/today')) return exchanged ? json({ date: '2026-08-31' }) : unauthorized();
      if (path.includes('/data/weight')) return exchanged ? json([]) : unauthorized();
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    const first = api.getTodaySummary();
    const second = api.data('weight');
    // Let both calls get their 401, fail their refresh, and reach the
    // exchange before the one in flight is allowed to complete.
    await new Promise(r => setTimeout(r, 5));
    releaseExchange();

    await expect(first).resolves.toBeTruthy();
    await expect(second).resolves.toBeTruthy();
    expect(exchanges, 'each exchange mints its own year-long refresh token').toBe(1);
  });
});

describe('logout suppression of the Cf-Access exchange', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not exchange after a logout, for a 401 raised anywhere in the app', async () => {
    // The login page checks the flag too, but that check protects only the
    // login page. This is the path that made logout look undone: a background
    // fetch — LanguageContext's settings poll is the real one — 401s, refresh
    // cannot fix it, and the exchange silently restores the session the user
    // had just ended, without any page ever navigating to /login.
    let exchanges = 0;
    installSessionStorage({ 'hcw:accessSignInSuppressed': '1' });
    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) return unauthorized();
      if (path.endsWith('/auth/cf-access')) {
        exchanges++;
        return noContent();
      }
      if (path.includes('/summary/today')) return unauthorized();
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    await expect(api.getTodaySummary()).rejects.toMatchObject({ status: 401 });
    expect(exchanges, 'logging out must not be undone by the next background 401').toBe(0);
  });

  it('still exchanges once the flag has been cleared', async () => {
    // The guard has to be a suppression, not a removal: after the login page's
    // sign-in button clears the flag, the ordinary recovery must work again.
    let exchanges = 0;
    let exchanged = false;
    const store = installSessionStorage({ 'hcw:accessSignInSuppressed': '1' });
    store.delete('hcw:accessSignInSuppressed');

    installFetch(async path => {
      if (path.endsWith('/auth/refresh')) return unauthorized();
      if (path.endsWith('/auth/cf-access')) {
        exchanges++;
        exchanged = true;
        return noContent();
      }
      if (path.includes('/summary/today')) return exchanged ? json({ date: '2026-08-31' }) : unauthorized();
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    await expect(api.getTodaySummary()).resolves.toBeTruthy();
    expect(exchanges).toBe(1);
  });

  it('reports a suppressed api.cfAccessLogin as a failure rather than as a sign-in', async () => {
    // The login page routes to `/` when this resolves, so resolving without
    // having exchanged anything would send the user to a page their absent
    // session cannot load.
    installSessionStorage({ 'hcw:accessSignInSuppressed': '1' });
    installFetch(async path => {
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    await expect(api.cfAccessLogin()).rejects.toMatchObject({ status: 401 });
  });
});

describe('api.cfAccessLogin', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  // The login page branches on the status alone — 404 hides the Access
  // control, 403 says the Google account is not authorized — so routing this
  // call through the shared exchange must not blur the two together.
  it.each([404, 403, 401])('surfaces status %i as an ApiError the login page can branch on', async status => {
    installFetch(async path => {
      if (path.endsWith('/auth/cf-access')) return new Response('nope', { status });
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    await expect(api.cfAccessLogin()).rejects.toMatchObject({ status });
  });

  it('resolves on a successful exchange', async () => {
    installFetch(async path => {
      if (path.endsWith('/auth/cf-access')) return noContent();
      throw new Error(`unexpected request: ${path}`);
    });

    const api = await freshApi();
    await expect(api.cfAccessLogin()).resolves.toBeUndefined();
  });
});
