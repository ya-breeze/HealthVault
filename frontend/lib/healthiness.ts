// Pure computation library for the Healthiness Label (the nutrition card's middle row) — see
// docs/specs/healthvault-nutrition-card-middle-row-th.md's "The heuristic" section for the full
// rationale behind each rule below, and ADR-004 for why this is a deterministic heuristic rather
// than an LLM judgment. Kept framework-free (no fetch, no React) so every step is unit-testable on
// its own, mirroring loggingGap.ts; the card component wires it up and owns all I/O.

import { isValidDay, type DayWindowData } from './loggingGap';

export const HEALTHINESS_WINDOW_DAYS = 7;
// ADR-007's 3-of-7 minimum-coverage floor, shared with the Logging Gap's own hard floor: below it
// there isn't enough logged history to say anything about eating patterns.
export const HEALTHINESS_MIN_ELIGIBLE_DAYS = 3;

export interface HealthinessWindow {
  /** Inclusive first day of the 7-day window, as a day offset (toDayOffset's units). */
  startDayOffset: number;
  /** Inclusive last day of the 7-day window — the same day the 28-day Logging Gap window ends on. */
  endDayOffset: number;
}

/**
 * The Healthiness Label's 7-day window: the last seven days of the 28-day Logging Gap window the
 * card already resolves (`resolveLoggingGapWindow`'s `windowLastDayOffset`), so this costs no
 * separate date arithmetic and no separate fetch — it's a slice of data already in hand. Today is
 * excluded because `windowLastDayOffset` already is (yesterday in the caller's Logged Day), for the
 * same reason Day Completeness excludes it: a day still in progress is not a light day.
 */
export function resolveHealthinessWindow(windowLastDayOffset: number): HealthinessWindow {
  return {
    startDayOffset: windowLastDayOffset - (HEALTHINESS_WINDOW_DAYS - 1),
    endDayOffset: windowLastDayOffset,
  };
}

// One day's inputs to the label: DayWindowData's state/unconfirmedMeals (so `isValidDay` applies
// unchanged) plus the four macro/sugar/sodium sums the daily-totals endpoint now carries.
export interface HealthinessDayData extends DayWindowData {
  proteinGrams: number;
  carbsGrams: number;
  fatGrams: number;
  sugarGrams: number;
  sodiumGrams: number;
}

export type HealthinessVerdict = 'ok' | 'off' | 'far';

export type HealthinessLabel = 'good' | 'fair' | 'needs_attention';

export type HealthinessReasonCode =
  | 'protein_low'
  | 'protein_high'
  | 'carbs_low'
  | 'carbs_high'
  | 'fat_low'
  | 'fat_high'
  | 'sugar_high'
  | 'sodium_high';

export interface HealthinessResult {
  label: HealthinessLabel;
  /** At most two, `far` before `off`, ties broken by signal order protein/sugar/sodium/fat/carbs. */
  reasons: HealthinessReasonCode[];
}

interface TwoSidedBands {
  farLow: number;
  offLow: number;
  offHigh: number;
  farHigh: number;
}

interface UpperOnlyBands {
  offLow: number;
  farLow: number;
}

