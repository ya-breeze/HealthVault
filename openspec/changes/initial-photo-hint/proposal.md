## Why

The first photo analysis currently starts with no user context, even when the user already knows details that are hard to infer visually, such as ingredients, preparation, or which food is being shown. Users can provide that context only after an inaccurate result by running a second, billed reanalysis, so accepting the hint with the initial photo can improve the first result and avoid an unnecessary round trip.

## What Changes

- Add an optional hint field to the initial meal-photo upload interface.
- Send the hint alongside both selected-file and in-app-camera uploads.
- Accept the optional hint in the existing multipart `POST /api/food/meals` request, validate it consistently with reanalysis hints, and include it in the first vision-recognition prompt.
- Preserve the current upload behavior when the hint is blank or omitted.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `food-photo-recognition`: initial meal-photo uploads may include an optional, bounded hint that is forwarded into the first recognition request.

## Impact

- Backend: `backend/pkg/server/food_upload.go` reads and validates the multipart hint and passes it into the existing hint-aware analysis pipeline; upload handler tests cover forwarding and rejection.
- Frontend: `frontend/app/food/upload/page.tsx` captures the optional hint, and `frontend/lib/api.ts` appends it to the upload form.
- E2E: `e2e/tests/food.spec.ts` verifies the upload UI sends the hint with the photo and enforces the same client-side limit.
- API compatibility: additive only; existing clients that send only `photo` continue to work unchanged. No database migration or new dependency is required.
