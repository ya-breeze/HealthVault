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

### Requirement: Hint-Driven Reanalysis
The system SHALL expose `POST /api/food/meals/{id}/reanalyze`, which re-runs vision analysis against the already-stored photo for a meal owned by the caller, together with a required free-text `hint` supplied by the caller. The request body SHALL be read through a size-limited reader (4 KiB) before decoding, and rejected with HTTP 413 if it exceeds that limit. The decoded `hint`, measured in Unicode characters (not bytes), SHALL be rejected with HTTP 400 if it is empty or whitespace-only, or if it exceeds 500 characters.

Reanalyze SHALL be accepted when the meal's status is `failed`, `pending_review`, or `confirmed`, and SHALL be rejected with HTTP 409 for `processing` or `pending_clarification`. It SHALL be rejected with HTTP 409 if the meal has no stored photo, matching `Retry`'s existing guard.

**Atomic claim with a lease token.** Eligibility SHALL be enforced as part of a single conditional update, matched against the *exact* `status` and `updated_at` observed when the meal was loaded for this request — not a broader `status IN (...)` match — and SHALL write a fresh `updated_at` as this attempt's lease token: `UPDATE ... SET status = 'processing', clarify_round = 0, clarify_log = '', updated_at = <lease> WHERE id = ? AND status = <observed status> AND updated_at = <observed updated_at>`. This is not a separate read followed by a later write, so two concurrent reanalyze calls for the same meal cannot both proceed and race each other's item replacement. A call that affects zero rows SHALL return HTTP 409 without calling the vision model. Matching on the exact observed status (not just membership in the eligible set) additionally prevents claiming a meal that a *different* concurrent operation (e.g. `ConfirmMeal`) has already transitioned since it was loaded. The meal's prior `status`, `clarify_round`, and `clarify_log` SHALL be captured before this claim, for use on failure (below).

Every later write this attempt makes — persisting a successful analysis, or reverting on failure — SHALL be conditioned on `updated_at` still matching the lease this claim wrote. `Retry`'s own eligibility (accepting `processing` once `updated_at` is older than the configured vision timeout) can overlap with a meal this handler has itself put into `processing`: if this attempt's own vision call runs long enough to cross that same threshold, a concurrent `Retry` can legitimately claim the meal as stale before this attempt's failure handling runs. The lease ensures that when a *newer* attempt has claimed the meal, this attempt's later writes become no-ops rather than overwriting or reverting over that newer attempt's in-flight or completed work.

**Success.** On a successful recognition, Reanalyze SHALL replace the meal's existing `FoodItem` rows in the same transaction that writes the resulting status, exactly as `Retry` already does, and SHALL set the meal's status to `pending_clarification` or `pending_review` according to the model's response — including when the meal's status was `confirmed` beforehand, so a reanalyzed confirmed meal returns to the normal review flow rather than remaining confirmed with stale items. The same write SHALL zero the meal's seven stored macro aggregate columns, so a meal leaving `confirmed` never displays its old totals against its new, unreviewed items, and SHALL be conditioned on the lease token described above, so a persistence failure or a lease loss cannot leave the item replacement partially applied.

**Failure.** A candidate-selection error (the model call that matches recognized items against retrieved candidates) SHALL be treated as a reanalysis failure, the same as a recognition error or timeout — it SHALL NOT be silently absorbed into an unresolved item set the way it may be for upload/retry/clarify. Selection shares this call's overall timeout with recognition, so a slow recognition call can leave selection to fail with a deadline; absorbing that failure here would let the destructive item-replacement path below run on what is actually a vision-provider failure, silently discarding a confirmed meal's reviewed items for a degraded, entirely unresolved set. On a vision error or timeout (recognition or selection), if this attempt's lease is still current, Reanalyze SHALL NOT mark the meal `failed` and SHALL NOT modify its items or aggregate. Instead it SHALL restore the meal's `status`, `clarify_round`, and `clarify_log` to the values captured before the atomic claim, leaving the meal exactly as it was before the call, and SHALL respond with HTTP 502 and an error body rather than HTTP 200, so the caller can distinguish "reanalysis failed, nothing changed" from a normal state transition. If the lease is no longer current (a newer attempt has since claimed the meal), the revert SHALL be a no-op — the newer attempt owns the row — and the response SHALL be HTTP 412 (Precondition Failed), not 502: this attempt's own reanalysis did fail, but the specific "the meal is unchanged" guarantee no longer holds once another attempt owns the row, and the caller SHALL treat HTTP 412 as a signal to refetch the meal rather than trust its previous view of it.

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

#### Scenario: A candidate-selection failure is treated as a reanalysis failure
- **GIVEN** a `confirmed` meal with existing, reviewed items and a nonzero stored aggregate
- **WHEN** the owner calls reanalyze with a hint, recognition succeeds, but the subsequent candidate-selection call fails (e.g. a context deadline from a slow recognition call consuming the shared timeout)
- **THEN** the system returns HTTP 502, and the meal's status, items, and aggregate are unchanged — it does not replace the reviewed items with an unresolved set

#### Scenario: A failed reanalysis of a pending_review meal changes nothing
- **GIVEN** a `pending_review` meal with existing items
- **WHEN** the owner calls reanalyze with a hint and the vision call fails
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

