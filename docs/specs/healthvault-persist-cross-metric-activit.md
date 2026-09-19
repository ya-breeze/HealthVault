# Preserve Health Connect provenance and exercise identity
Idea: ya-breeze/idea-forge#632

## Why

HealthVault cannot safely construct durable cross-metric activity episodes from its current normalized records. `ingest.PayloadJSON` in `backend/pkg/ingest/payload.go` drops the Health Connect record metadata already present in webhook records, while `ingest.Process` in `backend/pkg/ingest/ingest.go` upserts steps, heart rate, distance, and exercise only by `(user_id, start_time)` or `(user_id, time)`. Records from two data origins at the same instant therefore overwrite one another, while records at slightly different instants both survive with no retained origin available to explain or collapse them.

Exercise identity is also unreliable. `ExerciseJSON.Type` accepts only a string, while the Health Connect import path translates numeric codes through the private `exerciseTypeNames` table in `backend/pkg/hcimport/enums.go`. That table maps 33 to running and 53 to walking, but current production payloads use 56 for running and 79 for walking. The same activity can therefore acquire different or unknown names depending on whether it came through `/webhook/{username}` or `importHealthConnectHandler`.

This source-fidelity layer must land before episodes. An episode identifier, contributor membership rule, or overlap-aware total built on overwritten origins or an ambiguous activity type would be durable but wrong. The production evidence makes this timely: 36 walking sessions contain 36,605 heart-rate samples, enough volume that repeatedly reconstructing joins is costly, but also enough that an incorrect source rule would amplify quickly.

## How

Add one shared Health Connect metadata contract in `backend/pkg/ingest/payload.go` for the four inputs needed by the first explicit-session episode slice: exercise, heart rate, steps, and distance. Decode the exporter metadata object containing `id`, `data_origin`, `recording_method`, and optional device `type`, `manufacturer`, and `model`; tolerate the same fields at record level for backward compatibility. Represent optional scalar metadata with pointers so an omitted field remains distinguishable from a recorded zero. Keep `webhook_payloads.raw` unchanged as the complete audit source.

Give `database.Exercise`, `HeartRate`, `Steps`, and `Distance` an embedded provenance value containing nullable source-record ID, data origin, recording method, and device fields, plus an internal nullable `source_record_key`. Build keys in a dependency-free `backend/pkg/sourceidentity` helper shared by ingestion and migration. Hash a versioned, length-prefixed UTF-8 tuple with SHA-256 and store it as `hc1:<hex>`: interval records with an external ID use metric, origin, and ID; heart-rate series samples use metric, origin, parent record ID, and the sample timestamp; records without an ID use metric, origin, and the UTC `RFC3339Nano` primary time anchor. Including origin prevents origin-scoped IDs from colliding. Values, interval end times, recording method, and device details are excluded so corrections update the same row. Changing origin or external ID creates a new source identity; changing a heart-rate sample timestamp also creates a new sample because Health Connect supplies no separate sample identifier.

Because GORM `AutoMigrate` cannot safely replace populated unique indexes in one step, remove the four legacy `uniqueIndex` tags and add no automatic replacement tags. After `AutoMigrate` has added the nullable columns, have `database.Open` run an idempotent SQLite transaction that backfills every existing row through the same source-identity helper using an empty origin and its type/time anchor, verifies there are no null keys or collisions, creates stable unique indexes on `(user_id, source_record_key)`, and only then drops `idx_exercise_user_time`, `idx_hr_user_time`, `idx_steps_user_time`, and `idx_distance_user_time`. Any error must roll back before an old index is removed. The nullable schema is transitional compatibility for `AutoMigrate` and direct test fixtures; migrated rows and every `ingest.Process` write must have a nonempty key. The migration preserves IDs, payload links, measurements, and `updated_at`, and deletes or merges nothing.

