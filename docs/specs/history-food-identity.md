# Reject unrelated foods in recent meal nutrition reuse

Idea: https://ideaforge.ikoro.in/idea/1246

## Why

A confirmed apple pie and cottage cheese pancakes meal inherited cucumber and lettuce profiles after the model proposed valid indices for unrelated foods. The server accepted the mapping and retained the pastry names. The resulting 54 kcal total replaced independent estimates worth 597.5 kcal at the owner's current portions. Request logs identify application and confirmation, but do not explain the model's choice.

## How

Require equal normalized canonical names when both exist; otherwise require equal nonempty display names. Normalize case and whitespace only. Reject conflicting known preparation or state. Reject the entire proposal when any mapping fails. Repeat the identity check against current source and target items when applying a saved proposal, including proposals generated before this fix. Preserve independent estimates and portions on rejection. Return a distinct 422 for unsafe saved mappings; explain the mismatch in the UI, disable Apply and retain dismissal instead of offering a stale-snapshot refresh. Use the canonical name when a target display name is empty; show a neutral unnamed-food label when both names are absent. Test the rejection diagnostic fields and absence of food contents. Keep whole-meal repeat unchanged. Prefer missing a possible synonym match to accepting unrelated foods. Emit bounded structured rejection diagnostics with row IDs and reason, without food names or photos. Show source and target names explicitly in the suggestion UI.

Recover the one affected production meal from its already persisted estimates in a guarded transaction with a protected pre-change row snapshot. Keep current portions, names, confirmation and original recognition evidence. Verify through the connected user's read API. This record repair does not authorize a production application deployment.

WIP runtime revision `4c2308fe3481c0f31b11c0a5beb085f1f218d4ae` served the new UI and passed five history E2E scenarios without retries. The two opt-in live-model scenarios were skipped; handler regression tests exercise rejected model mappings. The final commit changes only the E2E alert selector and this completion record, with no application source changes.

## Validation Commands

- `make test-backend`
- `make test-frontend`
- `make lint`
- `make test-e2e E2E_ARGS='tests/meal-history-reuse.spec.ts --retries=0'`

### Task 1: Protect history matching and explain the mapping

- [x] Add failing regression coverage for unrelated foods, preparation conflicts, normalization, valid reuse and preexisting unsafe proposals.
- [x] Validate identity before suggesting and before applying history nutrition.
- [x] Log rejected mappings without meal contents and show both mapped names.
- [x] Run static checks, tests, the native Review Gate and one Claude peer review.
- [x] Validate the reviewed branch against WIP with the existing history E2E suite.
- [x] Mark completed.

### Task 2: Recover the affected meal

- [x] Preview exact row changes and verify unchanged source estimates and portions.
- [x] Save a protected original row snapshot and repair the affected meal atomically.
- [x] Verify the meal and daily totals through read-only owner APIs.
- [x] Mark completed.
