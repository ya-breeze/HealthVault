## ADDED Requirements

### Requirement: Hint-Driven Reanalysis
The system SHALL expose `POST /api/food/meals/{id}/reanalyze`, which re-runs vision analysis against the already-stored photo for a meal owned by the caller, together with a required free-text `hint` supplied by the caller. The request body SHALL be read through a size-limited reader (4 KiB) before decoding, and rejected with HTTP 413 if it exceeds that limit. The decoded `hint` SHALL be rejected with HTTP 400 if it is empty or whitespace-only, or if it exceeds 500 characters.

Reanalyze SHALL be accepted when the meal's status is `failed`, `pending_review`, or `confirmed`, and SHALL be rejected with HTTP 409 for `processing` or `pending_clarification`. It SHALL be rejected with HTTP 409 if the meal has no stored photo, matching `Retry`'s existing guard.

**Atomic claim.** Eligibility SHALL be enforced as part of a single conditional update — `UPDATE ... SET status = 'processing', clarify_round = 0, clarify_log = '' WHERE id = ? AND status IN ('failed', 'pending_review', 'confirmed')` — not as a separate read followed by a later write, so two concurrent reanalyze calls for the same meal cannot both proceed and race each other's item replacement. A call that affects zero rows SHALL return HTTP 409 without calling the vision model. The meal's prior `status`, `clarify_round`, and `clarify_log` SHALL be captured before this claim, for use on failure (below).

**Success.** On a successful recognition, Reanalyze SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, exactly as `Retry` already does, and SHALL set the meal's status to `pending_clarification` or `pending_review` according to the model's response — including when the meal's status was `confirmed` beforehand, so a reanalyzed confirmed meal returns to the normal review flow rather than remaining confirmed with stale items. The same write SHALL zero the meal's seven stored macro aggregate columns, so a meal leaving `confirmed` never displays its old totals against its new, unreviewed items.

**Failure.** On a vision error or timeout, Reanalyze SHALL NOT mark the meal `failed` and SHALL NOT modify its items or aggregate. Instead it SHALL restore the meal's `status`, `clarify_round`, and `clarify_log` to the values captured before the atomic claim, leaving the meal exactly as it was before the call, and SHALL respond with HTTP 502 and an error body rather than HTTP 200, so the caller can distinguish "reanalysis failed, nothing changed" from a normal state transition.

#### Scenario: Reanalyze a meal the model got wrong
- **WHEN** the owner calls reanalyze on a `pending_review` meal with hint "this is chicken and rice, not berries"
- **THEN** the system re-runs vision analysis on the stored photo with that hint included, and replaces the meal's items with the new result

#### Scenario: Reanalyze a confirmed meal reverts it to review
- **GIVEN** a `confirmed` meal with a nonzero stored aggregate
- **WHEN** the owner calls reanalyze with a hint and it succeeds
- **THEN** the system replaces the meal's items with the new analysis result, zeroes its stored aggregate, and sets its status to `pending_review` or `pending_clarification`, no longer `confirmed`

#### Scenario: A failed reanalysis of a confirmed meal changes nothing
- **GIVEN** a `confirmed` meal with existing items and a nonzero stored aggregate
- **WHEN** the owner calls reanalyze with a hint and the vision call fails
- **THEN** the system returns HTTP 502, and the meal's status, items, and aggregate are unchanged — still `confirmed` with its original items and totals

#### Scenario: A failed reanalysis of a pending_review meal changes nothing
- **GIVEN** a `pending_review` meal with existing items
- **WHEN** the owner calls reanalyze with a hint and the vision call fails
- **THEN** the system returns HTTP 502, and the meal's status and items are unchanged — still `pending_review` with its original items

#### Scenario: Concurrent reanalyze calls do not both proceed
- **GIVEN** a `pending_review` meal
- **WHEN** two reanalyze requests for the same meal are submitted concurrently
- **THEN** only one calls the vision model and replaces the meal's items; the other receives HTTP 409 and has no effect

#### Scenario: Stale clarification state does not leak into a fresh reanalysis
- **GIVEN** a meal that previously completed clarification rounds, now `confirmed`
- **WHEN** the owner calls reanalyze with a hint
- **THEN** the new analysis run starts from `clarify_round = 0` with no prior clarification history, and is not affected by the old round count or log

#### Scenario: Reanalyze without a hint is rejected
- **WHEN** the owner calls reanalyze with an empty or whitespace-only `hint`
- **THEN** the system returns HTTP 400 and does not call the vision model

#### Scenario: Oversized hint is rejected
- **WHEN** the owner calls reanalyze with a `hint` longer than 500 characters
- **THEN** the system returns HTTP 400 and does not call the vision model

#### Scenario: Oversized request body is rejected
- **WHEN** the owner calls reanalyze with a request body larger than 4 KiB
- **THEN** the system returns HTTP 413 before attempting to decode it or call the vision model

#### Scenario: Reanalyze a meal with no stored photo is rejected
- **WHEN** the owner calls reanalyze on a meal that has no `photo_path`
- **THEN** the system returns HTTP 409 and does not call the vision model

#### Scenario: Reanalyze rejected while analysis is in progress
- **WHEN** the owner calls reanalyze on a meal whose status is `processing` or `pending_clarification`
- **THEN** the system returns HTTP 409 and does not issue a vision request

#### Scenario: Cross-user reanalyze is rejected
- **WHEN** a user calls reanalyze on a meal owned by a different user
- **THEN** the system returns HTTP 404 and does not call the vision model

### Requirement: Vision Hint Included in Recognition Prompt
`vision.Client.Recognize` SHALL accept an optional hint string. When non-empty, the system SHALL include it in the prompt sent to the vision model for that call, so the model's next recognition attempt is informed by the caller-supplied correction. The image itself SHALL still be sent — this differs from clarification rounds, which are text-only.

#### Scenario: Hint is included in the vision request
- **WHEN** `Recognize` is called with a non-empty hint
- **THEN** the outgoing request to the vision model includes both the photo and the hint text

#### Scenario: No hint is the unchanged normal path
- **WHEN** `Recognize` is called with an empty hint, as happens for a photo upload or a no-hint `Retry`
- **THEN** the request is unchanged from today's behavior
