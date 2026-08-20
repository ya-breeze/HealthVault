<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# food-photo-recognition Specification

## Purpose
TBD - created by archiving change photo-food-nutrition-logging. Update Purpose after archive.
## Requirements
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

The claim into `processing` SHALL be a single conditional update matched against the exact `status` and `updated_at` observed when the meal was loaded (an optimistic-concurrency check), not a blind write following a separate read — closing the same class of double-claim race this requirement's own prior text already worried about, but for two *concurrent* retry-eligible requests observing the same row, not just a single re-tapping user. The claim SHALL write a fresh `updated_at` as this attempt's lease token, and every later write this attempt makes (persisting a successful analysis, or marking the meal `failed`) SHALL be conditioned on that lease still being current — since `Reanalyze` (see the Hint-Driven Reanalysis requirement) can put a meal into `processing` too, and a slow `Reanalyze` attempt crossing the same vision-timeout threshold this requirement uses to judge staleness can result in a concurrent `Retry` legitimately claiming the same meal; the lease ensures whichever attempt is superseded can't overwrite the other's in-flight or completed work.

Every analysis run, initial or retry, SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, so that a retry never appends a duplicate set of items.

If this attempt's own lease is lost by the time it would persist its outcome (a newer attempt superseded it — see above), the HTTP response SHALL reflect the meal's actual current state (reloaded from storage), not this attempt's local, now-stale view of it — a caller must never be shown a status this attempt itself set but which the database has since moved past. If that reload itself fails — including because the meal no longer exists, e.g. a newer attempt deleted it — the system SHALL return the corresponding error (HTTP 404 if the meal is gone, HTTP 500 for any other reload failure) rather than falling back to this attempt's known-stale local view.

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

#### Scenario: Retry responds with the current state when superseded
- **GIVEN** this attempt claimed a meal and is about to persist its analysis outcome
- **WHEN** a newer attempt (e.g. a concurrent `Reanalyze`) claims and completes against the same meal first, so this attempt's own write does not apply
- **THEN** the response reflects the meal's actual current state, not this attempt's own stale local status

#### Scenario: Retry reports not found when a superseding operation deletes the meal
- **GIVEN** this attempt claimed a meal and is about to persist its analysis outcome
- **WHEN** a newer attempt deletes the meal entirely before this attempt's own write applies
- **THEN** the response is HTTP 404, not a 200 response carrying this attempt's stale, no-longer-accurate local view of the meal

#### Scenario: Concurrent retry claims do not both proceed
- **GIVEN** a meal eligible for retry (`failed`, or stale `processing`)
- **WHEN** two retry requests for the same meal are submitted concurrently
- **THEN** only one claims the meal and calls the vision model; the other receives HTTP 409 and has no effect

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

### Requirement: Recognized Item Preparation and State
Each recognized item SHALL carry a `preparation` (one of `raw`, `boiled`, `steamed`, `roasted`, `baked`, `grilled`, `fried`, `breaded_fried`, `braised`, or unknown) and a `state` (`raw`, `cooked`, or unknown) alongside its name. Both SHALL be permitted to be unknown.

These exist because the reference data encodes preparation and state as trailing description qualifiers, so a name-only lookup leaves those tokens unused and ranks short processed entries above the canonical whole food. State also drives magnitude directly: raw and cooked forms of the same food differ by roughly a factor of three for grains.

#### Scenario: Preparation and state accompany a recognized item
- **WHEN** the vision model recognizes a food whose preparation is evident from the photo
- **THEN** the returned item includes both `preparation` and `state` alongside its name, weight, and confidence

#### Scenario: Preparation is not evident
- **WHEN** the vision model cannot determine preparation or state from the photo
- **THEN** it returns them as unknown rather than guessing, and the item is still returned with its name and weight

#### Scenario: Preparation and state are persisted
- **WHEN** an item is stored
- **THEN** its `preparation` and `state` are persisted with it, so that a later clarification answer can re-run food lookup without re-analyzing the photo

### Requirement: Clarification Answers Refine Food Lookup
When a clarification answer resolves a previously unknown preparation or state, the system SHALL re-run food lookup for the affected item using the enriched terms.

