## Purpose

Captures weighed-food ground truth and provides a manually invoked, reproducible benchmark for comparing current vision models by recognition quality, portion-weight accuracy, latency, and estimated price.

## ADDED Requirements

### Requirement: Calibration Ground-Truth Samples
The system SHALL allow an authenticated user to save a calibration photo with one or more expected food identities and measured gram weights. Calibration samples SHALL remain user/tenant scoped, SHALL use the same protected media storage controls as meal photos, and SHALL NOT create `FoodMeal` or `Nutrition` records.

#### Scenario: Save a weighed-food sample
- **WHEN** an authenticated user uploads a food photo with valid food labels and positive measured gram weights
- **THEN** the system stores the protected photo and ground truth as a calibration sample without affecting meal history or nutrition totals

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
The system SHALL calculate per-model structured-output success rate, food detection precision/recall/F1, matched-item weight mean absolute error and mean absolute percentage error, p50/p95 latency, token usage, and estimated cost from the operator-supplied input, cached-input, and output rates. Detection scoring SHALL use deterministic one-to-one matching based on normalized accepted names or a shared USDA/custom-food reference ID, and weight errors SHALL include only matched items.

#### Scenario: Select the lowest-cost acceptable model
- **WHEN** one or more candidates meet the operator-supplied minimum detection F1 and maximum weight-error thresholds
- **THEN** the report identifies the cheapest passing candidate and also presents the cost/quality Pareto frontier

#### Scenario: No model meets quality thresholds
- **WHEN** no candidate meets all supplied quality thresholds
- **THEN** the report states that there is no recommendation and does not select or configure a production model

### Requirement: Reproducible Calibration Reports
The system SHALL write machine-readable JSON and human-readable Markdown reports containing the dataset hash and sample count, timestamp, prompt/schema versions, requested and returned model IDs, inference settings, trial count, per-call results, usage, supplied pricing, aggregate metrics, thresholds, and selection outcome.

#### Scenario: Calibration with one sample
- **WHEN** the dataset contains only one valid calibration sample
- **THEN** the run completes but both reports prominently warn that the result is not representative and should not be used as the sole basis for a production model change
