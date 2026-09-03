const BASE = process.env.NEXT_PUBLIC_API_URL ?? '/api';

// Coordination keys for transparent-refresh-on-401 (see openspec authentication
// capability, "Frontend transparent refresh on 401"). AUTH_REFRESH_LOCK names a
// Web Lock shared across tabs of the same browser; LAST_REFRESH_KEY is a
// localStorage timestamp read/written inside that lock so a tab that loses the
// lock race can tell a fresh refresh already happened instead of resubmitting
// an already-rotated refresh token.
const AUTH_REFRESH_LOCK = 'hcw-auth-refresh';
const LAST_REFRESH_KEY = 'hcw:lastAuthRefreshAt';

function isAuthExemptPath(path: string): boolean {
  return path === '/auth/login' || path === '/auth/refresh';
}

// Completion time of the last successful refresh in *this* tab. The
// localStorage twin below is the cross-tab channel and this one cannot replace
// it, but the reverse is also true: localStorage is only ever read inside the
// Web Locks branch, which needs a secure context. This variable is what lets
// the same rule hold on an origin that has no Web Locks — see lastRefreshAt().
let lastRefreshAtInTab = 0;

// The most recent refresh this tab can prove happened, from either channel.
// Reading both means the guard degrades gracefully rather than conditionally:
// no localStorage (private browsing) still leaves the in-tab value, and a
// refresh performed by another tab still wins through localStorage.
function lastRefreshAt(): number {
  let stored = 0;
  try {
    stored = Number(localStorage.getItem(LAST_REFRESH_KEY)) || 0;
  } catch {
    // ignore; the in-tab value below still applies
  }
  return Math.max(stored, lastRefreshAtInTab);
}

// POSTs /auth/refresh directly (no retry wrapping — this IS the refresh call).
// Records the completion time so a later caller — in this tab, or in another
// one waiting on the Web Lock — can see a refresh already happened at/after
// its request was dispatched.
async function refreshAccessToken(): Promise<boolean> {
  const res = await fetch(`${BASE}/auth/refresh`, { method: 'POST', credentials: 'include' });
  if (!res.ok) return false;
  lastRefreshAtInTab = Date.now();
  try {
    localStorage.setItem(LAST_REFRESH_KEY, String(lastRefreshAtInTab));
  } catch {
    // localStorage may be unavailable (e.g. private browsing); same-tab dedup
    // still applies through lastRefreshAtInTab, cross-tab dedup just degrades.
  }
  return true;
}

// Same-tab dedup: concurrent 401s in one tab share this in-flight refresh.
let refreshPromise: Promise<boolean> | null = null;

// Coordinates a refresh across same-tab callers and, where the browser exposes
// a secure context, across tabs of the same browser too — see design.md's
// "Why dispatchedAt" note for why this must be the request's send time, not
// the time its 401 was received.
function coordinatedRefresh(dispatchedAt: number): Promise<boolean> {
  if (refreshPromise) return refreshPromise;

  const run = async (): Promise<boolean> => {
    // One rule, checked on both paths: a refresh that completed at or after
    // this request was dispatched already replaced the token its 401 was
    // complaining about, so the right move is to retry, not to refresh again.
    // Refreshing again is not merely wasteful — RotateRefreshToken consumes
    // the token it is given, so a second refresh presents a spent one, gets
    // its own 401, and reports the caller as logged out.
    if (lastRefreshAt() >= dispatchedAt) return true;

    if (typeof navigator !== 'undefined' && 'locks' in navigator && navigator.locks) {
      return navigator.locks.request(AUTH_REFRESH_LOCK, async () => {
        // Re-read inside the lock: another tab may have refreshed while this
        // one waited for it, which is the case the lock exists to serialize.
        if (lastRefreshAt() >= dispatchedAt) return true;
        return refreshAccessToken();
      });
    }
    // No Web Locks (older browser, or not a secure context — e.g. the local
    // http hcw-wip stack). The guard above still holds here because it reads
    // an in-tab variable rather than only localStorage, so same-tab callers
    // are covered on any origin. Cross-tab coordination still degrades: two
    // tabs can race into simultaneous refreshes, which no in-tab state can
    // see and only the Web Lock serializes.
    return refreshAccessToken();
  };

  refreshPromise = run().finally(() => {
    refreshPromise = null;
  });
  return refreshPromise;
}

