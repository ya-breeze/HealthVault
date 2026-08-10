## Why

Photo logging should stay effortless: by default, the model should identify the visible meal components and estimate their weights without making the user transcribe the plate first. When that automatic result is far from reality, the current free-text hint is useful but imprecise; the user also needs an expert correction mode that names the components to analyze separately while leaving weight estimation to the model.

## What Changes

- Keep initial photo analysis automatic by default: a user can take or choose a photo without entering ingredients, components, or weights, and the model identifies the visible foods and estimates their weights.
- Add a secondary, optional hint field to the initial meal-photo upload interface without putting text entry in the default path.
- Send the hint alongside both selected-file and in-app-camera uploads.
- Accept the optional hint in the existing multipart `POST /api/food/meals` request, validate it consistently with reanalysis hints, and include it in the first vision-recognition prompt.
- Preserve the current upload behavior when the hint is blank or omitted.
- Expand the existing reanalysis correction UI to offer two alternatives when the automatic result is substantially wrong: a free-text hint, or an expert mode where the user lists high-level meal components separately (for example, grilled chicken and red beans).
- In expert mode, turn the structured component list into bounded guidance for the existing hint-aware reanalysis pipeline; the model still estimates each component's weight from the stored photo, so the user does not enter grams or macros.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `food-photo-recognition`: initial meal-photo uploads may include an optional, bounded hint that is forwarded into the first recognition request; the correction experience gains an expert component-list mode while automatic model identification and weight estimation remain the default.

## Impact

- Backend: `backend/pkg/server/food_upload.go` reads and validates the multipart hint and passes it into the existing hint-aware analysis pipeline; upload handler tests cover forwarding and rejection.
- Frontend: `frontend/app/food/upload/page.tsx` offers secondary optional hint entry, `frontend/components/food/ReanalyzeControl.tsx` gains hint/expert correction modes, and `frontend/lib/api.ts` appends initial guidance to the upload form while continuing to use the existing reanalysis endpoint.
- E2E: `e2e/tests/food.spec.ts` verifies automatic upload remains the default, initial hints are sent and bounded, and expert component guidance reaches reanalysis without requesting user-entered weights.
- API compatibility: additive only; existing clients that send only `photo` continue to work unchanged. No database migration or new dependency is required.
