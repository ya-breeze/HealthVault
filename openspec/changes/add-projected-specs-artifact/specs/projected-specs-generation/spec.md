## ADDED Requirements

### Requirement: Projected spec generation scoped to touched capabilities
The generator SHALL determine which OpenSpec change ids are touched on the current branch by diffing `openspec/changes/*/specs/**` against a base ref (default `origin/main`), extracting the change id from each changed path's first segment after `openspec/changes/`. For each touched id, the generator SHALL run `openspec archive <id> --yes` inside an isolated worktree. From the touched change ids' delta paths, the generator SHALL derive the set of touched capability names (the path segment immediately after `specs/` in each touched delta file). The generator SHALL write projected output only for that touched-capability set — it SHALL NOT write output for any capability not touched by a touched change, even if that capability exists in the resulting `openspec/specs/` tree.

#### Scenario: Single touched change
- **WHEN** the branch's diff against the base ref includes files under `openspec/changes/add-foo/specs/widget-api/spec.md` and no other change's `specs/**` paths
- **THEN** the generator archives only `add-foo` in the isolated worktree and writes projected output only for capability `widget-api`

#### Scenario: Multiple touched changes in one PR
- **WHEN** the branch's diff against the base ref includes files under both `openspec/changes/add-foo/specs/widget-api/spec.md` and `openspec/changes/add-bar/specs/gadget-api/spec.md`
- **THEN** the generator archives `add-foo` and `add-bar` together, in sorted order, inside the same isolated worktree, and writes projected output for both `widget-api` and `gadget-api`

#### Scenario: Untouched capabilities are never written
- **WHEN** a touched change modifies only capability `widget-api`, and capability `gadget-api` exists in the base specs unchanged
- **THEN** the regenerated `openspec/specs.projected/` contains no `gadget-api` directory at all, regardless of what previously existed there

### Requirement: Each touched capability gets both final text and an explicit comparison
For each touched capability, the generator SHALL write `openspec/specs.projected/<capability>/spec.md` containing the full post-archive requirement text, and `openspec/specs.projected/<capability>/spec.diff` containing a unified diff (`diff -u`) between the current `openspec/specs/<capability>/spec.md` (or an empty input if the capability does not yet exist on the base) and that post-archive text. This exists because `openspec/specs.projected/` never persists on `main` (see "Projected specs never persist past merge" below), so there is no git-committed baseline for git's own diff view to compare a PR's projection against; the `.diff` file provides the old-to-new comparison independently of git history.

#### Scenario: Comparison file for a modified existing capability
- **WHEN** a touched change contains a `MODIFIED Requirements` section against existing capability `widget-api`
- **THEN** `openspec/specs.projected/widget-api/spec.diff` shows the current `openspec/specs/widget-api/spec.md` as the diff's "before" side and the post-archive text as the "after" side

#### Scenario: Comparison file for a brand-new capability
- **WHEN** a touched change introduces a capability with no existing `openspec/specs/<capability>/` directory
- **THEN** `openspec/specs.projected/<capability>/spec.diff` shows the post-archive text as entirely added, with no "before" content

### Requirement: Zero-change case leaves no projection
When no change ids are touched relative to the base ref, the generator SHALL remove `openspec/specs.projected/` entirely (if present) and exit successfully, without creating a worktree or invoking `openspec archive`.

#### Scenario: PR with no OpenSpec delta changes
- **WHEN** the branch's diff against the base ref contains no paths matching `openspec/changes/*/specs/**`
- **THEN** the generator exits 0, and `openspec/specs.projected/` does not exist afterward

#### Scenario: Self-cleaning after a change is archived
- **WHEN** a branch previously had a committed, non-empty `openspec/specs.projected/` for change `add-foo`, and `add-foo` has since been archived for real (moving its delta out of `openspec/changes/add-foo/specs/**` into the archive folder)
- **THEN** re-running the generator finds zero touched change ids and removes `openspec/specs.projected/`, leaving no projected content for `add-foo` to carry into a merge

