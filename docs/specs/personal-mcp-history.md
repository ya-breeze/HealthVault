# Personal MCP meal history and local-day totals

Idea: https://ideaforge.ikoro.in/idea/1008

## Why

The owner can record meals through ChatGPT but cannot ask what they recorded yesterday. The personal MCP only reads an individual meal. Add self-only history and daily nutrition evidence so answers use persisted HealthVault data.

## How

Add list_food_meals and get_food_daily_totals with one shared bounded local-calendar period contract: today, yesterday, last_7_days (seven completed days), or explicit inclusive dates. Use stored timezone settings and return resolved dates and timezone. History uses keyset pagination with timestamp and UUID, excludes deleted rows and returns bounded meal summaries without raw model or photo payloads. Daily totals reuse confirmed-only aggregation and expose unknown nutrition counts, unconfirmed meals, logged-day completeness and partial today. Extract a read-only DayRange variant because the existing completeness query deletes stale confirmations. Preserve that cleanup for existing callers. Read tools resolve the configured owner each call and hold the existing capture barrier. No family selectors, goal changes, OAuth, activity or recommendations in this slice.

## Validation Commands

`make test-backend`

`make lint`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS=--retries=0`

### Task 1: Add read-only history and nutrition evidence

- [ ] Implement owner-scoped history, bounded keyset pagination and local period resolution.
- [ ] Implement confirmed daily nutrients, counts, completeness evidence and partial today without mutating stored rows.
- [ ] Register discoverable read-only MCP tools and instructions; preserve existing write tools.
- [ ] Mark completed.

### Task 2: Verify boundary behavior and prepare reviewed WIP handoff

- [ ] Test protocol discovery/calls, ownership, invalid arguments, empty periods, deleted rows, unknown macros and pagination ties.
- [ ] Test local midnight and DST periods, partial today, completeness basis and persistent read-only state.
- [ ] Run backend tests, static checks and Review Gate; fix verified findings.
- [ ] Validate deployed WIP protocol and relevant E2E cases; document exact revision and tool-refresh/owner ChatGPT acceptance steps.
- [ ] Mark completed.
