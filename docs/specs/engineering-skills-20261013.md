# Configure engineering skills for Idea Store

## Why

The owner uses Idea Store as the tracker for every repository.
Engineering skills need explicit tracker operations and triage interpretations so they use
the existing lifecycle and preserve project ownership.

## How

Add the approved Agent skills block and repository-local tracker, triage and domain guidance.
Update CLAUDE.md when present; otherwise update or create AGENTS.md.
Preserve existing instructions except their absolute filesystem references. Remove the
external environment-rule pointer. Preserve the single-context layout.
Use repository-relative documentation pointers without requiring an external ecosystem
map. Change documentation only.

## Validation Commands

git diff --check
git diff main...HEAD -- AGENTS.md CLAUDE.md docs/agents/

### Task 1: Configure the repository
- [x] Add or update the Agent skills block without duplicating it.
- [x] Configure Idea Store, native dependencies and guarded writes.
- [x] Map the five standard triage roles to lifecycle conditions and actions.
- [x] Configure single-context domain docs without a required external map or checkout path.
- [x] Verify documentation, existing instruction preservation and Review Gate results.
- [x] Mark completed.