// The one place that calls fetch() and reacts to a 401 by transparently
// refreshing and retrying — apiRawFetch, apiFetchNoBody, and apiFetchForm all
// delegate here so the retry logic exists exactly once.
async function fetchWithAuthRetry(path: string, options: RequestInit): Promise<Response> {
  const dispatchedAt = Date.now();
  const res = await fetch(`${BASE}${path}`, options);
  if (res.status !== 401 || isAuthExemptPath(path)) return res;

  const refreshed = await coordinatedRefresh(dispatchedAt);
  if (!refreshed) return res;

  return fetch(`${BASE}${path}`, options);
}

// Shared request setup (credentials, JSON content-type) so every JSON call —
// including ones that need to branch on a specific status code before the
// generic !res.ok handling, like reanalyzeMeal's 502 — goes through the same
// fetch configuration instead of re-declaring it.
async function apiRawFetch(path: string, options?: RequestInit): Promise<Response> {
  return fetchWithAuthRetry(path, {
    credentials: 'include',
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
}

// ApiError carries the HTTP status alongside the message, so callers can
// branch on a specific status (e.g. a 409 conflict from a concurrent edit —
// see MealItemRow's commitWeight) without parsing response text. Same
// transpilation-proof `.name` + prototype-chain treatment as the
// Reanalyze-specific error classes below — see their comment for why.
export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    Object.setPrototypeOf(this, ApiError.prototype);
  }
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await apiRawFetch(path, options);
  if (!res.ok) throw new ApiError(res.status, (await res.text()) || `${res.status} ${res.statusText}`);
  return res.json();
}

