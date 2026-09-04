/**
 * The one place the e2e suite decides which deployment it is talking to.
 *
 * Before this module, eleven files resolved `BASE_URL` on their own and their
 * defaults disagreed: `playwright.config.ts` — which decides where the *browser*
 * goes — fell back to 8888, the prod stack, while a few spec files fell back to
 * 8892. A bare `npx playwright test` therefore drove the app against prod, where
 * `food.spec.ts` creates and deletes meals, `dashboard.spec.ts` rewrites user
 * settings, and `data-types.spec.ts` writes records. `make test-e2e` passes
 * BASE_URL and was never affected; the exposure was the direct invocation, which
 * is what `--ui`, `--debug`, and every IDE "run test" button use.
 *
 * Importing this module is the first thing Playwright does, so a target it
 * refuses fails before a browser starts — not midway through a run that has
 * already written three meals.
 */

/** hcw-wip. Throwaway data, safe to seed and delete. */
const WIP = 'http://192.168.1.54:8892';

/** hcw-prod on the LAN. Matched with the port, since :8892 is next door. */
const PROD_HOST_PORT = '192.168.1.54:8888';

/**
 * The Cloudflare tunnel hostname for the same container. Matched on hostname
 * alone, ignoring the port: this domain serves nothing but prod, so there is no
 * port on it worth allowing, and denying only the default one would let
 * `:8888`-style typos through.
 */
const PROD_HOSTNAMES = new Set(['healthvault.ikoro.in']);

/**
 * Resolves the deployment under test, refusing prod.
 *
 * Takes `env` rather than reading `process.env` so the guard is testable without
 * spawning a child process or mutating the ambient environment — see
 * `tests/e2e-target.spec.ts`.
 *
 * The parameter is a plain `Record`, not `NodeJS.ProcessEnv`: this package has
 * no `@types/node`, so that namespace does not resolve here. `process.env`
 * satisfies this shape regardless.
 *
 * @throws if `env.BASE_URL` names hcw-prod, or is not a URL at all.
 */
export function resolveTarget(env: Record<string, string | undefined>): string {
  const raw = env.BASE_URL || WIP;

  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(`BASE_URL is not a valid URL: ${JSON.stringify(raw)}`);
  }

  if (url.host === PROD_HOST_PORT || PROD_HOSTNAMES.has(url.hostname)) {
    throw new Error(
      `refusing to run the e2e suite against ${url.host} — that is hcw-prod, which holds ` +
      `the only copy of real health data. This suite creates and deletes meals, ` +
      `rewrites user settings, and writes records; against prod those are edits, not ` +
      `fixtures. Run \`make test-e2e\`, which targets hcw-wip (${WIP}), or set BASE_URL ` +
      `to another non-prod deployment.`
    );
  }

  // Callers build request URLs by concatenation — `${BASE_URL}/api/...` — so a
  // trailing slash here would produce `//api/...`, which the router answers with
  // a redirect the API clients do not follow.
  return raw.replace(/\/+$/, '');
}

/** The deployment under test. Defaults to hcw-wip; never prod. */
export const BASE_URL = resolveTarget(process.env);
