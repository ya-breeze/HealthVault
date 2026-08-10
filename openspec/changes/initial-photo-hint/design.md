## Context

The vision boundary already accepts an optional `hint` and includes it beside the image when non-empty. The initial upload handler is the remaining gap: `POST /api/food/meals` accepts only a multipart `photo`, and `analyzeMeal` always invokes the shared analysis pipeline with `""`. The frontend likewise uploads immediately after a camera capture or file selection, with no way to supply context first.

The current recognition contract already asks the model for recognized food items and estimated gram weights; no user-authored ingredient list is required. The existing reanalysis hint establishes the relevant safety limits: trim surrounding whitespace, count Unicode code points rather than UTF-8 bytes or JavaScript UTF-16 code units, and reject text longer than 500 characters. Initial upload hints are optional, unlike reanalysis hints, because automatic analysis without extra context remains the primary path.

The current correction UI exposes only one free-text reanalysis hint. Expert correction needs more structure for cases where the model has misunderstood the plate composition itself, but it should not turn into manual nutrition entry: the user identifies high-level visible components such as `grilled chicken` and `red beans`, and the model still estimates their weights from the photo.

## Goals / Non-Goals

**Goals:**

- Let a user add optional context before taking or choosing the initial meal photo.
- Keep photo-only, model-identified components and model-estimated weights as the default flow.
- Forward that context through the existing multipart upload and hint-aware vision pipeline.
- Offer free-text and structured expert alternatives for correcting a poor automatic result.
- Let expert users name components separately without asking them to enter weights or macros.
- Keep backend and frontend validation aligned with the existing 500-character reanalysis limit.
- Reject an invalid hint before the application stores a photo, creates a meal, or calls the vision provider.
- Preserve identical behavior for existing clients and users that omit the hint.

**Non-Goals:**

- Persisting or displaying the initial hint after the request completes.
- Automatically reusing the initial hint during Retry or later reanalysis.
- User-entered gram weights, calories, or macros in expert guidance; the existing manual logging and item-editing features remain separate.
- Recipe-level decomposition into every hidden ingredient; expert entries describe the high-level meal components visible or known to the user.
- A new expert-analysis endpoint or database representation.
- Changing recognition prompts, clarification behavior, upload size limits, or meal state transitions beyond supplying the optional context.

## Decisions

**Use an optional multipart `hint` field on the existing upload endpoint.** `POST /api/food/meals` already uses multipart form data for the photo, so adding a text part keeps photo and context in one atomic request and remains backward compatible. A separate pre-upload endpoint or a query parameter would add state or expose user text in URLs without improving the flow.

**Normalize and validate the hint before durable writes.** The backend trims surrounding whitespace. A missing, empty, or whitespace-only value becomes the empty string and follows the existing no-hint path. A non-empty value longer than 500 Unicode code points returns HTTP 400 before `photos.Save`, meal creation, or the vision call. The upload and reanalysis handlers will share the backend limit and normalization/validation helper so their interpretation cannot drift; reanalysis will continue to require the normalized value to be non-empty.

**Keep the hint ephemeral.** The normalized hint is passed to `runAnalysis` for this first recognition call but is not added to `FoodMeal` or another table. Its purpose is to guide one model request; persisting caller prompt text would introduce a data-model and privacy commitment that the requested first-pass improvement does not need. If the attempt fails, Retry remains the established no-hint recovery path and the user can instead choose hint-driven reanalysis when appropriate.

**Keep guidance secondary on initial upload.** The upload page's primary actions remain Take Photo and Choose Photo. An `Add a hint (optional)` affordance reveals a textarea and Unicode-aware `current/500` counter; the user is not presented with ingredient or weight fields in the normal path. Both the file-selection callback and camera-capture callback read the same state and send the same normalized value. Client-side validation prevents starting an upload over the limit, while the backend remains authoritative for non-browser clients.

**Offer expert mode as an alternative reanalysis authoring UI, not a new analysis protocol.** When correcting an existing result, the user chooses either free-text Hint mode or Expert mode. Expert mode presents repeatable component-name rows with add/remove controls. It requires at least one non-blank component, trims entries, discards blank rows, and formats the ordered names into a deterministic instruction such as `Treat these as separate meal components and estimate each weight from the photo:` followed by a numbered list. That generated instruction is submitted through the existing `POST /api/food/meals/{id}/reanalyze` `hint` field, so it inherits the same ownership, state, concurrency, failure, and non-persistence behavior. A new endpoint or request type would duplicate an already-correct lifecycle for what is ultimately model guidance.

**The model, not the expert user, owns weight estimation.** Expert mode has no gram or macro fields. Its generated instruction explicitly asks the model to analyze every listed high-level component separately and estimate its weight from the image. This avoids confusing expert guidance with the existing fully manual meal-entry flow, which is the appropriate path when a user wants to supply nutrition values directly.

**Reuse frontend hint-length semantics.** The upload UI and `ReanalyzeControl` will use a shared 500-character constant and Unicode code-point counter. Native `maxLength` is not sufficient because browsers count UTF-16 code units, causing emoji and other non-BMP characters to disagree with Go's rune count.

The 500-character limit applies to the final generated expert instruction, not just the raw component names, ensuring the existing backend contract remains authoritative. The UI shows an actionable error and does not start reanalysis if the generated instruction is over the limit.

## Risks / Trade-offs

- **[Risk] Multipart overhead now includes a small text field and its headers.** → The existing 64 KiB allowance above the configured photo limit is far larger than the bounded 500-character value; backend validation keeps the addition bounded.
- **[Risk] A user may expect a failed upload's Retry to remember the hint.** → The hint is explicitly described as context for the initial analysis only; later correction remains available through the existing reanalysis-with-hint action.
- **[Risk] Frontend and backend character counts could diverge for Unicode.** → Both count Unicode code points via shared helpers in their respective codebases, with boundary tests using non-ASCII/non-BMP input.
- **[Risk] “Expert” could imply manual weights or recipe ingredients.** → The UI calls the rows meal components, explains that AI still estimates weights, and provides no weight or macro inputs.
- **[Risk] Generated expert guidance consumes part of the existing 500-character budget.** → Count the deterministic prefix and separators in the displayed validation result, reject before issuing a request, and keep the component instruction concise.

## Migration Plan

Deploy the additive backend and frontend changes together. No database or configuration migration is required. Rollback is a normal code rollback: the feature creates no new stored state, and existing photo-only clients remain compatible throughout.

## Open Questions

None.