async function apiFetchNoBody(path: string, options?: RequestInit): Promise<void> {
  const res = await fetchWithAuthRetry(path, {
    credentials: 'include',
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
  if (!res.ok) throw new ApiError(res.status, (await res.text()) || `${res.status} ${res.statusText}`);
}

async function apiFetchForm<T>(path: string, form: FormData): Promise<T> {
  const res = await fetchWithAuthRetry(path, {
    method: 'POST',
    credentials: 'include',
    body: form,
  });
  if (!res.ok) throw new Error((await res.text()) || `${res.status} ${res.statusText}`);
  return res.json();
}

export interface NutrientProfile {
  calories_per_100g: number;
  protein_per_100g: number;
  carbs_per_100g: number;
  fat_per_100g: number;
  sugar_per_100g: number;
  sodium_per_100g: number;
  dietary_fiber_per_100g: number;
}

export interface FoodSearchResult {
  source: 'custom' | 'usda';
  custom_food_id?: string;
  fdc_id?: number;
  name: string;
  // English identity of a 'custom' result whose name is non-English — see
  // CustomFood.canonical_name below. Never set on 'usda' results.
  canonical_name?: string;
  profile: NutrientProfile;
}

export interface FoodSearchResponse {
  results: FoodSearchResult[];
  usda_unavailable?: boolean;
  translated_query?: string;
}

export interface CustomFood {
  id: string;
  name: string;
  // English identity of the food, set when created from a non-English
  // recognition (see the display-language capability). Empty/absent
  // otherwise — see food-nutrition-logging "Food Item and Custom Food Carry
  // a Canonical Name".
  canonical_name?: string;
  calories_per_100g: number;
  protein_per_100g: number;
  carbs_per_100g: number;
  fat_per_100g: number;
  sugar_per_100g: number;
  sodium_per_100g: number;
  dietary_fiber_per_100g: number;
}

export type CustomFoodInput = Omit<CustomFood, 'id'>;

export type MealStatus =
  | 'processing' | 'pending_clarification' | 'pending_review' | 'confirmed' | 'failed';

export type MacroSource = 'reference' | 'manual' | 'estimated' | 'none';

export interface FoodItem {
  id: string;
  meal_id: string;
  name: string;
  // See CustomFood.canonical_name above.
  canonical_name?: string;
  preparation: string;
  state: string;
  brand?: string;
  fdc_id?: number;
  custom_food_id?: string;
  off_code?: string;
  macro_source: MacroSource;
  weight_grams: number;
  confidence: number;
  calories: number;
  protein_grams: number;
  carbs_grams: number;
  fat_grams: number;
  sugar_grams: number;
  sodium_grams: number;
  dietary_fiber_grams: number;
}

export interface FoodMeal {
  id: string;
  photo_path?: string;
  description?: string;
  status: MealStatus;
  logged_at: string;
  name: string;
  clarify_round: number;
  clarify_log: string;
  calories: number;
  protein_grams: number;
  carbs_grams: number;
  fat_grams: number;
  sugar_grams: number;
  sodium_grams: number;
  dietary_fiber_grams: number;
  items: FoodItem[] | null;
}

export interface ClarifyEntry {
  round: number;
  question: string;
  answer: string;
}

export function pendingClarifyQuestions(meal: FoodMeal): string[] {
  if (!meal.clarify_log) return [];
  let entries: ClarifyEntry[];
  try {
    entries = JSON.parse(meal.clarify_log);
  } catch {
    return [];
  }
  const pendingRound = meal.clarify_round + 1;
  return entries.filter(e => e.round === pendingRound && e.answer === '').map(e => e.question);
}

export interface ManualMealItemInput {
  name: string;
  source: 'reference' | 'manual';
  weight_grams?: number;
  fdc_id?: number;
  custom_food_id?: string;
  off_code?: string;
  calories?: number;
  protein_grams?: number;
  carbs_grams?: number;
  fat_grams?: number;
  sugar_grams?: number;
  sodium_grams?: number;
  dietary_fiber_grams?: number;
}

export interface PatchItemInput {
  manual?: boolean;
  fdc_id?: number;
  custom_food_id?: string;
  off_code?: string;
  weight_grams?: number;
  name?: string;
  // Alongside manual: also creates a CustomFood from this request's name and
  // per-100g-converted macros, so a correction already typed here becomes
  // reusable without a separate visit to custom food management. Has no
  // effect without manual — see PatchMealItem's doc comment (food_item.go).
  save_as_custom_food?: boolean;
  calories?: number;
  protein_grams?: number;
  carbs_grams?: number;
  fat_grams?: number;
  sugar_grams?: number;
  sodium_grams?: number;
  dietary_fiber_grams?: number;
}

export interface ExpertComponentInput {
  name: string;
  weight_grams?: number;
}

export type ReanalyzeInput = { hint: string } | { components: ExpertComponentInput[] };

// CreateItemInput additionally requires name plus exactly one of
// (manual + macros) or (fdc_id/custom_food_id + a positive weight_grams) —
// see the backend's createItemRequest doc comment (food_item.go).
export type CreateItemInput = PatchItemInput & { name: string };

export interface MealSummary {
  id: string;
  name: string;
  logged_at: string;
  status: MealStatus;
  calories: number;
  protein_grams: number;
  carbs_grams: number;
  fat_grams: number;
}

export interface PatchMealInput {
  name?: string;
  logged_at?: string;
}

// Opaque per-user preferences blob (see the user-settings capability).
// dashboard_order is its first field; other keys pass through untouched.
export interface UserSettings {
  // The vitals-grid arrangement: which cards, in what order, and which are
  // hidden. Two shapes are readable — `{ type, hidden }` is what's written
  // today, while a bare string is the pre-visibility shape saved before
  // per-card show/hide existed. lib/vitals.ts#reconcileMetricOrder normalizes
  // both; nothing else should read this key raw.
  dashboard_order?: (string | { type: string; hidden: boolean })[];
  // BCP-47-ish code (e.g. "en", "ru") — see the display-language capability.
  // Absent means English. Read/written through components/LanguageContext.tsx.
  display_language?: string;
  // IANA zone name (e.g. "America/Los_Angeles") used to resolve the Logged
  // Day / Local Day boundary for food-day-completeness. Absent/invalid
  // falls back to UTC on the backend — see the "Local Day boundary" section
  // of openspec/changes/archive/2026-08-25-food-day-completeness/design.md.
  timezone?: string;
  // Eating Occasion count per day the backend treats as "day fully logged"
  // (food-day-completeness capability). Absent/non-positive falls back to
  // 3 on the backend.
  usual_meals_per_day?: number;
  // Profile fields feeding GET /users/me/nutrition-target (see
  // user-profile-and-nutrition-target). Malformed/absent values are
  // interpreted as "not set" by the backend, not schema-validated here — the
  // blob stays opaque at this layer, per user-settings's existing contract.
  birthdate?: string;
  sex?: 'male' | 'female';
  // Manual activity-tier override; unset means "infer from trailing steps".
  // See design.md's override-value -> tier/multiplier table — the values
  // below do NOT positionally match their tier display names ('active' ->
  // "Very active", 'very_active' -> "Extra active").
  activity_override?: 'sedentary' | 'light' | 'moderate' | 'active' | 'very_active';
  // Whether the dashboard's More Data section (secondary data-type links) is
  // collapsed from the read-only view. Absent or anything other than the
  // literal boolean `true` means "not hidden" — see frontend/app/page.tsx's
  // strict `=== true` normalization, mirroring dashboard_order's
  // `entry.hidden === true` check in lib/vitals.ts#reconcileMetricOrder.
  more_data_hidden?: boolean;
  [key: string]: unknown;
}

// One day's Day Completeness state (food-day-completeness capability) —
// mirrors the backend's database.DayCompleteness (food_completeness.go).
export type DayCompletenessState = 'complete' | 'confirmed_complete' | 'unconfirmed' | 'incomplete';

export interface DayCompleteness {
  date: string;
  occasion_count: number;
  state: DayCompletenessState;
}

// One day's logged-calorie total (food-daily-totals capability) — mirrors
// the backend's database.DailyTotal (food_daily_totals.go).
export interface DailyTotal {
  date: string;
  calories: number;
  // How many of that day's meals are in a status other than `confirmed`, and
  // so contributed nothing to `calories`. Non-zero means the day's total is
  // under-counted by an unknown amount, which is not the same thing as a low
  // total — see database.DailyTotal's own comment.
  unconfirmed_meals: number;
}

// Both error classes below set `.name` explicitly and restore the prototype
// chain via Object.setPrototypeOf in their constructors. TypeScript/SWC
// transpilation of `class X extends Error` can silently break `instanceof`
// checks against the subclass (a well-documented gotcha: Error's own
// constructor can return a different object than `this`, severing the
// prototype chain when downleveled) — the explicit `.name` tag gives
// callers a transpilation-proof way to distinguish them even if
// `instanceof` doesn't hold in some build configuration.

// ReanalyzeFailedError is thrown for HTTP 502 from POST .../reanalyze: the
// backend guarantees the meal is left exactly as it was found on this
// outcome (see design.md "Reanalyze failure reverts to the meal's prior
// state"), so callers can show "try again, nothing changed" rather than a
// generic error.
export class ReanalyzeFailedError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ReanalyzeFailedError';
    Object.setPrototypeOf(this, ReanalyzeFailedError.prototype);
  }
}

