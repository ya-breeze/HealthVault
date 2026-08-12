## 1. Generator script

- [ ] 1.1 Create `scripts/generate-projected-specs.sh`: accept an optional base-ref argument (default `origin/main`), compute touched change ids via `git diff --name-only "$BASE"...HEAD -- 'openspec/changes/*/specs/**'`
- [ ] 1.2 Zero-touched-ids path: `rm -rf openspec/specs.projected` and exit 0
- [ ] 1.3 Before creating any worktree: for each touched change id, parse its delta files under `openspec/changes/<id>/specs/**/spec.md` for `### Requirement: <name>` headers (any of ADDED/MODIFIED/REMOVED) and build `(capability, requirement name)` pairs per change id. If the same pair appears under more than one touched change id, print the conflicting change ids and requirement name and exit non-zero without creating a worktree or touching `openspec/specs.projected/`
- [ ] 1.4 Non-empty, non-conflicting path: create a detached worktree (`git worktree add --detach <tmp> HEAD`) with a `trap` that removes it on any exit (success, failure, or interrupt)
- [ ] 1.5 Inside the worktree, run `openspec archive <id> --yes` for each touched id in sorted order; on any failure, print the error and exit non-zero without touching `openspec/specs.projected/` in the primary tree
- [ ] 1.6 From the touched change ids' delta paths, derive the touched-capability set (the path segment after `specs/` in each changed delta file)
- [ ] 1.7 On success, rebuild `openspec/specs.projected/` from scratch (`rm -rf openspec/specs.projected && mkdir`), writing output only for touched capabilities — never the full `openspec/specs/` tree
- [ ] 1.8 For each touched capability, write `openspec/specs.projected/<capability>/spec.md` (full post-archive text from the worktree) and `openspec/specs.projected/<capability>/spec.diff` via `diff -u --label "openspec/specs/<capability>/spec.md" --label "openspec/specs/<capability>/spec.md" <before> <after>` (before = current `openspec/specs/<capability>/spec.md`, or `/dev/null` if new; after = post-archive text) — fixed labels, not the worktree's real temp path, so output is deterministic across runs
- [ ] 1.9 Prepend the generated-file header comment to every `.md` and `.diff` file written
- [ ] 1.10 Make the script executable (`chmod +x`) and verify `shellcheck` (if available) reports no issues

## 2. Task runner and CI integration

- [ ] 2.1 Add a `projected-specs:` target to the root `Makefile` that runs `scripts/generate-projected-specs.sh`
- [ ] 2.2 Add `.github/workflows/openspec-projected-specs.yml`: trigger on `pull_request` with a `paths` filter on `openspec/**`; checkout with `fetch-depth: 0`; install `@fission-ai/openspec@1.6.0` via npm; run the script against `origin/${{ github.base_ref }}`; run `git add -A -- openspec/specs.projected && git diff --cached --exit-code -- openspec/specs.projected` (staged comparison, not a plain working-tree diff, so an entirely untracked/missing projection is also caught) and fail with a message naming `make projected-specs` as the fix
- [ ] 2.3 Add `.gitattributes` entry: `openspec/specs.projected/** linguist-generated=true`

## 3. Documentation

- [ ] 3.1 Add `openspec/specs.projected/README.md` (or equivalent) describing: what the directory is, that it is generated and never hand-edited, how to regenerate (`make projected-specs`), the empty-on-main lifecycle invariant, and that PR review should read the projected file for final wording and the delta under `openspec/changes/<id>/specs/` for intent
- [ ] 3.2 Update the global environment CLAUDE.md (`/data/CLAUDE.md`, source at `Useful/ai/truenas/CLAUDE.md`, a separate repo) so the "Feature Branch Workflow" archive step explicitly includes re-running `make projected-specs` afterward and committing the resulting removal, before merging. This is a separate doc-only PR in the `Useful` repo, not part of this HealthVault PR.

## 4. Verification

- [ ] 4.1 Run the generator on this change's own branch (which touches `openspec/changes/add-projected-specs-artifact/specs/projected-specs-generation/spec.md`) and confirm `openspec/specs.projected/projected-specs-generation/spec.md` and `spec.diff` are produced with the generated-file header, and that no other capability directory appears under `openspec/specs.projected/`
- [ ] 4.2 Simulate the zero-change case: run the generator with `BASE=HEAD` (no diff) and confirm no `openspec/specs.projected/` directory is created, or that an existing one is removed
- [ ] 4.3 Simulate an archive failure (temporarily break a delta spec) and confirm the script exits non-zero without modifying `openspec/specs.projected/`
- [ ] 4.4 Simulate the multi-change case locally if a second open change is available, or note in the PR description that this path is covered by code review + the requirement's scenario rather than a live second change
- [ ] 4.5 Confirm `git worktree list` shows no leftover worktrees after both a successful and a failing run
- [ ] 4.6 Commit the generated `openspec/specs.projected/` for this change itself as part of this PR, demonstrating the mechanism end-to-end
- [ ] 4.7 Simulate two touched changes modifying the same requirement in the same capability; confirm the generator exits non-zero before creating a worktree (`git worktree list` shows nothing new from the run) and names both change ids and the requirement
- [ ] 4.8 Run the generator twice in a row with no commits between runs and confirm every `spec.diff` is byte-for-byte identical across both runs (`diff -q` between the two outputs, or a saved copy compared after the second run)
