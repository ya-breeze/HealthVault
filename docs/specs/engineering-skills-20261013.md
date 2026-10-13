# Configure engineering skills for Idea Store

## Why

The owner uses Idea Store for every repository and Home for the ecosystem dependency map.
Engineering skills need explicit tracker operations and triage interpretations so they use
the existing lifecycle and preserve project ownership.

## How

Add the approved Agent skills block and repository-local tracker, triage and domain guidance.
Update CLAUDE.md when present; otherwise update or create AGENTS.md.
Keep existing instructions outside the block unchanged. Preserve the single-context layout.
Refer cross-repository exploration to Home. Change documentation only.

## Validation Commands

git diff --check
python3 /tmp/engineering_skills_validate_01a13644.py

### Task 1: Configure the repository
- [ ] Add or update the Agent skills block without duplicating it.
- [ ] Configure Idea Store, native dependencies and guarded writes.
- [ ] Map the five standard triage roles to lifecycle conditions and actions.
- [ ] Configure single-context domain docs and the existing Home map pointer.
- [ ] Verify documentation, existing instruction preservation and Review Gate results.
- [ ] Mark completed.
