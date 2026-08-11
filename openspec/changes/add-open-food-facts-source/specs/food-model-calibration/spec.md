## MODIFIED Requirements

### Requirement: Quality and Cost Comparison

The system SHALL calculate per-model structured-output success rate, food detection precision/recall/F1, matched-item weight mean absolute error and mean absolute percentage error, p50/p95 latency, token usage, and estimated cost from the operator-supplied input, cached-input, and output rates. Detection scoring SHALL use deterministic one-to-one matching based on normalized accepted names or a shared USDA, Open Food Facts, or custom-food reference ID, and weight errors SHALL include only matched items.

#### Scenario: Select the lowest-cost acceptable model

- **WHEN** one or more candidates meet the operator-supplied minimum detection F1 and maximum weight-error thresholds
- **THEN** the report identifies the cheapest passing candidate and also presents the cost/quality Pareto frontier, without altering the configured production model

#### Scenario: No model meets quality thresholds

- **WHEN** no candidate meets all supplied quality thresholds
- **THEN** the report states that there is no recommendation and does not select or configure a production model

#### Scenario: An Open Food Facts ground-truth reference matches deterministically

- **WHEN** a calibration sample's ground truth specifies an `off_code` for an expected item, and a model's predicted item binds to that same `off_code`
- **THEN** the system counts it as a deterministic one-to-one match on the shared reference ID, the same as it would for a shared `fdc_id` or custom-food ID
