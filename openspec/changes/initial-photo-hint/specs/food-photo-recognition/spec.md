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
When an automatic result is substantially wrong, the reanalysis correction interface SHALL offer two mutually exclusive authoring modes: `Hint`, for the existing free-text correction, and `Expert`, for defining high-level meal components separately. Expert mode SHALL present repeatable rows with add and remove controls. Each row SHALL contain a component name and an optional gram weight. The interface SHALL explain that a supplied weight is used exactly and the model estimates weights left blank.

Expert mode SHALL require 1–20 components. Before submission, the frontend SHALL trim every component name and preserve row order. Every normalized name SHALL be non-blank and no longer than 100 Unicode characters, and their combined normalized length SHALL NOT exceed 500 Unicode characters. A supplied `weight_grams` SHALL be finite and greater than zero. The frontend SHALL reject invalid expert input without issuing a request, and the backend SHALL enforce the same validation authoritatively before claiming the meal or calling the vision provider.

`POST /api/food/meals/{id}/reanalyze` SHALL accept exactly one of the existing non-empty `hint` or a structured `components` array of `{name, weight_grams?}`. Supplying neither or both SHALL return HTTP 400 without claiming the meal or calling the vision provider. Expert reanalysis SHALL use the same stored photo and inherit Hint-Driven Reanalysis requirements for ownership, eligible states, atomic claiming, success, failure, and non-persistence.

For expert reanalysis, the system SHALL ask the model to analyze exactly the supplied components in the supplied order and estimate a weight only where `weight_grams` was omitted. Before nutrition resolution and persistence, the system SHALL use each normalized user-supplied name as the resulting item's name and SHALL overwrite each supplied weight with the exact user value. A successful expert result SHALL contain exactly one item per supplied component in the same order. If the model does not return a one-to-one result or does not provide a finite positive estimate for an omitted weight, the attempt SHALL fail through the existing non-destructive reanalysis failure behavior.

Because the expert list resolves the plate composition explicitly, a successful expert recognition SHALL proceed to candidate resolution and `pending_review` without entering model-generated clarification rounds. Expert mode SHALL NOT ask for or submit user-entered calories or macros.

#### Scenario: Correct with a free-text hint
- **WHEN** a user selects Hint mode, enters a valid correction, and requests reanalysis
- **THEN** the system submits the correction through the existing hint-driven reanalysis path

#### Scenario: Correct with expert component guidance
- **WHEN** a user selects Expert mode, enters `grilled chicken` at 180 grams and `red beans` with no weight, and requests reanalysis
- **THEN** the successful result contains those two components in that order, uses exactly 180 grams for grilled chicken, and uses the model's photo-derived estimate for red beans

#### Scenario: All expert weights may be supplied
- **WHEN** a user enters a valid positive gram weight for every Expert-mode component
- **THEN** every successful resulting item uses the corresponding user-defined weight exactly rather than a model estimate

#### Scenario: All expert weights may be omitted
- **WHEN** a user leaves the gram weight blank for every Expert-mode component
- **THEN** the model estimates every component's weight from the stored photo

#### Scenario: Invalid expert component is rejected
- **WHEN** Expert mode contains a blank component name, more than 20 components, a name longer than 100 Unicode characters, or combined names longer than 500 Unicode characters
- **THEN** the request is rejected with HTTP 400 and does not claim the meal or call the vision provider

#### Scenario: Invalid expert weight is rejected
- **WHEN** an Expert-mode component supplies a zero, negative, non-finite, or otherwise invalid `weight_grams`
- **THEN** the request is rejected with HTTP 400 and does not claim the meal or call the vision provider

#### Scenario: Hint and expert components are mutually exclusive
- **WHEN** a reanalysis request supplies both `hint` and `components`, or supplies neither
- **THEN** the system returns HTTP 400 without claiming the meal or calling the vision provider

#### Scenario: Provider result cannot be reconciled to expert input
- **WHEN** the provider omits, adds, or cannot estimate a required component so its response is not a valid one-to-one result
- **THEN** the system applies the existing non-destructive reanalysis failure behavior and leaves the meal unchanged

#### Scenario: Expert mode uses the established reanalysis lifecycle
- **WHEN** valid expert component guidance is submitted for a stored meal photo
- **THEN** the same reanalysis endpoint applies the existing eligibility, concurrency, success, and non-destructive failure behavior

#### Scenario: Expert mode skips clarification
- **WHEN** the model would otherwise ask a clarification question during valid expert reanalysis
- **THEN** the system uses the explicit expert components, proceeds to `pending_review`, and does not enter `pending_clarification`

