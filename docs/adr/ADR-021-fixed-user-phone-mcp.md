# ADR-021: Fixed-user meal MCP through a private tunnel

## Status

Accepted

## Context

The owner wants to record meals from ChatGPT on a phone. The administrative MCP uses a shared credential and exposes read tools with user selectors. Granting that interface write access would make a personal food connection unnecessarily broad. A disconnected tool call can also be replayed after the meal row has already been committed.

## Decision

Add a separate, disabled-by-default `/phone-mcp` endpoint. Require a dedicated bearer token and a configured user UUID. Resolve that user on each tool call and reuse the existing owned-meal handlers. Keep administrative `/mcp` unchanged. Carry the bearer header from the private OpenAI tunnel runtime; ChatGPT's personal tunnel connection uses no additional authentication.

Persist an optional description request ID and normalized fingerprint in the meal row. Enforce uniqueness per user, including deleted rows. Replay matching requests, reject conflicting reuse, and keep ordinary callers without IDs compatible. Bind clarification to its expected round and persisted question version and make confirmation replay return the existing meal.

## Consequences

One connection grants access to one user's meals. Sharing with multiple users requires a separate identity design. The runtime token must stay private. WIP uses a synthetic user; selecting a real production user and deploying on the VM require owner approval. Backup captures lock individual tool calls rather than long-lived MCP streams.