// ReanalyzeSupersededError is thrown for HTTP 412 from POST .../reanalyze —
// a materially different outcome from ReanalyzeFailedError's 502. It means
// this reanalysis attempt failed AND a newer operation (e.g. a concurrent
// Retry) claimed the meal in the meantime, so the "meal is unchanged"
// guarantee does not hold: the newer operation may already have changed it.
// Callers should refetch rather than assume the previously displayed meal
// is still current.
export class ReanalyzeSupersededError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ReanalyzeSupersededError';
    Object.setPrototypeOf(this, ReanalyzeSupersededError.prototype);
  }
}

// The four reason codes GET /users/me/nutrition-target's 422 response can
// carry — see design.md's "Unmet-precondition responses" decision. Checked
// in this order server-side; only the first unmet one is ever reported.
export type NutritionTargetUnmetReason =
  | 'missing_profile'
  | 'missing_measurements'
  | 'missing_goal_weight'
  | 'insufficient_activity_data';

// NutritionTargetUnmetError is thrown for HTTP 422 from
// GET /users/me/nutrition-target: `reason` lets callers show a specific,
// actionable message (e.g. "add your profile") instead of a generic error.
export class NutritionTargetUnmetError extends Error {
  reason: NutritionTargetUnmetReason;
  constructor(reason: NutritionTargetUnmetReason, message: string) {
    super(message);
    this.name = 'NutritionTargetUnmetError';
    this.reason = reason;
    Object.setPrototypeOf(this, NutritionTargetUnmetError.prototype);
  }
}