## MODIFIED Requirements

### Requirement: Hint-Driven Reanalysis
The system SHALL expose `POST /api/food/meals/{id}/reanalyze`, which re-runs vision analysis against the already-stored photo for a meal owned by the caller, together with exactly one caller-supplied guidance form: a non-empty free-text `hint`, or structured expert `components` as defined by the Expert Component Guidance for Reanalysis requirement. The request body SHALL be read through a size-limited reader (4 KiB) before decoding, and rejected with HTTP 413 if it exceeds that limit. A decoded `hint`, measured in Unicode characters (not bytes), SHALL be rejected with HTTP 400 if it is empty or whitespace-only, or if it exceeds 500 characters. Supplying both guidance forms or neither SHALL be rejected with HTTP 400.

Reanalyze SHALL be accepted when the meal's status is `failed`, `pending_review`, or `confirmed`, and SHALL be rejected with HTTP 409 for `processing` or `pending_clarification`. It SHALL be rejected with HTTP 409 if the meal has no stored photo, matching `Retry`'s existing guard.

**Atomic claim with a lease token.** Eligibility SHALL be enforced as part of a single conditional update, matched against the *exact* `status` and `updated_at` observed when the meal was loaded for this request — not a broader `status IN (...)` match — and SHALL write a fresh `updated_at` as this attempt's lease token: `UPDATE ... SET status = 'processing', clarify_round = 0, clarify_log = '', updated_at = <lease> WHERE id = ? AND status = <observed status> AND updated_at = <observed updated_at>`. This is not a separate read followed by a later write, so two concurrent reanalyze calls for the same meal cannot both proceed and race each other's item replacement. A call that affects zero rows SHALL return HTTP 409 without calling the vision model. Matching on the exact observed status (not just membership in the eligible set) additionally prevents claiming a meal that a *different* concurrent operation (e.g. `ConfirmMeal`) has already transitioned since it was loaded. The meal's prior `status`, `clarify_round`, and `clarify_log` SHALL be captured before this claim, for use on failure (below).

Every later write this attempt makes — persisting a successful analysis, or reverting on failure — SHALL be conditioned on `updated_at` still matching the lease this claim wrote. `Retry`'s own eligibility (accepting `processing` once `updated_at` is older than the configured vision timeout) can overlap with a meal this handler has itself put into `processing`: if this attempt's own vision call runs long enough to cross that same threshold, a concurrent `Retry` can legitimately claim the meal as stale before this attempt's failure handling runs. The lease ensures that when a *newer* attempt has claimed the meal, this attempt's later writes become no-ops rather than overwriting or reverting over that newer attempt's in-flight or completed work.

**Success.** On a successful recognition, Reanalyze SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, exactly as `Retry` already does, and SHALL set the meal's status to `pending_clarification` or `pending_review` according to the model's response — including when the meal's status was `confirmed` beforehand, so a reanalyzed confirmed meal returns to the normal review flow rather than remaining confirmed with stale items. Expert reanalysis is the exception specified by Expert Component Guidance for Reanalysis: its explicit component list proceeds directly to `pending_review`. The same write SHALL zero the meal's seven stored macro aggregate columns, so a meal leaving `confirmed` never displays its old totals against its new, unreviewed items, and SHALL be conditioned on the lease token described above, so a persistence failure or a lease loss cannot leave the item replacement partially applied.

**Failure.** A candidate-selection error (the model call that matches recognized items against retrieved candidates) SHALL be treated as a reanalysis failure, the same as a recognition error, an unreconcilable expert result, or a timeout — it SHALL NOT be silently absorbed into an unresolved item set the way it may be for upload/retry/clarify. Selection shares this call's overall timeout with recognition, so a slow recognition call can leave selection to fail with a deadline; absorbing that failure here would let the destructive item-replacement path below run on what is actually a vision-provider failure, silently discarding a confirmed meal's reviewed items for a degraded, entirely unresolved set. On such an analysis failure, if this attempt's lease is still current, Reanalyze SHALL NOT mark the meal `failed` and SHALL NOT modify its items or aggregate. Instead it SHALL restore the meal's `status`, `clarify_round`, and `clarify_log` to the values captured before the atomic claim, leaving the meal exactly as it was before the call, and SHALL respond with HTTP 502 and an error body rather than HTTP 200, so the caller can distinguish "reanalysis failed, nothing changed" from a normal state transition. If the lease is no longer current (a newer attempt has since claimed the meal), the revert SHALL be a no-op — the newer attempt owns the row — and the response SHALL be HTTP 412 (Precondition Failed), not 502: this attempt's own reanalysis did fail, but the specific "the meal is unchanged" guarantee no longer holds once another attempt owns the row, and the caller SHALL treat HTTP 412 as a signal to refetch the meal rather than trust its previous view of it.

