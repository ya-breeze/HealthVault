# Localize health-data table headers and record-table states
Idea: ya-breeze/idea-forge#266

## Why

Every `/data/<type>/` page builds its record table from the keys of the first raw API record. In `frontend/app/data/[type]/DataTypeClient.tsx`, `displayColumns` filters a small set of internal identifiers but the header loop still renders each remaining key verbatim. Current responses therefore expose schema names such as `created_at`, `start_time`, `rmssd_millis`, `systolic`, and `dietary_fiber_grams` as user-facing labels. The adjacent Actions heading and the table's loading, empty, deletion, confirmation, cancellation, and accessibility text are also English literals.

The page already consumes `useLanguage`, but only the steps diagnostic disclosure uses it. The title is still derived with `type.replace(/_/g, ' ')`, even though `metricLabel` in `frontend/lib/i18n/index.ts` already provides typed English and Russian names for every entry in `DATA_TYPES`. The comments at the top of `frontend/lib/i18n/en.ts` explicitly identify per-type detail pages as untranslated, confirming that the mixed-language result is known rather than intentional.

The owner's later comments expand the idea to complete English and Russian localization of the entire detail page. That full scope crosses three independently complex surfaces: the dynamic record table, Recharts legends and weight-projection copy, and the reusable write form. This first change establishes and applies the schema-to-presentation contract to the record table, which is the defect that filed the idea; the remaining chart and form surface is deferred to a child change so each can be reviewed against a coherent UI boundary.

## How

Keep presentation metadata in the frontend. `backend/pkg/server/api.go`'s `typeRegistry` and `frontend/lib/api.ts`'s `DATA_TYPES` describe transport and routing contracts, while translated copy belongs in `frontend/lib/i18n/en.ts` and `frontend/lib/i18n/ru.ts`. Add a small `frontend/lib/dataColumnMeta.ts` module that maps raw response keys to typed dictionary keys and exposes a `dataColumnLabel(t, type, column)` resolver. This follows the dashboard convention in `frontend/lib/vitals.ts`: registries carry identifiers, while `metricLabel` resolves their localized display names.

Use a shared base map for unambiguous columns such as creation/update timestamps, start/end/time fields, meal name, and status, plus per-`DataType` overrides for keys whose meaning depends on the metric. In particular, `meters`, `kilograms`, `percentage`, `bpm`, `calories`, and `duration_seconds` need metric-specific labels and units. Labels must describe the raw value shown by the table: distance records are stored in metres and sleep durations in seconds even though the chart converts them to kilometres and hours through `toDisplayUnit`. Audit the complete current raw schema in `backend/pkg/database/models.go` and the `food_meals` projection in `columnAllowlist` within `backend/pkg/database/storage_impl.go`; a current column must not reach the localized fallback. For an unexpected future key, show a localized generic field label rather than exposing snake_case, and retain the raw key only in developer-facing diagnostics.

In `DataTypeClient`, preserve the existing data-driven `displayColumns`, column order, row rendering, ownership rules, and deletion flow. Resolve the page title with `metricLabel`, every table header with `dataColumnLabel`, and the Actions header plus loading, empty, confirmation, cancellation, deletion-error, and delete-button accessible labels through `t`. Use `dateLocaleFor(language)` for timestamps rendered in the record table, and reuse `mealStatusLabel` for the `food_meal` status column; user-authored names and opaque health-data values remain data and are not translated. Do not display raw backend error bodies in this localized table surface: retain error state by category and render a localized operation-level deletion failure message.

This change deliberately does not alter the API response, database models, `displayColumns` filtering, table markup, responsive classes, or horizontal scrolling. It also does not localize the zoom controls, nutrition macro selector, chart legends and tooltips, statistics, BMI and projection copy, weight shortcuts, `AddRecordForm`, or the Suspense fallback; those form the deferred child change. No public hostname, deployment stack, credential, or owner-only action is required.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Define localized record-column metadata

- [x] Add `frontend/lib/dataColumnMeta.ts` with typed shared column-label keys, per-`DataType` overrides, and a `dataColumnLabel(t, type, column)` resolver that prefers a type override, then the shared map, then a localized unknown-field fallback.
- [x] Cover every user-visible raw column currently declared by the health models in `backend/pkg/database/models.go`, including the interval, point, blood-pressure, skin-temperature, sleep, exercise, speed, and nutrition-specific fields, as well as every field selected for `food_meals` by `columnAllowlist` in `backend/pkg/database/storage_impl.go`.
- [x] Give ambiguous value columns metric-specific labels with their stored units, so similarly named database fields remain understandable without changing or converting the table values.
- [x] Add the required `dataTable.*` column, action, state, and error keys to `frontend/lib/i18n/en.ts` and matching Russian translations to `frontend/lib/i18n/ru.ts`; keep the existing `Dictionary = typeof en` parity check intact and update the catalog scope comments that currently call detail pages untranslated.
- [x] Keep literal English display copy out of the metadata module by storing only dictionary keys typed against `Dictionary`.
- [x] Mark completed

### Task 2: Apply the metadata and translations to the record table

- [x] Update `frontend/app/data/[type]/DataTypeClient.tsx` to read both `t` and `language` from `useLanguage`, render the heading through the existing `metricLabel`, and render each dynamic header through `dataColumnLabel(t, dataType, k)` without changing `displayColumns` or table layout.
- [x] Translate the Actions heading, table loading and empty states, delete confirmation and cancellation controls, delete-button `aria-label`, and deletion failure message through the new catalog keys.
- [x] Change `handleConfirmDelete` and its state so an `ApiError` or other exception cannot place an English backend message directly into the localized UI; preserve the existing pending-row and retry behavior while showing a localized operation-level failure.
- [x] Format record-table timestamps with `dateLocaleFor(language)` and localize a `food_meal` row's known status values through the existing `mealStatusLabel`; leave user-authored meal names and other stored string values unchanged.
- [x] Preserve the owner/family-member distinction: the Actions column and delete controls remain absent whenever `userParam` is present.
- [x] Mark completed

### Task 3: Add regression coverage for both languages and all current columns

- [ ] Add `frontend/lib/dataColumnMeta.test.ts` covering shared labels, ambiguous per-type overrides, unique metric fields, the `food_meal` allowlist fields, and the localized fallback for an unknown key.
- [ ] In that unit test, enumerate the current user-visible raw columns for every `DATA_TYPES` member and assert that both English and Russian resolve each one without returning the raw key or the unknown-field fallback; this is the guard against a model field being exposed without presentation metadata.
- [ ] Extend `e2e/tests/data-types.spec.ts` with an English record-table case that verifies a real metric page shows readable column and action labels and does not render its snake_case database keys as headings.
- [ ] Add a Russian case following the settings-save and `try`/`finally` cleanup convention in `e2e/tests/settings.spec.ts`; verify the localized metric title, representative shared and per-type table headers, Actions heading, and delete confirmation/cancellation text, then restore English even if an assertion fails.
- [ ] Cover a family-member view or equivalent mocked state to prove localization does not accidentally add the owner-only Actions column.
- [ ] Mark completed
