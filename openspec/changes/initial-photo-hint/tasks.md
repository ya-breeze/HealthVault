## 1. Backend hint validation and upload plumbing

- [ ] 1.1 Extract shared backend hint normalization and 500-Unicode-character validation so initial upload and reanalysis use identical limits while retaining their optional-vs-required semantics
- [ ] 1.2 Read the optional multipart `hint` in `CreateMeal`, reject an over-limit normalized value before saving the photo or creating the meal, and pass valid context through `analyzeMeal` into the existing hint-aware recognition pipeline; keep Retry on the empty-hint path
- [ ] 1.3 Extend upload-handler unit tests to cover exact normalized hint forwarding, omitted/whitespace-only hints, the 500-character Unicode boundary, and over-limit rejection with no photo, meal, or vision-call side effects

## 2. Frontend upload experience

- [ ] 2.1 Add a shared frontend hint limit and Unicode code-point counter, and update `ReanalyzeControl` to use them without changing its required-hint behavior
- [ ] 2.2 Extend `api.uploadMeal` to append a non-empty normalized hint to the existing multipart form while preserving photo-only requests
- [ ] 2.3 Add the optional hint textarea, guidance, character counter, and over-limit validation to the initial upload page, and use the same hint state for both chosen-file and camera-capture uploads

## 3. End-to-end and static validation

- [ ] 3.1 Add deterministic Playwright coverage that intercepts an initial file upload, verifies the multipart request contains the photo and normalized hint, and verifies an over-limit hint prevents the request; retain coverage showing camera capture uses the shared upload path
- [ ] 3.2 Run `make test` and `make lint`, plus the frontend TypeScript/lint/build checks exposed by `frontend/package.json`, and fix all failures
- [ ] 3.3 Validate the OpenSpec change strictly, deploy the feature branch to the `hcw-wip` stack, and run the relevant Playwright tests against that deployed stack
- [ ] 3.4 Self-review every modified file for validation order, no-hint compatibility, Unicode-count consistency, and accidental hint persistence
