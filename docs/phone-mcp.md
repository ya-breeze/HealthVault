# Record meals from ChatGPT

Configure `HCW_PHONE_MCP_TOKEN` and `HCW_PHONE_MCP_USER_ID` in the backend environment. Use a distinct secret for the phone endpoint. Use the existing user's UUID, not the username. Leave both unset to disable `/phone-mcp`. Store managed credentials in Infisical and restrict runtime copies to their consumer.

Connect the private OpenAI Secure MCP Tunnel runtime to the backend's `/phone-mcp` URL. Inject the bearer header on the server side. Pass secret references rather than values in process arguments:

```sh
tunnel-client run \
  --control-plane.tunnel-id tunnel_YOUR_ID \
  --control-plane.api-key env:CONTROL_PLANE_API_KEY \
  --mcp.server-url url=http://127.0.0.1:8080/phone-mcp,channel=main \
  --mcp.extra-headers 'Authorization: env:PHONE_MCP_AUTHORIZATION' \
  --mcp.discovery-extra-headers 'Authorization: env:PHONE_MCP_AUTHORIZATION' \
  --health.listen-addr 127.0.0.1:0
```

Set `PHONE_MCP_AUTHORIZATION` to `Bearer ` followed by the dedicated token through a protected launcher. Set `CONTROL_PLANE_API_KEY` from the tunnel's Infisical recovery record. Keep raw HTTP payload logging disabled. On a VM, provision secrets from the home side; do not give the VM access to home-hosted Infisical or the LAN.

In ChatGPT, select the tunnel in your personal custom plugin and choose **No authentication**. The runtime supplies HealthVault's bearer header. Refresh the plugin's tools after switching from the connectivity demo to HealthVault.

Use `describe_food_meal` with a new `request_id` for each intended meal. Reuse the same ID and inputs when retrying an interrupted call. Read `status` and `saved`; a draft contributes no daily totals. Its nutrition totals preview the analyzed items; items without known macros contribute no known nutrients. Ask the returned `questions` in order. Send their answers with the returned `expected_round` and `expected_version` through `clarify_food_meal`. Send `confirm: true` through `confirm_food_meal` after the user authorizes the meal and portions. An explicit request to record a clearly specified meal counts as authorization. Say the meal was saved only when `saved` is true.

Use `get_food_meal` to recover the current state after a lost response. Use `retry_food_meal` on a failed or stale processing meal. Keep the existing meal ID. The connection cannot select another user or call administrative tools.

## WIP pilot

The pilot uses `hcw-wip`, a synthetic account, and tunnel `tunnel_6ac942c121d48191bfa9aabd07afe83f`. Test records belong to WIP. The initial runtime is temporary in the agent container; permanent supervision and production VM deployment are separate steps.

Infisical recovery for the tunnel key: project `pilot`, environment `dogfood`, root path `/`, secret `OPENAI_MCP_TUNNEL_WIP_API_KEY`. The phone bearer is recovered from project `pilot`, environment `dogfood`, root path `/`, secret `HCW_PHONE_MCP_WIP_TOKEN`. Portainer consumes its managed `HCW_PHONE_MCP_TOKEN` environment setting. The pilot launcher passes both secrets in memory to the runtime environment and keeps no plaintext key copy. Runtime metadata and the health URL are in the protected directory `/tmp/healthvault-mcp-tunnel-pilot-01a12217/`; they do not contain secrets.

The production account and deployment are not configured by this change. Require the owner to approve the reviewed PR and the concrete VM plan before changing production.


The temporary WIP runtime's `/readyz` can report a discovery failure because nginx returns HTML for nonexistent OAuth metadata. The MCP endpoint itself and the authenticated protocol workflow are verified. This diagnostic does not stop the tunnel client's Noauth dispatcher. For permanent deployment, target the backend directly so nonexistent metadata returns 404. Verify actual ChatGPT discovery after selecting **Refresh**; a healthy local protocol check alone does not prove a phone command was delivered.
