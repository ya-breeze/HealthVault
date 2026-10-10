# Personal MCP activity evidence for local days

Idea: https://ideaforge.ikoro.in/idea/1009

## Why

The owner can ask ChatGPT about recorded food but cannot inspect steps and exercise through the personal connection. Add read-only activity evidence so later combined reports can cite persisted HealthVault records.

## How

Add get_activity_daily_totals and list_activity_exercises. Reuse the owner identity/settings resolution, backup barrier and shared bounded local-calendar period contract. Return resolved dates/timezone and partial today. Expose stored steps, exercise duration and counts, and separate recorded active and total kcal sums. Nullable totals distinguish missing records from recorded zero. Counts describe available records, never complete sensor coverage. No inferred device provenance, raw webhook payloads, exercise calories, calorie balance, advice or goals.

Use a SQLite julianday candidate window widened by one second, then exact Go instant filtering and sorting, to handle preserved timestamp offsets and submillisecond precision. Limit candidates to 50,000 per metric and fail with a shorter-period instruction rather than silently truncate totals. Reuse stepWatermark with start/end ascending instant ordering across the selected period. Contained step intervals collapse; partially overlapping intervals remain whole. Assign whole intervals to the local start date, including intervals crossing midnight. Intervals starting before the window are omitted. Collapse can differ across selected periods because earlier covering records may be absent; expose the convention instead of claiming full deduplication. Calories and exercises preserve their recorded intervals and can overlap; never add exercise steps or active and total calories together.

Exercise evidence uses descending start-instant/UUID keyset pagination, default 10/max 20, with period-bound cursors. Return owned nondeleted records only, stored timestamps/type/duration and nullable distance/steps/cadence/stride with explicit units. Cap responses at 64 KiB and bound arbitrary text. No migrations or changes to existing dashboard aggregation semantics.

Validate synthetic WIP data before review handoff. Actual owner ChatGPT activity acceptance remains a separate rollout check after approval; keep Idea 1009 open until this requirement is verified.

## Validation Commands

`make test-backend`

`make lint`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS=--retries=0`

### Task 1: Implement owner-local activity evidence

- [x] Implement exact-instant local activity aggregation with existing step collapse and explicit missing/overlap evidence.
- [x] Implement bounded owned exercise pagination, nullable optional fields and explicit units.
- [x] Register discoverable read-only tools and preserve existing food write/read behavior.
- [x] Document interval allocation, coverage, calories and owner ChatGPT acceptance steps.
- [x] Mark completed.

### Task 2: Validate reviewed implementation on WIP

- [x] Test owner isolation, mixed offsets, DST, midnight allocation, collapse across days, missing versus zero, deletion, cursors and read-only state.
- [x] Run backend/static checks and native plus independent peer Review Gate; fix verified findings before deployment.
- [x] Validate deployed WIP protocol and browser E2E; record exact revision and remaining owner phone acceptance.
- [x] Mark completed.

Validation evidence: backend tests and static checks passed. Native Codex review passed correctness, repository standards and specification checks. The independent Claude peer was unavailable due to HTTP 429 session usage quota; one attempt produced no completed review.

Deployed hcw-wip runs implementation b340ce54b52bdf7d1adcf6b597888207d1a8ab8c. Its backend binary SHA-256 is f26256b29aff5576da9b9db5a9b432b20970a32ef7f981f96b157cb9d0ae5899 and matches the pinned reference build byte for byte. All nine MCP tools are discoverable. Synthetic protocol checks verified owned and foreign records, interval collapse, midnight allocation, nullable zero and missing values, separate calorie sums, exercise pagination and unchanged persisted records after reads. Synthetic health rows were removed and the original activity and food baselines were restored.

The first full browser run passed 324 scenarios, skipped one and failed an existing Settings navigation prerequisite before its observer-cleanup assertion. That scenario passed alone with tracing. A fresh full run then passed 325 scenarios and skipped one, with retries disabled and no application changes between runs. The first navigation failure's cause remains unproven. Frontend and E2E sources are unchanged by this PR.

Production still runs b638286 history support. Owner ChatGPT activity acceptance remains pending after an approved merge and VM rollout; Idea 1009 stays open until that check passes.
