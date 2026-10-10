# Readable exercise types in the personal MCP

Idea: https://ideaforge.ikoro.in/idea/1009

## Why

The owner confirmed activity reads through ChatGPT, but eight sessions appeared as type 79 and one as type 56. The Android webhook exporter sends ExerciseSessionRecord.exerciseType as a decimal string. The MCP currently returns that stored value without a name. Android Health Connect defines 79 as walking and 56 as running. The existing hcimport numeric lookup uses different assignments and cannot decode these webhook values safely.

## How

Keep the existing bounded exercise_type value and add nullable exercise_type_name plus exercise_type_mapping. Decode the published Health Connect session constants into readable English names; ChatGPT can translate these names into the conversation language. Mark this interpretation as health_connect, not evidence of a device or source. Unknown numeric codes and empty values have no name and mapping unknown. Preserve bounded nonnumeric names as stored_text without pretending they were decoded. Decode the original value before truncation so a truncated numeric prefix cannot become a known type. Keep text_truncated truthful for both returned text fields. Numeric values with a sign or truncated raw text remain unknown. Accept surrounding whitespace and bounded leading zeros without changing the stored text.

Use the Android reference https://developer.android.com/reference/androidx/health/connect/client/records/ExerciseSessionRecord and the AndroidX source https://github.com/androidx/androidx/blob/androidx-main/health/connect/connect-client/src/main/java/androidx/health/connect/client/records/ExerciseSessionRecord.kt verified on 2026-10-10. Preserve database rows, ingestion, hcimport behavior, owner isolation, pagination and activity totals. Do not migrate historical records or change UI. Update MCP descriptions and documentation, with a patch server version.

## Validation Commands

`make test-backend`

`make lint`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS=--retries=0`

### Task 1: Return safe names with raw exercise types

- [x] Implement the verified Health Connect name lookup and explicit mapping evidence.
- [x] Add nullable names to exercise evidence while preserving raw type, bounds and pagination.
- [x] Update tool guidance and document known, unknown and stored text behavior.
- [x] Mark completed.

### Task 2: Validate the follow-up before owner rollout

- [x] Test walking/running, other workout, further enum values, unknown numeric codes, stored text and truncation through the protocol.
- [x] Run backend/static checks and the native plus best-effort peer Review Gate.
- [x] Verify actual WIP source bytes, decoded synthetic protocol responses and full browser E2E.
- [x] Record WIP evidence and the pending approved production rollout and owner ChatGPT check.
- [x] Mark completed.

Validation evidence: full backend tests, Go vet and lint passed. The new protocol test covers 20 persisted sessions, including the owner-reported 79 and 56 codes, further Health Connect values, unknowns, signed codes, bounded leading zeros, whitespace, Unicode text and truncated numeric input. The test re-reads stored rows to verify unchanged raw types and duration.

Native Codex correctness, standards and specification reviews passed. Independent Claude system review completed; verified signed-code and truncated-numeric findings were fixed, and a new final gate completed without blocking findings. Malformed nonnumeric strings remain stored_text under the stated text contract; no workout is inferred from them.

WIP implementation 1321357b2bb1807609d05241560f7b379a6889f8 advertises MCP version 1.2.1. Its backend binary SHA-256 c2fa061b92c8f1c7305e52f84f4fefd2c6fc3465818bc548b137513781462e40 matches the pinned reference build byte for byte. Synthetic exercise pages return 79/Walking and 56/Running with health_connect mapping. Owner filtering, interval totals, pagination and unchanged persisted records pass. Synthetic health rows were removed and baseline activity and food records restored.

The initial browser invocation could not launch because the required Playwright Chromium binary was absent. That invocation was stopped. After installing the matching local browser, the complete suite passed 325 scenarios and skipped one with retries disabled. No application changes were required.

Production still runs activity release 8bce4c4. This PR awaits owner-approved merge and VM rollout, followed by an actual refreshed ChatGPT exercise-name check. The owner already confirmed production activity reads and requested this naming follow-up; Idea 1009 remains open until that feedback is resolved.
