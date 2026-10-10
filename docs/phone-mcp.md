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

## Read recorded food

Use `list_food_meals` for the connected user's meal history. Use `get_food_daily_totals` for complete daily sums independent of history pagination. Set `period` to `yesterday` (default), `today`, `last_7_days`, or `range` with inclusive `start_date` and `end_date`. Seven-day periods exclude today. Explicit ranges support up to 92 calendar days, including older records.

Keep the period unchanged and pass `next_cursor` into the next history call until it is empty. Use `get_food_meal` for full meal details when a summary reports `text_truncated`. Report the returned dates and timezone. Energy is kcal; nutrient fields ending in `_grams` are grams. Sodium means elemental sodium, not added salt.

Set an IANA timezone such as `Europe/Prague` in HealthVault settings. If `timezone_fallback` is true, configure that setting before assuming day boundaries match the app. Legacy `Local` settings use explicit UTC in MCP; the app may use the server timezone.

Count only confirmed meals in nutrition totals. Explain unconfirmed meals and unknown/estimated nutrient counts. An empty day means no recorded food, not fasting. Day completeness uses the same occasion-count heuristic and stored owner date flags as HealthVault. Meal changes can make an older flag outdated. Read-only queries never clean up flags. Retract and reconfirm a day's completeness in HealthVault when needed. Today reports `completeness: null`, basis `not_evaluated_today`, and `partial_today: true`; it has no completeness verdict.

After an approved rollout, open the existing HealthVault connection in ChatGPT settings and select **Refresh**. Confirm that the two read tools appear. Start a new chat with HealthVault selected and ask “Расскажи, сколько я вчера всего скушал”. Check the dates, meal list and confirmed totals against HealthVault. Then ask about today and the last seven completed days. Local protocol and WIP checks do not establish that ChatGPT used the updated tools; verify this phone scenario separately.

## Read recorded activity

Exercise evidence preserves `exercise_type` and adds `exercise_type_name` and `exercise_type_mapping`. Numeric types use the [Android Health Connect session constants](https://developer.android.com/reference/androidx/health/connect/client/records/ExerciseSessionRecord), for example `79` → `Walking` and `56` → `Running`, with mapping `health_connect`. These readable English names can be translated into the conversation language. This interpretation does not establish the record's device or source. Unknown numeric codes, signed decimal codes, truncated numeric text and empty values have a null name and mapping `unknown`; do not guess a workout. Nonnumeric names remain bounded stored text with mapping `stored_text`. Both text fields retain the 512-rune bound and `text_truncated` signal. Name lookup never rewrites persisted records or uses the different legacy `hcimport` code table.

Use `get_activity_daily_totals` for recorded steps, exercise duration and separate active/total kcal over the same local periods. Use `list_activity_exercises` for stored exercise sessions. Exercise pages default to 10 records, maximum 20; follow `next_cursor` with the same dates and timezone. Daily aggregates do not depend on exercise pagination. A shortened exercise type reports `text_truncated`.

Null totals mean no records, not zero movement or energy. Zero with records is a stored aggregate; inspect kept/dropped step counts before interpreting zero steps. Counts of recorded intervals or days never establish full-day sensor coverage. Today is partial. Exercise distance and stride are meters, duration is seconds, cadence is steps per minute and energy is kcal. Null exercise fields are unavailable; optional zero is a recorded value. Normalized rows cannot establish a device/origin or exercise calories, and raw webhook JSON is not exposed.

Whole intervals belong to the local date of their start. A cross-midnight record stays in its start day; its count or duration is not divided proportionally. Records starting before the selected period are omitted. Steps apply HealthVault's existing Step Interval Collapse across the selected period: covered intervals drop, partial overlaps remain whole. Different period bounds can change which records collapse. These totals are recorded evidence with incomplete overlap removal. Exercises and calorie intervals retain their recorded overlaps. Never add exercise steps to step totals, or active kcal to total kcal. Do not infer calorie balance or sensor coverage from these values.

Queries reject periods with more than 50,000 candidate rows in any metric and ask for a shorter period. Evidence results are capped at 64 KiB. A response-size error asks for a shorter period or smaller exercise page instead of silently truncating aggregates.

After an approved activity rollout, refresh the existing HealthVault connection in ChatGPT. Confirm `get_activity_daily_totals` and `list_activity_exercises` appear. In a new chat with HealthVault selected, ask “Сколько я вчера прошёл и какие тренировки записаны?”. Compare the returned dates, recorded steps and exercise sessions with HealthVault. Repeat for seven completed days and confirm missing records are described as unavailable. Verify an actual tool call in ChatGPT; local protocol checks alone do not prove phone acceptance. Keep Idea 1009 open until this check passes.

## Historical WIP pilot

The following notes describe the initial WIP pilot. Do not redirect an existing production connection to WIP for acceptance testing.

The pilot uses `hcw-wip`, a synthetic account, and tunnel `tunnel_6ac942c121d48191bfa9aabd07afe83f`. Test records belong to WIP. The initial runtime is temporary in the agent container; permanent supervision and production VM deployment are separate steps.

Infisical recovery for the tunnel key: project `pilot`, environment `dogfood`, root path `/`, secret `OPENAI_MCP_TUNNEL_WIP_API_KEY`. The phone bearer is recovered from project `pilot`, environment `dogfood`, root path `/`, secret `HCW_PHONE_MCP_WIP_TOKEN`. Portainer consumes its managed `HCW_PHONE_MCP_TOKEN` environment setting. The pilot launcher passes both secrets in memory to the runtime environment and keeps no plaintext key copy. Runtime metadata and the health URL are in the protected directory `/tmp/healthvault-mcp-tunnel-pilot-01a12217/`; they do not contain secrets.

The production account and deployment are not configured by this change. Require the owner to approve the reviewed PR and the concrete VM plan before changing production.


The temporary WIP runtime's `/readyz` can report a discovery failure because nginx returns HTML for nonexistent OAuth metadata. The MCP endpoint itself and the authenticated protocol workflow are verified. This diagnostic does not stop the tunnel client's Noauth dispatcher. For permanent deployment, target the backend directly so nonexistent metadata returns 404. Verify actual ChatGPT discovery after selecting **Refresh**; a healthy local protocol check alone does not prove a phone command was delivered.

On 2026-10-10 the owner refreshed the ChatGPT connection and confirmed that a phone meal command worked against WIP. The pilot now uses native managed runtime alias `healthvault-wip-food`, launched through `tunnel-client runtimes connect`; the foreground test process has been replaced. Production VM installation remains a separate deployment.
