# ADR-023: Discoverable health datasets in the fixed-user MCP

## Status

Accepted

## Context

The owner requires ChatGPT and Codex to read saved health evidence beyond food and activity summaries. Adding a separate tool and schema enum for every metric leaves deployed clients with an incomplete, stale type list. Granting the administrative MCP credential would broaden access to other users. Blind table discovery would expose authentication and implementation data.

## Decision

Extend the existing fixed-user MCP with a server-owned dataset catalog and generic bounded queries. Register safe projections, units, timestamp semantics and ownership rules in one health read service. Discover types at call time. Keep tool inputs independent of the dataset inventory. Keep identity resolution and the backup barrier around every call.

Preserve normalized measurements, nested health records and original webhook evidence as separate representations. Provide ordered pagination and UTF-8 JSON chunks for complete access without silent truncation. Exclude credentials, authentication, operational records and raw food-analysis artifacts. Preserve existing summary tools and their aggregation rules. Administrative MCP remains separate.

## Consequences

One existing user credential authorizes reading a larger set of that user's private health evidence after rollout. The owner must approve production activation. A new supported dataset needs one reviewed server registration and ownership/projection tests, not a new MCP tool. Unknown fields remain available in original import evidence without pretending they are normalized metrics. Actual ChatGPT and Codex acceptance remains separate from protocol tests.