### Requirement: Cross-change requirement conflicts are detected before archiving
Before creating a worktree or invoking `openspec archive` for any touched change id, the generator SHALL parse every touched change id's delta files for `### Requirement: <name>` headers (under any of `ADDED`/`MODIFIED`/`REMOVED`), and for every `## RENAMED Requirements` entry SHALL treat both the `FROM:` name and the `TO:` name as touched requirement names, and build the set of `(capability, requirement name)` pairs each touched change id touches. If the same `(capability, requirement name)` pair is touched by more than one touched change id, the generator SHALL exit non-zero immediately, naming the conflicting change ids and the requirement, and SHALL NOT create a worktree, invoke `openspec archive`, or modify `openspec/specs.projected/`. This exists because `openspec archive` does not itself detect or reject this case (confirmed: two independently valid changes modifying the same requirement both archive successfully, with the second silently overwriting the first, order-dependent on archive order), and `openspec` 1.6.0's `RENAMED` operation (a `FROM:`/`TO:` pair applied before `REMOVED`/`MODIFIED`/`ADDED` at archive time) contains no `### Requirement:` header of its own, so a parser that only looks for that header pattern misses renames entirely — letting one change rename a requirement while another touched change modifies, removes, or adds under the old or new name go undetected.

#### Scenario: Conflicting touched changes are rejected
- **WHEN** two touched change ids each contain a delta touching the same capability's same requirement name (regardless of whether each delta section is `ADDED`, `MODIFIED`, or `REMOVED`)
- **THEN** the generator exits non-zero before creating a worktree, and its output names both conflicting change ids and the requirement

#### Scenario: A rename conflicts with an operation on either of its endpoint names
- **WHEN** one touched change's delta contains a `RENAMED` entry with `FROM: `### Requirement: A`` and `TO: `### Requirement: B``, and another touched change's delta contains an `ADDED`, `MODIFIED`, or `REMOVED` operation on requirement `A` or requirement `B` in the same capability
- **THEN** the generator exits non-zero before creating a worktree, and its output names both conflicting change ids and the shared requirement name

#### Scenario: Non-conflicting touched changes proceed normally
- **WHEN** touched change ids touch disjoint requirements, including cases where they touch different requirements within the same capability
- **THEN** the generator proceeds to create the worktree and archive both, as in the "Multiple touched changes in one PR" scenario above

### Requirement: Generated diffs are deterministic across runs
The generator SHALL produce `spec.diff` using stable, fixed labels (e.g. `diff -u --label "openspec/specs/<capability>/spec.md" --label "openspec/specs/<capability>/spec.md" <before> <after>`) rather than the raw filesystem paths of its inputs. This exists because the post-archive "after" file lives inside a freshly created, randomly-named temporary worktree, so its real path (and modification timestamp) differs on every run even when content is identical; unlabeled `diff -u` output would therefore differ byte-for-byte between any two generations of the same committed state, permanently failing the CI staged-diff comparison regardless of actual content drift.

#### Scenario: Identical content produces byte-identical diffs across runs
- **WHEN** the generator is run twice in succession against the same committed state, with no commits made in between (each run using its own freshly created temporary worktree)
- **THEN** the resulting `spec.diff` files are byte-for-byte identical across both runs

#### Scenario: Diff headers show the canonical path, not a temporary worktree path
- **WHEN** the generator writes `openspec/specs.projected/<capability>/spec.diff`
- **THEN** its `---`/`+++` header lines show the canonical `openspec/specs/<capability>/spec.md` path, not a path under the temporary worktree

### Requirement: Generation is isolated from the primary working tree
The generator SHALL perform all `openspec archive` calls inside a disposable, detached git worktree checked out from `HEAD`, and SHALL NOT modify any file in the primary working tree other than `openspec/specs.projected/`. The worktree SHALL be removed on exit, including on failure.

#### Scenario: Safe to run with uncommitted changes elsewhere
- **WHEN** the primary working tree has uncommitted edits to files outside `openspec/`
- **THEN** running the generator does not alter, stage, or discard those uncommitted edits