/**
 * Threshold bands for the five Healthiness Label signals (spec's "The heuristic" §"Five signals,
 * each with three verdicts"). Boundaries are inclusive on the `ok` side — and, between `off` and
 * `far`, inclusive on the `off` side — so a value exactly on a boundary is never the worse verdict.
 * Exported constants, not configuration: nothing reads them from settings and nothing tunes them
 * per user (spec's "Deliberately not in scope").
 *
 * **Macro shares** (protein/carbs/fat, each as a share of macro energy `4P + 4C + 9F`) start from
 * the IOM's Acceptable Macronutrient Distribution Ranges (protein 10-35%, carbs 45-65%, fat
 * 20-35%) and are widened for one reason: this app's own Nutrition Target does not sit inside them.
 * `computeNutritionTarget` sets protein at 1.6 g/kg of Goal Weight and splits the rest evenly by
 * energy between carbs and fat, then applies a 0.8 g/kg fat floor that pushes fat up and carbs
 * down — an ordinary target lands near 22% protein / 39% carbs / 39% fat, and an aggressive goal
 * weight clamps carbs lower still. Bands that call the app's own advice unhealthy would be a bug,
 * not a heuristic, so the widened bands contain every target this app can produce. That makes the
 * label answer "is this pattern nutritionally sound", not "did you follow your split" — the top row
 * already answers the latter.
 *
 * **Sugar share** (`4S/M`) is against **total** sugars (USDA nutrient 2000, "Sugars, total
 * including NLEA"), not free sugars. WHO's 10%-of-energy limit is a free-sugar limit; applied to
 * total sugars it flags a diet of fruit and yoghurt. 15% is where total sugars stop being
 * explicable by whole foods, so it sits at the `ok` boundary. One-sided: there is no "too little
 * sugar" verdict.
 *
 * **Mean sodium**, g/day, is elemental sodium, not salt — `usda/fdc.go` converts FDC's milligrams
 * to grams on import. 2.3 g/day is the US dietary upper limit (5.75 g of salt); it sits at the `ok`
 * boundary rather than lower because salt added while cooking is invisible to photo recognition and
 * to most reference rows, so this signal systematically under-reports. A sodium flag is strong
 * evidence; the absence of one is not a clean bill — the hint copy says so. One-sided, like sugar.
 */
export const HEALTHINESS_THRESHOLDS: {
  proteinShare: TwoSidedBands;
  carbShare: TwoSidedBands;
  fatShare: TwoSidedBands;
  sugarShare: UpperOnlyBands;
  sodiumGramsPerDay: UpperOnlyBands;
} = {
  proteinShare: { farLow: 0.1, offLow: 0.15, offHigh: 0.4, farHigh: 0.45 },
  carbShare: { farLow: 0.15, offLow: 0.25, offHigh: 0.65, farHigh: 0.72 },
  fatShare: { farLow: 0.15, offLow: 0.2, offHigh: 0.4, farHigh: 0.48 },
  sugarShare: { offLow: 0.15, farLow: 0.22 },
  sodiumGramsPerDay: { offLow: 2.3, farLow: 3.5 },
};

function verdictTwoSided(value: number, bands: TwoSidedBands): HealthinessVerdict {
  if (value >= bands.offLow && value <= bands.offHigh) return 'ok';
  if (value >= bands.farLow && value < bands.offLow) return 'off';
  if (value > bands.offHigh && value <= bands.farHigh) return 'off';
  return 'far';
}

function verdictUpperOnly(value: number, bands: UpperOnlyBands): HealthinessVerdict {
  if (value <= bands.offLow) return 'ok';
  if (value <= bands.farLow) return 'off';
  return 'far';
}

// Fixed signal order for reason-list tie-breaking (spec: "protein, sugar, sodium, fat, carbs"),
// encoded as the order signals are evaluated in rather than as a separate sort step.
interface SignalEval {
  verdict: HealthinessVerdict;
  reason: HealthinessReasonCode | null;
}

function evalTwoSided(
  value: number,
  bands: TwoSidedBands,
  lowReason: HealthinessReasonCode,
  highReason: HealthinessReasonCode
): SignalEval {
  const verdict = verdictTwoSided(value, bands);
  if (verdict === 'ok') return { verdict, reason: null };
  return { verdict, reason: value < bands.offLow ? lowReason : highReason };
}

function evalUpperOnly(value: number, bands: UpperOnlyBands, highReason: HealthinessReasonCode): SignalEval {
  const verdict = verdictUpperOnly(value, bands);
  return { verdict, reason: verdict === 'ok' ? null : highReason };
}

