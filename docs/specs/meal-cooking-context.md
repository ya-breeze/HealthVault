# Low-added-salt context during food entry

## Why

The owner also wants to enter or edit the photo hint after capturing the photo, before recognition.

The owner reports sodium overestimation despite little added salt. Real-record replay showed that “homemade” has no stable meaning for the model. The owner now asks for only a little-added-salt checkbox.

## How

Show one shared “Little added salt” checkbox on photo and description entry, selected by default for each new meal. Remove homemade from UI, API types, persisted context, and model guidance. Submit the optional low_added_salt boolean separately from the original hint or description. Omission keeps unknown context; false means added salt is unspecified, not high. True tells the model little salt was added during preparation without assuming origin, a measured amount, or zero sodium. Preserve intrinsic and processed-ingredient sodium and explicit label/quantity precedence. Keep context through recognition, retry, clarification, and normal photo reanalysis. Preserve the existing clarification safeguard that carries forward the original nutrient profile for the same item; clarification alone does not revise it. Structured manual nutrients and original text limits remain unchanged. Do not recalculate old meals or change thresholds. Qualitative guidance cannot guarantee accurate sodium; real-record replay remains a separate diagnostic.

Stage camera captures and picked files in a local preview before any upload. Show the optional hint editor after selecting a photo, preserve hints entered beforehand, and let the user change the salt checkbox before pressing an explicit Analyze action. Support photo replacement and removal, camera cancellation, and retry after upload failure without losing the selected photo or hint. Revoke object URLs on replacement/removal/unmount. Keep the 500-character Unicode limit, guard duplicate submissions, and send photo, hint and salt choice together only on explicit submission. Localize new preview actions.

## Validation Commands

- `make test-backend`
- `make test-frontend`
- `make lint`
- `make test-e2e E2E_ARGS='tests/food.spec.ts --retries=0'`
- Bounded synthetic WIP recognition with low-added-salt context and explicit sodium on a label; inspect persisted flags, estimates, and subsequent retry/clarification behavior separately from mocked tests.

### Task 1: Persist and apply optional meal salt context
- [x] Add optional typed cooking context to meals and photo/description requests, with validation for missing or malformed salt flags.
- [x] Preserve context across recognition, retry, clarification, and normal reanalysis while leaving omitted context unknown.
- [x] Add meaningful storage and handler tests for forwarding, persistence, validation, and lifecycle reuse.

### Task 2: Show and submit salt checkbox
- [x] Add shared localized controls to photo and text entry with the selected default and independent toggle.
- [x] Extend API types and submissions without altering user text or structured manual values.
- [x] Cover photo/text defaults and checked/unchecked salt submissions in browser tests.

### Task 3: Stage a photo before recognition
- [ ] Add preview, post-photo hint editing, replacement/removal, and explicit submission for camera and file paths.
- [ ] Preserve photo and hint after failures, release object URLs, and cover staged flow, no premature request, submitted hints, and retry in browser tests.

### Task 4: Validate and review
- [ ] Run tests, static checks, Review Gate, WIP deployment, browser validation, and real-provider probes.
- [ ] Mark completed
