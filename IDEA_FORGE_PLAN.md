# Implement: Goal weight, BMI bands, and weight-trend projection

## Overview
The spec for this change has been **approved by the repository owner**. This satisfies the
spec-first gate in CLAUDE.md, so implementation may now proceed.

Approval evidence, which you should verify yourself before starting rather than taking on
trust: in the body of https://github.com/ya-breeze/HealthVault/pull/28 the checkbox
"User review/approval of the spec (this PR)" is ticked. Check with:
`gh pr view 28 --repo ya-breeze/HealthVault --json body`.
The approval applies to commit 3879c27, which is the current head of this branch.

The approved spec is in `openspec/changes/goal-weight-bmi-bands-trend-projection/`.
`tasks.md` there is the authoritative checklist; each task below implements one of its
sections. Tick the checkboxes in that file as you complete them — it, not this plan, is the
record of implementation progress.

Out of scope, deliberately: do NOT archive the OpenSpec change and do NOT regenerate
`openspec/specs.projected/`. Those are the final steps of the PR and are handled separately
as two commits after implementation has been reviewed.

## Validation Commands
- `make lint`
- `make test`

### Task 1: Backend: `weight_goal` metric type registration
- [x] Implement section 1 ("Backend: `weight_goal` metric type registration") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 2: Backend: allowlisted write path
- [x] Implement section 2 ("Backend: allowlisted write path") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 3: Frontend: `weight_goal` registry + i18n
- [x] Implement section 3 ("Frontend: `weight_goal` registry + i18n") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 4: Frontend: reusable Add-record form
- [x] Implement section 4 ("Frontend: reusable Add-record form") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 5: Frontend: BMI bands + readout
- [x] Implement section 5 ("Frontend: BMI bands + readout") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 6: Frontend: goal line
- [x] Implement section 6 ("Frontend: goal line") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 7: Frontend: trend projection
- [x] Implement section 7 ("Frontend: trend projection") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 8: Testing: Vitest setup + unit coverage
- [x] Implement section 8 ("Testing: Vitest setup + unit coverage") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 9: E2E
- [x] Implement section 9 ("E2E") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [x] Mark completed

### Task 10: Docs
- [ ] Implement section 10 ("Docs") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [ ] Mark completed

### Task 11: Verification
- [ ] Implement section 11 ("Verification") of `openspec/changes/goal-weight-bmi-bands-trend-projection/tasks.md`, ticking that file's checkboxes as you go
- [ ] Mark completed