#### Scenario: Cooking method answer improves the match
- **GIVEN** an item whose preparation was unknown and whose food lookup produced no suitable candidate
- **WHEN** the user answers a clarification question identifying the cooking method
- **THEN** the system re-runs lookup for that item with the answer included and offers the updated candidates

### Requirement: Food Recognition and Clarification Questions
The system SHALL analyze the food image using OpenAI Vision and return recognized food items with estimated weights in grams, confidence scores, and any clarification questions. Each recognized item SHALL include a Display Name, produced in the caller's current Display Language (see `display-language` "Display Language Passed to Recognition"), and a Canonical Name, produced in English in the same model call. When the caller's Display Language is English, the Canonical Name SHALL be omitted (or left empty) rather than duplicating the Display Name in storage.

#### Scenario: Recognition with clarification questions
- **WHEN** the vision model detects ambiguity in cooking method or portion size
- **THEN** the model returns structured JSON containing food candidates and non-empty `clarification_questions`, and the system transitions `FoodMeal` status to `pending_clarification`

#### Scenario: Recognition without ambiguity
- **WHEN** the vision model returns items with no clarification questions
- **THEN** the system transitions `FoodMeal` status to `pending_review` for the user to confirm or adjust

#### Scenario: Recognition in a non-English Display Language returns both names
- **WHEN** a user whose Display Language is `ru` uploads a food photo
- **THEN** each recognized item SHALL carry a Display Name in Russian and a Canonical Name in English, both produced by the same recognition call

#### Scenario: Recognition in English Display Language does not duplicate the name
- **WHEN** a user whose Display Language is English (or unset) uploads a food photo
- **THEN** each recognized item SHALL carry a Display Name in English, and its Canonical Name SHALL be empty rather than a duplicate copy of the same English text

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

### Requirement: Hint-Driven Reanalysis
The system SHALL expose `POST /api/food/meals/{id}/reanalyze`, which either re-runs analysis against the already-stored photo or applies structured expert components to that photo's meal, for a meal owned by the caller. The request SHALL contain exactly one caller-supplied guidance form selected by top-level JSON field presence: a free-text `hint`, or structured expert `components` as defined by the Expert Component Guidance for Reanalysis requirement. The request body SHALL be read through a size-limited reader (4 KiB) before decoding, and rejected with HTTP 413 if it exceeds that limit. If both guidance keys are present — even with empty, whitespace-only, or `null` values — or neither is present, the request SHALL be rejected with HTTP 400. A sole decoded `hint`, measured in Unicode characters (not bytes), SHALL be rejected with HTTP 400 if it is null, empty or whitespace-only, or if it exceeds 500 characters. A sole `components` value SHALL be rejected with HTTP 400 unless it is a non-null valid expert array.

Reanalyze SHALL be accepted when the meal's status is `failed`, `pending_review`, or `confirmed`, and SHALL be rejected with HTTP 409 for `processing` or `pending_clarification`. It SHALL be rejected with HTTP 409 if the meal has no stored photo, matching `Retry`'s existing guard.

**Atomic claim with a lease token.** Eligibility SHALL be enforced as part of a single conditional update, matched against the *exact* `status` and `updated_at` observed when the meal was loaded for this request — not a broader `status IN (...)` match — and SHALL write a fresh `updated_at` as this attempt's lease token: `UPDATE ... SET status = 'processing', clarify_round = 0, clarify_log = '', updated_at = <lease> WHERE id = ? AND status = <observed status> AND updated_at = <observed updated_at>`. This is not a separate read followed by a later write, so two concurrent reanalyze calls for the same meal cannot both proceed and race each other's item replacement. A call that affects zero rows SHALL return HTTP 409 without calling the vision model. Matching on the exact observed status (not just membership in the eligible set) additionally prevents claiming a meal that a *different* concurrent operation (e.g. `ConfirmMeal`) has already transitioned since it was loaded. The meal's prior `status`, `clarify_round`, and `clarify_log` SHALL be captured before this claim, for use on failure (below).

Every later write this attempt makes — persisting a successful analysis, or reverting on failure — SHALL be conditioned on `updated_at` still matching the lease this claim wrote. `Retry`'s own eligibility (accepting `processing` once `updated_at` is older than the configured vision timeout) can overlap with a meal this handler has itself put into `processing`: if this attempt's own vision call runs long enough to cross that same threshold, a concurrent `Retry` can legitimately claim the meal as stale before this attempt's failure handling runs. The lease ensures that when a *newer* attempt has claimed the meal, this attempt's later writes become no-ops rather than overwriting or reverting over that newer attempt's in-flight or completed work.

