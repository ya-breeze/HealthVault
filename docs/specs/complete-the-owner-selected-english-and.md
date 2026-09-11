# Localize data-detail charts and weight insights in English and Russian
Idea: ya-breeze/idea-forge#393

## Why

The predecessor change documented in `docs/specs/healthvault-data-detail-page-the-table-h.md` localizes the `/data/<type>/` heading, raw-record table, deletion states, and steps diagnostic copy, but deliberately leaves the rest of the route English. The current `frontend/app/data/[type]/DataTypeClient.tsx` still hard-codes Day/Week/Month/Year, Set goal/Set height, chart summary labels, BMI category names, projection outcomes, and every explicit Recharts series name. `NUTRITION_MACROS` in `frontend/lib/dataTypeMeta.ts` carries English presentation labels, while unnamed `Line` and `Bar` series expose transport-oriented data keys in tooltips. A Russian owner therefore still sees a visibly mixed-language page after the table-localization change lands.

Formatting is mixed for the same reason. `bucketLabel`, Day-axis ticks and tooltip timestamps, the steps diagnostic date, and the projection ETA use the browser locale instead of the selected language; `formatMetricValue` is pinned to `en-US`; and BMI uses `toFixed(1)`. The Suspense boundary in `frontend/app/data/[type]/page.tsx` also renders a literal `Loading...`. These are actionable now because the predecessor branch has established `useLanguage`, `dateLocaleFor`, typed English/Russian dictionaries, and `metricLabel` on this route.

The complete filed idea also includes `frontend/components/AddRecordForm.tsx`, but chart/page presentation and writable-form validation/API errors are independent review surfaces. Landing the chart/page half first completes read-only visualization localization without mixing form error-policy changes into the same review; the reusable form is the deferred child.

## How

Add a `dataDetail.*` catalog in `frontend/lib/i18n/en.ts` and matching Russian entries in `frontend/lib/i18n/ru.ts`, retaining `Dictionary = typeof en` as the compile-time parity check and updating the English catalog's scope comment. Use typed `keyof Dictionary` label keys rather than English presentation strings in `ZOOMS` and `NUTRITION_MACROS`; the current tree uses `NUTRITION_MACROS` only from `DataTypeClient`. Keep the existing English copy, including Day, Week, Month, Year, Set goal, Set height, Calories, Protein, Carbs, Fat, Sugar, Sodium, Fiber, Systolic, Diastolic, Systolic range, Diastolic range, Goal, Range, Avg, Trend, Projection, Avg, Max, Total, BMI, and the five existing projection messages. Use the corresponding Russian copy День, Неделя, Месяц, Год, Задать цель, Указать рост, Калории, Белки, Углеводы, Жиры, Сахар, Натрий, Клетчатка, Систолическое, Диастолическое, Диапазон систолического давления, Диапазон диастолического давления, Цель, Диапазон, Среднее, Тренд, Прогноз, Среднее, Макс., Всего, and ИМТ. Translate the BMI categories as Недостаточный вес, Нормальный вес, Избыточный вес, and Ожирение. Translate the projection outcomes as Вы достигли целевого веса; С текущей динамикой цель не будет достигнута; При текущей динамике вы достигнете цели примерно {date}; Пока недостаточно данных для прогноза; and Не удалось загрузить историю веса. Use `interpolate` for the dated outcome and preserve the current English sentences and punctuation.

Give every tooltip-visible Recharts series an explicit localized `name`. The ordinary Day `Line` and non-nutrition cumulative `Bar` use `metricLabel`; nutrition's Day `Line` and bucketed `Bar` use the selected macro label. Blood-pressure Day and bucketed average lines use Systolic and Diastolic, while the two bucketed range `Area` elements use their specific range names and retain `legendType="none"`. The generic point-range `Area` uses Range, its average `Line` uses Avg, and weight's additional lines use Trend and Projection. Translate the Goal `ReferenceLine` label. Preserve all `dataKey` values, legend visibility, chart calculations, projection gates, series visibility, colors, strokes, and interaction props.

