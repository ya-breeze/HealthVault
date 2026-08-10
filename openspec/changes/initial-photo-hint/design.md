## Context

The vision boundary already accepts an optional `hint` and includes it beside the image when non-empty. The initial upload handler is the remaining gap: `POST /api/food/meals` accepts only a multipart `photo`, and `analyzeMeal` always invokes the shared analysis pipeline with `""`. The frontend likewise uploads immediately after a camera capture or file selection, with no way to supply context first.

The existing reanalysis hint establishes the relevant safety limits: trim surrounding whitespace, count Unicode code points rather than UTF-8 bytes or JavaScript UTF-16 code units, and reject text longer than 500 characters. Initial upload hints are optional, unlike reanalysis hints, because an upload without extra context remains a valid and common path.

## Goals / Non-Goals

**Goals:**

- Let a user add optional context before taking or choosing the initial meal photo.
- Forward that context through the existing multipart upload and hint-aware vision pipeline.
- Keep backend and frontend validation aligned with the existing 500-character reanalysis limit.
- Reject an invalid hint before the application stores a photo, creates a meal, or calls the vision provider.
- Preserve identical behavior for existing clients and users that omit the hint.

**Non-Goals:**

- Persisting or displaying the initial hint after the request completes.
- Automatically reusing the initial hint during Retry or later reanalysis.
- Changing recognition prompts, clarification behavior, upload size limits, or meal state transitions beyond supplying the optional context.

## Decisions

**Use an optional multipart `hint` field on the existing upload endpoint.** `POST /api/food/meals` already uses multipart form data for the photo, so adding a text part keeps photo and context in one atomic request and remains backward compatible. A separate pre-upload endpoint or a query parameter would add state or expose user text in URLs without improving the flow.

**Normalize and validate the hint before durable writes.** The backend trims surrounding whitespace. A missing, empty, or whitespace-only value becomes the empty string and follows the existing no-hint path. A non-empty value longer than 500 Unicode code points returns HTTP 400 before `photos.Save`, meal creation, or the vision call. The upload and reanalysis handlers will share the backend limit and normalization/validation helper so their interpretation cannot drift; reanalysis will continue to require the normalized value to be non-empty.

**Keep the hint ephemeral.** The normalized hint is passed to `runAnalysis` for this first recognition call but is not added to `FoodMeal` or another table. Its purpose is to guide one model request; persisting caller prompt text would introduce a data-model and privacy commitment that the requested first-pass improvement does not need. If the attempt fails, Retry remains the established no-hint recovery path and the user can instead choose hint-driven reanalysis when appropriate.

**Collect the hint before either photo source is chosen.** The upload page will show one optional textarea with guidance and a Unicode-aware `current/500` counter above the Take Photo and Choose Photo actions. Both the file-selection callback and camera-capture callback read the same state and send the same normalized value. Client-side validation prevents starting an upload over the limit, while the backend remains authoritative for non-browser clients.

**Reuse frontend hint-length semantics.** The upload UI and `ReanalyzeControl` will use a shared 500-character constant and Unicode code-point counter. Native `maxLength` is not sufficient because browsers count UTF-16 code units, causing emoji and other non-BMP characters to disagree with Go's rune count.

## Risks / Trade-offs

- **[Risk] Multipart overhead now includes a small text field and its headers.** → The existing 64 KiB allowance above the configured photo limit is far larger than the bounded 500-character value; backend validation keeps the addition bounded.
- **[Risk] A user may expect a failed upload's Retry to remember the hint.** → The hint is explicitly described as context for the initial analysis only; later correction remains available through the existing reanalysis-with-hint action.
- **[Risk] Frontend and backend character counts could diverge for Unicode.** → Both count Unicode code points via shared helpers in their respective codebases, with boundary tests using non-ASCII/non-BMP input.

## Migration Plan

Deploy the additive backend and frontend changes together. No database or configuration migration is required. Rollback is a normal code rollback: the feature creates no new stored state, and existing photo-only clients remain compatible throughout.

## Open Questions

None.