export interface NutritionTarget {
  calories: number;
  protein_grams: number;
  carbs_grams: number;
  fat_grams: number;
  bmr: number;
  measured_weight_kg: number;
  goal_weight_kg: number;
  height_m: number;
  age_years: number;
  sex: 'male' | 'female';
  activity_multiplier: number;
  activity_tier: string;
}

// The `target` field of TodaySummary, discriminated on `available` so a caller
// cannot read `calories` without having checked first — a flat
// optional-number shape would let `target.calories` typecheck its way into an
// arithmetic `undefined`.
//
// The four numeric fields are required in the available branch, which holds
// only because `summaryTargetPayload` (backend/pkg/server/summary_today.go)
// carries no `omitempty` on them. It did once, and that is a trap worth
// naming: `omitempty` drops a zero, zero is a legitimate carbs target, and the
// key's absence would then reach `Math.round(undefined)` and render "NaN" on
// the dashboard. A backend test asserts the keys are present; if that ever
// changes, these fields become optional and every read site needs `?? 0`.
export type TodaySummaryTarget =
  | { available: false; reason: NutritionTargetUnmetReason }
  | {
      available: true;
      calories: number;
      protein_grams: number;
      carbs_grams: number;
      fat_grams: number;
      bmr: number;
      measured_weight_kg: number;
      goal_weight_kg: number;
      height_m: number;
      age_years: number;
      sex: 'male' | 'female';
      activity_multiplier: number;
      activity_tier: string;
    };

/**
 * GET /api/summary/today — the caller's Logged Day so far, plus their
 * Nutrition Target, in one response.
 *
 * Preferred over `getNutritionTarget()` by any caller that needs both, which
 * is the whole reason the endpoint exists (see SummaryTodayHandler's own
 * "one cheap call" comment). Two differences from that endpoint matter to
 * callers:
 *
 * - **It never 422s.** An unavailable target is a normal state here, reported
 *   as `target.available === false` with the same four reason codes, so there
 *   is no `NutritionTargetUnmetError` to catch on this path.
 * - **The consumed totals count `confirmed` meals only** (database.TodaySummary),
 *   so a photographed but unconfirmed meal is absent from them. This matches
 *   the rule the Logging Gap's own valid-day filter applies.
 *
 * `date` is the caller's Logged Day as the *server* resolves it, from the same
 * timezone setting client-side window arithmetic reads. It is the authority on
 * which day these totals cover; a caller that needs to name the day should use
 * it rather than re-deriving "today" locally, so the two cannot disagree across
 * a local midnight.
 *
 * `recommendation` is always `null` today — it is the reserved home for Phase
 * 4's advice lines (see todo.md), not something any caller can rely on yet.
 */
export interface TodaySummary {
  date: string;
  calories_consumed: number;
  protein_grams_consumed: number;
  carbs_grams_consumed: number;
  fat_grams_consumed: number;
  meal_count: number;
  last_logged_at: string | null;
  display_language: string;
  target: TodaySummaryTarget;
  recommendation: null;
}

// Named rather than left inline on `api.me` below: AuthenticatedShell holds
// the session and passes it to both Header and MoreSheet, so three call
// sites now need to spell this shape.
export interface Me {
  id: string;
  username: string;
  family_id: string;
}

