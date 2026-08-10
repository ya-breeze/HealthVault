## Why

Photo logging should stay effortless: by default, the model should identify the visible meal components and estimate their weights without making the user transcribe the plate first. When that automatic result is far from reality, the current free-text hint is useful but imprecise; the user also needs an expert correction mode that names the components to analyze separately and can supply exact weights where known.

## What Changes

- Keep initial photo analysis automatic by default: a user can take or choose a photo without entering ingredients, components, or weights, and the model identifies the visible foods and estimates their weights.
- Add a secondary, optional hint field to the initial meal-photo upload interface without putting text entry in the default path.
- Send the hint alongside both selected-file and in-app-camera uploads.
- Accept the optional hint in the existing multipart `POST /api/food/meals` request, validate it consistently with reanalysis hints, and include it in the first vision-recognition prompt.
- Preserve the current upload behavior when the hint is blank or omitted.
- Expand the existing reanalysis correction UI to offer two alternatives when the automatic result is substantially wrong: a free-text hint, or an expert mode where the user lists high-level meal components separately (for example, grilled chicken and red beans).
- In expert mode, let each component carry an optional exact gram weight. User-supplied weights are authoritative; the model estimates only weights left blank.
- Send expert components as structured input through the existing reanalysis endpoint so exact weights are enforced by the application rather than left to the model to interpret from prose.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `food-photo-recognition`: initial meal-photo uploads may include an optional, bounded hint that is forwarded into the first recognition request; the correction experience gains an expert component-list mode while automatic model identification and weight estimation remain the default.

## Impact

- Backend: `backend/pkg/server/food_upload.go` reads and validates the multipart hint and passes it into the existing hint-aware analysis pipeline; upload handler tests cover forwarding and rejection.
- Backend: `backend/pkg/server/food_reanalyze.go` accepts mutually exclusive free-text or structured expert guidance and applies authoritative component names/weights before the existing nutrition-resolution and persistence steps.
- Frontend: `frontend/app/food/upload/page.tsx` offers secondary optional hint entry, `frontend/components/food/ReanalyzeControl.tsx` gains hint/expert correction modes with optional gram fields, and `frontend/lib/api.ts` supports both additive request forms.
- E2E: `e2e/tests/food.spec.ts` verifies automatic upload remains the default, initial hints are sent and bounded, and expert component names and optional weights reach reanalysis exactly.
- API compatibility: additive only; existing clients that send only `photo` or the existing reanalysis `hint` continue to work unchanged. No database migration or new dependency is required.
