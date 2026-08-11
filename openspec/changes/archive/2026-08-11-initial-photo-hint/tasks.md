## 1. Backend hint validation and upload plumbing

- [x] 1.1 Extract shared backend hint normalization and 500-Unicode-character validation so initial upload and reanalysis use identical limits while retaining their optional-vs-required semantics
- [x] 1.2 Read the optional multipart `hint` in `CreateMeal`, reject an over-limit normalized value before saving the photo or creating the meal, and pass valid context through `analyzeMeal` into the existing hint-aware recognition pipeline; keep Retry on the empty-hint path
- [x] 1.3 Extend upload-handler unit tests to cover exact normalized hint forwarding, omitted/whitespace-only hints, the 500-character Unicode boundary, and over-limit rejection with no photo, meal, or vision-call side effects

## 2. Frontend upload experience

- [x] 2.1 Add a shared frontend hint limit and Unicode code-point counter, and update `ReanalyzeControl` to use them without changing its required-hint behavior
- [x] 2.2 Extend `api.uploadMeal` to append a non-empty normalized hint to the existing multipart form while preserving photo-only requests
- [x] 2.3 Keep Take Photo and Choose Photo as the initial page's no-entry default, add a secondary `Add a hint (optional)` control with textarea, guidance, character counter, and over-limit validation, and use the same hint state for both chosen-file and camera-capture uploads

## 3. Expert-guided correction

- [x] 3.1 Extend the existing reanalysis request with top-level `hint` or structured `components: {name, weight_grams?}[]`; track raw JSON field presence so both keys are rejected even when one is null/empty/whitespace and neither is rejected, then validate the sole selected mode plus the 4 KiB body cap before claiming the meal
- [x] 3.2 Extend `vision.Client` with `EstimateWeights(ctx, image, mimeType, components)` plus index-keyed input/result types; implement it in `OpenAIClient`, `Fake`, and `Unconfigured`, then assign request-array `component_index` values, skip the operation when every weight is supplied, request estimates only for missing indices otherwise, map reordered results by index, and reject missing/duplicate/unknown/already-supplied indices or invalid estimates through non-destructive reanalysis failure
- [x] 3.3 Construct expert items in original request order using authoritative normalized names, exact supplied weights, and only validated index-mapped estimates; skip clarification, run existing strict nutrition candidate resolution, and persist through the existing leased reanalysis transaction
- [x] 3.4 Add backend tests for field-presence exclusivity (including null/empty combinations), every expert validation boundary, all-supplied bypass, all-estimated/mixed weights, exact weight preservation, reordered-index success, invalid-index rollback, ownership, eligible states, and existing lease/error behavior
- [x] 3.5 Expand `ReanalyzeControl` with mutually exclusive Hint and Expert modes; Expert mode provides repeatable component rows with name and optional grams, add/remove controls, and clear copy that supplied weights are exact while blank weights are AI-estimated
- [x] 3.6 Extend `api.reanalyzeMeal` for the additive structured request and preserve all current success, 502 unchanged, and 412 superseded UI behavior in either mode

## 4. End-to-end and static validation

- [x] 4.1 Add deterministic Playwright coverage that proves initial upload requires no text entry, intercepts a hinted file upload and verifies the multipart photo plus normalized hint, rejects an over-limit hint without a request, and retains coverage showing camera capture uses the shared upload path
- [x] 4.2 Add deterministic Playwright coverage for switching between Hint and Expert correction, component add/remove, name/weight validation, exact ordered structured input with supplied and omitted weights, authoritative supplied weights in the response, field-presence error handling, and unchanged 502/412 handling from either mode
- [x] 4.3 Run `make test` and `make lint`, plus the frontend TypeScript/lint/build checks exposed by `frontend/package.json`, and fix all failures
- [x] 4.4 Validate the OpenSpec change strictly, deploy the feature branch to the `hcw-wip` stack, and run the relevant Playwright tests against that deployed stack
- [x] 4.5 Self-review every modified file for validation order, automatic-default compatibility, Unicode-count consistency, exact expert-weight preservation, index-keyed estimator reconciliation, correct reanalysis error handling, and accidental guidance persistence
