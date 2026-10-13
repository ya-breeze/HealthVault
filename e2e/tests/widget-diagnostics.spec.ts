import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import './helpers/target';

test('widget diagnostic flow survives ingestion and replay', async ({ request }) => {
  const login = await request.post('/api/auth/login', {
    data: { username: process.env.HCW_USER || 'alice', password: process.env.HCW_PASS || 'pass1' },
  });
  expect(login.ok()).toBeTruthy();
  const flow = randomUUID();
  const work = randomUUID();
  const stages = [
    ['widget_browser', 'launched'], ['widget_browser', 'returned'],
    ['widget_work', 'enqueue_requested'], ['widget_work', 'started'],
    ['widget_fetch', 'success'], ['widget_snapshot', 'success'],
    ['widget_redraw', 'started'], ['widget_redraw', 'success'], ['widget_work', 'success'],
  ];
  const events = stages.map(([operation, category]) => ({
    id: randomUUID(), occurred_at: new Date().toISOString(), operation, category,
    request_id: flow, work_id: work, trigger: 'food_entry', provider: 'home',
    snapshot_age_millis: 1000, snapshot_changed: false, attempt: 1,
    http_code: 0, duration_millis: 0, app_version: 'telemetry-e2e', android_api: 36,
  }));
  for (let attempt = 0; attempt < 2; attempt++) {
    const response = await request.post('/api/diagnostics/events', { data: { events } });
    expect(response.status()).toBe(202);
    expect((await response.json()).accepted_ids).toEqual(events.map(event => event.id));
  }
  const response = await request.get('/api/diagnostics/events');
  expect(response.status()).toBe(200);
  const saved = (await response.json()).events.filter((event: { request_id: string }) => event.request_id === flow);
  expect(saved).toHaveLength(events.length);
  for (const event of saved) {
    expect(event.work_id).toBe(work);
    expect(event.snapshot_changed).toBe(false);
    expect(event.snapshot_age_millis).toBe(1000);
  }
  const rejected = await request.post('/api/diagnostics/events', {
    data: { events: [{ ...events[0], id: randomUUID(), trigger: 'arbitrary private text' }] },
  });
  expect(rejected.status()).toBe(400);
});
