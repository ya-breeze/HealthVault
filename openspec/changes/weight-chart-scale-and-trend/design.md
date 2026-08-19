## Context

`frontend/app/data/[type]/DataTypeClient.tsx` renders every point-in-time metric (weight, heart
rate, blood pressure, etc.) through one shared Recharts setup. None of its four `<YAxis>`
instances set a `domain`, so the axis defaults toward including zero — for a metric like `weight`
whose readings cluster in a narrow band far from zero, this hides real changes. The chart-zoom
behavior (Day = raw line, Week/Month/Year = per-bucket avg line with min/max band) is already
specified in `openspec/specs/chart-zoom-aggregation/spec.md`; this change extends that same file
rather than introducing a new capability.

This is small enough in mechanism (two localized changes to one file) that most of this document
is about pinning down the two numeric algorithms precisely, so implementation doesn't have to
improvise constants mid-coding.

## Goals / Non-Goals

**Goals:**
- Y-axis domain reflects the visible data's own range for every point-in-time metric, at every
  zoom level.
- A weight-only trend line at Week/Month/Year zoom that reads as a genuine smoothed trend, not
  just a re-plot of the existing bucket-average line.
- Zero backend/API/schema changes.

**Non-Goals:**
- Goal weight, BMI bands, trend projection, trend line for Day zoom or for metrics other than
  weight — deferred, see `todo.md`.
- Pixel-perfect replication of any specific third-party app's chart.
- User-configurable smoothing (trend algorithm/constants are fixed, not exposed as settings).

## Decisions

### Y-axis domain: data-relative with padding and a floor, not Recharts' bare `auto`

Recharts' own `domain={['auto', 'auto']}` was considered and rejected as insufficient on its own:
it removes the zero-anchoring but can still round to "nice" tick values that, on a tightly
clustered dataset, produce padding uneven enough to look arbitrary, and it has no floor for a
near-constant data window (a day of identical readings would still zoom to a hairline-thin band).

Instead, compute the domain explicitly from the series feeding each chart (the same array already
driving the `<Line>`/`<Area>` — raw values at Day zoom, `avg`/`min`/`max` at Week/Month/Year):

```
range = dataMax - dataMin
pad = range > 0 ? range * 0.1 : Math.max(Math.abs(dataMax), 1) * 0.02
domain = [dataMin - pad, dataMax + pad]
```

- 10% padding keeps the line off the chart edges without visually understating the range.
- The `range > 0` fallback handles a fully flat series (e.g. one reading, or several identical
  ones): pad by 2% of the value's own magnitude instead of 10% of a zero range, so the chart still
  shows a sensible band instead of collapsing to a single horizontal line. The `Math.max(..., 1)`
  guards the degenerate case of a value at or near 0 (not expected for any current point-in-time
  metric, but keeps the formula from dividing into a zero-width domain).
- Computed once per chart render from whatever data is currently loaded — it does not need to
  react to anything beyond the existing `chartRows`/`records` state.

At Week/Month/Year zoom, the domain is computed from the band's `min`/`max` (not just `avg`), so
the shaded min-max band itself is never clipped.

### Trend line: fixed-alpha EMA over bucketed averages, frontend-only

An exponential moving average was chosen over a simple moving average (SMA) because it weights
recent readings more heavily without a hard cliff at the window edge, which is what makes a trend
line in apps like Libra feel like it's tracking "now" rather than lagging a full window behind.
Computing it frontend-only (rather than adding a backend endpoint/field) keeps this change
confined to one file and matches the "no backend changes" scope decision — revisit only if the
frontend-only version turns out visually wrong once built.

```
alpha = 0.25   // ≈ a 7-period EMA (alpha = 2 / (N + 1), N = 7)
trend[0] = bucketAvg[0]
trend[i] = trend[i-1] + alpha * (bucketAvg[i] - trend[i-1])
```

Applied uniformly to whichever bucket series is active for the zoom level (`avg` per day for
Week/Month, `avg` per month for Year) — the same `alpha` at every granularity, rather than a
different constant per zoom level, to keep the algorithm simple and its behavior predictable
across zoom changes.

Only the trend values for the zoom's own visible range are plotted; the extra lookback days exist
purely to seed `trend[0]` closer to a realistic value before the visible window starts.

### Lookback: widen Week and Year zoom's bucketed fetch

An EMA with alpha=0.25 is within about 2% of its converged value after roughly 14-16 periods.
Week's normal 7-day range and Year's ~12-13 monthly buckets both fall short of that — seeding
either from its own unwidened range would show a visible ramp across most of the visible window.
Month's 30-day range does not have this problem (30 daily buckets already exceeds 14-16), so it's
left untouched.

Widened for `weight` only, specifically:
- Week: the bucketed (`?bucket=day`) fetch widens from 7 to 14 trailing days.
- Year: the bucketed (`?bucket=month`) fetch widens from ~1 to ~2 trailing years, giving ~24-25
  monthly buckets — comfortably past the 14-16 period minimum without needing a more precise
  (and more fragile) "exactly enough" calculation.

In both cases the chart itself still only renders the trend (and the existing avg/band series) for
the zoom's normal visible window. The extra lookback rows are filtered out by comparing each row's
own `bucket_start` against the visible range's real start date — *not* by slicing the last N rows
by array position. The backend's bucket query is a plain SQL `GROUP BY` with no zero-fill for
missing days (`storage_impl.go`'s `QueryAggregate`), so a sparse logger can get back fewer rows
than the widened range spans; slicing by position could then pull genuinely-old buckets into what
gets labeled and rendered as the current view. Filtering by date holds regardless of how sparse the
data is, and also keeps the trend series aligned with the filtered rows by construction (both are
derived from the same boolean mask over the original array).

This only widens the *bucketed* fetch (`chartRows`), which weight already requests for
Week/Month/Year; the raw-record fetch (`records`, used for Day zoom and the data table) is
unchanged.

## Risks / Trade-offs

- **[Risk]** The 10%/2% padding constants and alpha=0.25 are reasoned defaults, not values tuned
  against real HealthVault data. → **Mitigation**: cheap to adjust after visual review against the
  deployed WIP stack; not a correctness property that needs to be "right" on the first pass.
- **[Risk]** Widening the Week/Year-zoom fetch for `weight` only (not other point-in-time types)
  adds a small type-specific branch to otherwise-uniform fetch logic. → **Mitigation**: scope it
  narrowly (`dataType === 'weight' && (zoom === 'week' || zoom === 'year')`) with a comment
  pointing at this change, rather than generalizing a "lookback" concept no other metric needs yet.
- **[Trade-off]** A fixed alpha across all three zoom levels is simpler than granularity-aware
  smoothing, but means the Year-zoom trend (over monthly buckets) smooths at the same *relative*
  rate as Week/Month (over daily buckets) rather than an equivalent *time* constant. Accepted for
  this change; revisit only if the Year-zoom trend looks wrong once built.

## Migration Plan

None — frontend-only rendering/computation change, no data migration, no feature flag. Ships and
reverts like any other frontend change (redeploy the previous commit).