Change only the four relevant loops in `ingest.Process` to conflict on `(user_id, source_record_key)`. On conflict, update the measurement, interval end, normalized provenance, exercise name/code, `SourcePayloadID`, and `updated_at`, while preserving the row ID, creation timestamp, user, family, and source key. `SourcePayloadID` continues to identify the latest payload that supplied the normalized row. Thus replay and correction remain idempotent, different origins at one timestamp coexist, and future episodes can retain stable foreign references. Other health types keep their current conflict behavior.

Move exercise-code normalization into a single helper under `backend/pkg/ingest`, used by both webhook decoding and `hcimport.readExercise`. Accept canonical string values or JSON numbers, persist the canonical name in `Exercise.ExerciseType`, and add a nullable `ExerciseTypeCode` for the original number. An unrecognized number stores canonical name `unknown` without discarding its code; an unrecognized nonempty string is normalized to lowercase snake case rather than discarded. Derive the full numeric table from the authoritative AndroidX Health Connect `ExerciseSessionRecord.EXERCISE_TYPE_*` constants used by the exporter, record the upstream revision or release date in the ADR, and pin the production-observed mappings 56 to `running` and 79 to `walking` in tests. Numeric strings are not treated as codes.

Extend `backend/pkg/hcimport/reader.go` to inspect `PRAGMA table_info` before composing metadata projections. For interval tables, recognize the export columns for UUID, package/data origin, recording method, and device fields. For heart rate, join `heart_rate_record_series_table.parent_key` to the record-level metadata row when the parent table exists, and use the parent UUID plus each series timestamp as sample identity. Missing metadata columns or a legacy fixture without the parent table must leave metadata absent rather than fail the import. Import and webhook records then flow through the same `ingest.Process` identity and exercise normalization rules.

The existing authenticated raw-data surface remains the verification surface: `GET /api/data/exercise`, `/api/data/heart_rate`, `/api/data/steps`, and `/api/data/distance` may return `source_record_id`, `data_origin`, `recording_method`, device fields, and `exercise_type_code`, with absent values encoded as null. No provenance filter is added in this split. Keep `source_record_key` out of `QueryRecords` and therefore out of both the HTTP response and MCP `query_data`; it is an internal idempotency key, not a public identifier.

This split deliberately stops at trustworthy normalized source records. It does not create activity-episode tables, contributor summaries, recomputation hooks, episode backfills, the bounded episode API, the MCP episode tool, or measurement drill-down. Those belong in the child change, where grouping and the read contract can be reviewed together against stable source identity. It also excludes inferred or manual episodes, heart-rate zones, planned-exercise coupling, speed/cadence/calorie contributors, production-data inspection, and owner-controlled deployment work.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Share metadata, source identity, and exercise decoding
- [ ] Add the normalized Health Connect metadata and device shapes to `backend/pkg/ingest/payload.go`, attach them to `ExerciseJSON`, `HeartRateJSON`, `StepsJSON`, and `DistanceJSON`, and preserve compatibility with payloads that omit metadata.
- [ ] Add `backend/pkg/sourceidentity/key.go` with the versioned key algorithm from `How`, including interval-ID, heart-rate-series-sample, and metadata-free fallback variants.
- [ ] Add a shared exercise-type decoder and normalizer under `backend/pkg/ingest` that accepts canonical strings and JSON numeric codes, returns a canonical name plus nullable original code, and retains unknown values according to `How`.
- [ ] Replace `hcimport.exerciseTypeName` and its private stale vocabulary with the shared normalizer sourced from the exporter-compatible AndroidX constants, including mappings 56 to `running` and 79 to `walking`.
- [ ] Add table-driven tests for nested and record-level metadata, omitted and zero-valued metadata, deterministic source keys, string and numeric exercise types, the two production-observed codes, and unknown values.
- [ ] Mark completed

