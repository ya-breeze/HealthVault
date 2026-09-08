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

/**
 * Every published port of hcw-prod. Its nginx container maps `:80` and `:443`
 * (docker-compose.yml), so `http_port` and `https_port` from data.json are two
 * doors into the same database — 8888 alone would leave 9888 open.
 */
const PROD_PORTS = new Set(['8888', '9888']);

/**
 * Names that reach that container on the LAN. The IP is how it is addressed from
 * elsewhere on the network; the loopback spellings are how it is addressed from
 * the TrueNAS host itself, or through an `ssh -L 8888:localhost:8888` forward,
 * which is a normal way in from off-network.
 *
 * These are only refused *in combination with* a prod port: hcw-wip is
 * 8892/9892 on the same IP, and `make run-backend` serves from loopback too.
 */
const PROD_ADDRESSES = new Set(['192.168.1.54', 'localhost', '127.0.0.1', '[::1]', '0.0.0.0']);

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
 * @throws if `env.BASE_URL` names hcw-prod, or is not an http(s) URL.
 */
export function resolveTarget(env: NodeJS.ProcessEnv): string {
  const raw = env.BASE_URL || WIP;

  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(`BASE_URL is not a valid URL: ${JSON.stringify(raw)}`);
  }

  // Both the suite and the check below assume an http(s) origin: `url.origin` is
  // the string "null" for anything else, which would sail past the deny-list and
  // then be concatenated into every request URL.
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error(`BASE_URL must be http or https, not ${url.protocol} — got ${JSON.stringify(raw)}`);
  }

  // `new URL()` has already lowercased the host and collapsed the exotic
  // spellings (`:08888`, integer IPs, `user@…`) onto their canonical form. The
  // trailing-dot FQDN is the one it keeps, so drop that here.
  const hostname = url.hostname.replace(/\.$/, '');
  const port = url.port || (url.protocol === 'https:' ? '443' : '80');

  if (PROD_HOSTNAMES.has(hostname) || (PROD_ADDRESSES.has(hostname) && PROD_PORTS.has(port))) {
    throw new Error(
      `refusing to run the e2e suite against ${url.host} — that is hcw-prod, which holds ` +
      `the only copy of real health data. This suite creates and deletes meals, ` +
      `rewrites user settings, and writes records; against prod those are edits, not ` +
      `fixtures. Run \`make test-e2e\`, which targets hcw-wip (${WIP}), or set BASE_URL ` +
      `to another non-prod deployment.`
    );
  }

  // Return what was *checked*, not what was typed: `new URL()` tolerates
  // surrounding whitespace and odd spellings, so returning `raw` could hand
  // callers a string the deny-list never saw. Trailing slashes go too — callers
  // concatenate `${BASE_URL}/api/...`, which would otherwise be `//api/...`.
  return `${url.origin}${url.pathname}`.replace(/\/+$/, '');
}

/** The deployment under test. Defaults to hcw-wip; never prod. */
export const BASE_URL = resolveTarget(process.env);