#### Scenario: Reanalyze a meal the model got wrong
- **WHEN** the owner calls reanalyze on a `pending_review` meal with hint "this is chicken and rice, not berries"
- **THEN** the system re-runs vision analysis on the stored photo with that hint included, and replaces the meal's items with the new result

#### Scenario: Reanalyze a confirmed meal reverts it to review
- **GIVEN** a `confirmed` meal with a nonzero stored aggregate
- **WHEN** the owner calls reanalyze with valid hint or expert guidance and it succeeds
- **THEN** the system replaces the meal's items with the new analysis result, zeroes its stored aggregate, and sets its status to `pending_review` or (for hint guidance only) `pending_clarification`, no longer `confirmed`

#### Scenario: A failed reanalysis of a confirmed meal changes nothing
- **GIVEN** a `confirmed` meal with existing items and a nonzero stored aggregate
- **WHEN** the owner calls reanalyze with valid guidance and the analysis fails
- **THEN** the system returns HTTP 502, and the meal's status, items, and aggregate are unchanged — still `confirmed` with its original items and totals

#### Scenario: A candidate-selection failure is treated as a reanalysis failure
- **GIVEN** a `confirmed` meal with existing, reviewed items and a nonzero stored aggregate
- **WHEN** the owner calls reanalyze with valid guidance, recognition succeeds, but the subsequent candidate-selection call fails (e.g. a context deadline from a slow recognition call consuming the shared timeout)
- **THEN** the system returns HTTP 502, and the meal's status, items, and aggregate are unchanged — it does not replace the reviewed items with an unresolved set

#### Scenario: A failed reanalysis of a pending_review meal changes nothing
- **GIVEN** a `pending_review` meal with existing items
- **WHEN** the owner calls reanalyze with valid guidance and the analysis fails
- **THEN** the system returns HTTP 502, and the meal's status and items are unchanged — still `pending_review` with its original items

#### Scenario: Concurrent reanalyze calls do not both proceed
- **GIVEN** a `pending_review` meal
- **WHEN** two reanalyze requests for the same meal are submitted concurrently
- **THEN** only one calls the vision model and replaces the meal's items; the other receives HTTP 409 and has no effect

#### Scenario: A concurrent confirm invalidates a stale claim attempt
- **GIVEN** a `pending_review` meal
- **WHEN** `ConfirmMeal` transitions it to `confirmed` in the gap between this request loading the meal and its own claim running
- **THEN** the claim fails (0 rows affected) and the system returns HTTP 409, rather than claiming the meal and later reverting it to the stale `pending_review` it observed before the concurrent confirm

#### Scenario: A failed attempt does not revert a newer attempt's claim
- **GIVEN** a `pending_review` meal, and this request's reanalysis attempt claimed it and is now failing
- **WHEN** a concurrent `Retry` claims the same meal (e.g. because this attempt's own call ran long enough to look stale) before this attempt's revert runs
- **THEN** this attempt's revert is a no-op — the meal's status stays whatever the concurrent `Retry` claim set it to, not reverted to this attempt's stale prior value — and this request returns HTTP 412, not 502

#### Scenario: Stale clarification state does not leak into a fresh reanalysis
- **GIVEN** a meal that previously completed clarification rounds, now `confirmed`
- **WHEN** the owner calls reanalyze with valid hint or expert guidance
- **THEN** the new analysis run starts from `clarify_round = 0` with no prior clarification history, and is not affected by the old round count or log

#### Scenario: Reanalyze without guidance is rejected
- **WHEN** the owner calls reanalyze with neither a non-empty `hint` nor valid expert `components`
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
`vision.Client.Recognize` SHALL accept an optional hint string. When non-empty, the system SHALL include it in the prompt sent to the vision model for that call, so the recognition attempt is informed by the caller-supplied context or correction. The image itself SHALL still be sent — this differs from clarification rounds, which are text-only.

#### Scenario: Hint is included in the vision request
- **WHEN** `Recognize` is called with a non-empty hint from an initial photo upload or a free-text correction
- **THEN** the outgoing request to the vision model includes both the photo and the hint text

#### Scenario: No hint is the unchanged normal path
- **WHEN** `Recognize` is called with an empty hint, as happens for an upload whose optional hint is omitted or for a no-hint Retry
- **THEN** the request is unchanged from today's no-hint behavior
