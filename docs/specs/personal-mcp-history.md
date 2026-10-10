# Personal MCP meal history and local-day totals

Idea: https://ideaforge.ikoro.in/idea/1008

## Why

The owner can record meals through ChatGPT but cannot ask what they recorded yesterday. The personal MCP only reads an individual meal. Add self-only history and daily nutrition evidence so answers use persisted HealthVault data.

## How

Add list_food_meals and get_food_daily_totals with one shared bounded local-calendar period contract: today, yesterday, last_7_days (seven completed days), or explicit inclusive dates. Use stored timezone settings and return resolved dates and timezone. History uses keyset pagination with timestamp and UUID, excludes deleted rows and returns bounded meal summaries without raw model or photo payloads. Daily totals reuse confirmed-only aggregation and expose unknown nutrition counts, unconfirmed meals, logged-day completeness and partial today. Extract a read-only DayRange variant because the existing completeness query deletes stale confirmations. Preserve that cleanup for existing callers. Read tools resolve the configured owner each call and hold the existing capture barrier. No family selectors, goal changes, OAuth, activity or recommendations in this slice.

Calendar arithmetic uses date keys independently of local midnight instants. Share boundary resolution with existing daily totals/completeness so historical midnight DST transitions do not rewrite the requested day. Return explicit kcal/gram units and cap encoded evidence results.

Today has no completeness verdict (`null`). Read-only completeness preserves the same stored owner date flags and heuristic as existing HealthVault callers. It never deletes stale flags. An assertion can therefore outlive changes to the recorded meals; document this limitation and instruct the owner to retract/reconfirm when needed. This slice does not redesign day confirmation writes.

Use stored IANA timezone settings. Legacy `Local` settings fall back explicitly to UTC in MCP instead of depending on a server host timezone; set an IANA timezone to align all app and MCP boundaries.

## Validation Commands

`make test-backend`

`make lint`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS=--retries=0`

### Task 1: Add read-only history and nutrition evidence

- [x] Implement owner-scoped history, bounded keyset pagination and local period resolution.
- [x] Implement confirmed daily nutrients, counts, completeness evidence and partial today without mutating stored rows.
- [x] Register discoverable read-only MCP tools and instructions; preserve existing write tools.
- [x] Mark completed.

### Task 2: Verify boundary behavior and prepare reviewed WIP handoff

- [x] Test protocol discovery/calls, ownership, invalid arguments, empty periods, deleted rows, unknown macros and pagination ties.
- [x] Test local midnight and DST periods, partial today, completeness basis and persistent read-only state.
- [x] Run backend tests, static checks and Review Gate; fix verified findings.
- [x] Validate deployed WIP protocol and relevant E2E cases; document exact revision and tool-refresh/owner ChatGPT acceptance steps.
- [x] Mark completed.

## Validation evidence

On 2026-10-10, backend tests and static checks passed. Review Gate completed with three native Codex review angles and independent Claude peer review; verified findings were fixed before deployment.

`hcw-wip` served implementation revision `78c433dbb183322469f3cce5adc5c42d73d9db58`. Its running backend binary matched the pinned source build byte-for-byte (SHA-256 `e7cbc6fd958468796ca04da6b3a06a6fed8f132fd321112b78df433f8fa93ac1`). MCP discovery returned seven tools, including two read-only history tools. Synthetic protocol checks passed for tied-time pagination, foreign-user exclusion, unchanged persisted meal records, partial today, seven completed days and empty history. Fixtures were removed and baseline totals restored.

The final full browser run against WIP passed 325 tests with one skipped, with retries disabled. An earlier run lacked the required webhook fixture token and hit one mobile logout navigation assertion. All nine affected cases passed in a targeted rerun with the correct environment, then the final full run passed without application changes.

Production remains unchanged. Owner acceptance in ChatGPT is pending after an approved rollout; `docs/phone-mcp.md` specifies tool refresh and yesterday/today/week checks. These WIP results do not establish phone acceptance.
