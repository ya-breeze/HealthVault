/** The WIP-only secret used by webhook tests. Never print its value. */
export function webhookAuthHeaders(): Record<string, string> {
  const token = process.env.HCW_E2E_WEBHOOK_TOKEN;
  if (!token) throw new Error('HCW_E2E_WEBHOOK_TOKEN is required for webhook E2E tests');
  return { 'X-HCW-Webhook-Token': token };
}
