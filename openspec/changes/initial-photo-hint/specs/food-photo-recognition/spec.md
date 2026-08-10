## ADDED Requirements

### Requirement: Automatic Initial Analysis with Optional Hint
The initial meal-photo upload interface SHALL make automatic analysis the default: the user SHALL be able to take or choose a photo without entering ingredients, meal components, weights, or a hint, and the model SHALL identify visible food components and estimate their gram weights from the photo.

The interface SHALL provide a secondary `Add a hint (optional)` affordance that reveals free-text entry without placing text or structured component entry in the default path. The frontend SHALL send the same hint with either photo source in the multipart `POST /api/food/meals` request.

The backend SHALL read the optional multipart `hint`, trim surrounding whitespace, and pass a non-empty normalized value into the first vision-recognition call alongside the uploaded photo. A missing, empty, or whitespace-only hint SHALL preserve the existing no-hint upload behavior.

The normalized hint, measured in Unicode characters rather than bytes, SHALL be rejected with HTTP 400 if it exceeds 500 characters. The frontend SHALL show the same limit and prevent an over-limit upload, while the backend remains authoritative. Rejection SHALL occur before the photo is saved to application storage, a `FoodMeal` is created, or the vision provider is called.

The hint SHALL apply only to this initial recognition request and SHALL NOT be persisted on the meal or automatically reused by Retry.

#### Scenario: Initial upload includes a hint
- **WHEN** a user enters "this is grilled tofu with rice, not chicken" and takes or chooses a valid meal photo
- **THEN** the frontend sends the photo and normalized hint together, and the first vision-recognition request includes both

#### Scenario: Automatic analysis requires no transcription
- **WHEN** a user opens the initial upload interface and takes or chooses a valid meal photo without opening the optional hint control
- **THEN** no ingredient, component, weight, or hint entry is required, and the model identifies the food components and estimates their weights

#### Scenario: Initial upload supplies only whitespace as a hint
- **WHEN** a user opens the optional hint control, enters only whitespace, and supplies a valid meal photo
- **THEN** the upload and first recognition proceed with an empty hint exactly as the existing no-hint path

#### Scenario: Initial hint is trimmed
- **WHEN** a user uploads a valid photo with surrounding whitespace in the multipart `hint`
- **THEN** the first vision-recognition request receives the hint without the surrounding whitespace

#### Scenario: Oversized initial hint is rejected without side effects
- **WHEN** an authenticated user uploads a valid photo with a normalized `hint` longer than 500 Unicode characters
- **THEN** the system returns HTTP 400, stores neither a photo nor a `FoodMeal`, and does not call the vision provider

#### Scenario: Hint is not retained after initial analysis
- **WHEN** an initial photo analysis with a hint completes or the resulting meal is later retried
- **THEN** the hint is absent from stored meal data and Retry does not automatically reuse it

### Requirement: Expert Component Guidance for Reanalysis
When an automatic result is substantially wrong, the reanalysis correction interface SHALL offer two mutually exclusive authoring modes: `Hint`, for the existing free-text correction, and `Expert`, for naming high-level meal components separately. Expert mode SHALL present repeatable component-name inputs with add and remove controls and SHALL explain that the model, not the user, will estimate weights from the stored photo.

Expert mode SHALL require at least one non-blank component. Before submission, the frontend SHALL trim every component name, discard blank rows, preserve the remaining order, and format the names into deterministic guidance instructing the model to treat them as separate meal components and estimate each component's weight from the photo. The final generated guidance SHALL obey the existing 500-Unicode-character hint limit; if it exceeds that limit, the frontend SHALL reject it without issuing a request.

The generated guidance SHALL be submitted as the `hint` to the existing `POST /api/food/meals/{id}/reanalyze` endpoint. Expert reanalysis SHALL therefore use the same stored photo and SHALL inherit Hint-Driven Reanalysis requirements for ownership, eligible states, atomic claiming, success, failure, and non-persistence. Expert mode SHALL NOT ask for or submit user-entered weights, calories, or macros.

#### Scenario: Correct with a free-text hint
- **WHEN** a user selects Hint mode, enters a valid correction, and requests reanalysis
- **THEN** the system submits the correction through the existing hint-driven reanalysis path

#### Scenario: Correct with expert component guidance
- **WHEN** a user selects Expert mode, enters `grilled chicken` and `red beans` as separate components, and requests reanalysis
- **THEN** the system sends deterministic guidance identifying those as separate meal components and asks the model to estimate each weight from the stored photo

#### Scenario: Expert mode does not require weights
- **WHEN** a user supplies valid component names in Expert mode
- **THEN** the interface presents no gram, calorie, or macro fields and leaves all weight estimation to the model

#### Scenario: Blank expert component list is rejected
- **WHEN** every Expert-mode component row is empty or whitespace-only
- **THEN** the interface shows an actionable validation error and does not issue a reanalysis request

#### Scenario: Oversized generated expert guidance is rejected
- **WHEN** the normalized component list plus deterministic instruction exceeds 500 Unicode characters
- **THEN** the interface shows an actionable validation error and does not issue a reanalysis request

#### Scenario: Expert mode uses the established reanalysis lifecycle
- **WHEN** valid expert component guidance is submitted for a stored meal photo
- **THEN** the same reanalysis endpoint applies the existing eligibility, concurrency, success, and non-destructive failure behavior

## MODIFIED Requirements

### Requirement: Vision Hint Included in Recognition Prompt
`vision.Client.Recognize` SHALL accept an optional hint string. When non-empty, the system SHALL include it in the prompt sent to the vision model for that call, so the recognition attempt is informed by the caller-supplied context or correction. The image itself SHALL still be sent — this differs from clarification rounds, which are text-only.

#### Scenario: Hint is included in the vision request
- **WHEN** `Recognize` is called with a non-empty hint from an initial photo upload, a free-text correction, or generated expert component guidance
- **THEN** the outgoing request to the vision model includes both the photo and the hint text

#### Scenario: No hint is the unchanged normal path
- **WHEN** `Recognize` is called with an empty hint, as happens for an upload whose optional hint is omitted or for a no-hint Retry
- **THEN** the request is unchanged from today's no-hint behavior
