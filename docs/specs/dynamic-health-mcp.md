# Discover and read complete user health history through MCP

Idea: https://ideaforge.ikoro.in/idea/1344

## Why

The connected ChatGPT and Codex MCP exposes food and a narrow activity subset while HealthVault saves many other health measurements. Agents cannot discover the saved measurement types or retrieve pulse, sleep, weight and original import evidence through this user connection.

## How

Extend the existing fixed-user `/phone-mcp` with a live dataset catalog and generic bounded record reads. Keep the dataset selector a string rather than a static JSON Schema enum. Put server-owned dataset descriptions, safe projections, units and read behavior behind one shared service. New supported datasets require one server registration; discovery and reading use the same registration without client changes. Include every normalized health metric, sleep stages and owned food/weather/location evidence. Preserve existing food and activity tools. Provide paginated access to original webhook evidence, including unknown future fields, without dropping large payloads or returning secrets/operational records. Scope parent and child queries to the configured user and family. Use stable keyset cursors bound to the dataset, user and filters. Return explicit missing-data, source and time semantics. Keep original imported data distinct from normalized records and warn against double counting. Do not repair Android import or mutate real data in this change.

Use standard Streamable HTTP MCP tools, structured content with text fallback and explicit read-only annotations. Document ChatGPT/Codex discovery and refresh requirements. Protocol acceptance and actual client acceptance remain distinct. Keep administrative MCP and existing credentials unchanged.

Validation on 2026-11-08: backend tests and static checks passed. Native correctness, standards and spec/tests reviews found no unresolved findings. The Claude peer was unavailable because its five-hour usage limit stopped the single direct system-review attempt before a review subagent completed. Disposable run `a54c914545ee475d9354684bbd726cc1` passed all 336 tests across 22 captured spec files and completed cleanup. Its verified snapshot used implementation revision `e605f086cd44dbfdbda31bbb532dd7e91937d132`; the final checklist update changes this document only. The disposable regression profile keeps personal MCP disabled; dedicated HTTP integration tests validate its handler and both tested protocol versions. Production rollout and refreshed installed ChatGPT/Codex acceptance require the owner's release decision and remain pending.

## Validation Commands

`make test-backend`

`make lint`

`make test-e2e-disposable DISPOSABLE_HTTP_PORT=<free-port> DISPOSABLE_SESSION=<session>`

### Task 1: Add the health dataset read service

- [x] Register every saved health dataset with fields, units, source/time semantics and user-scoped availability.
- [x] Implement bounded, ordered pagination and full raw-import evidence access with strict identity/filter binding and safe errors.
- [x] Test every dataset, nested ownership, deleted rows, projection exclusions, large imports and extension through a single registration.
- [x] Mark completed

### Task 2: Expose portable generic MCP tools

- [x] Add live dataset discovery and generic data reads to the existing fixed-user MCP without a static type enum.
- [x] Preserve existing tools and identity/backup-barrier behavior; include structured and textual results.
- [x] Test actual HTTP MCP discovery/calls, pagination, errors, isolation and compatible schema output.
- [x] Document dataset semantics, extension and ChatGPT/Codex connection refresh and verification.
- [x] Mark completed

### Task 3: Validate and prepare the change for review

- [x] Run backend tests, static checks and native/cross-platform Review Gate; resolve valid findings.
- [x] Validate the exact reviewed checkout through the full disposable E2E suite and verify cleanup.
- [x] Publish the PR and exact validation evidence; state remaining production and client acceptance gates.
- [x] Mark completed
