# ADR-006: Activity Level Inferred From Steps, Not Stored as a Profile Field

## Status
Accepted

## Context and Problem Statement

The Mifflin-St Jeor calorie target (ADR-003) needs an activity multiplier. HealthVault already
ingests `steps` for most users. Should activity level be a `UserSettings` field the user picks
once and updates manually, or a value the backend infers from recent `steps` history each time
the target is computed?

## Decision Drivers

- A manually-picked activity level goes stale silently — a user's actual activity changes
  (new job, injury, season) far more often than they'd think to revisit a settings form, and a
  stale multiplier produces a wrong calorie target with no signal that it's wrong.
- `steps` is already ingested for most users, so a steps-based inference needs no new data
  collection, only a read of data that already exists.
- Not every user has step history (a new account, a device that doesn't sync steps, or a user who
  simply prefers to state their own level) — a pure-inference design with no override would make
  the target uncomputable for them.
- The inference has to be resilient to the two data wrinkles the production `steps` data actually
  has: multi-day sync gaps, and a partial (fragment) trailing day.

## Considered Options

- **Manual only** — a `UserSettings` activity-level field the user sets once. Simple, always
  available, but drifts from actual behavior with no way for the system to notice.
- **Inferred only, no override** — always compute from trailing `steps`. Self-correcting, but
  uncomputable for users with no step history, and offers no escape hatch for someone who knows
  their steps don't reflect their real activity (e.g. a cyclist whose steps undercount).
- **Inferred by default, with a manual override** — steps-inference is the primary signal;
  `activity_override` is an optional `UserSettings` enum that, when set, is used verbatim instead
  of computing from steps at all (not blended with it). Covers both gaps above with one field.

## Decision Outcome

Chosen: **inferred by default, with a manual override**. `GET /api/users/me/nutrition-target`
resolves the activity tier by checking `activity_override` first; if unset, it computes a trailing
28-day step average (excluding zero-record days anywhere in the window, and trimming a
sub-500-step trailing edge run only) and maps it onto a standard 5-tier multiplier table
(Sedentary/Lightly active/Moderately active/Very active/Extra active →
1.2/1.375/1.55/1.725/1.9). Fewer than 7 valid trailing days with no override set produces the
`insufficient_activity_data` 422 reason rather than a guessed default tier. Full mechanism detail
— the exclusion rules, the tier boundaries, and the override enum-to-tier mapping — is in this
change's `design.md`.

The inference is deliberately framed as an interim proxy (steps are a rough correlate of activity,
not a direct energy-expenditure measurement), not an attempt at adaptive TDEE; a data-driven TDEE
refinement is its own backlog item, not in scope here.

### Consequences

- No new write path for activity level in the common case: most users never touch
  `activity_override` and their calorie target adjusts automatically as their step pattern
  changes, without a stale manual setting to notice and fix.
- `steps` history — already ingested for most other reasons (dashboard cards, trend data) — is now
  also a load-bearing input to a second endpoint (`nutrition-target`), so a gap or quality
  regression in step ingestion now has a second downstream effect beyond the dashboard.
- Users with no step history, or whose steps don't reflect their real activity, are not left
  stuck: setting `activity_override` bypasses inference entirely for that user, with no blending
  between the two signals to reason about.
- The 28-day trailing window and its exclusion rules are computed fresh on every request rather
  than cached or stored, matching this change's "computed on read" decision for the rest of the
  Nutrition Target — a corrected or newly-synced step record is reflected on the very next call,
  with nothing to invalidate.
