## Requirements

### Requirement: MCP endpoint and bearer token protection
The system SHALL expose a Model Context Protocol (MCP) server at the path prefix `/mcp`. All requests to `/mcp` SHALL require an `Authorization: Bearer <token>` header matching the value of the `HCW_MCP_TOKEN` environment variable. If `HCW_MCP_TOKEN` is empty or unset, the endpoint SHALL return HTTP 503 (not 401) so that a misconfigured deployment fails visibly rather than silently allowing unauthenticated access.

#### Scenario: Valid bearer token
- **WHEN** a request to `/mcp` includes the correct `Authorization: Bearer <token>` header
- **THEN** the request SHALL be forwarded to the MCP handler

#### Scenario: Invalid bearer token
- **WHEN** a request to `/mcp` includes a wrong or missing token
- **THEN** the system SHALL return HTTP 401

#### Scenario: Token not configured
- **WHEN** `HCW_MCP_TOKEN` is empty
- **THEN** any request to `/mcp` SHALL return HTTP 503 with body `"MCP endpoint not configured"`

---

### Requirement: list_users tool
The MCP server SHALL expose a tool named `list_users` that takes no parameters and returns a JSON array of all users in the system. Each entry SHALL contain `username` and `id` (UUID string).

#### Scenario: Users exist
- **WHEN** `list_users` is called and the system has registered users
- **THEN** the tool SHALL return a JSON array with one entry per user

#### Scenario: No users
- **WHEN** `list_users` is called and there are no users
- **THEN** the tool SHALL return an empty JSON array `[]`

#### Scenario: Storage error
- **WHEN** the database query fails
- **THEN** the tool SHALL return a result with `isError: true` and the error message as text content

---

### Requirement: query_data tool
The MCP server SHALL expose a tool named `query_data` that accepts `user` (string, required), `type` (string, required), `from` (RFC3339 string, optional), and `to` (RFC3339 string, optional). It SHALL return a JSON array of health records identical in format to `GET /api/data/{type}`. The same 25 type names and the same time-range defaulting logic (7-day window ending now) SHALL apply as the REST API.

#### Scenario: Valid query
- **WHEN** `query_data` is called with a valid username and type
- **THEN** the tool SHALL return a JSON array of health records for that user and type in the resolved time range

#### Scenario: Unknown type
- **WHEN** `query_data` is called with a `type` value not in the supported set
- **THEN** the tool SHALL return a result with `isError: true` and a message identifying the unknown type

#### Scenario: Unknown user
- **WHEN** `query_data` is called with a `user` that does not exist
- **THEN** the tool SHALL return a result with `isError: true`

#### Scenario: No records in range
- **WHEN** the query returns no records
- **THEN** the tool SHALL return an empty JSON array `[]`

---

### Requirement: summary tool
The MCP server SHALL expose a tool named `summary` that accepts `user` (string, required), `from` (RFC3339 string, optional), and `to` (RFC3339 string, optional). It SHALL return a JSON object containing `user`, `from`, `to`, `steps`, `avg_heart_rate`, and `sleep_seconds`, computed identically to `GET /api/data/summary`.

#### Scenario: Summary for existing user
- **WHEN** `summary` is called with a valid username
- **THEN** the tool SHALL return a JSON object with all six fields for the resolved time range

#### Scenario: Unknown user
- **WHEN** `summary` is called with a username that does not exist
- **THEN** the tool SHALL return a result with `isError: true`

#### Scenario: Default time range
- **WHEN** `from` and `to` are omitted
- **THEN** the tool SHALL use a 7-day window ending at the current UTC time

---

### Requirement: MCP error format
MCP tool errors (unknown user, unknown type, storage failures) SHALL be returned as `CallToolResult` with `IsError: true` and the error message as a `TextContent` item. They SHALL NOT be returned as HTTP error responses, as the MCP protocol expects all tool outcomes — including failures — to be delivered as successful HTTP 200 responses with structured content.

#### Scenario: Tool error returned as MCP result
- **WHEN** a tool encounters an error condition (unknown user, unknown type, DB error)
- **THEN** the HTTP response SHALL be 200 and the `CallToolResult.IsError` field SHALL be `true`
