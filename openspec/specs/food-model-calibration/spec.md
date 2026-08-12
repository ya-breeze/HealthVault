# food-model-calibration Specification

## Purpose
TBD - created by archiving change photo-food-nutrition-logging. Update Purpose after archive.
## Requirements
### Requirement: Calibration Ground-Truth Samples
The system SHALL allow an authenticated user to save a calibration photo with one or more expected food identities and measured gram weights. Calibration samples SHALL remain user/tenant scoped, SHALL be subject to the same upload validation and owner-scoped photo access controls as meal photos (served via `GET /api/food/calibration-samples/{id}/photo`), and SHALL NOT create `FoodMeal` records.

#### Scenario: Save a weighed-food sample
- **WHEN** an authenticated user uploads a food photo with valid food labels and positive measured gram weights
- **THEN** the system stores the protected photo and ground truth as a calibration sample without affecting meal history

#### Scenario: Delete a calibration sample
- **WHEN** the owner deletes a calibration sample
- **THEN** the system removes the sample row and its stored photo file from disk

#### Scenario: Cross-user calibration access
- **WHEN** another user attempts to list, read, or delete a calibration sample they do not own
- **THEN** the system returns 404 Not Found and does not reveal the sample metadata or photo

### Requirement: Manual Multi-Model Calibration Run
The system SHALL provide a manually invoked CLI that evaluates explicitly supplied image-capable model IDs against a user-scoped selection of the stored calibration dataset using the same prompt version, structured output schema, image detail, and compatible inference settings. The CLI SHALL support repeated trials, set `store: false`, isolate individual call failures, and leave the production model configuration unchanged.

#### Scenario: Preview calibration cost scope
- **WHEN** an operator invokes the CLI without `--execute`
- **THEN** the command lists the sample count, models, trials per sample, and total planned external API calls without uploading any image or calling a model

#### Scenario: Execute a calibration run
- **WHEN** an operator supplies candidate model IDs, a complete pricing file, and explicit execution confirmation
- **THEN** the CLI runs every planned sample/model/trial combination, records failures without aborting remaining candidates, and captures the requested and returned model IDs, latency, token usage, and structured result for each call

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

### Requirement: Reproducible Calibration Reports
The system SHALL write machine-readable JSON and human-readable Markdown reports containing the dataset hash and sample count, timestamp, prompt/schema versions, requested and returned model IDs, inference settings, trial count, per-call results, usage, supplied pricing, aggregate metrics, thresholds, and selection outcome.

#### Scenario: Calibration with one sample
- **WHEN** the dataset contains only one valid calibration sample
- **THEN** the run completes but both reports prominently warn that the result is not representative and should not be used as the sole basis for a production model change

