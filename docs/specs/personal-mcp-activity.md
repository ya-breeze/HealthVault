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
- [ ] Validate deployed WIP protocol and browser E2E; record exact revision and remaining owner phone acceptance.
- [ ] Mark completed.
