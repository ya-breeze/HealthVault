## 1. Y-axis domain helper

- [x] 1.1 Add a small domain-computation helper in `DataTypeClient.tsx` (or a co-located util)
      implementing `range = max - min; pad = range > 0 ? range * 0.1 : Math.max(Math.abs(max), 1) * 0.02; [min - pad, max + pad]`
- [x] 1.2 Wire the helper into the Day-zoom `<YAxis domain=...>` for point-in-time types, fed by
      the raw values already used for that chart's `<Line>`(s) — for `blood_pressure`, combine
      both the systolic and diastolic series before computing min/max, not just one, so neither
      line gets clipped
- [x] 1.3 Wire the helper into the Week/Month/Year `<YAxis domain=...>` for point-in-time types
      (both the single-series `ComposedChart` and the `blood_pressure` dual-series chart), fed by
      the `min`/`max` band values so the shaded band is never clipped
- [x] 1.4 Confirm cumulative-type `<YAxis>` (bar charts) are untouched — no `domain` prop added
      there

  **Implementation note (found during visual QA, not anticipated by this task list):** the
  existing min/max shaded band used Recharts' stacked-Area trick (transparent bottom Area +
  visible band Area, both `stackId`'d). That trick's baseline is the stack's absolute value
  origin (0), not the Y-axis domain — so it silently pulled the axis back toward zero regardless
  of the `domain` prop, defeating 1.3 for every point-in-time type. Fixed by switching to
  Recharts 3.9's ranged-Area support: a `dataKey` resolving to a `[min, max]` tuple renders
  directly between the two values with no baseline/stacking involved. `bucketBandData`/
  `bucketBPData` now carry `range`/`sysRange`/`diaRange` tuples instead of separate `min`+`band`
  fields, and each chart uses one `<Area dataKey="...Range">` instead of two stacked ones.

## 2. Weight trend line

- [x] 2.1 Add a `dataType === 'weight' && (zoom === 'week' || zoom === 'year')` branch that widens
      the bucketed fetch (14 trailing days for Week, ~2 trailing years for Year) instead of the
      zoom's normal range. `chartRows` itself will now hold the widened set of buckets for these
      cases — do not let that leak into the existing avg-line/min-max band or X-axis labels (see
      2.3); `records`/`from`/`to` (raw fetch, stats row) stay on the existing range regardless

  **Correction (found during `/code-review`, see 2.3's note):** the original task list and the
  approved spec/design only widened Week, on the stated assumption that Year's ~12-13 monthly
  buckets "already exceed the 14-period minimum." That's arithmetically false (12-13 < 14-16), so
  the Year-zoom trend line rendered from the unwidened implementation would never have converged —
  it would show as a visible ramp across most/all of the Year view. Fixed by widening Year too (to
  ~2 trailing years) and correcting spec.md/design.md's "Weight trend line" sections to match.

- [x] 2.2 Add an EMA helper (`alpha = 0.25`, seeded from the first bucket in the fetched — possibly
      widened — range) that maps a bucketed series to a trend series
- [x] 2.3 For `weight` at Week/Month/Year zoom: compute the trend series from the full (possibly
      widened) bucketed series, then filter *both* the trend series and `bucketBandData`
      (avg/min/max/labels) down to just the zoom's own visible range before rendering, so the
      existing avg-line/band/X-axis labels keep showing exactly the same window they did before
      this change.

  **Correction (found during `/code-review`):** the original implementation filtered by slicing
  the last N rows by array position (`chartRows.slice(-7)`), not by calendar date. The backend's
  bucket query (`storage_impl.go`'s `QueryAggregate`) is a plain SQL `GROUP BY` with no zero-fill
  for missing buckets, so a sparse logger (e.g. no weight entries in the last week) could get back
  fewer rows than the widened range spans — a positional slice would then pull genuinely-old
  buckets into what's labeled and rendered as the current Week/Year view. Fixed by filtering each
  row against the visible range's real start date (`bucket_start >= from`) via a shared boolean
  mask applied to both `chartRows` and the trend series, so they stay index-aligned regardless of
  how sparse the underlying data is.
- [x] 2.4 Add a `<Line>` series for the trend, distinguishable from the existing avg line (e.g. a
      different stroke color/dash), to the `weight` Week/Month/Year `ComposedChart` only
- [x] 2.5 Confirm no trend line renders at Day zoom, and none renders for any point-in-time type
      other than `weight`

## 3. Verification

- [x] 3.1 `make lint` in the frontend — no frontend lint target exists (`make lint` is
      backend-only `go vet`). Ran `npx tsc --noEmit` (clean) and `npm run build` (static export
      succeeds, all pages generate) instead.
- [x] 3.2 Manually check the deployed WIP stack: `weight`, `heart_rate`, and `blood_pressure`
      charts at each zoom level — axis no longer defaults toward 0, trend line appears only for
      `weight` at Week/Month/Year, band is never clipped by the axis. Deployed to `hcw-wip`;
      screenshots and DOM tick-value inspection confirmed the fix (see 1.4 note) after the first
      pass rendered incorrectly.
- [x] 3.3 Run/extend `e2e/tests/data-types.spec.ts` to cover the new Y-axis behavior and the
      weight trend line. Added a `Point-in-time Y-axis domain and weight trend line` describe
      block: non-zero-anchored axis for `weight`/`heart_rate`/`blood_pressure` at Year zoom,
      zero-anchored axis retained for cumulative `steps`, trend line present for `weight` at
      Week/Month/Year and absent at Day, trend line absent for `heart_rate`. Extended with 3
      network-level tests (added during the `/code-review` fixups above) asserting the actual
      outgoing bucketed-fetch date range: weight/Week widens to >=14 days, weight/Year widens to
      >=~2 years, heart_rate/Week is not widened. Full suite (97 passed, 1 pre-existing unrelated
      skip) passes against `hcw-wip`.
- [ ] 3.4 Add a follow-up unit test for the EMA and domain-padding helpers if they're extracted
      into standalone functions — deferred; `computeYDomain`/`emaSeries` were extracted into
      `dataTypeMeta.ts` but no unit test framework is wired up for the frontend in this repo yet.