**Success.** On a successful analysis, Reanalyze SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, exactly as `Retry` already does, and SHALL set the meal's status to `pending_clarification` or `pending_review` according to the model's response — including when the meal's status was `confirmed` beforehand, so a reanalyzed confirmed meal returns to the normal review flow rather than remaining confirmed with stale items. Expert reanalysis is the exception specified by Expert Component Guidance for Reanalysis: its explicit component list proceeds directly to `pending_review`, and a fully weighted list does not require photo recognition or weight estimation. The same write SHALL zero the meal's seven stored macro aggregate columns, so a meal leaving `confirmed` never displays its old totals against its new, unreviewed items, and SHALL be conditioned on the lease token described above, so a persistence failure or a lease loss cannot leave the item replacement partially applied.

**Failure.** A candidate-selection error (the model call that matches recognized items against retrieved candidates) SHALL be treated as a reanalysis failure, the same as a recognition error, an invalid/missing weight-estimation result, or a timeout — it SHALL NOT be silently absorbed into an unresolved item set the way it may be for upload/retry/clarify. Selection shares this call's overall timeout with any recognition or estimation work, so a slow earlier call can leave selection to fail with a deadline; absorbing that failure here would let the destructive item-replacement path below run on what is actually a vision-provider failure, silently discarding a confirmed meal's reviewed items for a degraded, entirely unresolved set. On such an analysis failure, if this attempt's lease is still current, Reanalyze SHALL NOT mark the meal `failed` and SHALL NOT modify its items or aggregate. Instead it SHALL restore the meal's `status`, `clarify_round`, and `clarify_log` to the values captured before the atomic claim, leaving the meal exactly as it was before the call, and SHALL respond with HTTP 502 and an error body rather than HTTP 200, so the caller can distinguish "reanalysis failed, nothing changed" from a normal state transition. If the lease is no longer current (a newer attempt has since claimed the meal), the revert SHALL be a no-op — the newer attempt owns the row — and the response SHALL be HTTP 412 (Precondition Failed), not 502: this attempt's own reanalysis did fail, but the specific "the meal is unchanged" guarantee no longer holds once another attempt owns the row, and the caller SHALL treat HTTP 412 as a signal to refetch the meal rather than trust its previous view of it.

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
- **WHEN** the owner calls reanalyze with valid guidance, the analysis reaches candidate selection, and that call fails (e.g. a context deadline consumed by earlier recognition or weight estimation)
- **THEN** the system returns HTTP 502, and the meal's status, items, and aggregate are unchanged — it does not replace the reviewed items with an unresolved set

#### Scenario: A failed reanalysis of a pending_review meal changes nothing
- **GIVEN** a `pending_review` meal with existing items
- **WHEN** the owner calls reanalyze with valid guidance and the analysis fails
- **THEN** the system returns HTTP 502, and the meal's status and items are unchanged — still `pending_review` with its original items

#### Scenario: Concurrent reanalyze calls do not both proceed
- **GIVEN** a `pending_review` meal
- **WHEN** two reanalyze requests for the same meal are submitted concurrently
- **THEN** only one claims the meal and performs reanalysis; the other receives HTTP 409 and has no effect

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
- **WHEN** the owner calls reanalyze with neither a `hint` key nor a `components` key
- **THEN** the system returns HTTP 400 and does not call the vision model

#### Scenario: Reanalyze without a hint is rejected
- **WHEN** the owner calls reanalyze with a sole `hint` that is null, empty, or whitespace-only
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

`POST /api/food/meals/{id}/reanalyze` SHALL accept exactly one top-level guidance key by JSON field presence: the existing `hint`, or a structured `components` array of `{name, weight_grams?}`. If both keys are present, the system SHALL return HTTP 400 even when either value is `null`, empty, or whitespace-only. If neither key is present, the system SHALL return HTTP 400. When it is the sole guidance key, `hint` SHALL be a non-null, non-empty valid hint and `components` SHALL be a non-null, non-empty valid expert array. Every guidance-shape or value error SHALL be rejected before claiming the meal or calling a vision operation. Expert reanalysis SHALL inherit Hint-Driven Reanalysis requirements for ownership, eligible states, atomic claiming, success, failure, and non-persistence.

