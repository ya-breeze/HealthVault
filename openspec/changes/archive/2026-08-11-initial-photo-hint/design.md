## Context

The vision boundary already accepts an optional `hint` and includes it beside the image when non-empty. The initial upload handler is the remaining gap: `POST /api/food/meals` accepts only a multipart `photo`, and `analyzeMeal` always invokes the shared analysis pipeline with `""`. The frontend likewise uploads immediately after a camera capture or file selection, with no way to supply context first.

The current recognition contract already asks the model for recognized food items and estimated gram weights; no user-authored ingredient list is required. The existing reanalysis hint establishes the relevant safety limits: trim surrounding whitespace, count Unicode code points rather than UTF-8 bytes or JavaScript UTF-16 code units, and reject text longer than 500 characters. Initial upload hints are optional, unlike reanalysis hints, because automatic analysis without extra context remains the primary path.

The current correction UI exposes only one free-text reanalysis hint. Expert correction needs more structure for cases where the model has misunderstood the plate composition itself: the user identifies high-level visible components such as `grilled chicken` and `red beans`, and may enter exact gram weights just as meal items can be weight-corrected today. A blank expert weight remains model-estimated.

## Goals / Non-Goals

**Goals:**

- Let a user add optional context before taking or choosing the initial meal photo.
- Keep photo-only, model-identified components and model-estimated weights as the default flow.
- Forward that context through the existing multipart upload and hint-aware vision pipeline.
- Offer free-text and structured expert alternatives for correcting a poor automatic result.
- Let expert users name components separately and optionally define exact gram weights, while the model estimates any omitted weight.
- Keep backend and frontend validation aligned with the existing 500-character reanalysis limit.
- Reject an invalid hint before the application stores a photo, creates a meal, or calls the vision provider.
- Preserve identical behavior for existing clients and users that omit the hint.

**Non-Goals:**

- Persisting or displaying the initial hint after the request completes.
- Automatically reusing the initial hint during Retry or later reanalysis.
- User-entered calories or macros in expert guidance; the existing manual logging and item-editing features remain the path for direct nutrition values.
- Recipe-level decomposition into every hidden ingredient; expert entries describe the high-level meal components visible or known to the user.
- A new expert-analysis endpoint or database representation; expert input extends the existing reanalysis endpoint and produces ordinary `FoodItem` rows.
- Changing the default recognition prompt, upload size limits, or meal state transitions outside the explicitly guided initial-upload and expert-reanalysis paths.

## Decisions

**Use an optional multipart `hint` field on the existing upload endpoint.** `POST /api/food/meals` already uses multipart form data for the photo, so adding a text part keeps photo and context in one atomic request and remains backward compatible. A separate pre-upload endpoint or a query parameter would add state or expose user text in URLs without improving the flow.

**Normalize and validate the hint before durable writes.** The backend trims surrounding whitespace. A missing, empty, or whitespace-only value becomes the empty string and follows the existing no-hint path. A non-empty value longer than 500 Unicode code points returns HTTP 400 before `photos.Save`, meal creation, or the vision call. The upload and reanalysis handlers will share the backend limit and normalization/validation helper so their interpretation cannot drift; reanalysis will continue to require the normalized value to be non-empty.

**Keep the hint ephemeral.** The normalized hint is passed to `runAnalysis` for this first recognition call but is not added to `FoodMeal` or another table. Its purpose is to guide one model request; persisting caller prompt text would introduce a data-model and privacy commitment that the requested first-pass improvement does not need. If the attempt fails, Retry remains the established no-hint recovery path and the user can instead choose hint-driven reanalysis when appropriate.

**Keep guidance secondary on initial upload.** The upload page's primary actions remain Take Photo and Choose Photo. An `Add a hint (optional)` affordance reveals a textarea and Unicode-aware `current/500` counter; the user is not presented with ingredient or weight fields in the normal path. Both the file-selection callback and camera-capture callback read the same state and send the same normalized value. Client-side validation prevents starting an upload over the limit, while the backend remains authoritative for non-browser clients.

