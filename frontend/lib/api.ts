const BASE = process.env.NEXT_PUBLIC_API_URL ?? '/api';

// Shared request setup (credentials, JSON content-type) so every JSON call —
// including ones that need to branch on a specific status code before the
// generic !res.ok handling, like reanalyzeMeal's 502 — goes through the same
// fetch configuration instead of re-declaring it.
async function apiRawFetch(path: string, options?: RequestInit): Promise<Response> {
  return fetch(`${BASE}${path}`, {
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
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
  if (!res.ok) throw new ApiError(res.status, (await res.text()) || `${res.status} ${res.statusText}`);
}

async function apiFetchForm<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
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
  profile: NutrientProfile;
}

export interface FoodSearchResponse {
  results: FoodSearchResult[];
  usda_unavailable?: boolean;
}

export interface CustomFood {
  id: string;
  name: string;
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

export type MacroSource = 'reference' | 'manual' | 'none';

export interface FoodItem {
  id: string;
  meal_id: string;
  name: string;
  preparation: string;
  state: string;
  fdc_id?: number;
  custom_food_id?: string;
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
  weight_grams?: number;
  name?: string;
  calories?: number;
  protein_grams?: number;
  carbs_grams?: number;
  fat_grams?: number;
  sugar_grams?: number;
  sodium_grams?: number;
  dietary_fiber_grams?: number;
}

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
}

export interface PatchMealInput {
  name?: string;
  logged_at?: string;
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

export const api = {
  login: (username: string, password: string) =>
    apiFetch('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => apiFetch('/auth/logout', { method: 'POST' }),

  me: () => apiFetch<{ id: string; username: string; family_id: string }>('/users/me'),

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

  deleteRecord: (type: string, id: string): Promise<void> =>
    apiFetchNoBody(`/data/${type}/${id}`, { method: 'DELETE' }),

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

  searchFood: (q: string, preparation?: string, state?: string) => {
    const params = new URLSearchParams({ q });
    if (preparation) params.set('preparation', preparation);
    if (state) params.set('state', state);
    return apiFetch<FoodSearchResponse>(`/food/search?${params}`);
  },

  listCustomFoods: () => apiFetch<CustomFood[]>('/food/custom'),
  createCustomFood: (input: CustomFoodInput) =>
    apiFetch<CustomFood>('/food/custom', { method: 'POST', body: JSON.stringify(input) }),
  updateCustomFood: (id: string, input: CustomFoodInput) =>
    apiFetch<CustomFood>(`/food/custom/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteCustomFood: (id: string): Promise<void> =>
    apiFetchNoBody(`/food/custom/${id}`, { method: 'DELETE' }),

  uploadMeal: (file: File): Promise<FoodMeal> => {
    const form = new FormData();
    form.append('photo', file);
    return apiFetchForm('/food/meals', form);
  },
  createManualMeal: (input: { name?: string; logged_at?: string; items: ManualMealItemInput[] }) =>
    apiFetch<FoodMeal>('/food/meals/manual', { method: 'POST', body: JSON.stringify(input) }),
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

  // before_id must be paired with before to get the backend's lossless
  // (logged_at, id) keyset cursor — a before-only request falls back to a
  // plain "meals logged before this instant" filter, which can drop meals
  // that share the exact logged_at at a page boundary. Always pass both
  // when continuing from a previous page (see the history page's loadMore).
  listMeals: (opts?: { limit?: number; before?: string; beforeId?: string }) => {
    const params = new URLSearchParams();
    if (opts?.limit) params.set('limit', String(opts.limit));
    if (opts?.before) params.set('before', opts.before);
    if (opts?.beforeId) params.set('before_id', opts.beforeId);
    const qs = params.toString();
    return apiFetch<MealSummary[]>(`/food/meals${qs ? `?${qs}` : ''}`);
  },
  patchMeal: (id: string, input: PatchMealInput) =>
    apiFetch<FoodMeal>(`/food/meals/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  reanalyzeMeal: async (id: string, hint: string): Promise<FoodMeal> => {
    const res = await apiRawFetch(`/food/meals/${id}/reanalyze`, {
      method: 'POST',
      body: JSON.stringify({ hint }),
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
  'speed', 'food_meal',
] as const;

export type DataType = typeof DATA_TYPES[number];