/**
 * The Healthiness Label over `perDayData` (all Healthiness-relevant days the card has fetched,
 * keyed by day offset), computed from the 7-day slice `window` selects out of it. `null` means the
 * row renders nothing at all (spec's "Rendering, and when the row is silent"): fewer than 3 eligible
 * days, or zero pooled macro energy.
 *
 * Eligibility reuses `isValidDay` from loggingGap.ts unchanged, so the two rows can never disagree
 * about what counts as logged — a day whose photos failed vision is Complete with macro sums of
 * zero, and pooling a zero-protein day into the window would be a fabricated finding.
 *
 * Pooling, not day-averaging: every field is summed across eligible days first, and shares are
 * computed from the pooled totals. Averaging per-day shares would give a 400 kcal day the same
 * weight as a 2500 kcal one, which answers the wrong question about what this person has been
 * eating.
 *
 * The denominator is macro energy (`4P + 4C + 9F`), not logged `calories`: `FoodMeal.Calories` is
 * stored independently of the three macros and does not always equal their 4/4/9 sum — alcohol,
 * fibre, rounding and reference-database inconsistency all separate them. Shares have to sum to 1,
 * so the denominator has to be the thing they are shares of.
 */
export function computeHealthinessLabel(
  perDayData: Record<number, HealthinessDayData>,
  window: HealthinessWindow
): HealthinessResult | null {
  const days: HealthinessDayData[] = [];
  for (let offset = window.startDayOffset; offset <= window.endDayOffset; offset++) {
    const day = perDayData[offset];
    if (day) days.push(day);
  }
  const eligible = days.filter(isValidDay);
  if (eligible.length < HEALTHINESS_MIN_ELIGIBLE_DAYS) return null;

  const pooled = eligible.reduce(
    (acc, d) => ({
      protein: acc.protein + d.proteinGrams,
      carbs: acc.carbs + d.carbsGrams,
      fat: acc.fat + d.fatGrams,
      sugar: acc.sugar + d.sugarGrams,
      sodium: acc.sodium + d.sodiumGrams,
    }),
    { protein: 0, carbs: 0, fat: 0, sugar: 0, sodium: 0 }
  );

  const macroEnergy = 4 * pooled.protein + 4 * pooled.carbs + 9 * pooled.fat;
  if (!(macroEnergy > 0)) return null;

  const proteinShare = (4 * pooled.protein) / macroEnergy;
  const carbShare = (4 * pooled.carbs) / macroEnergy;
  const fatShare = (9 * pooled.fat) / macroEnergy;
  const sugarShare = (4 * pooled.sugar) / macroEnergy;
  const sodiumGramsPerDay = pooled.sodium / eligible.length;

  // Order is the fixed tie-break order the spec specifies: protein, sugar, sodium, fat, carbs.
  const evals: SignalEval[] = [
    evalTwoSided(proteinShare, HEALTHINESS_THRESHOLDS.proteinShare, 'protein_low', 'protein_high'),
    evalUpperOnly(sugarShare, HEALTHINESS_THRESHOLDS.sugarShare, 'sugar_high'),
    evalUpperOnly(sodiumGramsPerDay, HEALTHINESS_THRESHOLDS.sodiumGramsPerDay, 'sodium_high'),
    evalTwoSided(fatShare, HEALTHINESS_THRESHOLDS.fatShare, 'fat_low', 'fat_high'),
    evalTwoSided(carbShare, HEALTHINESS_THRESHOLDS.carbShare, 'carbs_low', 'carbs_high'),
  ];

  const farCount = evals.filter(e => e.verdict === 'far').length;
  const offCount = evals.filter(e => e.verdict === 'off').length;

  let label: HealthinessLabel;
  if (farCount > 0 || offCount >= 3) label = 'needs_attention';
  else if (offCount >= 1) label = 'fair';
  else label = 'good';

  // Three macro shares are not independent — they sum to 1, so one being high forces another
  // down — which is why the combination rule above never lets a single `off` reach past `fair`.
  const farReasons = evals.filter(e => e.verdict === 'far' && e.reason).map(e => e.reason as HealthinessReasonCode);
  const offReasons = evals.filter(e => e.verdict === 'off' && e.reason).map(e => e.reason as HealthinessReasonCode);
  const reasons = [...farReasons, ...offReasons].slice(0, 2);

  return { label, reasons };
}
