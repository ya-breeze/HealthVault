## 1. Y-axis domain helper

- [ ] 1.1 Add a small domain-computation helper in `DataTypeClient.tsx` (or a co-located util)
      implementing `range = max - min; pad = range > 0 ? range * 0.1 : Math.max(Math.abs(max), 1) * 0.02; [min - pad, max + pad]`
- [ ] 1.2 Wire the helper into the Day-zoom `<YAxis domain=...>` for point-in-time types, fed by
      the raw values already used for that chart's `<Line>`
- [ ] 1.3 Wire the helper into the Week/Month/Year `<YAxis domain=...>` for point-in-time types
      (both the single-series `ComposedChart` and the `blood_pressure` dual-series chart), fed by
      the `min`/`max` band values so the shaded band is never clipped
- [ ] 1.4 Confirm cumulative-type `<YAxis>` (bar charts) are untouched — no `domain` prop added
      there

## 2. Weight trend line

- [ ] 2.1 Add a `type === 'weight' && zoom === 'week'` branch that requests bucketed
      (`?bucket=day`) data for a 14-day range instead of the zoom's normal 7-day range, while
      leaving `records`/`from`/`to` (raw fetch, chart labels, stats row) on the existing 7-day
      range
- [ ] 2.2 Add an EMA helper (`alpha = 0.25`, seeded from the first bucket in the fetched — possibly
      widened — range) that maps a bucketed series to a trend series
- [ ] 2.3 For `weight` at Week/Month/Year zoom, compute the trend series from `bucketBandData`'s
      `avg` values, then slice it down to just the zoom's own visible range before rendering
- [ ] 2.4 Add a `<Line>` series for the trend, distinguishable from the existing avg line (e.g. a
      different stroke color/dash), to the `weight` Week/Month/Year `ComposedChart` only
- [ ] 2.5 Confirm no trend line renders at Day zoom, and none renders for any point-in-time type
      other than `weight`

## 3. Verification

- [ ] 3.1 `make lint` in the frontend
- [ ] 3.2 Manually check the deployed WIP stack: `weight`, `heart_rate`, and `blood_pressure`
      charts at each zoom level — axis no longer defaults toward 0, trend line appears only for
      `weight` at Week/Month/Year, band is never clipped by the axis
- [ ] 3.3 Run/extend `e2e/tests/data-types.spec.ts` to cover the new Y-axis behavior and the
      weight trend line
- [ ] 3.4 Add a follow-up unit test for the EMA and domain-padding helpers if they're extracted
      into standalone functions
