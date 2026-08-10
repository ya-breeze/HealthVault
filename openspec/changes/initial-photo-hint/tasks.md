## 1. Backend hint validation and upload plumbing

- [ ] 1.1 Extract shared backend hint normalization and 500-Unicode-character validation so initial upload and reanalysis use identical limits while retaining their optional-vs-required semantics
- [ ] 1.2 Read the optional multipart `hint` in `CreateMeal`, reject an over-limit normalized value before saving the photo or creating the meal, and pass valid context through `analyzeMeal` into the existing hint-aware recognition pipeline; keep Retry on the empty-hint path
- [ ] 1.3 Extend upload-handler unit tests to cover exact normalized hint forwarding, omitted/whitespace-only hints, the 500-character Unicode boundary, and over-limit rejection with no photo, meal, or vision-call side effects

## 2. Frontend upload experience

- [ ] 2.1 Add a shared frontend hint limit and Unicode code-point counter, and update `ReanalyzeControl` to use them without changing its required-hint behavior
- [ ] 2.2 Extend `api.uploadMeal` to append a non-empty normalized hint to the existing multipart form while preserving photo-only requests
- [ ] 2.3 Keep Take Photo and Choose Photo as the initial page's no-entry default, add a secondary `Add a hint (optional)` control with textarea, guidance, character counter, and over-limit validation, and use the same hint state for both chosen-file and camera-capture uploads

## 3. Expert-guided correction

- [ ] 3.1 Extend the existing reanalysis request with mutually exclusive `hint` or structured `components: {name, weight_grams?}[]`; validate the 4 KiB body cap, 1–20 rows, non-blank names, 100-character per-name and 500-character combined-name limits, and finite positive optional weights before claiming the meal
- [ ] 3.2 Add expert vision handling that requests one ordered result per component, uses normalized user names, overwrites supplied weights exactly, accepts model-estimated finite positive weights only where omitted, skips clarification, and treats an unreconcilable provider result as a non-destructive reanalysis failure
- [ ] 3.3 Add backend tests for hint/components exclusivity, every expert validation boundary, all-supplied/all-estimated/mixed weights, exact weight preservation, ordered one-to-one items, skipped clarification, provider mismatch rollback, ownership, eligible states, and existing lease/error behavior
- [ ] 3.4 Expand `ReanalyzeControl` with mutually exclusive Hint and Expert modes; Expert mode provides repeatable component rows with name and optional grams, add/remove controls, and clear copy that supplied weights are exact while blank weights are AI-estimated
- [ ] 3.5 Extend `api.reanalyzeMeal` for the additive structured request and preserve all current success, 502 unchanged, and 412 superseded UI behavior in either mode

## 4. End-to-end and static validation

- [ ] 4.1 Add deterministic Playwright coverage that proves initial upload requires no text entry, intercepts a hinted file upload and verifies the multipart photo plus normalized hint, rejects an over-limit hint without a request, and retains coverage showing camera capture uses the shared upload path
- [ ] 4.2 Add deterministic Playwright coverage for switching between Hint and Expert correction, component add/remove, name/weight validation, exact ordered structured input with supplied and omitted weights, authoritative supplied weights in the response, and unchanged 502/412 handling from either mode
- [ ] 4.3 Run `make test` and `make lint`, plus the frontend TypeScript/lint/build checks exposed by `frontend/package.json`, and fix all failures
- [ ] 4.4 Validate the OpenSpec change strictly, deploy the feature branch to the `hcw-wip` stack, and run the relevant Playwright tests against that deployed stack
- [ ] 4.5 Self-review every modified file for validation order, automatic-default compatibility, Unicode-count consistency, exact expert-weight preservation, one-to-one provider reconciliation, correct reanalysis error handling, and accidental guidance persistence