For expert reanalysis, the backend SHALL assign each normalized component a zero-based `component_index` equal to its request-array position. User-supplied names and weights SHALL be authoritative. If every component supplies `weight_grams`, the backend SHALL skip photo recognition and weight estimation, construct one item per component directly from those names and exact weights, and proceed to the existing nutrition candidate-resolution step.

If any weights are omitted, the system SHALL send the stored photo and only the missing components as `{component_index, name}` to a dedicated weight-estimation vision operation. Its response SHALL contain `{component_index, weight_grams}` for every requested missing index exactly once. The backend SHALL map estimates by `component_index`, not response order, and SHALL reject a response containing a missing, duplicate, unknown, or already-supplied index, or a non-finite/non-positive estimate. Such a rejection SHALL use the existing non-destructive reanalysis failure behavior.

Before nutrition resolution and persistence, the backend SHALL construct final items in original request order using each normalized user-supplied name, every exact supplied weight, and valid index-mapped estimates only for omitted weights. A successful expert result SHALL therefore contain exactly one item per supplied component in request order. The subsequent existing candidate-resolution step MAY still use model-assisted selection; skipping recognition when all weights are supplied does not skip nutrition matching.

Because the expert list resolves the plate composition explicitly, a successful expert analysis SHALL proceed to candidate resolution and `pending_review` without entering model-generated clarification rounds. Expert mode SHALL NOT ask for or submit user-entered calories or macros.

#### Scenario: Correct with a free-text hint
- **WHEN** a user selects Hint mode, enters a valid correction, and requests reanalysis
- **THEN** the system submits the correction through the existing hint-driven reanalysis path

#### Scenario: Correct with expert component guidance
- **WHEN** a user selects Expert mode, enters `grilled chicken` at 180 grams and `red beans` with no weight, and requests reanalysis
- **THEN** the successful result contains those two components in that order, uses exactly 180 grams for grilled chicken, and uses the model's photo-derived estimate for red beans

#### Scenario: Fully specified expert input bypasses recognition
- **WHEN** a user enters a valid positive gram weight for every Expert-mode component
- **THEN** the backend performs no photo-recognition or weight-estimation call, constructs the component items directly with the exact user-defined weights, and proceeds to nutrition candidate resolution

#### Scenario: All expert weights may be omitted
- **WHEN** a user leaves the gram weight blank for every Expert-mode component
- **THEN** the model estimates every component's weight from the stored photo

#### Scenario: Invalid expert component is rejected
- **WHEN** Expert mode contains a blank component name, more than 20 components, a name longer than 100 Unicode characters, or combined names longer than 500 Unicode characters
- **THEN** the request is rejected with HTTP 400 and does not claim the meal or call the vision provider

#### Scenario: Invalid expert weight is rejected
- **WHEN** an Expert-mode component supplies a zero, negative, non-finite, or otherwise invalid `weight_grams`
- **THEN** the request is rejected with HTTP 400 and does not claim the meal or call the vision provider

#### Scenario: Guidance keys are mutually exclusive by presence
- **WHEN** a reanalysis request contains both top-level `hint` and `components` keys, including when either value is empty, whitespace-only, or `null`, or contains neither key
- **THEN** the system returns HTTP 400 without claiming the meal or calling the vision provider

#### Scenario: Sole guidance key still requires a valid value
- **WHEN** a request contains only `hint` but its value is null or blank, or only `components` but its value is null or an empty/invalid array
- **THEN** the system returns HTTP 400 without claiming the meal or calling the vision provider

#### Scenario: Missing-weight estimates are mapped by stable index
- **WHEN** the weight estimator returns valid entries for all requested missing `component_index` values in a different order than requested
- **THEN** the backend maps each estimate to its indexed component and persists final items in the original expert request order

#### Scenario: Provider result cannot be reconciled to missing weights
- **WHEN** the estimator omits an index, repeats an index, returns an unknown or already-supplied index, or returns an invalid estimate
- **THEN** the system applies the existing non-destructive reanalysis failure behavior and leaves the meal unchanged

#### Scenario: Expert mode uses the established reanalysis lifecycle
- **WHEN** valid expert component guidance is submitted for a stored meal photo
- **THEN** the same reanalysis endpoint applies the existing eligibility, concurrency, success, and non-destructive failure behavior

