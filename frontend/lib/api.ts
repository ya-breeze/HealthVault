const BASE = process.env.NEXT_PUBLIC_API_URL ?? '/api';

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
  if (!res.ok) throw new Error((await res.text()) || `${res.status} ${res.statusText}`);
  return res.json();
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

export const api = {
  login: (username: string, password: string) =>
    apiFetch('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  logout: () => apiFetch('/auth/logout', { method: 'POST' }),

  me: () => apiFetch<{ id: string; username: string; family_id: string }>('/users/me'),

  data: (type: string, from?: string, to?: string, user?: string) => {
    const params = new URLSearchParams();
    if (from) params.set('from', from);
    if (to) params.set('to', to);
    if (user) params.set('user', user);
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

  deleteRecord: async (type: string, id: string): Promise<void> => {
    const res = await fetch(`${BASE}/data/${type}/${id}`, {
      method: 'DELETE',
      credentials: 'include',
    });
    if (!res.ok) throw new Error((await res.text()) || `${res.status} ${res.statusText}`);
  },

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
};

export const DATA_TYPES = [
  'steps', 'heart_rate', 'heart_rate_variability', 'sleep', 'distance',
  'active_calories', 'total_calories', 'weight', 'height', 'blood_pressure',
  'blood_glucose', 'oxygen_saturation', 'body_temperature', 'skin_temperature',
  'respiratory_rate', 'resting_heart_rate', 'exercise', 'hydration', 'nutrition',
  'basal_metabolic_rate', 'body_fat', 'lean_body_mass', 'vo2_max', 'bone_mass',
  'speed',
] as const;

export type DataType = typeof DATA_TYPES[number];
