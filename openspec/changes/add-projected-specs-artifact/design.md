## Context

This repo (like all projects in this environment) requires proposing an OpenSpec change, getting the spec-only PR approved, then implementing, then archiving the change (`openspec archive --yes`) in the *same* PR before merge. Delta specs under `openspec/changes/<id>/specs/<capability>/spec.md` only record `## ADDED/MODIFIED/REMOVED Requirements`; a `MODIFIED` entry has no diff against the requirement it replaces, and the delta file is new in git, so a reviewer looking at the PR diff sees new prose with no way to tell what changes relative to the current merged spec. `openspec/specs/` (the merged, canonical view) isn't updated until the archive step, which happens right before merge — after the review that matters most.

No CI pipeline exists in this repo, or in any sibling repo in this environment, today. Deployment goes through Portainer git-deploy pulling `main`, not GitHub Actions. There is also no pre-commit hook framework (no husky/lefthook/pre-commit) anywhere in this environment. The only existing task runner is a root-level `Makefile` (currently backend-only targets: `build`, `test`, `lint`, `run-backend`).

## Goals / Non-Goals

**Goals:**
- Let a reviewer read the post-change spec wording directly in the PR diff, for changes touched by that PR.
- Keep delta specs as the sole source of truth; the projection is purely derived, regenerable, and never hand-edited.
- Make it structurally hard to forget: a CI check fails the PR if the committed projection doesn't match what regenerating produces.
- Guarantee `openspec/specs.projected/` never lands on `main` as persistent content — it is a review-time artifact scoped to the PR's lifetime, not a permanent duplicate of `openspec/specs/`.

**Non-Goals:**
- Not changing when deltas actually merge into `openspec/specs/` (still only at `openspec archive`).
- Not building a general diff-viewer or web UI — GitHub's native file diff on a committed file is the delivery mechanism.
- Not handling cross-change requirement conflicts (two open changes editing the same requirement differently) — if `openspec archive` itself errors on that, the script surfaces the error and stops; resolving such conflicts is unrelated to this change.
- Not retrofitting this into any other repo in this environment. HealthVault is the first; rollout elsewhere is a separate decision.

## Decisions

**1. Full rebuild each run, not incremental sync.**
The script always does `rm -rf openspec/specs.projected && cp -r <worktree>/openspec/specs openspec/specs.projected` (when there are touched changes) rather than an incremental rsync-with-delete. A full rebuild is trivially idempotent (same inputs always produce byte-identical output) and needs no extra dependency (`rsync` isn't guaranteed present in the CI image or contributors' machines) versus `cp`/`rm`, which are universal. The cost — a few dozen markdown files, sub-second — is negligible at this repo's scale (18 capabilities today).

**2. A disposable detached git worktree, not `git stash` or a second clone.**
`git worktree add --detach <tmp> HEAD` gives an isolated checkout of the exact committed tree without disturbing the primary working tree — critical since the script must be safe to run with uncommitted changes present (confirmed necessary: this repo currently has substantial uncommitted work on an unrelated feature branch at time of writing). A second full clone would be network-bound and slower; `git stash` would require unstashing correctly even on failure and would still mutate the primary tree's index. A worktree shares the object store, is local-only, and is safely disposable via `git worktree remove`.

**3. Rebuilding is based on committed state at HEAD, not the dirty working tree.**
Uncommitted edits to a delta spec are not reflected in the projection until committed. This is a deliberate simplification: reflecting uncommitted state would mean copying dirty files into the worktree (or running the CLI directly against the primary tree, which conflicts with keeping the primary tree undisturbed and unblocked for the multi-change / zero-change cases). Documented as "commit your delta edits, then regenerate."

**4. The `openspec/specs.projected/` lifecycle is derived purely from "which change ids are currently touched," with no separate "is this the final archive commit" flag.**
When a PR is mid-review, its delta specs live under `openspec/changes/<id>/specs/**`, so the diff-based detection finds them and the script populates the projection. When the same PR later runs the real `openspec archive <id> --yes` (per this repo's existing workflow, before merge), the delta moves to `openspec/changes/archive/<date>-<id>/`, which no longer matches the `openspec/changes/*/specs/**` glob (the glob's single `*` can't match the two path segments `archive/<date>-<id>`). Re-running the generator at that point finds zero touched ids and wipes the projection back to empty — the exact state needed before merge. One code path handles both directions; no special-casing archive vs. non-archive runs.

**5. CI enforces via regenerate-and-diff, not a bespoke "is this stale" heuristic.**
The workflow runs the identical script contributors run locally, then `git diff --exit-code` against what's committed. This means the CI gate is provably consistent with the local command named in its own error message (`make projected-specs`) — there's no separate logic in CI that could drift from the script's behavior.

**6. Bash, not Node, for the script.**
The repo's only root-level tooling is the Makefile invoking shell/Go commands; there is no root `package.json` (the frontend's is scoped to `frontend/`). A bash script matches the existing pattern and avoids adding a Node dependency to run a script whose job is mostly `git` and file operations. The `openspec` CLI itself is a separate, already-required binary.

## Risks / Trade-offs

- **[Risk]** A contributor edits a delta spec, forgets to regenerate, and pushes → CI's drift check catches this on any PR touching `openspec/**`, failing with the exact fix command. Mitigation: documented in the new `openspec/specs.projected/README.md` and this repo's CLAUDE.md workflow notes.
- **[Risk]** A contributor archives the change (finishing step) but forgets to re-run the generator afterward, leaving a stale non-empty `openspec/specs.projected/` in the final commit → the same CI drift check catches this too: regenerating post-archive independently produces "zero touched ids → empty," which will differ from whatever non-empty content is still committed. No separate mechanism needed.
- **[Risk]** `openspec archive <id> --yes` fails inside the worktree (e.g., a validation error) → the script aborts before touching `openspec/specs.projected/` in the primary tree, leaving it exactly as it was; the error from `openspec archive` is surfaced directly so the contributor can fix the delta and rerun.
- **[Risk]** Two changes touched by the same PR modify the same requirement in conflicting ways → `openspec archive` itself is expected to error in the worktree; this surfaces as a script failure, not silent wrong output. Out of scope to auto-resolve.
- **[Trade-off]** Full rebuild copies the entire `openspec/specs/` output (all capabilities merged by the worktree's `openspec archive` runs, not just the touched ones) into `openspec/specs.projected/` each run. Untouched capabilities are byte-identical to what's already committed (since only touched ids get archived in the worktree), so they produce no diff — but the script does touch every file's mtime and re-copy every byte on each run. Acceptable at current repo scale; would need reconsideration if this repo's spec set grew by an order of magnitude.
- **[Trade-off]** GitHub's `linguist-generated=true` collapses the projection in the PR's "Files changed" view by default (reviewers must click to expand), rather than hiding the diff entirely (`diff=false`, which was deliberately not used) — a small extra click, in exchange for the diff still being fully visible and searchable when needed.

## Migration Plan

No data migration. Rollout is additive:
1. Land the script, Makefile target, `.gitattributes` entry, and docs first (no CI yet) — a no-op for any PR that doesn't touch `openspec/**`.
2. Add the CI workflow last, once the script is verified to behave correctly by hand (including the zero-change and multi-change cases) against this change's own PR.
3. No rollback beyond reverting the added files; nothing else in the repo depends on `openspec/specs.projected/` existing.

## Open Questions

None outstanding — architecture, lifecycle, and rollout scope (HealthVault only, others deferred) were confirmed with the user before writing this design.