### Task 2: Migrate the four normalized source tables
- [ ] Add the embedded provenance fields, nullable internal source-record key, and nullable exercise type code to the relevant models in `backend/pkg/database/models.go`, retaining each model's existing `SourcePayloadID` link.
- [ ] Remove the old GORM uniqueness tags from the four models and add an explicit post-`AutoMigrate` migration in `backend/pkg/database/db.go` with stable replacement index names and transactional backfill, collision checks, index creation, and old-index removal in that order.
- [ ] Generate migrated fallback keys through `backend/pkg/sourceidentity`, verify every existing row has a nonempty key before dropping any old index, and update columns without advancing `updated_at`.
- [ ] Adjust `QueryRecords` projection in `backend/pkg/database/storage_impl.go` so the retained provenance fields are readable but `source_record_key` is never serialized through HTTP or MCP.
- [ ] Extend `backend/pkg/database/db_test.go` with file-backed reopen coverage proving rows and timestamps survive unchanged, migration is repeatable, replacement indexes are not recreated in the wrong order, and two nonempty source keys may share a timestamp while duplicate source keys may not.
- [ ] Mark completed

### Task 3: Make ingest source-aware and idempotent
- [ ] In `backend/pkg/ingest/ingest.go`, derive nonempty source keys for exercise, heart rate, steps, and distance and use explicit conflict targets on `(user_id, source_record_key)` for those four loops while leaving every other health type unchanged.
- [ ] Persist the four records' provenance and canonical exercise name/code, use explicit update-column lists that preserve stable row IDs and creation fields, and return distance and exercise write errors instead of discarding them.
- [ ] Extend `backend/pkg/ingest/ingest_test.go` to prove same-source replay stays one row, correction updates it in place, two origins at one timestamp coexist, heart-rate samples sharing a parent record ID remain distinct, and identity-changing metadata creates a separate row.
- [ ] Add a migration-to-replay regression proving a metadata-free record ingested after migration updates the backfilled legacy row because SQL backfill and Go ingestion generate byte-identical fallback keys.
- [ ] Add a webhook handler regression test showing a metadata-rich numeric exercise payload reaches normalized storage with its provenance, original code, canonical type, and stable row identity intact.
- [ ] Mark completed

### Task 4: Preserve provenance in Health Connect imports
- [ ] Add schema-introspection helpers in `backend/pkg/hcimport/reader.go` for the current export's UUID, package/data-origin, recording-method, and device columns, including the record-parent join needed by heart-rate series.
- [ ] Populate the shared metadata shape from `readExercise`, `readHeartRate`, `readSteps`, and `readDistance`, use parent ID plus sample time for heart-rate identity, and send exercise codes through the shared current vocabulary.
- [ ] Update `backend/pkg/hcimport/reader_test.go` with a metadata-rich fixture proving all four readers preserve provenance, multiple heart-rate samples survive, and codes 56 and 79 normalize correctly.
- [ ] Retain a legacy minimal fixture with no metadata columns or heart-rate parent table, and prove it imports successfully with absent provenance.
- [ ] Add an import replay test showing that importing the same metadata-rich database twice updates the same normalized rows rather than duplicating them.
- [ ] Mark completed

### Task 5: Pin the boundary and end-to-end behavior
- [ ] Add `docs/adr/ADR-016-health-connect-source-identity.md` documenting the exact key algorithm, migration ordering and rollback constraint, normalized metadata fields, authoritative exercise-vocabulary revision, and why episode grouping is deferred until provenance is trustworthy.
- [ ] Add Health Record Provenance and Canonical Activity Type entries to `CONTEXT.md`, distinguishing a Data Origin from `SourcePayloadID`, a parent heart-rate record from its samples, and a numeric source code from the stored canonical activity name.
- [ ] Extend `e2e/tests/import.spec.ts` or add a focused test that posts metadata-rich webhook records from two origins at one timestamp, replays and corrects one source, then reads the existing authenticated `/api/data/{type}` endpoints and proves both origins remain separate while only the matched source changes.
- [ ] Cover a numeric exercise record in the same end-to-end path and assert code 79 reads back as canonical `walking` with its provenance; do not introduce an episode endpoint or UI in this split.
- [ ] Mark completed
