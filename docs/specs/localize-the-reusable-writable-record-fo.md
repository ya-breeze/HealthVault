# Localize the reusable writable-record form in English and Russian
Idea: ya-breeze/idea-forge#467

## Why

The predecessor change in `docs/specs/complete-the-owner-selected-english-and.md` localizes the data-detail route’s headings, controls, charts, summaries, loading state, and record table, but explicitly leaves `frontend/components/AddRecordForm.tsx` in English. Consequently, Russian users still see English value and time labels, validation errors, Add/Saving/Cancel actions, and backend-provided error text on the otherwise localized weight, height, and goal-weight pages and shortcuts.

`AddRecordForm` currently embeds `kg` and `m` directly in `WRITE_UNITS`, even though the dictionaries already provide `unit.kg`. It also renders `ApiError.message` or another `Error.message` after failed writes, allowing English server responses from `frontend/lib/api.ts` and `backend/pkg/server/api.go` to leak into Russian UI. This is actionable now because the prerequisite branch already supplies `useLanguage`, typed English/Russian dictionaries, `interpolate`, localized shortcut controls, and the established operation-level error convention used by the record-delete flow.

This is the remaining writable surface split from idea 393. Its validation and asynchronous save behavior form one cohesive review surface and can now be covered deterministically without altering the chart/page work that has already landed.

## How

Add a dedicated `addRecord.*` catalog to `frontend/lib/i18n/en.ts` and matching entries in `frontend/lib/i18n/ru.ts`. Preserve the current English copy for Value, Value ({unit}), Time (optional), Enter a positive number, Enter a value between {min} and {max} {unit}, Add, Saving…, and Cancel. Use “Could not save the record. Try again.” for the operation-level English failure. Add the Russian copy Значение, Значение ({unit}), Время (необязательно), Введите положительное число, Введите значение от {min} до {max} {unit}, Добавить, Сохранение…, Отмена, and Не удалось сохранить запись. Попробуйте ещё раз. Add `unit.m` as `m`/`м` and reuse the existing `unit.kg` as `kg`/`кг`. Keep `Dictionary = typeof en` as the exact key-parity check and update the English dictionary’s scope comment so `AddRecordForm` is no longer described as an English-only part of the data-detail route.

In `frontend/components/AddRecordForm.tsx`, type the unit metadata as dictionary keys, preferably with an `Extract<keyof Dictionary, \`unit.${string}\`>` unit-key type and the repository’s `Partial<Record<DataType, ...>>` convention. Rename the metadata field from the rendered `unit` string to `unitKey`, while retaining weight and weight-goal bounds of 20–500 and height bounds of 0.5–2.5. Resolve the selected unit through `t`, and use `interpolate` for both the unit-bearing Value label and the range-validation sentence so the label and error cannot drift to different units.

Read `t` from `useLanguage` and translate the plain Value fallback, Time (optional), positive-number validation, Add, Saving…, and Cancel. Represent validation and save failures as semantic local state rather than storing a backend message, resolving that state through the current dictionary when rendered. Catch save failures without inspecting or exposing `ApiError.message` or arbitrary `Error.message`, remove the now-unused `ApiError` import, and always show the localized operation-level failure. This intentionally trades diagnostic detail in this user-facing form for a stable message that cannot leak backend English or implementation details.

Preserve the existing input attributes and behavior: `step="any"`, `required`, per-type `min`/`max`, `maxLocalDateTime`, conversion of a supplied local datetime through `new Date(time).toISOString()`, omission of `time` when blank, the `{ value, time? }` request shape, clearing value and time only after success, invoking `onSuccess` after the reset, retaining entered fields after failure, and restoring the submit button in `finally`. Do not change `DataTypeClient`’s writable-type allowlist, owner/family-member gates, Set goal or Set height opening conditions, cancellation callbacks, refresh-key behavior, chart logic, backend bounds, API handlers, or database behavior.

Extend `e2e/tests/data-types.spec.ts` with method-aware, in-memory route mocks for the three writable types. The mocks must distinguish GET requests from POST requests, capture request bodies, support deterministic delayed and failed POST responses, and update their GET result after a successful POST so reset and refresh are proved rather than inferred. Exercise React’s submit handler deliberately for invalid values because the existing native `required` and `min`/`max` constraints may otherwise stop the submit event before the localized client-validation branches run. Avoid fixed sleeps: hold and release mocked responses explicitly when asserting Saving… and the disabled submit state.

Cover weight, height, and weight-goal forms in both English and Russian. For each language, assert the localized unit-bearing Value label, Time label, actions, exact `min`/`max`, the positive-number path, the out-of-range path with the correct bounds and localized unit, and that neither client-validation failure sends a POST. Cover failed saves using an unmistakably English mocked backend body, assert only the localized operation-level message is shown, and verify the form remains populated and usable. Cover a delayed successful save, the localized saving label and disabled submit button, request-body and timestamp semantics, field reset, re-enabled Add action, and refreshed records. Cover cancellation through both weight-page shortcut forms, while retaining the absence of Cancel on directly mounted forms and the absence of all write forms and shortcuts in a family-member view.

