## Purpose

Provides secure photo upload, resilient pre-LLM storage, OpenAI Vision recognition of food items and portion weights, recovery after analysis failure, and bounded clarification handling.

## ADDED Requirements

### Requirement: Photo Upload Validation
The system SHALL reject an upload before storing it when the request body exceeds the configured maximum size (`HCW_MAX_UPLOAD_BYTES`, default 10 MiB) or when the sniffed content is not JPEG, PNG, or WebP. Image type SHALL be determined by sniffing the content, not by the declared `Content-Type`. The stored file path SHALL be generated entirely by the server and SHALL NOT incorporate any client-supplied filename.

The accepted set SHALL be limited to formats the vision provider also accepts. HEIC SHALL be rejected rather than stored: accepting it would let the most common iPhone capture format pass validation and then fail every subsequent analysis and retry, stranding the meal in `failed` permanently. The upload UI SHALL declare `accept="image/jpeg,image/png,image/webp"` on its file input, which causes iOS to transcode a HEIC library photo to JPEG during selection, and the in-app camera path SHALL encode JPEG client-side.

#### Scenario: Oversized upload rejected
- **WHEN** an authenticated user uploads a file larger than the configured maximum
- **THEN** the system returns HTTP 413 and stores neither a file nor a `FoodMeal` record

#### Scenario: Non-image content rejected
- **WHEN** an upload declares `Content-Type: image/jpeg` but its content is not a supported image
- **THEN** the system returns HTTP 400 and stores neither a file nor a `FoodMeal` record

#### Scenario: HEIC upload rejected with an actionable error
- **WHEN** a non-browser client uploads a HEIC image
- **THEN** the system returns HTTP 415, names HEIC as unsupported, states that JPEG, PNG and WebP are accepted, and stores neither a file nor a `FoodMeal` record

#### Scenario: Every stored photo is analyzable
- **WHEN** any photo has been accepted and stored
- **THEN** its format is one the vision provider accepts, so no stored photo can be permanently unanalyzable

#### Scenario: Client filename is not used in the stored path
- **WHEN** an upload supplies a filename such as `../../etc/passwd.jpg`
- **THEN** the system stores the photo under its own generated path derived only from the user ID and the new meal ID

### Requirement: Photo-First Resilient Storage
The system SHALL save an uploaded food photo to local storage and commit a `FoodMeal` record with status `processing` **before** initiating LLM analysis, so that no analysis outcome can cause the photo to be lost.

#### Scenario: Photo upload success
- **WHEN** an authenticated user uploads a valid food photo
- **THEN** the system saves the image file, creates a `FoodMeal` record with status `processing`, and commits that transaction before invoking OpenAI Vision

#### Scenario: LLM analysis failure
- **WHEN** the OpenAI Vision call fails or exceeds the configured timeout
- **THEN** the system sets the `FoodMeal` status to `failed`, retains the saved photo, and returns the meal record rather than an error that would discard it

### Requirement: Analysis Retry Without Re-Upload
The system SHALL expose `POST /api/food/meals/{id}/retry`, which re-runs vision analysis against the already-stored photo for a meal owned by the caller. Retry SHALL be accepted only when the meal's status is `failed`, or is `processing` **and** its `updated_at` is older than `HCW_VISION_TIMEOUT`; any other state SHALL return HTTP 409.

Because analysis is synchronous, `processing` means "a call is running right now" during normal operation and only means "crash remnant" once the timeout has elapsed. Treating it as always retryable would let a user re-tapping a slow request start a second concurrent analysis of the same meal — double-billing the provider and letting a late failure overwrite an earlier success.

Every analysis run, initial or retry, SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, so that a retry never appends a duplicate set of items.

#### Scenario: Retry a failed meal
- **WHEN** the owner calls retry on a meal with status `failed`
- **THEN** the system re-runs analysis on the stored photo without requiring a new upload and updates the meal status according to the result