#### Scenario: Expert mode skips clarification
- **WHEN** the model would otherwise ask a clarification question during valid expert reanalysis
- **THEN** the system uses the explicit expert components, proceeds to `pending_review`, and does not enter `pending_clarification`

### Requirement: Recognized Item Brand

Each recognized item SHALL carry a `brand` alongside its name, preparation, and state. It SHALL be the product/manufacturer brand as legibly shown on packaging visible in the photo, or empty when no brand is legible or the food is unpackaged/homemade.

This exists so that later food-reference matching (see `usda-nutrition-database` "Match Selection and Explicit Non-Match") has a real signal for distinguishing among differently-branded packaged products, which candidate selection cannot otherwise do: selection is a text-only model call with no access to the photo, so a generic name alone cannot tell which of several branded products with materially different macros was actually photographed.

#### Scenario: Brand extracted when a label is visible

- **WHEN** the vision model recognizes a packaged food whose brand is legible in the photo
- **THEN** the returned item includes a non-empty `brand` alongside its name, preparation, state, weight, and confidence

#### Scenario: No brand to extract

- **WHEN** the vision model recognizes an unpackaged or homemade food, or a package whose brand is not legible
- **THEN** the returned item's `brand` is empty rather than guessed, and the item is still returned with its name and weight

#### Scenario: Brand is persisted

- **WHEN** an item is stored
- **THEN** its `brand` is persisted with it, so that a later clarification answer or reanalysis can re-run food lookup without re-extracting it from the photo

### Requirement: Composite Dish Naming

Recognize SHALL default to naming a visually composite dish (multiple ingredients combined into one served item, e.g. a curry, a stew, a pre-mixed side) as a single recognized item, and SHALL only return multiple items for a photo when it visually observes separately identifiable components (e.g. distinct piles on a plate, or clearly separate foods placed next to each other). This granularity judgment is made by the model on each photo; it is not a user-facing setting and there is no per-upload or per-user toggle for it.

#### Scenario: A single composite dish is recognized as one item

- **WHEN** a photo shows a homogeneous composite dish, such as a mixed vegetable side or a sauced dish, with no visually separate components
- **THEN** Recognize returns exactly one item for that dish, named to reflect the dish as a whole, rather than one item per ingredient it may contain

#### Scenario: A plate with visually separate components is still decomposed

- **WHEN** a photo shows a plate with clearly separate components, such as a portion of rice, a piece of grilled protein, and a side salad plated apart from each other
- **THEN** Recognize returns one item per visually separate component, as it does today

### Requirement: Macro Estimate Fallback for Unmatched Items

Recognize SHALL include, for every recognized item, an optional per-item estimated nutrient profile (calories, protein, carbohydrates, fat, sugar, sodium, dietary fiber, per 100g) produced in the same model call as recognition, without a separate model call. This profile SHALL be persisted on the item's stored record as soon as the item is created — not held only transiently in the recognition response — so it remains available after a clarification round (see "Bounded Text-Only Clarification Rounds"), whose follow-up call is text-only and cannot regenerate it, and so a later weight edit has a per-100g basis to rescale from (see `food-nutrition-logging` "Item Resolution").

An estimated profile is **usable for automatic resolution** when its values pass validation (a valid, non-negative estimate, as before) AND pass all of these plausibility checks: protein, carbohydrates, and fat are each at most 102g/100g and their sum is at most 102g/100g; sugar and dietary fiber together exceed carbohydrates by no more than 2g/100g, checked as a combined sum rather than independently, since both are subsets of carbohydrates; and declared calories are no lower than `atwater - max(25 kcal, atwater×0.15)`, where `atwater = protein×4 + carbs×4 + fat×9`. The 2g allowances absorb rounding while the combined-macro and combined-sugar/fiber bounds reject profiles whose total mass, or whose sugar-plus-fiber content, is physically impossible. The calorie check is one-sided: declared calories exceeding the Atwater figure is not a violation, since sources such as alcohol contribute calories the three tracked macros do not capture. An estimate that is present but fails this plausibility check SHALL be treated as absent for automatic-resolution precedence, but remains persisted so an item already stored with `macro_source = estimated` can continue to rescale consistently on later weight edits.