#### Scenario: Worktree removed after a successful run
- **WHEN** the generator completes successfully
- **THEN** no worktree registered by the run remains (`git worktree list` shows none of the run's temporary paths)

#### Scenario: Worktree removed after a failed run
- **WHEN** the generator aborts due to an `openspec archive` failure
- **THEN** no worktree registered by the run remains, despite the failure

### Requirement: Archive failure aborts without partial changes
If `openspec archive <id> --yes` fails for any touched change id inside the worktree, the generator SHALL exit non-zero, SHALL report the failure, and SHALL leave `openspec/specs.projected/` in the primary working tree exactly as it was before the run.

#### Scenario: Invalid delta causes archive to fail
- **WHEN** a touched change's delta spec fails `openspec archive`'s validation (e.g. a malformed requirement)
- **THEN** the generator exits non-zero, prints the underlying `openspec archive` error, and does not write, remove, or partially update `openspec/specs.projected/`

### Requirement: Generated files are marked non-editable
Every file written into `openspec/specs.projected/` (both `spec.md` and `spec.diff`) SHALL begin with a header comment identifying the file as generated and naming the command to regenerate it.

#### Scenario: Header present on every projected file
- **WHEN** the generator writes any `openspec/specs.projected/<capability>/spec.md` or `openspec/specs.projected/<capability>/spec.diff`
- **THEN** the file's first line is a comment stating it is generated, must not be hand-edited, and naming `make projected-specs` as the regeneration command

### Requirement: CI gate enforces regenerated output matches committed output, including entirely missing output
A CI workflow SHALL run on pull requests that touch `openspec/**`, regenerate `openspec/specs.projected/` against the PR's base branch, and fail the check if the regenerated content differs from what is committed in the PR — including the case where the PR contains no committed `openspec/specs.projected/` content at all (untracked or absent). The check SHALL stage the regenerated output before comparing, using an existing parent pathspec (`git add -A -- openspec && git diff --cached --exit-code -- openspec/specs.projected`), not compare only tracked files and not stage `openspec/specs.projected` directly — staging that path directly errors (non-zero exit, no comparison performed) whenever it has never existed on the checkout, which is the correct state for a PR with no touched changes or one whose changes have all been archived, so staging from the always-present `openspec` parent instead is required for the check to pass on that state rather than erroring out on it. On failure it SHALL name the exact local command to run.

#### Scenario: Stale projection fails CI
- **WHEN** a PR touching `openspec/changes/*/specs/**` is opened without a matching regeneration of `openspec/specs.projected/`
- **THEN** the CI job fails and its output names `make projected-specs` as the command to run and commit

#### Scenario: Entirely missing projection fails CI
- **WHEN** a PR touches `openspec/changes/*/specs/**` but never runs or commits the generator's output, so `openspec/specs.projected/` does not exist in the PR at all
- **THEN** the CI job still fails, because regenerating produces non-empty content that the staged-diff comparison detects as entirely new, not silently passed as "no diff"

#### Scenario: Correctly empty projection passes CI without a pathspec error
- **WHEN** a PR has no touched OpenSpec deltas, or all of its touched changes have been archived, so the correct `openspec/specs.projected/` state is absent and it has never existed on that checkout (neither on disk nor in git history)
- **THEN** the CI job succeeds — the staging step does not error, because it stages from the existing `openspec` parent path rather than the nonexistent `openspec/specs.projected` path

#### Scenario: Up-to-date projection passes CI
- **WHEN** a PR's committed `openspec/specs.projected/` matches what regenerating it against the PR's base branch produces
- **THEN** the CI job succeeds

### Requirement: Projected specs never persist past merge
Because `main` always reaches a state where every touched change has been archived (this repo's workflow requires archiving in the same PR before merge), `openspec/specs.projected/` SHALL NOT contain committed content on `main`.

#### Scenario: Archived change results in empty projection before merge
- **WHEN** a PR's change has been archived for real as the final step before merge, per this repo's workflow
- **THEN** regenerating the projection at that point produces no content, and the PR's final committed state has no `openspec/specs.projected/` directory

### Requirement: Task runner integration
Running `make projected-specs` SHALL invoke the generator script against the default base ref, applying all of the above behavior.

#### Scenario: Make target invokes the generator
- **WHEN** a contributor runs `make projected-specs`
- **THEN** the generator script runs exactly as it would when invoked directly, updating or removing `openspec/specs.projected/` per the current branch's touched changes
