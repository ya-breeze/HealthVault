## Purpose

Provides secure photo upload, resilient pre-LLM storage, OpenAI Vision recognition of food items and portion weights, and interactive clarification handling.

## ADDED Requirements

### Requirement: Photo Upload and Resilient Storage
The system SHALL save uploaded food meal photos to secure local server storage and persist a `FoodMeal` record in SQLite before initiating LLM analysis.

#### Scenario: Photo upload success
- **WHEN** an authenticated user uploads a food photo file
- **THEN** the system saves the image file to local storage, creates a `FoodMeal` record with status `processing`, and commits the transaction before invoking OpenAI Vision.

#### Scenario: LLM analysis failure
- **WHEN** the OpenAI Vision API call fails or times out
- **THEN** the system updates the `FoodMeal` status to `failed` while retaining the saved photo and allows the user to re-trigger analysis without re-uploading.

### Requirement: Authenticated Media Access
The system SHALL require user authentication to access stored meal photos and enforce tenant/user privacy boundaries.

#### Scenario: Unauthorized photo access
- **WHEN** an unauthenticated request or a different user attempts to access a photo via `/api/media/{photo_id}`
- **THEN** the system responds with a 401 Unauthorized or 404 Not Found error.

### Requirement: Food Recognition and Clarification Questions
The system SHALL analyze the food image using OpenAI Vision and return recognized food items with estimated weights in grams, confidence scores, and any clarification questions.

#### Scenario: Recognition with clarification questions
- **WHEN** the vision model detects ambiguity in cooking method or portion size
- **THEN** the model returns structured JSON containing food candidates and non-empty `clarification_questions`, and the system transitions `FoodMeal` status to `pending_clarification`.
