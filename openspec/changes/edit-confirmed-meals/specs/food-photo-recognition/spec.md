## ADDED Requirements

### Requirement: Hint-Driven Reanalysis
The system SHALL expose `POST /api/food/meals/{id}/reanalyze`, which re-runs vision analysis against the already-stored photo for a meal owned by the caller, together with a required free-text `hint` supplied by the caller. The request SHALL be rejected with HTTP 400 if `hint` is empty or whitespace-only.

Reanalyze SHALL be accepted when the meal's status is `failed`, `pending_review`, or `confirmed`, and SHALL be rejected with HTTP 409 for `processing` or `pending_clarification`. It SHALL be rejected with HTTP 409 if the meal has no stored photo, matching `Retry`'s existing guard.

Reanalyze SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, exactly as `Retry` already does, and SHALL set the meal's status to `pending_clarification` or `pending_review` according to the model's response — including when the meal's status was `confirmed` beforehand, so a reanalyzed confirmed meal returns to the normal review flow rather than remaining confirmed with stale items.

#### Scenario: Reanalyze a meal the model got wrong
- **WHEN** the owner calls reanalyze on a `pending_review` meal with hint "this is chicken and rice, not berries"
- **THEN** the system re-runs vision analysis on the stored photo with that hint included, and replaces the meal's items with the new result

#### Scenario: Reanalyze a confirmed meal reverts it to review
- **GIVEN** a `confirmed` meal
- **WHEN** the owner calls reanalyze with a hint
- **THEN** the system replaces the meal's items with the new analysis result and sets its status to `pending_review` or `pending_clarification`, no longer `confirmed`

#### Scenario: Reanalyze without a hint is rejected
- **WHEN** the owner calls reanalyze with an empty or whitespace-only `hint`
- **THEN** the system returns HTTP 400 and does not call the vision model

#### Scenario: Reanalyze a meal with no stored photo is rejected
- **WHEN** the owner calls reanalyze on a meal that has no `photo_path`
- **THEN** the system returns HTTP 409 and does not call the vision model

#### Scenario: Reanalyze rejected while analysis is in progress
- **WHEN** the owner calls reanalyze on a meal whose status is `processing` or `pending_clarification`
- **THEN** the system returns HTTP 409 and does not issue a vision request

### Requirement: Vision Hint Included in Recognition Prompt
`vision.Client.Recognize` SHALL accept an optional hint string. When non-empty, the system SHALL include it in the prompt sent to the vision model for that call, so the model's next recognition attempt is informed by the caller-supplied correction. The image itself SHALL still be sent — this differs from clarification rounds, which are text-only.

#### Scenario: Hint is included in the vision request
- **WHEN** `Recognize` is called with a non-empty hint
- **THEN** the outgoing request to the vision model includes both the photo and the hint text

#### Scenario: No hint is the unchanged normal path
- **WHEN** `Recognize` is called with an empty hint, as happens for a photo upload or a no-hint `Retry`
- **THEN** the request is unchanged from today's behavior