export const api = {
  login: (username: string, password: string) =>
    apiFetch('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  // `apiFetchNoBody`, not `apiFetch`: the endpoint answers 204 with an empty
  // body, so parsing it as JSON throws — and every caller awaits this before
  // routing to /login, so the rejection meant logout cleared the session
  // server-side and then left the user sitting on the authenticated page.
  // Surfaced by the More sheet's logout e2e case, which is the first test
  // to assert on what happens after the click rather than on the control.
  logout: () => apiFetchNoBody('/auth/logout', { method: 'POST' }),

  me: () => apiFetch<Me>('/users/me'),

  getSettings: () => apiFetch<UserSettings>('/users/me/settings'),
  putSettings: (settings: UserSettings) =>
    apiFetch<UserSettings>('/users/me/settings', { method: 'PUT', body: JSON.stringify(settings) }),

  // Self-only, like getNutritionTarget below. See the TodaySummary doc
  // comment for why a caller needing both today's intake and the target
  // should reach for this instead of getNutritionTarget.
  getTodaySummary: () => apiFetch<TodaySummary>('/summary/today'),

  // Self-only: no ?user= support, unlike most /data endpoints — see
  // design.md's "Self-only" decision. Throws NutritionTargetUnmetError on
  // 422 so callers can branch on the specific unmet reason.
  //
  // No caller in the app today: LoggingGapCard, the only one there was, moved
  // to getTodaySummary above when it grew a row needing today's intake too,
  // and TodaySummaryTarget's available branch now carries the same
  // derivation fields this route returns. Kept as the client only because the
  // backend still serves the route (server.go's /users/me/nutrition-target);
  // delete both this and NutritionTargetUnmetError if that route ever goes.
  getNutritionTarget: async (): Promise<NutritionTarget> => {
    const res = await apiRawFetch('/users/me/nutrition-target');
    if (res.status === 422) {
      const body = await res.json().catch(() => ({}) as { error?: string });
      const reason = (body.error as NutritionTargetUnmetReason) || 'missing_profile';
      throw new NutritionTargetUnmetError(reason, reason);
    }
    if (!res.ok) throw new ApiError(res.status, (await res.text()) || `${res.status} ${res.statusText}`);
    return res.json();
  },

  // Read-modify-write: fetches the latest settings, merges patch onto them,
  // and PUTs the result. Shared by every settings-writing feature (dashboard
  // order in app/page.tsx, Display Language in LanguageContext.tsx) so each
  // doesn't reimplement the same "refetch immediately before writing"
  // race-avoidance itself: without a fresh read right before the write, two
  // features saving in the same session with no navigation in between could
  // each PUT a stale snapshot that clobbers the other's already-saved field.
  // Returns the merged settings that were just written.
  //
  // No caller keeps a cached copy of the blob any more — this function's own
  // refetch is the only read that matters before a write, so a second,
  // longer-lived copy in a component would only be another way to go stale.
  //
  // A failed refetch aborts the write rather than falling back to a cached
  // copy. PUT /users/me/settings is a whole-document upsert, not a merge, so
  // proceeding from a stale or empty snapshot is exactly the clobbering this
  // function exists to prevent: if the caller's cache is still its initial
  // `{}` (LanguageProvider's mount GET 401'd on /login and the user changed
  // language before any later GET succeeded), the fallback path would PUT
  // `{"display_language":"ru"}` and silently erase dashboard_order and every
  // other key in the blob. Rejecting instead surfaces a toast at the call
  // site and leaves the stored document untouched. Found in code review.
  updateSettings: async (patch: Partial<UserSettings>): Promise<UserSettings> => {
    const current = await api.getSettings();
    const next: UserSettings = { ...current, ...patch };
    await api.putSettings(next);
    return next;
  },

  data: (type: string, from?: string, to?: string, user?: string, bucket?: 'day' | 'month') => {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    if (user) params.set('user', user);
    if (bucket) params.set('bucket', bucket);
    return apiFetch<Record<string, unknown>[]>(`/data/${type}?${params}`);
  },

  summary: (from?: string, to?: string, user?: string) => {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    if (user) params.set('user', user);
    return apiFetch<{ steps: number; avg_heart_rate: number; sleep_seconds: number }>(
      `/data/summary?${params}`
    );
  },

  dataTypesPresence: () => apiFetch<Record<string, boolean>>('/data-types/presence'),

  deleteRecord: (type: string, id: string): Promise<void> =>
    apiFetchNoBody(`/data/${type}/${id}`, { method: 'DELETE' }),

  // POST /api/data/{type} — allowlisted manual write (see WRITABLE_TYPES
  // below and the backend's writeAllowlist in api.go). `time` omitted lets
  // the backend default to now(); a value <= 0 is rejected server-side (400).
  createRecord: (type: string, input: { value: number; time?: string }) =>
    apiFetch<Record<string, unknown>>(`/data/${type}`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  importHealthConnect: (file: File): Promise<Record<string, number>> => {
    const form = new FormData();
    form.append('file', file);
    return apiFetchForm('/import/health-connect', form);
  },

  importLibra: (file: File): Promise<Record<string, number>> => {
    const form = new FormData();
    form.append('file', file);
    return apiFetchForm('/import/libra', form);
  },

  searchFood: (q: string, preparation?: string, state?: string, refresh?: boolean) => {
    const params = new URLSearchParams({ q });
    if (preparation) params.set('preparation', preparation);
    if (state) params.set('state', state);
    if (refresh) params.set('refresh', 'true');
    return apiFetch<FoodSearchResponse>(`/food/search?${params}`);
  },

  listCustomFoods: () => apiFetch<CustomFood[]>('/food/custom'),
  createCustomFood: (input: CustomFoodInput) =>
    apiFetch<CustomFood>('/food/custom', { method: 'POST', body: JSON.stringify(input) }),
  updateCustomFood: (id: string, input: CustomFoodInput) =>
    apiFetch<CustomFood>(`/food/custom/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteCustomFood: (id: string): Promise<void> =>
    apiFetchNoBody(`/food/custom/${id}`, { method: 'DELETE' }),

  uploadMeal: (file: File, hint = ''): Promise<FoodMeal> => {
    const form = new FormData();
    form.append('photo', file);
    const normalizedHint = hint.trim();
    if (normalizedHint) form.append('hint', normalizedHint);
    return apiFetchForm('/food/meals', form);
  },
  createManualMeal: (input: { name?: string; logged_at?: string; items: ManualMealItemInput[] }) =>
    apiFetch<FoodMeal>('/food/meals/manual', { method: 'POST', body: JSON.stringify(input) }),
  describeMeal: (input: { description: string; name?: string; logged_at?: string }) =>
    apiFetch<FoodMeal>('/food/meals/describe', { method: 'POST', body: JSON.stringify(input) }),
  getMeal: (id: string) => apiFetch<FoodMeal>(`/food/meals/${id}`),
  mealPhotoUrl: (id: string) => `${BASE}/food/meals/${id}/photo`,
  retryMeal: (id: string) => apiFetch<FoodMeal>(`/food/meals/${id}/retry`, { method: 'POST' }),
  clarifyMeal: (id: string, answers: string[]) =>
    apiFetch<FoodMeal>(`/food/meals/${id}/clarify`, { method: 'POST', body: JSON.stringify({ answers }) }),
  confirmMeal: (id: string, loggedAt?: string) =>
    apiFetch<FoodMeal>(`/food/meals/${id}/confirm`, {
      method: 'PUT',
      body: JSON.stringify(loggedAt ? { logged_at: loggedAt } : {}),
    }),
  // Returns the full updated meal (items + current aggregate), not just the
  // changed item — see design.md "Item mutation endpoints return the full
  // updated FoodMeal".
  patchMealItem: (mealId: string, itemId: string, input: PatchItemInput) =>
    apiFetch<FoodMeal>(`/food/meals/${mealId}/items/${itemId}`, {
      method: 'PATCH',
      body: JSON.stringify(input),
    }),
  createMealItem: (mealId: string, input: CreateItemInput) =>
    apiFetch<FoodMeal>(`/food/meals/${mealId}/items`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  deleteMealItem: (mealId: string, itemId: string) =>
    apiFetch<FoodMeal>(`/food/meals/${mealId}/items/${itemId}`, { method: 'DELETE' }),

  // GET /food/completeness — the caller's per-day Day Completeness states
  // across an inclusive Logged-Day range (design.md §5 "API"). Capped at 92
  // days server-side; callers spanning a longer range must split into
  // consecutive windows themselves (see food-day-completeness tasks.md 8.1).
  getCompleteness: (from: string, to: string) => {
    const params = new URLSearchParams({ from, to });
    return apiFetch<DayCompleteness[]>(`/food/completeness?${params}`);
  },
  // POST/DELETE /food/completeness/{date}/confirm — assert/retract that a
  // below-threshold day is nonetheless complete. Both discard the response
  // body (200/201 row on confirm, 204 on unconfirm); callers refetch
  // getCompleteness to pick up the new state.
  confirmDay: (date: string): Promise<void> =>
    apiFetchNoBody(`/food/completeness/${date}/confirm`, { method: 'POST' }),
  unconfirmDay: (date: string): Promise<void> =>
    apiFetchNoBody(`/food/completeness/${date}/confirm`, { method: 'DELETE' }),

  // GET /food/daily-totals — the caller's per-day logged-calorie totals
  // across an inclusive Logged-Day range (design.md §7 "Backend: GET
  // /api/food/daily-totals"). Capped at 92 days server-side, same as
  // getCompleteness.
  getFoodDailyTotals: (from: string, to: string) => {
    const params = new URLSearchParams({ from, to });
    return apiFetch<DailyTotal[]>(`/food/daily-totals?${params}`);
  },

  // before_id must be paired with before to get the backend's lossless
  // (logged_at, id) keyset cursor — a before-only request falls back to a
  // plain "meals logged before this instant" filter, which can drop meals
  // that share the exact logged_at at a page boundary. Always pass both
  // when continuing from a previous page (see the history page's loadMore).
  listMeals: (opts?: { limit?: number; before?: string; beforeId?: string; status?: string[] }) => {
    const params = new URLSearchParams();
    if (opts?.limit) params.set('limit', String(opts.limit));
    if (opts?.before) params.set('before', opts.before);
    if (opts?.beforeId) params.set('before_id', opts.beforeId);
    opts?.status?.forEach(s => params.append('status', s));
    const qs = params.toString();
    return apiFetch<MealSummary[]>(`/food/meals${qs ? `?${qs}` : ''}`);
  },
  needsAttentionCount: () => apiFetch<{ count: number }>('/food/meals/needs-attention-count'),
  patchMeal: (id: string, input: PatchMealInput) =>
    apiFetch<FoodMeal>(`/food/meals/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  reanalyzeMeal: async (id: string, input: ReanalyzeInput): Promise<FoodMeal> => {
    const res = await apiRawFetch(`/food/meals/${id}/reanalyze`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
    if (res.status === 502) {
      throw new ReanalyzeFailedError((await res.text()) || 'Reanalysis failed; the meal is unchanged.');
    }
    if (res.status === 412) {
      throw new ReanalyzeSupersededError(
        (await res.text()) || 'Reanalysis failed; the meal was claimed by another operation.'
      );
    }
    if (!res.ok) throw new Error((await res.text()) || `${res.status} ${res.statusText}`);
    return res.json();
  },
};

export const DATA_TYPES = [
  'steps', 'heart_rate', 'heart_rate_variability', 'sleep', 'distance',
  'active_calories', 'total_calories', 'weight', 'height', 'blood_pressure',
  'blood_glucose', 'oxygen_saturation', 'body_temperature', 'skin_temperature',
  'respiratory_rate', 'resting_heart_rate', 'exercise', 'hydration', 'nutrition',
  'basal_metabolic_rate', 'body_fat', 'lean_body_mass', 'vo2_max', 'bone_mass',
  'speed', 'food_meal', 'weight_goal',
] as const;

export type DataType = typeof DATA_TYPES[number];

// Mirrors the backend's writeAllowlist (backend/pkg/server/api.go) — the
// only types POST /api/data/{type} accepts a manual write for. Kept in sync
// by hand since it's a small, deliberately narrow set (see design.md); the
// frontend must not offer an Add-record form for any other type.
export const WRITABLE_TYPES: readonly DataType[] = ['weight', 'height', 'weight_goal'];