#### Scenario: Retry a meal stranded by a restart
- **WHEN** the server restarted while a meal was in `processing` and its `updated_at` is older than the configured vision timeout
- **THEN** retry re-runs analysis on the stored photo and the meal leaves `processing`

#### Scenario: Retry rejected while analysis is still running
- **WHEN** the owner calls retry on a meal in `processing` whose `updated_at` is within the configured vision timeout
- **THEN** the system returns HTTP 409 and does not issue a second vision request for that meal

#### Scenario: Retry replaces prior items
- **GIVEN** a meal in `failed` that already has `FoodItem` rows from an earlier partial run
- **WHEN** retry succeeds
- **THEN** the meal's items are exactly those from the new run, with no rows left over from the earlier one

#### Scenario: Retry a meal that is already complete
- **WHEN** the owner calls retry on a meal with status `confirmed`
- **THEN** the system returns HTTP 409 and does not call the vision model

### Requirement: Authenticated Media Access
The system SHALL serve stored photos only through owner-scoped routes — `GET /api/food/meals/{id}/photo` and `GET /api/food/calibration-samples/{id}/photo` — resolving the owning record for the authenticated user before reading from the filesystem.

#### Scenario: Unauthenticated photo access
- **WHEN** a request without a valid JWT calls a photo endpoint
- **THEN** the system returns HTTP 401

#### Scenario: Cross-user photo access
- **WHEN** an authenticated user requests the photo of a meal or calibration sample owned by a different user
- **THEN** the system returns HTTP 404, does not stream the file, and does not reveal whether the record exists

### Requirement: Third-Party Transmission and Non-Retention
The system SHALL set `store: false` on every request to the external vision model, including production meal analysis and not only calibration runs. The upload interface SHALL state that the photo will be sent to an external model provider.

#### Scenario: Production analysis does not opt into provider retention
- **WHEN** the system analyzes an uploaded meal photo
- **THEN** the request sets `store: false`

#### Scenario: User is told photos leave the server
- **WHEN** a user opens the meal photo upload interface
- **THEN** the interface states that the photo will be sent to an external model provider for analysis

### Requirement: Food Recognition and Clarification Questions
The system SHALL analyze the food image using OpenAI Vision and return recognized food items with estimated weights in grams, confidence scores, and any clarification questions.

#### Scenario: Recognition with clarification questions
- **WHEN** the vision model detects ambiguity in cooking method or portion size
- **THEN** the model returns structured JSON containing food candidates and non-empty `clarification_questions`, and the system transitions `FoodMeal` status to `pending_clarification`

#### Scenario: Recognition without ambiguity
- **WHEN** the vision model returns items with no clarification questions
- **THEN** the system transitions `FoodMeal` status to `pending_review` for the user to confirm or adjust

### Requirement: Bounded Text-Only Clarification Rounds
Clarification rounds SHALL send the stored structured result and the accumulated question/answer pairs **without re-sending the image**, and SHALL be limited to a maximum of 3 rounds per meal. On exceeding the limit the meal SHALL move to `pending_review` for manual completion.

Every question asked and answer given SHALL be persisted on the meal and included in each subsequent round, so that a later round carries the answers from all earlier ones. Without that history a round-3 request cannot see the round-1 and round-2 answers and may re-ask a question the user has already answered, which itself drives the meal into the round cap.

#### Scenario: Clarification round does not resend the image
- **WHEN** a user submits answers to clarification questions
- **THEN** the follow-up model request contains the prior structured result and the answers, and contains no image content

#### Scenario: Later rounds carry earlier answers
- **WHEN** a meal reaches its third clarification round
- **THEN** the request includes the question/answer pairs from rounds one and two as well as the current one

#### Scenario: Clarification round limit reached
- **WHEN** a meal has completed 3 clarification rounds and the model returns further questions
- **THEN** the system sets the meal status to `pending_review` and does not issue another model request for that meal
