# Readable exercise types in the personal MCP

Idea: https://ideaforge.ikoro.in/idea/1009

## Why

The owner confirmed activity reads through ChatGPT, but eight sessions appeared as type 79 and one as type 56. The Android webhook exporter sends ExerciseSessionRecord.exerciseType as a decimal string. The MCP currently returns that stored value without a name. Android Health Connect defines 79 as walking and 56 as running. The existing hcimport numeric lookup uses different assignments and cannot decode these webhook values safely.

## How

Keep the existing bounded exercise_type value and add nullable exercise_type_name plus exercise_type_mapping. Decode the published Health Connect session constants into readable English names; ChatGPT can translate these names into the conversation language. Mark this interpretation as health_connect, not evidence of a device or source. Unknown numeric codes and empty values have no name and mapping unknown. Preserve bounded nonnumeric names as stored_text without pretending they were decoded. Decode the original value before truncation so a truncated numeric prefix cannot become a known type. Keep text_truncated truthful for both returned text fields.

Use the Android reference https://developer.android.com/reference/androidx/health/connect/client/records/ExerciseSessionRecord and the AndroidX source https://github.com/androidx/androidx/blob/androidx-main/health/connect/connect-client/src/main/java/androidx/health/connect/client/records/ExerciseSessionRecord.kt verified on 2026-10-10. Preserve database rows, ingestion, hcimport behavior, owner isolation, pagination and activity totals. Do not migrate historical records or change UI. Update MCP descriptions and documentation, with a patch server version.

## Validation Commands

`make test-backend`

`make lint`

`make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS=--retries=0`

### Task 1: Return safe names with raw exercise types

- [ ] Implement the verified Health Connect name lookup and explicit mapping evidence.
- [ ] Add nullable names to exercise evidence while preserving raw type, bounds and pagination.
- [ ] Update tool guidance and document known, unknown and stored text behavior.
- [ ] Mark completed.

### Task 2: Validate the follow-up before owner rollout

- [ ] Test walking/running, other workout, further enum values, unknown numeric codes, stored text and truncation through the protocol.
- [ ] Run backend/static checks and the native plus best-effort peer Review Gate.
- [ ] Verify actual WIP source bytes, decoded synthetic protocol responses and full browser E2E.
- [ ] Record WIP evidence and the pending approved production rollout and owner ChatGPT check.
- [ ] Mark completed.
