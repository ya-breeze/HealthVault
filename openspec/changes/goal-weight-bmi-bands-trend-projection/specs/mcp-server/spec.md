## MODIFIED Requirements

### Requirement: query_data tool
The MCP server SHALL expose a tool named `query_data` that accepts `user` (string, required), `type` (string, required), `from` (RFC3339 string, optional), and `to` (RFC3339 string, optional). It SHALL return a JSON array of health records identical in format to `GET /api/data/{type}`. The same 26 type names and the same time-range defaulting logic (7-day window ending now) SHALL apply as the REST API, including the `weight_goal` type registered by the goal-weight/BMI/trend-projection change.

#### Scenario: Valid query
- **WHEN** `query_data` is called with a valid username and type
- **THEN** the tool SHALL return a JSON array of health records for that user and type in the resolved time range

#### Scenario: Query goal weight
- **WHEN** `query_data` is called with a valid username and `type: "weight_goal"`
- **THEN** the tool SHALL return a JSON array of that user's goal-weight records in the resolved time range

#### Scenario: Unknown type
- **WHEN** `query_data` is called with a `type` value not in the supported set
- **THEN** the tool SHALL return a result with `isError: true` and a message identifying the unknown type

#### Scenario: Unknown user
- **WHEN** `query_data` is called with a `user` that does not exist
- **THEN** the tool SHALL return a result with `isError: true`

#### Scenario: No records in range
- **WHEN** the query returns no records
- **THEN** the tool SHALL return an empty JSON array `[]`