Make number formatting follow the selected language without changing unrelated dashboard output. Add `numberLocaleFor` beside `dateLocaleFor` in `frontend/lib/i18n/index.ts`, mapping English to `en-US` and Russian to `ru-RU`. Extend `formatMetricValue` in `frontend/lib/dataTypeMeta.ts` with an optional locale argument whose default remains `en-US`, then pass `numberLocaleFor(language)` from `DataTypeClient` for Y ticks, tooltip values including range tuples, and Avg/Max/Total values. Format BMI through the same selected number locale with exactly one fractional digit. Change `BmiCategory` and `classifyBmi` from English presentation values to the stable identifiers `underweight`, `normal`, `overweight`, and `obese`; current consumers are limited to `DataTypeClient` and `frontend/lib/dataTypeMeta.test.ts`. Preserve the existing lower-inclusive boundaries at 18.5, 25, and 30.

Pass `dateLocaleFor(language)` into `bucketLabel`, Day-axis time labels, Day tooltip timestamps, the steps diagnostic date, and the projection crossing date. Preserve the existing formatting options: abbreviated month plus day for daily buckets, abbreviated month plus two-digit year for yearly zoom, hour-only Day ticks, the current full Day-tooltip timestamp, and abbreviated month plus day and year for the ETA. Retain `timeZone: 'UTC'` for bucket-derived labels and the crossing date so localization cannot reintroduce the behind-UTC one-day shift. Keep raw record-table values and its already-localized timestamp behavior unchanged.

Replace `page.tsx`'s server-authored English fallback with a colocated client component, `frontend/app/data/[type]/DataTypeLoading.tsx`, that reads `useLanguage` and renders `dataDetail.loading` with the existing fallback layout. The root `LanguageProvider` already wraps route content, so no provider or settings changes are needed. This route fallback is distinct from `weightContextStatus === 'loading'`, which remains intentionally silent until the weight context settles.

This part deliberately does not change `AddRecordForm`, write bounds, API contracts, database schemas, record-table markup, table horizontal scrolling, chart layout, chart math, or mobile touch behavior. It requires no public hostname, protected deployment, credential, or owner-only action.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Define localized chart metadata and formatting contracts

- [x] Add the complete `dataDetail.*` English catalog to `frontend/lib/i18n/en.ts` and matching Russian entries to `frontend/lib/i18n/ru.ts`: zooms, shortcuts, seven nutrition macros, blood-pressure labels and ranges, Goal/Range/Avg/Trend/Projection, Avg/Max/Total/BMI summaries, four BMI categories, reached/not-on-track/on-track/insufficient-data/load-failure projection messages, and the page loading state, using the copy specified in `## How`.
- [x] Update the localization scope comment in `en.ts` so data-detail charts and page loading are covered while writable forms remain explicitly English until the deferred child; keep `Dictionary = typeof en` enforcing exact Russian key parity.
- [x] Change `ZOOMS` in `DataTypeClient.tsx` and `NUTRITION_MACROS` in `frontend/lib/dataTypeMeta.ts` to carry `keyof Dictionary` label keys instead of English display strings, without changing their zoom/macro values, order, or sole current consumer.
- [x] Add `numberLocaleFor` to `frontend/lib/i18n/index.ts`, extend `formatMetricValue` with an optional locale that defaults to `en-US`, and update its documentation so existing dashboard callers retain their current output while data-detail callers can request `ru-RU` formatting.
- [x] Change `BmiCategory` and `classifyBmi` to the stable lower-case identifiers specified in `## How`; extend `frontend/lib/dataTypeMeta.test.ts` to retain interior and exact-boundary BMI coverage and to assert English and Russian grouping/decimal behavior from `formatMetricValue` without changing precision.
- [x] Mark completed

### Task 2: Localize controls, every Recharts series, and chart formatting

