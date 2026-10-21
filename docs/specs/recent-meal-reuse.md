# Reuse recent Food Meals without losing corrected nutrition

Idea: https://ideaforge.ikoro.in/idea/1171

## Why

An owner often eats the same prepared food on several consecutive days. A correction to a
photographed Food Item currently stays in that single Food Meal. New recognition does not see
ordinary meal history. Saving every component as a Custom Food is unnecessary work. The owner
approved suggestions from recent meals and a direct repeat action, with new portion weights and
explicit review before saving intake.

## How

Use up to 20 confirmed caller-owned Food Meals and 100 usable Food Items from seven local dates,
including today, ending at the current instant. Exclude future, deleted, unconfirmed and unusable
records. Provide grouped names, preparation, cooking context and current per-100g profiles to the
photo model as bounded, untrusted evidence. Never send other users' history. A separate optional
recognition capability keeps existing vision implementations compatible. The model may propose
a source meal and an unambiguous item mapping; the server validates that mapping against its own
candidate snapshot. Invalid or absent suggestions leave ordinary recognition working. Preserve
this capability through photo clarification rounds. Explain the historical context sent to the
external provider in the existing English and Russian photo-upload disclosure.

Expose a source date and composition on review. The owner explicitly accepts or dismisses the
suggestion. Acceptance uses immutable server-owned profiles derived from current source totals
and positive source weights. Keep target weights and unmatched target items. Check target and
source revisions before changing anything. Retain provenance and original uncertainty. Later
weight edits scale every nutrient from the copied per-100g snapshot. Later source edits or deletion
cannot change the accepted copy. Direct manual correction or reference rebinding clears the copied
profile. Do not promote estimated nutrients to measured data.

Add a repeat action to confirmed meals in history. Repeating a recent meal creates an independent,
photo-free pending_review meal with fresh IDs and current logged time, copied cooking context and
frozen profiles. Stable request IDs prevent duplicates. The owner edits portions and confirms with
the existing flow. Repeating or accepting history never auto-confirms intake and never alters the
original. Offer Repeat only when the server verifies every source item has a usable profile.
Empty history, stale sources and unavailable suggestions remain clear recoverable states. A stale
source can be dismissed or used as a reason to reanalyze; reloading target portions cannot make
an obsolete source snapshot fresh. Existing nutrition-chat and personal-MCP evidence retains the
original source and confidence of copied nutrition.

Keep one implementation and one PR. Do not change nutrition targets, add model training, create
Custom Foods implicitly, expose private history through generic family reads, or deploy production.
The existing synthetic WIP stack is the validation target. Keep production read-only.

## Validation Commands

- `make test-backend`
- `make test-frontend`
- `make lint`
- `cd frontend && npm run build`
- `make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS='--retries=0'`
- `HCW_LIVE_AI_E2E=1 make test-e2e BASE_URL=http://192.168.1.54:8892 E2E_ARGS='--retries=0 meal-history-reuse.spec.ts --grep "deployed photo recognition"'`
- Codex native Review Gate and one best-effort Claude system code-review peer run.

### Task 1: Preserve copied nutrition and repeat a recent meal

- [x] Add immutable per-100g snapshots and source provenance without changing existing sources.
- [x] Scale copied profiles on subsequent weight edits and clear provenance on explicit replacement.
- [x] Add owner-scoped recent meal selection and idempotent repeat into pending_review.
- [x] Add the repeat action, review navigation and English/Russian copy.
- [x] Test isolation, local date boundaries, source mutation, replay and editable repeat behavior.
- [x] Mark completed.

### Task 2: Offer and explicitly apply recent history during photo review

- [x] Supply bounded recent meal context to photo recognition without changing ordinary clients.
- [x] Validate and persist a server-owned source suggestion and complete source-to-target mapping.
- [x] Add explicit accept/dismiss actions with freshness checks and preserve fresh target weights.
- [x] Show source date, composition and original uncertainty in review.
- [x] Test model payload, invalid matches, stale edits and accepted profile scaling.
- [x] Mark completed.

### Task 3: Integrate and validate the complete user flow

- [x] Document snapshot and provenance semantics with an ADR and domain terms.
- [x] Add meaningful Playwright coverage for repeat, portion edits, acceptance and dismissal.
- [x] Run static checks and the Review Gate; address valid findings.
- [x] Push the reviewed feature branch, deploy to owned WIP and run deployed E2E checks.
- [x] Publish a draft PR and reconcile the Idea with exact validation evidence.
- [x] Mark completed.