**Extend the existing reanalysis endpoint with structured expert input and presence-based mode selection.** When correcting an existing result, the user chooses either free-text Hint mode or Expert mode. Expert mode presents repeatable rows containing a required component name and an optional positive gram weight. `POST /api/food/meals/{id}/reanalyze` accepts exactly one top-level key: the existing `hint`, or a new `components` array shaped as `{name, weight_grams?}`. Presence is syntactic, not based on post-normalization truthiness: if both keys occur, the request is rejected even when one is `null`, empty, or whitespace; if neither occurs, it is rejected. A lone `hint` must be a valid non-blank string, and lone `components` must be a valid non-null array. Tracking raw field presence avoids a decoded zero value making the frontend and backend disagree about which mode was selected. The additive request shape retains the endpoint's ownership, eligibility, atomic claim, failure/revert, and response behavior while giving the backend machine-readable weights it can enforce. Encoding expert rows into prose alone was rejected: a model may round, reinterpret, or ignore an exact weight, so it cannot make a user-defined value authoritative.

**Component names and supplied weights are authoritative; omitted weights use an index-keyed estimator.** The expert request is normalized in order: trim names, reject blank names, preserve row order, and reject a supplied weight unless it is finite and greater than zero. The backend assigns each normalized row a zero-based `component_index` derived from its array position. If every row has a weight, it skips photo recognition and weight estimation entirely, constructs the recognized-item set directly from the authoritative names and weights, and proceeds to the existing nutrition candidate-resolution step. This prevents a fully specified expert correction from failing merely because the recognition model cannot echo information the user already supplied.

If any weights are omitted, a dedicated vision request receives the stored photo plus only the missing rows as `{component_index, name}` and returns `{component_index, weight_grams}` estimates. The backend validates that the response contains every requested index exactly once and no duplicate, unknown, or supplied-weight index, then maps estimates by `component_index` rather than response order. An invalid set or non-positive/non-finite estimate fails through the existing non-destructive reanalysis revert. The backend always constructs final items in original request order with normalized user names and exact user weights where supplied. Candidate selection may still use the existing model-assisted resolution step in both the fully supplied and mixed paths; that is distinct from asking recognition to rediscover the expert-defined plate.

**Expert input goes directly to review rather than clarification.** The component list is itself the user's disambiguation of plate composition. A successfully constructed expert result therefore proceeds through candidate resolution to `pending_review`; model-generated clarification questions are not part of this mode. This avoids losing authoritative structured input across a later text-only clarification round, which currently has no persisted expert-guidance record to reapply.

**Bound structured expert input independently from free-text hints.** The existing 4 KiB reanalysis body cap still applies. Expert mode accepts 1–20 components, each normalized name is at most 100 Unicode characters, and the sum of normalized names is at most 500 Unicode characters. These limits keep prompt size comparable to the existing hint contract. A supplied `weight_grams` follows the current item-weight rule: it must be finite and greater than zero, with no new arbitrary upper bound.

**Reuse frontend Unicode-length semantics.** The upload UI and `ReanalyzeControl` will use shared constants and a Unicode code-point counter. Native `maxLength` is not sufficient because browsers count UTF-16 code units, causing emoji and other non-BMP characters to disagree with Go's rune count. Expert mode validates row count, per-name length, combined-name length, and optional weights client-side while the backend remains authoritative.

## Risks / Trade-offs

- **[Risk] Multipart overhead now includes a small text field and its headers.** → The existing 64 KiB allowance above the configured photo limit is far larger than the bounded 500-character value; backend validation keeps the addition bounded.
- **[Risk] A user may expect a failed upload's Retry to remember the hint.** → The hint is explicitly described as context for the initial analysis only; later correction remains available through the existing reanalysis-with-hint action.
- **[Risk] Frontend and backend character counts could diverge for Unicode.** → Both count Unicode code points via shared helpers in their respective codebases, with boundary tests using non-ASCII/non-BMP input.
- **[Risk] “Expert” could imply recipe-level ingredients or direct macro entry.** → The UI calls the rows meal components, offers only names and optional grams, and keeps calories/macros in the existing manual/editing flows.
- **[Risk] The estimator may omit, duplicate, or reorder results for missing weights.** → Require explicit `component_index` values, accept reordered responses by mapping indices, and reject missing, duplicate, unknown, or already-supplied indices through the established non-destructive failure path.

## Migration Plan

Deploy the additive backend and frontend changes together. No database or configuration migration is required. Rollback is a normal code rollback: the feature creates no new stored state, and existing photo-only clients remain compatible throughout.

## Open Questions

None.