The system SHALL use the item's persisted usable estimated profile, scaled by the item's weight, as that item's macros and set its `macro_source` to `estimated`, by default whenever a usable estimate is present, EXCEPT when the item is bound to a reference food via a deterministic identity match — an exact case-insensitive match against one of the user's own saved `CustomFood` entries (see `usda-nutrition-database` "Match Selection and Explicit Non-Match"), or a reference explicitly supplied by the caller on a PATCH, item-add, or manual-meal request (see `food-nutrition-logging` "Item Resolution") — in which case that deterministic match SHALL take unconditional precedence over the estimate, exactly as it does today.

When the item has no usable estimate (Recognize produced none, or produced one that fails validation or the plausibility check), the system SHALL fall back to whatever candidate selection resolved for the item (see `usda-nutrition-database` "Match Selection and Explicit Non-Match"): a selected candidate's macros with `macro_source = reference` if one was found, or `macro_source = none` with zeroed macros, unchanged from today, if none was found.

When the item has a usable estimate but candidate selection instead resolved it to a non-deterministic match — a `Select`-picked USDA, Open Food Facts, or frequency/recency-ranked custom-food candidate, none of which are a deterministic identity match — the usable estimate SHALL still take precedence over that matched candidate; the matched candidate's macros are discarded in favor of the estimate.

An `estimated` item's macros are model-produced guesses, not values bound to a database or custom-food record or supplied directly by the user, and SHALL be surfaced in the review UI with a visual treatment that distinguishes it from `reference` and `manual` items, so the user is prompted to verify or correct it.

#### Scenario: No candidate found falls back to a model estimate

- **WHEN** candidate selection finds no suitable custom food, Open Food Facts, or USDA candidate for a recognized item, and Recognize produced a usable estimated profile for it
- **THEN** the system stores that item with `macro_source = estimated` and macros scaled from that persisted estimated profile, instead of `macro_source = none` with no usable macros

#### Scenario: No candidate and no usable estimate remains unresolved

- **WHEN** candidate selection finds no suitable candidate for a recognized item, and Recognize produced no estimated profile for it, or produced one that fails validation or the plausibility check
- **THEN** the system stores that item with `macro_source = none` and zeroed macros, exactly as it does today

#### Scenario: A usable estimate takes precedence over a fuzzy-matched candidate

- **WHEN** candidate selection selects a USDA, Open Food Facts, or ranked-custom-food candidate for a recognized item (a `Select`-picked, non-deterministic match), and the item carries a usable (valid and plausible) estimated profile
- **THEN** the system uses that estimated profile, scaled by weight, and sets `macro_source = estimated`, discarding the matched candidate's macros

#### Scenario: A matched candidate takes precedence over the estimate

- **WHEN** an item is bound to a reference food via an exact case-insensitive match against the user's own saved custom food, or via a caller-supplied `fdc_id`/`off_code`/`custom_food_id` on a PATCH, item-add, or manual-meal request, and the item also carries an estimated nutrient profile
- **THEN** the system uses the deterministically-matched reference's macros and `macro_source = reference`, and does not use the discarded estimate

#### Scenario: An implausible estimate falls back to a fuzzy-matched candidate

- **WHEN** Recognize produces an estimated profile for a recognized item that is valid (non-negative) but fails the plausibility check (e.g. combined protein/carbs/fat exceeds 102g/100g, or declared calories fall below the exact one-sided Atwater threshold), and candidate selection selects a suitable USDA, Open Food Facts, or custom-food candidate for that item
- **THEN** the system treats the estimate as unusable, uses the matched candidate's macros instead, and sets `macro_source = reference`

#### Scenario: An estimate persists through a clarification round

- **GIVEN** a recognized item with a persisted estimated profile whose meal enters `pending_clarification`
- **WHEN** the clarification round concludes and candidate selection still finds no suitable match for that item
- **THEN** the system uses the estimated profile persisted at the item's original creation, since the clarification follow-up call is text-only and cannot produce a fresh one

#### Scenario: An estimated item is visually distinguished in review

- **WHEN** the user opens the confirm-review screen for a meal containing an item with `macro_source = estimated`
- **THEN** that item is shown with a visual treatment distinct from `reference` and `manual` items, indicating its macros are an unverified AI estimate

