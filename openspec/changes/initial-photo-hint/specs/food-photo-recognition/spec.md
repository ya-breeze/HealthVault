## ADDED Requirements

### Requirement: Optional Hint on Initial Photo Analysis
The initial meal-photo upload interface SHALL allow the user to enter an optional free-text hint before taking or choosing a photo. The frontend SHALL send the same hint with either photo source in the multipart `POST /api/food/meals` request.

The backend SHALL read the optional multipart `hint`, trim surrounding whitespace, and pass a non-empty normalized value into the first vision-recognition call alongside the uploaded photo. A missing, empty, or whitespace-only hint SHALL preserve the existing no-hint upload behavior.

The normalized hint, measured in Unicode characters rather than bytes, SHALL be rejected with HTTP 400 if it exceeds 500 characters. The frontend SHALL show the same limit and prevent an over-limit upload, while the backend remains authoritative. Rejection SHALL occur before the photo is saved to application storage, a `FoodMeal` is created, or the vision provider is called.

The hint SHALL apply only to this initial recognition request and SHALL NOT be persisted on the meal or automatically reused by Retry.

#### Scenario: Initial upload includes a hint
- **WHEN** a user enters "this is grilled tofu with rice, not chicken" and takes or chooses a valid meal photo
- **THEN** the frontend sends the photo and normalized hint together, and the first vision-recognition request includes both

#### Scenario: Initial upload omits the hint
- **WHEN** a user takes or chooses a valid meal photo without entering a hint, or enters only whitespace
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

## MODIFIED Requirements

### Requirement: Vision Hint Included in Recognition Prompt
`vision.Client.Recognize` SHALL accept an optional hint string. When non-empty, the system SHALL include it in the prompt sent to the vision model for that call, so the recognition attempt is informed by the caller-supplied context or correction. The image itself SHALL still be sent — this differs from clarification rounds, which are text-only.

#### Scenario: Hint is included in the vision request
- **WHEN** `Recognize` is called with a non-empty hint from an initial photo upload or hint-driven reanalysis
- **THEN** the outgoing request to the vision model includes both the photo and the hint text

#### Scenario: No hint is the unchanged normal path
- **WHEN** `Recognize` is called with an empty hint, as happens for an upload whose optional hint is omitted or for a no-hint Retry
- **THEN** the request is unchanged from today's no-hint behavior
