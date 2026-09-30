# ADR-018: Nutrition Chat uses Luna high through Responses

**Status: Proposed**

## Context

The owner requires GPT-6 Luna with high reasoning effort for Nutrition Chat. The chat uses read-only history tools. Luna supports reasoning with function tools through the Responses API; Chat Completions requires reasoning effort none for function tools.

## Decision

Use `gpt-6-luna` with `reasoning.effort: high` for Nutrition Chat through `/v1/responses`. Preserve `store:false` and the strict answer schema. Keep all response output items, including encrypted reasoning, only in the request-local tool loop and replay them before matching function outputs. Bound calls and retain the authenticated history execution interface. Do not use a stored response ID or persist reasoning. Other food vision and advice calls keep their configured model and Chat Completions endpoint.

This supersedes only the endpoint and disabled-reasoning part of ADR-013. The bounded history, authorization, and privacy decisions still apply.

## Consequences

The Nutrition Chat model no longer depends on the legacy photo model setting. High reasoning can increase latency; the existing request timeout bounds the whole tool loop. Provider failure returns unavailable and the user can retry the same ephemeral request.

## Sources

- https://developers.openai.com/api/docs/models/gpt-6-luna
- https://developers.openai.com/api/docs/guides/function-calling
- https://developers.openai.com/api/docs/guides/reasoning
