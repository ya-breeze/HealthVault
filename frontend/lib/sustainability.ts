// Pure computation module for the nutrition card's middle-row sustainability warning — see
// docs/specs/healthvault-nutrition-card-middle-row-he.md's "How" section for the full rationale.
// Kept framework-free (no fetch, no React), matching how loggingGap.ts is structured, so both
// checks are unit-testable on their own.

import type { LoggingGapResult } from './loggingGap';

export const MAX_SUSTAINABLE_LOSS_PCT_PER_WEEK = 1.0;
export const BMR_SHORTFALL_MARGIN = 0.05;

export type SustainabilityWarning =
  | { kind: 'loss_too_fast'; percentPerWeek: number }
  | { kind: 'intake_below_bmr'; meanLoggedIntake: number; bmr: number };

export interface SustainabilityInputs {
  gap: LoggingGapResult;
  meanLoggedIntake: number | null;
  bmr: number | null;
  trend: { slope: number; se: number | null; weightAtWindowEnd: number } | null;
}

/**
 * The two sustainability checks (design.md's "Two checks, one pure module"). Order matters: when
 * both fire, `loss_too_fast` is reported first, because it rests on measured weight while the
 * intake check rests on self-reported food that only a corroborating gate can vouch for.
 *
 * `intake_below_bmr` alone is gated on `gap.kind === 'on_track'`. Logged intake is only
 * trustworthy when the weight trend corroborates it — if the logging gap line reports a large
 * unlogged-food gap, "you are eating below your BMR" is false; the user is merely *logging* below
 * BMR. `loss_too_fast` needs no such gate, because weight is measured rather than self-reported.
 */
export function evaluateSustainability(inputs: SustainabilityInputs): SustainabilityWarning[] {
  const warnings: SustainabilityWarning[] = [];

  const { trend } = inputs;
  if (
    trend !== null &&
    trend.se !== null &&
    Number.isFinite(trend.weightAtWindowEnd) &&
    trend.weightAtWindowEnd > 0 &&
    trend.slope < 0
  ) {
    const conservativePercentPerWeek = (-(trend.slope + trend.se) * 7) / trend.weightAtWindowEnd * 100;
    if (conservativePercentPerWeek > MAX_SUSTAINABLE_LOSS_PCT_PER_WEEK) {
      const percentPerWeek = (-trend.slope * 7) / trend.weightAtWindowEnd * 100;
      warnings.push({ kind: 'loss_too_fast', percentPerWeek });
    }
  }

  const { gap, meanLoggedIntake, bmr } = inputs;
  if (
    gap.kind === 'on_track' &&
    meanLoggedIntake !== null &&
    Number.isFinite(meanLoggedIntake) &&
    bmr !== null &&
    Number.isFinite(bmr) &&
    bmr > 0 &&
    meanLoggedIntake < bmr * (1 - BMR_SHORTFALL_MARGIN)
  ) {
    warnings.push({ kind: 'intake_below_bmr', meanLoggedIntake, bmr });
  }

  return warnings;
}