- [x] Update `DataTypeClient.tsx` to translate Day/Week/Month/Year, Set goal, Set height, and all seven nutrition macro selectors through `t`, preserving selected state, ordering, owner/family gates, and existing form-opening behavior.
- [x] Assign the explicit localized names described in `## How` to every tooltip-visible `Line`, `Bar`, and `Area`, including ordinary Day metrics, Day and bucketed nutrition, Day and bucketed blood pressure, both blood-pressure ranges, cumulative bars, point ranges and averages, weight trend, and projection; preserve the range Areas' hidden legend behavior.
- [x] Translate the Goal `ReferenceLine` label and ensure the same translated names reach Recharts legends and both pointer and touch tooltip paths without changing `dataKey`, chart interaction, or series visibility.
- [x] Pass `numberLocaleFor(language)` through every Y-axis tick, scalar/range tooltip value, and Avg/Max/Total readout, and format BMI to one localized decimal without changing the underlying values, conversions, or precision.
- [x] Make `bucketLabel` accept the selected date locale and apply `dateLocaleFor(language)` to every bucket label, Day-axis time tick, Day tooltip timestamp, steps-diagnostic day, and projection ETA while preserving the current options, UTC rules, and invalid-label fallback.
- [x] Mark completed

### Task 3: Localize weight summaries, BMI, and projection outcomes

- [x] Translate Avg, Max, Total, and BMI summary headings and resolve `classifyBmi`'s stable result through the matching `dataDetail.bmi.*` key, without changing summary calculations, height gating, BMI thresholds, band rendering, or the one-decimal readout.
- [x] Translate Goal, Range, Avg, Trend, and Projection everywhere they surface through reference labels, legends, and tooltips, preserving the current Week/Month/Year trend and Month/Year projection-line rules.
- [x] Replace the projection literals with dictionary-backed copy for reached, not-on-track, on-track ETA, insufficient data, and weight-history load failure; interpolate the localized UTC crossing date into the on-track message while preserving `weightContextStatus`, goal gates, and outcome priority.
- [x] Keep the weight-context loading state silent and keep a missing goal free of projection copy exactly as today; localization must not collapse loading, ready-with-insufficient-data, and fetch failure into one state.
- [x] Mark completed

### Task 4: Localize the Suspense fallback

- [x] Add `frontend/app/data/[type]/DataTypeLoading.tsx` as a client component that reads `dataDetail.loading` from `useLanguage` and preserves the current fallback spacing and muted styling.
- [x] Use `DataTypeLoading` as the `<Suspense>` fallback in `frontend/app/data/[type]/page.tsx`, leaving `generateStaticParams`, async params unwrapping, and `DataTypeClient` mounting unchanged.
- [x] Mark completed

### Task 5: Cover both languages across every chart family

- [ ] Extend `e2e/tests/data-types.spec.ts` with deterministic mocked raw and bucketed responses for `heart_rate` as the ordinary point page, `blood_pressure`, `nutrition`, and `weight`; route weight context requests by type, bucket, and requested range so height, goal, projection-ready, insufficient-data, and failed-history fixtures cannot satisfy the wrong request.
- [ ] Add English assertions for heart-rate Day and Week views, blood-pressure Day and Week views, nutrition Day and Week views, and weight Week and Month views, covering zoom and shortcut controls, macro selectors, every applicable tooltip/legend name, summary headings, BMI category, representative date/number formatting, and projection copy.
- [ ] Add matching Russian assertions across the same four page shapes, including Russian bucket/tooltip dates, `ru-RU` grouped and comma-decimal chart values, the BMI category, and reached, not-on-track, on-track ETA, insufficient-data, and load-failure projection states; use deterministic mocks rather than persistent seeded health records.
- [ ] Compute locale-sensitive expected date and number strings inside the browser when non-breaking spaces or platform punctuation may vary, while still asserting the exact translated labels and messages from the dictionaries.
- [ ] Put every test that selects Russian inside the existing `withSettingsSave` plus `try`/`finally` convention and restore `display_language` to `en` even after assertion failure, so existing English selectors and later suites cannot inherit Russian state.
- [ ] Update existing English-only selectors in `e2e/tests/data-types.spec.ts` and `e2e/tests/chart-touch-readout.spec.ts` only where the newly explicit series names require it; preserve their Y-domain, zoom-fetch, touch, write, and localized-table assertions rather than weakening them.
- [ ] Mark completed