Every Russian test must use the existing `withSettingsSave`-based language helpers and wrap its assertions in `try`/`finally`, calling `restoreEnglishDisplayLanguage` in the `finally` block so a failure cannot leak `display_language=ru` into later English tests. This change requires no hostname, deployment, credential, production-stack, or other owner-only action.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

### Task 1: Define the writable-form localization contract

- [ ] Add `addRecord.*` entries to `frontend/lib/i18n/en.ts` for the plain and unit-bearing Value labels, optional Time label, positive-number and interpolated range validation messages, Add, Saving…, Cancel, and the operation-level save failure, using the exact English copy from `## How`.
- [ ] Add matching Russian entries to `frontend/lib/i18n/ru.ts` using the exact Russian copy from `## How`, preserving compile-time parity through `Dictionary = typeof en`.
- [ ] Add `unit.m` as `m` in English and `м` in Russian, and retain the existing `unit.kg` translations for kilogram forms.
- [ ] Update `en.ts`’s localization scope comment to identify the complete data-detail writable form as covered and remove the obsolete statement that `AddRecordForm` remains English.
- [ ] Mark completed

### Task 2: Localize AddRecordForm without changing its write contract

- [ ] Update `WRITE_UNITS` in `frontend/components/AddRecordForm.tsx` to carry typed `unit.*` dictionary keys and the unchanged 20–500 kg and 0.5–2.5 m bounds, following the repository’s typed dictionary-key and partial `DataType` metadata conventions.
- [ ] Read `t` through `useLanguage`, resolve each unit key once, and translate the plain Value fallback, interpolated unit-bearing Value label, Time (optional), Add, Saving…, and conditional Cancel UI.
- [ ] Replace stringly validation state with semantic error states resolved through `t` and `interpolate`, preserving the positive-number check before the per-type range check and rendering the correct localized bounds and unit.
- [ ] Replace `ApiError`/`Error.message` rendering with the localized operation-level save failure and remove the unused `ApiError` import so no server response body can appear in either language.
- [ ] Preserve request construction, blank-time omission, local-datetime ISO conversion, `maxLocalDateTime`, input constraints, success reset and callback order, failed-input retention, saving cleanup, cancellation behavior, and every `DataTypeClient` ownership/opening gate.
- [ ] Mark completed

### Task 3: Add deterministic English writable-form coverage

- [ ] Add reusable method-aware route fixtures to `e2e/tests/data-types.spec.ts` for weight, height, and weight-goal GET/POST traffic, with captured bodies, mutable returned records, explicit failure responses, and explicitly controlled delayed responses; keep them isolated from the existing chart mocks and persistent seeded account data.
- [ ] Add table-driven English assertions for all three direct forms’ Value-with-unit and Time labels, Add action, absence of Cancel, and exact `min`/`max` attributes, expecting `kg`, `m`, and `kg` respectively.
- [ ] Exercise both custom client-validation branches for each type by dispatching submission past native constraint blocking: assert the exact English positive-number and bounded-range messages and prove no POST was issued.
- [ ] For each writable type, mock a failed POST with a distinctive English server body; assert the operation-level English failure, absence of the server body, retained value/time, and restored enabled Add action.
- [ ] For each writable type, hold a valid POST response, assert Saving… and a disabled submit action while it is pending, then release it and assert the captured `{ value, time? }` semantics, cleared value/time fields, restored Add action, and refreshed record output.
- [ ] Cover Cancel on both Set goal and Set height shortcut forms by asserting that it closes the form without a POST and restores the relevant shortcut; also assert a weight family-member view renders neither direct nor shortcut write forms.
- [ ] Mark completed

### Task 4: Mirror writable-form coverage in Russian and contain language state

- [ ] Run the Russian writable-form scenarios through the existing `selectRussianDisplayLanguage`/`withSettingsSave` convention, placing every Russian test body in `try`/`finally` and calling `restoreEnglishDisplayLanguage` from `finally`.
- [ ] Assert all three forms render Значение with the translated `кг`/`м` unit, Время (необязательно), Добавить, and their unchanged numeric bounds; keep direct forms free of an Отмена action.
- [ ] Exercise both client-validation paths for weight, height, and weight goal, asserting the exact Russian positive-number and interpolated range messages and no POST on either failure.
- [ ] Cover failed POSTs for all three types with a distinctive English backend body, asserting Не удалось сохранить запись. Попробуйте ещё раз., absence of the backend text, retained fields, and a re-enabled Добавить action.
- [ ] Cover explicitly delayed successful saves with Сохранение… and a disabled submit action, then verify request-body/timestamp semantics, reset fields, Добавить restoration, and refreshed records after release.
- [ ] Cover Отмена on the localized Set goal and Set height shortcut forms, proving cancellation sends no POST and returns to the localized shortcut state, while preserving owner-only visibility.
- [ ] Mark completed
